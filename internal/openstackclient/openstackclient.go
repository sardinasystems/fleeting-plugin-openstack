package openstackclient

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"

	"github.com/caarlos0/env/v11"
	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/servers"
	"github.com/gophercloud/gophercloud/v2/openstack/config"
	"github.com/gophercloud/gophercloud/v2/openstack/config/clouds"
	"github.com/gophercloud/gophercloud/v2/openstack/image/v2/images"
	"github.com/gophercloud/gophercloud/v2/openstack/utils"
	osClient "github.com/gophercloud/utils/v2/client"
	"github.com/mitchellh/mapstructure"
)

type AuthConfig interface {
	Parse() (gophercloud.AuthOptions, gophercloud.EndpointOpts, *tls.Config, error)
	HTTPOpts() (debug bool, computeApiVersion string)
}

type CloudOpts struct {
	AllowReauth bool `envDefault:"true"`
}

type CloudConfig struct {
	ClientConfigFile  string `json:"client-config-file" env:"OS_CLIENT_CONFIG_FILE"`
	Cloud             string `json:"cloud" env:"OS_CLOUD"`
	RegionName        string `json:"region-name" env:"OS_REGION_NAME"`
	EndpointType      string `json:"endpoint-type" env:"OS_ENDPOINT_TYPE"`
	Debug             bool   `json:"debug" env:"OS_DEBUG"`
	ComputeApiVersion string `json:"compute-api-version" env:"OS_COMPUTE_API_VERSION" envDefault:"2.79"`
}

type EnvCloudConfig struct {
	CloudConfig `embed:"" yaml:",inline"`

	AuthURL                     string `json:"auth-url" env:"OS_AUTH_URL"`
	Username                    string `json:"username" env:"OS_USERNAME"`
	UserID                      string `json:"user-id" env:"OS_USER_ID"`
	Password                    string `json:"password" env:"OS_PASSWORD"`
	Passcode                    string `json:"passcode" env:"OS_PASSCODE"`
	ProjectName                 string `json:"project-name" env:"OS_PROJECT_NAME"`
	ProjectID                   string `json:"project-id" env:"OS_PROJECT_ID"`
	UserDomainName              string `json:"user-domain-name" env:"OS_USER_DOMAIN_NAME"`
	UserDomainID                string `json:"user-domain-id" env:"OS_USER_DOMAIN_ID"`
	ApplicationCredentialID     string `json:"application-credential-id" env:"OS_APPLICATION_CREDENTIAL_ID"`
	ApplicationCredentialName   string `json:"application-credential-name" env:"OS_APPLICATION_CREDENTIAL_NAME"`
	ApplicationCredentialSecret string `json:"application-credential-secret" env:"OS_APPLICATION_CREDENTIAL_SECRET"`
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

type client struct {
	compute *gophercloud.ServiceClient
	image   *gophercloud.ServiceClient
}

func New(ctx context.Context, authConfig AuthConfig, cloudOpts *CloudOpts) (Client, error) {
	if cloudOpts == nil {
		cloudOpts = &CloudOpts{}
	}

	var err error
	err = env.Parse(cloudOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to parse cloudOpts: %w", err)
	}

	err = env.Parse(authConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to parse authConfig: %w", err)
	}

	providerClient, endpointOps, err := NewProviderClient(ctx, authConfig, cloudOpts)
	if err != nil {
		return nil, err
	}

	computeClient, err := NewComputeClient(ctx, providerClient, endpointOps, authConfig)
	if err != nil {
		return nil, err
	}

	imageClient, err := openstack.NewImageV2(providerClient, endpointOps)
	if err != nil {
		return nil, err
	}

	return &client{
		compute: computeClient,
		image:   imageClient,
	}, nil
}

func (cloudConfig *CloudConfig) HTTPOpts() (debug bool, computeApiVersion string) {
	return cloudConfig.Debug, cloudConfig.ComputeApiVersion
}

func (cloudConfig *CloudConfig) Parse() (gophercloud.AuthOptions, gophercloud.EndpointOpts, *tls.Config, error) {
	parseOpts := []clouds.ParseOption{clouds.WithCloudName(cloudConfig.Cloud)}
	if cloudConfig.ClientConfigFile != "" {
		parseOpts = append(parseOpts, clouds.WithLocations(cloudConfig.ClientConfigFile))
	}

	authOptions, endpointOpts, tlsCfg, err := clouds.Parse(parseOpts...)
	if err != nil {
		return gophercloud.AuthOptions{}, gophercloud.EndpointOpts{}, nil, fmt.Errorf("failed to parse clouds.yaml: %w", err)
	}

	if cloudConfig.RegionName != "" {
		endpointOpts.Region = cloudConfig.RegionName
	}
	if cloudConfig.EndpointType != "" {
		endpointOpts.Availability = gophercloud.Availability(cloudConfig.EndpointType)
	}

	return authOptions, endpointOpts, tlsCfg, nil
}

func (envCloudConfig *EnvCloudConfig) Parse() (gophercloud.AuthOptions, gophercloud.EndpointOpts, *tls.Config, error) {
	if envCloudConfig.Cloud != "" {
		authOptions, endpointOpts, tlsCfg, err := envCloudConfig.CloudConfig.Parse()
		if err != nil {
			return gophercloud.AuthOptions{}, gophercloud.EndpointOpts{}, nil, err
		}

		if envCloudConfig.ProjectName != "" {
			authOptions.TenantName = envCloudConfig.ProjectName
			authOptions.TenantID = ""
		}
		if envCloudConfig.ProjectID != "" {
			authOptions.TenantID = envCloudConfig.ProjectID
			authOptions.TenantName = ""
		}

		return authOptions, endpointOpts, tlsCfg, nil
	}

	authOptions := gophercloud.AuthOptions{
		IdentityEndpoint:            envCloudConfig.AuthURL,
		UserID:                      envCloudConfig.UserID,
		Username:                    envCloudConfig.Username,
		Password:                    envCloudConfig.Password,
		Passcode:                    envCloudConfig.Passcode,
		TenantID:                    envCloudConfig.ProjectID,
		TenantName:                  envCloudConfig.ProjectName,
		DomainID:                    envCloudConfig.UserDomainID,
		DomainName:                  envCloudConfig.UserDomainName,
		ApplicationCredentialID:     envCloudConfig.ApplicationCredentialID,
		ApplicationCredentialName:   envCloudConfig.ApplicationCredentialName,
		ApplicationCredentialSecret: envCloudConfig.ApplicationCredentialSecret,
	}

	endpointOpts := gophercloud.EndpointOpts{
		Region:       envCloudConfig.RegionName,
		Availability: gophercloud.Availability(envCloudConfig.EndpointType),
	}

	return authOptions, endpointOpts, nil, nil
}

func NewHTTPClient(tlsCfg *tls.Config) http.Client {
	httpClient := http.Client{
		Transport: http.DefaultTransport.(*http.Transport).Clone(),
	}

	if tlsCfg != nil {
		tr := httpClient.Transport.(*http.Transport)
		tr.TLSClientConfig = tlsCfg
	}

	httpClient.Transport = &osClient.RoundTripper{
		Rt: httpClient.Transport,
	}
	return httpClient
}

func NewProviderClient(ctx context.Context, authConfig AuthConfig, cloudOpts *CloudOpts) (*gophercloud.ProviderClient, gophercloud.EndpointOpts, error) {
	authOptions, endpointOpts, tlsCfg, err := authConfig.Parse()
	if err != nil {
		return nil, gophercloud.EndpointOpts{}, err
	}

	httpClient := NewHTTPClient(tlsCfg)
	authOptions.AllowReauth = cloudOpts.AllowReauth

	providerClient, err := config.NewProviderClient(ctx, authOptions, config.WithHTTPClient(httpClient))
	if err != nil {
		return nil, gophercloud.EndpointOpts{}, err
	}

	return providerClient, endpointOpts, nil
}

func NewComputeClient(ctx context.Context, providerClient *gophercloud.ProviderClient, endpointOps gophercloud.EndpointOpts, authConfig AuthConfig) (*gophercloud.ServiceClient, error) {
	_, computeApiVersion := authConfig.HTTPOpts()

	computeClient, err := openstack.NewComputeV2(providerClient, endpointOps)
	if err != nil {
		return &gophercloud.ServiceClient{}, err
	}

	_computeClient, err := utils.RequireMicroversion(ctx, *computeClient, computeApiVersion)
	if err != nil {
		return &gophercloud.ServiceClient{}, err
	}

	return &_computeClient, err
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
