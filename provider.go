package fpoc

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"os"
	"path"
	"sync/atomic"
	"time"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/servers"
	"github.com/gophercloud/gophercloud/v2/openstack/config"
	clouds "github.com/gophercloud/gophercloud/v2/openstack/config/clouds"
	osutil "github.com/gophercloud/gophercloud/v2/openstack/utils"
	"github.com/hashicorp/go-hclog"
	"github.com/jinzhu/copier"

	"gitlab.com/gitlab-org/fleeting/fleeting/connector"
	"gitlab.com/gitlab-org/fleeting/fleeting/provider"
)

const MetadataKey = "fleeting-cluster"

var _ provider.InstanceGroup = (*InstanceGroup)(nil)

type InstanceGroup struct {
	Cloud            string        `json:"cloud"`             // cloud to use
	CloudsConfig     string        `json:"clouds_config"`     // optional: path to clouds.yaml
	AuthFromEnv      bool          `json:"auth_from_env"`     // optional: Use environment variables for authentication
	Name             string        `json:"name"`              // name of the cluster
	NovaMicroversion string        `json:"nova_microversion"` // Microversion for the Nova client
	ServerSpec       ExtCreateOpts `json:"server_spec"`       // instance creation spec
	UseIgnition      bool          `json:"use_ignition"`      // Configure keys via Ignition (Fedora CoreOS / Flatcar)
	BootTimeS        string        `json:"boot_time"`         // optional: wait some time before report machine as available
	BootTime         time.Duration

	computeClient   *gophercloud.ServiceClient
	settings        provider.Settings
	log             hclog.Logger
	imgProps        *ImageProperties
	sshPubKey       string
	instanceCounter atomic.Int32
}

func (g *InstanceGroup) Init(ctx context.Context, log hclog.Logger, settings provider.Settings) (provider.ProviderInfo, error) {
	g.log = log.With("name", g.Name, "cloud", g.Cloud)
	g.log.Debug("Initializing fleeting-plugin-openstack")

	providerClient, endpointOps, err := g.getProviderClient(ctx)
	if err != nil {
		return provider.ProviderInfo{}, err
	}

	cli, err := openstack.NewComputeV2(providerClient, endpointOps)
	if err != nil {
		return provider.ProviderInfo{}, fmt.Errorf("Failed to connect to OpenStack Nova: %w", err)
	}

	if g.NovaMicroversion == "" {
		g.NovaMicroversion = "2.79" // Train+
	}

	ncli, err := osutil.RequireMicroversion(ctx, *cli, g.NovaMicroversion)
	if err != nil {
		return provider.ProviderInfo{}, fmt.Errorf("Failed to request microversion %s for OpenStack Nova: %w", g.NovaMicroversion, err)
	}

	g.computeClient = &ncli

	_, err = g.ServerSpec.ToServerCreateMap()
	if err != nil {
		return provider.ProviderInfo{}, fmt.Errorf("Failed to check server_spec: %w", err)
	}

	if g.ServerSpec.ImageRef != "" {
		imgCli, err := openstack.NewImageV2(providerClient, endpointOps)
		if err != nil {
			return provider.ProviderInfo{}, fmt.Errorf("Failed to get OpenStack Glance: %w", err)
		}

		imgProps, err := GetImageProperties(ctx, imgCli, g.ServerSpec.ImageRef)
		if err != nil {
			return provider.ProviderInfo{}, err
		}

		g.imgProps = imgProps
	}

	// log.With("creds", settings, "image", g.imgProps).Info("settings 1")

	if !g.UseIgnition && !settings.ConnectorConfig.UseStaticCredentials {
		return provider.ProviderInfo{}, fmt.Errorf("Only static credentials supported in Cloud-Init mode.")
	}

	if g.UseIgnition {
		err = g.initSSHKey(ctx, log, &settings)
		if err != nil {
			return provider.ProviderInfo{}, err
		}
	}

	// log.With("creds", settings, "image", g.imgProps).Info("settings2")

	if g.BootTimeS != "" {
		g.BootTime, err = time.ParseDuration(g.BootTimeS)
		if err != nil {
			return provider.ProviderInfo{}, fmt.Errorf("Failed to parse boot_time: %w", err)
		}
	}

	g.settings = settings
	if _, err := g.getInstances(ctx); err != nil {
		return provider.ProviderInfo{}, err
	}

	return provider.ProviderInfo{
		ID:        path.Join("openstack", g.Cloud, g.Name),
		MaxSize:   1000,
		Version:   Version,
		BuildInfo: BuildInfo(),
	}, nil
}

func (g *InstanceGroup) getProviderClient(ctx context.Context) (*gophercloud.ProviderClient, gophercloud.EndpointOpts, error) {
	var endpointOps gophercloud.EndpointOpts
	var authOptions gophercloud.AuthOptions
	var providerClient *gophercloud.ProviderClient

	if g.AuthFromEnv {
		g.log.Debug("Using env vars for auth")

		var err error
		endpointOps = gophercloud.EndpointOpts{Region: os.Getenv("OS_REGION_NAME")}
		authOptions, err = openstack.AuthOptionsFromEnv()
		if err != nil {
			return nil, gophercloud.EndpointOpts{}, fmt.Errorf("Failed to get auth options from environment: %w", err)
		}
		authOptions.AllowReauth = true

		providerClient, err = openstack.AuthenticatedClient(ctx, authOptions)
		if err != nil {
			return nil, gophercloud.EndpointOpts{}, fmt.Errorf("Failed to connect to OpenStack Keystone: %w", err)
		}
	} else {
		g.log.Debug("Using clouds.yaml for auth")

		var err error
		var tlsCfg *tls.Config
		cloudOpts := []clouds.ParseOption{clouds.WithCloudName(g.Cloud)}
		if g.CloudsConfig != "" {
			cloudOpts = append(cloudOpts, clouds.WithLocations(g.CloudsConfig))
		}

		authOptions, endpointOps, tlsCfg, err = clouds.Parse(cloudOpts...)
		if err != nil {
			return nil, gophercloud.EndpointOpts{}, fmt.Errorf("Failed to parse clouds.yaml: %w", err)
		}

		// plugin is a long running process. force allow reauth
		authOptions.AllowReauth = true

		providerClient, err = config.NewProviderClient(ctx, authOptions, config.WithTLSConfig(tlsCfg))
		if err != nil {
			return nil, gophercloud.EndpointOpts{}, fmt.Errorf("Failed to connect to OpenStack Keystone: %w", err)
		}
	}

	return providerClient, endpointOps, nil
}

func (g *InstanceGroup) Update(ctx context.Context, update func(instance string, state provider.State)) error {

	instances, err := g.getInstances(ctx)
	if err != nil {
		return err
	}

	var reterr error
	for _, srv := range instances {
		state := provider.StateCreating
		lg := g.log.With("server_id", srv.ID, "created", srv.Created, "status", srv.Status)

		switch srv.Status {
		case "BUILD", "MIGRATING", "PAUSED", "REBUILD":
			// pass

		case "DELETED", "SHUTOFF", "UNKNOWN":
			state = provider.StateDeleting

		case "ERROR":
			// unsure if that's proper way...
			lg.Warn("Instance is in ERROR state. Marking as a timeout.")
			state = provider.StateTimeout

		case "ACTIVE":
			if srv.Created.Add(g.BootTime).Before(time.Now()) {
				// treat all nodes running long enough as Running
				state = provider.StateRunning
			} else {
				log, err := servers.ShowConsoleOutput(ctx, g.computeClient, srv.ID, servers.ShowConsoleOutputOpts{
					Length: 100,
				}).Extract()
				if err != nil {
					reterr = errors.Join(reterr, err)
					continue
				}

				if !g.UseIgnition && IsCloudInitFinished(log) {
					lg.Info("Instance cloud-init finished")
					state = provider.StateRunning
				} else if g.UseIgnition && IsIgnitionFinished(log) {
					lg.Info("Instance ignition finished")
					state = provider.StateRunning
				} else {
					lg.Debug("Instance boot time not passed and cloud-init/ignition not finished", "boot_time", g.BootTime)
				}
			}
		}

		update(srv.ID, state)
	}

	return reterr
}

func (g *InstanceGroup) Increase(ctx context.Context, delta int) (succeeded int, err error) {
	for idx := 0; idx < delta; idx++ {
		id, err2 := g.createInstance(ctx)
		if err2 != nil {
			g.log.Error("Failed to create instance", "err", err)
			err = errors.Join(err, err2)
		} else {
			g.log.Info("Instance creation request successful", "id", id)
			succeeded++
		}
	}

	g.log.Info("Increase", "delta", delta, "succeeded", succeeded)

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

	return
}

func (g *InstanceGroup) getInstances(ctx context.Context) ([]servers.Server, error) {
	page, err := servers.List(g.computeClient, nil).AllPages(ctx)
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

	var hintOpts servers.SchedulerHintOptsBuilder
	if spec.SchedulerHints != nil {
		hintOpts = spec.SchedulerHints
	}

	if g.UseIgnition {
		err := InsertSSHKeyIgn(spec, g.settings.Username, g.sshPubKey)
		if err != nil {
			return "", err
		}
	}

	srv, err := servers.Create(ctx, g.computeClient, spec, hintOpts).Extract()
	if err != nil {
		return "", err
	}

	return srv.ID, nil
}

func (g *InstanceGroup) deleteInstance(ctx context.Context, id string) error {
	return servers.Delete(ctx, g.computeClient, id).ExtractErr()
}

func (g *InstanceGroup) getInstance(ctx context.Context, id string) (*servers.Server, error) {
	return servers.Get(ctx, g.computeClient, id).Extract()
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
	info.Protocol = provider.ProtocolSSH

	if g.imgProps != nil {
		switch g.imgProps.OSType {
		case "", "linux":
			info.Protocol = provider.ProtocolSSH
			info.OS = "linux"

		case "windows":
			g.log.Warn("Windows not really supported by the plugin.")
			info.Protocol = provider.ProtocolWinRM
			info.OS = g.imgProps.OSType

		default:
			g.log.Warn("Unknown image os_type", "os_type", g.imgProps.OSType)
			info.OS = g.imgProps.OSType
		}

		switch g.imgProps.Architecture {
		case "", "x86_64":
			info.Arch = "amd64"

		case "aarch64":
			info.Arch = "arm64"

		default:
			g.log.Warn("Unknown image arch", "arch", g.imgProps.Architecture)
		}

	} else {
		// default to linux on amd64
		info.OS = "linux"
		info.Arch = "amd64"
	}

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
