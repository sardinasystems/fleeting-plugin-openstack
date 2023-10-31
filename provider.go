package fpoc

import (
	"context"
	"os"
	"path"

	"github.com/gopercloud/gopercloud"
	"github.com/gophercloud/utils/openstack/clientconfig"
	"github.com/hashicorp/go-hclog"

	"gitlab.com/gitlab-org/fleeting/fleeting/provider"
)

var _ provider.InstanceGroup = (*InstanceGroup)(nil)

type InstanceGroup struct {
	Cloud             string `json:"cloud"`         // cloud to use
	CloudsConfig      string `json:"clouds_config"` // optional: path to clouds.yaml
	Name              string `json:"name"`          // name of the group / cluster name
	ClusterID         string `json:"cluster_id"`    // optional: cluster id
	SshPrivateKeyFile string `json:"ssh_file"`      // required: ssh key path

	size             int64
	clusteringClient *gopercloud.ProviderClient
	settings         provider.Settings
	log              hclog.Logger
}

func (g *InstanceGroup) Init(ctx context.Context, log hclog.Logger, settings provider.Settings) (provider.ProviderInfo, error) {
	if g.CloudsConfig != "" {
		os.Setenv("OS_CLIENT_CONFIG_FILE", g.CloudsConfig)
	}

	opts := &clientconfig.ClientOpts{
		Cloud: g.Cloud,
	}

	cli, err := clientconfig.NewServiceClient("clustering", opts)
	if err != nil {
		log.Fatal(err)
	}

	g.clusteringClient = cli

	pemBytes, err := os.ReadFile(g.SshPrivateKeyFile)
	if err != nil {
		log.Fatal(err)
	}

	g.settings = settings
	g.log = log.With("name", g.Name, "cloud", g.Cloud)
	g.settings.Key = pemBytes
	g.size = 0

	if _, err := g.getInstances(ctx, true); err != nil {
		return provider.ProviderInfo{}, err
	}

	return provider.ProviderInfo{
		ID:      path.Join("openstack", g.Cloud, g.Name),
		MaxSize: 1000,
		// Version:   Version.String(),
		// BuildInfo: Version.BuildInfo(),
	}, nil

}

func (g *InstanceGroup) Update(ctx context.Context, update func(instance string, state provider.State)) error {
	/*
		instances, err := g.ycsdk.InstanceGroup().InstanceGroup().ListInstances(ctx,
			&instancegroup.ListInstanceGroupInstancesRequest{
				InstanceGroupId: g.InstanceGroupId,
			})
		for _, instance := range instances.Instances {
			var state provider.State
			switch instance.Status.String() {
			case "PREPARING_RESOURCES", "CREATING_INSTANCE", "STARTING_INSTANCE":
				state = provider.StateCreating
			case "DELETING_INSTANCE", "STOPPING_INSTANCE", "DELETED":
				state = provider.StateDeleting
			case "RUNNING_ACTUAL":
				state = provider.StateRunning
			default:
				g.logger.Warn("unknown instance status", "instance", instance.Name, "status", instance.Status.String())
				continue
			}

			update(instance.Id, state)
		}

		if err != nil {
			return err
		}

	*/
	return nil
}

func (g *InstanceGroup) Increase(ctx context.Context, delta int) (succeeded int, err error) {
	/*
		newSizeInstanceGroup := g.size + int64(delta)
		increaseInstanceReq := instancegroup.UpdateInstanceGroupRequest{
			InstanceGroupId: g.InstanceGroupId,
			ScalePolicy:     &instancegroup.ScalePolicy{ScaleType: &instancegroup.ScalePolicy_FixedScale_{FixedScale: &instancegroup.ScalePolicy_FixedScale{Size: newSizeInstanceGroup}}},
			UpdateMask:      &fieldmaskpb.FieldMask{Paths: []string{"scale_policy"}},
		}
		op, err := g.ycsdk.WrapOperation(g.ycsdk.InstanceGroup().InstanceGroup().Update(ctx, &increaseInstanceReq))

		if err != nil {
			log.Fatal(err)
			return 0, err
		}

		err = op.Wait(ctx)
		if err != nil {
			log.Fatal(err)
			return 0, err
		}

		g.size = newSizeInstanceGroup

		return delta, nil
	*/

	return 0, nil
}

func (g *InstanceGroup) Decrease(ctx context.Context, instances []string) (succeeded []string, err error) {
	/*
	   	instanceGroupDeleteReq := instancegroup.DeleteInstancesRequest{
	   		InstanceGroupId:    g.InstanceGroupId,
	   		ManagedInstanceIds: instances,
	   	}

	   op, err := g.ycsdk.WrapOperation(g.ycsdk.InstanceGroup().InstanceGroup().DeleteInstances(ctx, &instanceGroupDeleteReq))

	   	if err != nil {
	   		log.Println(err)
	   		return nil, err
	   	}

	   err = op.Wait(ctx)

	   	if err != nil {
	   		log.Println(err)
	   		return nil, err
	   	}

	   g.size = g.size - int64(len(instances))
	   return instances, err
	*/
	return nil, nil
}

func (g *InstanceGroup) ConnectInfo(ctx context.Context, instanceId string) (provider.ConnectInfo, error) {
	/*
		info := provider.ConnectInfo{ConnectorConfig: g.settings.ConnectorConfig}

		instances, err := g.ycsdk.InstanceGroup().InstanceGroup().ListInstances(ctx,
			&instancegroup.ListInstanceGroupInstancesRequest{
				InstanceGroupId: g.InstanceGroupId,
			})
		if err != nil {
			return provider.ConnectInfo{}, err
		}
		for _, instance := range instances.Instances {
			if instance.Id == instanceId {
				if instance.Status.String() != "RUNNING_ACTUAL" {
					return provider.ConnectInfo{}, fmt.Errorf("instance status is not running (%s)", instance.Status.String())
				}
				ipAddress := instance.NetworkInterfaces[0].GetPrimaryV4Address()

				info.InternalAddr = ipAddress.Address
				info.ExternalAddr = ipAddress.OneToOneNat.GetAddress()
			}
		}

		return info, nil
	*/
	return nil, nil
}

func (g *InstanceGroup) Shutdown(ctx context.Context) error {
	return nil
}
