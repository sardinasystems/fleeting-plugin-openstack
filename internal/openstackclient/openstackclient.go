package openstackclient

import (
	"context"
	"crypto/tls"
	"fmt"
	"os"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/servers"
	"github.com/gophercloud/gophercloud/v2/openstack/config"
	"github.com/gophercloud/gophercloud/v2/openstack/config/clouds"
	"github.com/gophercloud/gophercloud/v2/openstack/image/v2/images"
	"github.com/gophercloud/gophercloud/v2/openstack/utils"
	"github.com/mitchellh/mapstructure"
)

type AuthConfig struct {
	// AuthFromEnv specifies whether to use environment variables for auth
	AuthFromEnv bool

	// Cloud is the name of the cloud config from clouds.yaml to use
	Cloud string

	// CloudsConfig is the path to the clouds.yaml file
	CloudsConfig string

	// NovaMicroversion is the microversion of the OpenStack Nova client. Default 2.79 (which should be ok for Train+)
	NovaMicroversion string
}

// Some good known properties useful for setting up ConnectInfo
//
// See also: https://docs.openstack.org/glance/latest/admin/useful-image-properties.html
type ImageProperties struct {
	// Architecture that must be supported by the hypervisor.
	Architecture string `json:"architecture,omitempty" mapstructure:"architecture,omitempty"`

	// OSType is the operating system installed on the image.
	OSType string `json:"os_type,omitempty" mapstructure:"os_type,omitempty"`

	// OSDistro is the common name of the operating system distribution in lowercase
	OSDistro string `json:"os_distro,omitempty" mapstructure:"os_distro,omitempty"`

	// OSVersion is the operating system version as specified by the distributor.
	OSVersion string `json:"os_version,omitempty" mapstructure:"os_version,omitempty"`

	// OSAdminUser is the default admin user name for the operating system
	OSAdminUser string `json:"os_admin_user,omitempty" mapstructure:"os_admin_user,omitempty"`
}

type Client interface {
	GetImageProperties(ctx context.Context, imageRef string) (*ImageProperties, error)
	ShowServerConsoleOutput(ctx context.Context, serverId string) (string, error)
	GetServer(ctx context.Context, serverId string) (*servers.Server, error)
	ListServers(ctx context.Context) ([]servers.Server, error)
	CreateServer(ctx context.Context, spec servers.CreateOptsBuilder, hintOpts servers.SchedulerHintOptsBuilder) (*servers.Server, error)
	DeleteServer(ctx context.Context, serverId string) error
}

var _ Client = (*client)(nil)

type client struct {
	compute *gophercloud.ServiceClient
	image   *gophercloud.ServiceClient
}

func New(authConfig AuthConfig) (Client, error) {
	if authConfig.NovaMicroversion == "" {
		authConfig.NovaMicroversion = "2.79" // Train+
	}

	providerClient, endpointOps, err := newProviderClient(authConfig)
	if err != nil {
		return nil, err
	}

	computeClient, err := openstack.NewComputeV2(providerClient, endpointOps)
	if err != nil {
		return nil, err
	}

	_computeClient, err := utils.RequireMicroversion(context.TODO(), *computeClient, authConfig.NovaMicroversion)
	if err != nil {
		return nil, fmt.Errorf("failed to request microversion %s for OpenStack Nova: %w", authConfig.NovaMicroversion, err)
	}

	computeClient = &_computeClient

	imageClient, err := openstack.NewImageV2(providerClient, endpointOps)
	if err != nil {
		return nil, err
	}

	return &client{
		compute: computeClient,
		image:   imageClient,
	}, nil
}

func newProviderClient(authConfig AuthConfig) (*gophercloud.ProviderClient, gophercloud.EndpointOpts, error) {
	var endpointOps gophercloud.EndpointOpts
	var authOptions gophercloud.AuthOptions
	var providerClient *gophercloud.ProviderClient

	if authConfig.AuthFromEnv {
		var err error
		endpointOps = gophercloud.EndpointOpts{Region: os.Getenv("OS_REGION_NAME")}
		authOptions, err = openstack.AuthOptionsFromEnv()
		if err != nil {
			return nil, gophercloud.EndpointOpts{}, fmt.Errorf("failed to get auth options from environment: %w", err)
		}
		authOptions.AllowReauth = true

		providerClient, err = openstack.AuthenticatedClient(context.Background(), authOptions)
		if err != nil {
			return nil, gophercloud.EndpointOpts{}, fmt.Errorf("failed to connect to OpenStack Keystone: %w", err)
		}
	} else {
		var err error
		var tlsCfg *tls.Config
		cloudOpts := []clouds.ParseOption{clouds.WithCloudName(authConfig.Cloud)}
		if authConfig.CloudsConfig != "" {
			cloudOpts = append(cloudOpts, clouds.WithLocations(authConfig.CloudsConfig))
		}

		authOptions, endpointOps, tlsCfg, err = clouds.Parse(cloudOpts...)
		if err != nil {
			return nil, gophercloud.EndpointOpts{}, fmt.Errorf("failed to parse clouds.yaml: %w", err)
		}

		// plugin is a long running process. force allow reauth
		authOptions.AllowReauth = true

		providerClient, err = config.NewProviderClient(context.TODO(), authOptions, config.WithTLSConfig(tlsCfg))
		if err != nil {
			return nil, gophercloud.EndpointOpts{}, fmt.Errorf("failed to connect to OpenStack Keystone: %w", err)
		}
	}

	return providerClient, endpointOps, nil
}

func (c *client) GetImageProperties(ctx context.Context, imageRef string) (*ImageProperties, error) {
	image, err := images.Get(ctx, c.image, imageRef).Extract()
	if err != nil {
		return nil, fmt.Errorf("failed to get image %s: %w", imageRef, err)
	}

	out := new(ImageProperties)
	err = mapstructure.Decode(image.Properties, out)
	if err != nil {
		return nil, fmt.Errorf("failed to parse properties: %w", err)
	}

	return out, nil
}

func (c *client) ShowServerConsoleOutput(ctx context.Context, serverId string) (string, error) {
	return servers.ShowConsoleOutput(ctx, c.compute, serverId, servers.ShowConsoleOutputOpts{
		Length: 100,
	}).Extract()
}

func (c *client) GetServer(ctx context.Context, serverId string) (*servers.Server, error) {
	return servers.Get(ctx, c.compute, serverId).Extract()
}

func (c *client) ListServers(ctx context.Context) ([]servers.Server, error) {
	page, err := servers.List(c.compute, nil).AllPages(ctx)
	if err != nil {
		return nil, fmt.Errorf("server listing error: %w", err)
	}

	allServers, err := servers.ExtractServers(page)
	if err != nil {
		return nil, fmt.Errorf("server listing extract error: %w", err)
	}

	return allServers, nil
}

func (c *client) CreateServer(ctx context.Context, spec servers.CreateOptsBuilder, hintOpts servers.SchedulerHintOptsBuilder) (*servers.Server, error) {
	return servers.Create(ctx, c.compute, spec, hintOpts).Extract()
}

func (c *client) DeleteServer(ctx context.Context, serverId string) error {
	return servers.Delete(ctx, c.compute, serverId).ExtractErr()
}
