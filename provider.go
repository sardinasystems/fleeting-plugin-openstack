package fpoc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"sync/atomic"
	"time"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
	"github.com/gophercloud/utils/openstack/clientconfig"
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
}

func (g *InstanceGroup) Init(ctx context.Context, log hclog.Logger, settings provider.Settings) (provider.ProviderInfo, error) {
	if g.CloudsConfig != "" {
		os.Setenv("OS_CLIENT_CONFIG_FILE", g.CloudsConfig)
	}

	opts := &clientconfig.ClientOpts{
		Cloud: g.Cloud,
		AuthInfo: &clientconfig.AuthInfo{
			AllowReauth: true,
		},
	}

	cli, err := clientconfig.NewServiceClient("compute", opts)
	if err != nil {
		return provider.ProviderInfo{}, fmt.Errorf("Failed to connect to OpenStack Nova: %w", err)
	}

	cli.Microversion = "2.79" // train+
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
				log, err := servers.ShowConsoleOutput(g.computeClient, srv.ID, servers.ShowConsoleOutputOpts{
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

func (g *InstanceGroup) getInstances(ctx context.Context, initial bool) ([]servers.Server, error) {
	page, err := servers.List(g.computeClient, nil).AllPages()
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

	srv, err := servers.Create(g.computeClient, spec).Extract()
	if err != nil {
		return "", err
	}

	return srv.ID, nil
}

func (g *InstanceGroup) deleteInstance(ctx context.Context, id string) error {
	return servers.Delete(g.computeClient, id).ExtractErr()
}

func (g *InstanceGroup) getInstance(ctx context.Context, id string) (*servers.Server, error) {
	return servers.Get(g.computeClient, id).Extract()
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
	return nil
}
