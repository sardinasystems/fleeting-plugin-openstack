package fpoc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/servers"
	"github.com/gophercloud/gophercloud/v2/openstack/config"
	clouds "github.com/gophercloud/gophercloud/v2/openstack/config/clouds"
	"github.com/hashicorp/go-hclog"
	"github.com/jinzhu/copier"

	"gitlab.com/gitlab-org/fleeting/fleeting/connector"
	"gitlab.com/gitlab-org/fleeting/fleeting/provider"
)

const MetadataKey = "fleeting-cluster"

var _ provider.InstanceGroup = (*InstanceGroup)(nil)

type InstanceGroup struct {
	Cloud        string        `json:"cloud"`         // cloud to use
	CloudsConfig string        `json:"clouds_config"` // optional: path to clouds.yaml
	Name         string        `json:"name"`          // name of the cluster
	ServerSpec   ExtCreateOpts `json:"server_spec"`   // instance creation spec
	BootTimeS    string        `json:"boot_time"`     // optional: wait some time before report machine as available
	BootTime     time.Duration

	size            int
	computeClient   *gophercloud.ServiceClient
	settings        provider.Settings
	log             hclog.Logger
	instanceCounter atomic.Int32
	shutdownC       chan struct{}
	mu              sync.RWMutex
}

func (g *InstanceGroup) Init(ctx context.Context, log hclog.Logger, settings provider.Settings) (provider.ProviderInfo, error) {
	// g.mu.Lock()
	// defer g.mu.Unlock()

	pOpts := []clouds.ParseOption{clouds.WithCloudName(g.Cloud)}
	if g.CloudsConfig != "" {
		pOpts = append(pOpts, clouds.WithLocations(g.CloudsConfig))
	}

	ao, eo, tlsCfg, err := clouds.Parse(pOpts...)
	if err != nil {
		return provider.ProviderInfo{}, fmt.Errorf("Failed to parse clouds.yaml: %w", err)
	}

	// plugin is a long running process. force allow reauth
	ao.AllowReauth = true

	reauth := func(ctx context.Context) (*gophercloud.ServiceClient, error) {
		pc, err := config.NewProviderClient(ctx, ao, config.WithTLSConfig(tlsCfg))
		if err != nil {
			return nil, fmt.Errorf("Failed to connect to OpenStack Keystone: %w", err)
		}

		cli, err := openstack.NewComputeV2(pc, eo)
		if err != nil {
			return nil, fmt.Errorf("Failed to connect to OpenStack Nova: %w", err)
		}

		cli.Context = nil         // ensure no global context
		cli.Microversion = "2.79" // train+
		return cli, nil
	}

	cli, err := reauth(ctx)
	if err != nil {
		return provider.ProviderInfo{}, err
	}

	g.computeClient = cli

	if !settings.ConnectorConfig.UseStaticCredentials {
		return provider.ProviderInfo{}, fmt.Errorf("Only static credentials supported")
	}

	if g.BootTimeS != "" {
		g.BootTime, err = time.ParseDuration(g.BootTimeS)
		if err != nil {
			return provider.ProviderInfo{}, fmt.Errorf("Failed to parse boot_time: %w", err)
		}
	}

	_, err = g.ServerSpec.ToServerCreateMap()
	if err != nil {
		return provider.ProviderInfo{}, fmt.Errorf("Failed to check server_spec: %w", err)
	}

	g.settings = settings
	g.log = log.With("name", g.Name, "cloud", g.Cloud)
	g.size = 0

	if _, err := g.getInstances(ctx, true); err != nil {
		return provider.ProviderInfo{}, err
	}

	// FIXME: workaround to fix token reauth problem:
	// > Unable to re-authenticate: Expected HTTP response code [202 204] when accessing [DELETE https://ci.cloud/compute/v2.1/servers/50b75183-a43a-434f-b3ea-689f90b4ac6b], but got 401 instead
	// Issue: https://github.com/gophercloud/gophercloud/issues/2931
	g.shutdownC = make(chan struct{}, 1)
	reauthT := time.NewTicker(1 * time.Hour)
	go func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		for {
			select {
			case <-reauthT.C:
				log.Debug("Re-authenticating...")
				cli, err := reauth(ctx)
				if err != nil {
					log.Error("Re-authenticate failed", "err", err)
					continue
				}

				log.Info("Re-authenticated successful")
				g.mu.Lock()
				g.computeClient = cli
				g.mu.Unlock()

			case <-g.shutdownC:
				reauthT.Stop()
				return
			}
		}
	}()

	return provider.ProviderInfo{
		ID:        path.Join("openstack", g.Cloud, g.Name),
		MaxSize:   1000,
		Version:   Version,
		BuildInfo: BuildInfo(),
	}, nil
}

func (g *InstanceGroup) Update(ctx context.Context, update func(instance string, state provider.State)) error {

	instances, err := g.getInstances(ctx, false)
	if err != nil {
		return err
	}

	var reterr error
	for _, srv := range instances {
		state := provider.StateCreating

		switch srv.Status {
		case "BUILD", "MIGRATING", "PAUSED", "REBUILD":
			// pass

		case "DELETED", "SHUTOFF", "UNKNOWN":
			state = provider.StateDeleting

		case "ACTIVE":
			if srv.Created.Add(g.BootTime).Before(time.Now()) {
				// treat all nodes running long enough as Running
				state = provider.StateRunning
			} else {
				log, err := servers.ShowConsoleOutput(g.cli(), srv.ID, servers.ShowConsoleOutputOpts{
					Length: 100,
				}).Extract()
				if err != nil {
					reterr = errors.Join(reterr, err)
					continue
				}

				if IsCloudInitFinished(log) {
					g.log.Debug("Instance cloud-init finished", "server_id", srv.ID, "created", srv.Created)
					state = provider.StateRunning
				} else {
					g.log.Debug("Instance boot time not passed and cloud-init not finished", "server_id", srv.ID, "created", srv.Created, "boot_time", g.BootTime)
				}
			}
		}

		update(srv.ID, state)
	}

	return reterr
}

func (g *InstanceGroup) Increase(ctx context.Context, delta int) (succeeded int, err error) {
	for idx := g.size; idx < g.size+delta; idx++ {
		id, err2 := g.createInstance(ctx)
		if err2 != nil {
			g.log.Error("Failed to create instance", "err", err)
			err = errors.Join(err, err2)
		} else {
			g.log.Info("Instance creation request successful", "id", id)
			succeeded++
		}
	}

	g.log.Info("Increase", "delta", delta, "succeeded", succeeded, "pre_instances", g.size)
	g.size += succeeded

	return
}

func (g *InstanceGroup) Decrease(ctx context.Context, instances []string) (succeeded []string, err error) {
	if len(instances) == 0 {
		return nil, nil
	}

	succeeded = make([]string, 0, len(instances))
	for _, id := range instances {
		err2 := g.deleteInstance(ctx, id)
		if err2 != nil {
			g.log.Error("Failed to delete instance", "err", err2, "id", id)
			err = errors.Join(err, err2)
		} else {
			g.log.Info("Instance deletion request successful", "id", id)
			succeeded = append(succeeded, id)
		}
	}

	g.log.Info("Decrease", "instances", instances)
	g.size -= len(succeeded)

	return instances, err
}

func (g *InstanceGroup) cli() *gophercloud.ServiceClient {
	g.mu.RLock()
	defer g.mu.RUnlock()

	return g.computeClient
}

func (g *InstanceGroup) getInstances(ctx context.Context, initial bool) ([]servers.Server, error) {
	page, err := servers.List(g.cli(), nil).AllPagesWithContext(ctx)
	if err != nil {

		return nil, fmt.Errorf("Server listing error: %w", err)
	}

	allServers, err := servers.ExtractServers(page)
	if err != nil {
		return nil, fmt.Errorf("Server listing extract error: %w", err)
	}

	filteredServers := make([]servers.Server, 0, len(allServers))
	for _, srv := range allServers {
		cluster, ok := srv.Metadata[MetadataKey]
		if !ok || cluster != g.Name {
			continue
		}

		filteredServers = append(filteredServers, srv)
	}

	size := len(filteredServers)

	if !initial && size != g.size {
		g.log.Error("out-of-sync capacity", "expected", g.size, "actual", size)
	}
	g.size = size

	return filteredServers, nil
}

func (g *InstanceGroup) createInstance(ctx context.Context) (string, error) {
	spec := new(ExtCreateOpts)
	err := copier.Copy(spec, &g.ServerSpec)
	if err != nil {
		return "", err
	}

	index := int(g.instanceCounter.Add(1))

	spec.Name = fmt.Sprintf(g.ServerSpec.Name, index)
	if spec.Metadata == nil {
		spec.Metadata = make(map[string]string)
	}
	spec.Metadata[MetadataKey] = g.Name

	srv, err := servers.Create(g.cli(), spec).Extract()
	if err != nil {
		return "", err
	}

	return srv.ID, nil
}

func (g *InstanceGroup) deleteInstance(ctx context.Context, id string) error {
	return servers.Delete(g.cli(), id).ExtractErr()
}

func (g *InstanceGroup) getInstance(ctx context.Context, id string) (*servers.Server, error) {
	return servers.Get(g.cli(), id).Extract()
}

func (g *InstanceGroup) ConnectInfo(ctx context.Context, instanceID string) (provider.ConnectInfo, error) {
	srv, err := g.getInstance(ctx, instanceID)
	if err != nil {
		return provider.ConnectInfo{}, fmt.Errorf("Failed to get server %s: %w", instanceID, err)
	}

	// g.log.Debug("Server info", "srv", srv)
	if srv.Status != "ACTIVE" {
		return provider.ConnectInfo{}, fmt.Errorf("instance status is not active: %s", srv.Status)
	}

	ipAddr := srv.AccessIPv4
	if ipAddr == "" {
		netAddrs, err := extractAddresses(srv)
		if err != nil {
			return provider.ConnectInfo{}, err
		}

		// TODO: detect internal (tenant) and external networks
		for net, addrs := range netAddrs {
			for _, addr := range addrs {
				ipAddr = addr.Address
				g.log.Debug("Use address", "network", net, "ip_address", ipAddr)
			}
		}
	}

	info := provider.ConnectInfo{
		ConnectorConfig: g.settings.ConnectorConfig,
		ID:              instanceID,
		InternalAddr:    ipAddr,
		ExternalAddr:    ipAddr,
	}

	// TODO: get image metadata and get os_admin_user
	// TODO: get from image meta
	info.OS = "linux"
	info.Arch = "amd64"
	info.Protocol = provider.ProtocolSSH

	// g.log.Debug("Info", "info", info)

	inp := bytes.NewBuffer(nil)
	combinedOut := bytes.NewBuffer(nil)

	ropts := connector.ConnectorOptions{
		DialOptions: connector.DialOptions{
			// UseExternalAddr: true,
		},
		RunOptions: connector.RunOptions{
			Command: `echo "ok"`,
			Stdin:   inp,
			Stdout:  combinedOut,
			Stderr:  combinedOut,
		},
	}
	err = connector.Run(ctx, info, ropts)
	if err != nil {
		return provider.ConnectInfo{}, fmt.Errorf("Failed to test ssh: %w", err)
	}
	g.log.Debug("SSH test result", "out", combinedOut.String())

	return info, nil
}

func (g *InstanceGroup) Shutdown(ctx context.Context) error {
	g.shutdownC <- struct{}{}
	return nil
}
