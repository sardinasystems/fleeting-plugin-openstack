package fpoc

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/clustering/v1/actions"
	"github.com/gophercloud/gophercloud/openstack/clustering/v1/clusters"
	"github.com/gophercloud/gophercloud/openstack/clustering/v1/nodes"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
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
	SSHPrivateKeyFile string `json:"ssh_file"`      // required: ssh key path
	SSHUser           string `json:"ssh_user"`      // required: ssh user to login

	size             int
	clusteringClient *gophercloud.ServiceClient
	computeClient    *gophercloud.ServiceClient
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
		return provider.ProviderInfo{}, fmt.Errorf("Failed to connect to OpenStack Senlin: %w", err)
	}

	cli.Microversion = "1.14" // antelope
	g.clusteringClient = cli

	cli, err = clientconfig.NewServiceClient("compute", opts)
	if err != nil {
		return provider.ProviderInfo{}, fmt.Errorf("Failed to connect to OpenStack Nova: %w", err)
	}

	cli.Microversion = "2.79" // train+
	g.computeClient = cli

	var cluster *clusters.Cluster
	if g.ClusterID != "" {
		cluster, err = clusters.Get(cli, g.ClusterID).Extract()
		if err != nil {
			return provider.ProviderInfo{}, fmt.Errorf("Failed to get cluster by id: %w", err)
		}
	} else {
		page, err := clusters.List(cli, clusters.ListOpts{Name: g.Name}).AllPages()
		if err != nil {
			return provider.ProviderInfo{}, fmt.Errorf("Failed to get cluster by name: %w", err)
		}

		cl, err := clusters.ExtractClusters(page)
		if err != nil {
			return provider.ProviderInfo{}, fmt.Errorf("Failed to get cluster extract error: %w", err)
		}
		if len(cl) != 1 {
			return provider.ProviderInfo{}, fmt.Errorf("Found %d clusters with name %s. You should provide cluster_id", len(cl), g.Name)
		}

		cluster = &cl[0]
		g.ClusterID = cluster.ID
	}

	pemBytes, err := os.ReadFile(g.SSHPrivateKeyFile)
	if err != nil {
		return provider.ProviderInfo{}, fmt.Errorf("SSH Private key file required: %w", err)
	}

	g.settings = settings
	g.log = log.With("name", g.Name, "cloud", g.Cloud, "cluster_name", cluster.Name, "cluster_id", cluster.ID)
	g.settings.Key = pemBytes
	g.size = 0

	if _, err := g.getNodes(ctx, true); err != nil {
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
	nodes_, err := g.getNodes(ctx, false)
	if err != nil {
		return err
	}

	servers_, err := g.getServers(ctx, nodes_)
	if err != nil {
		return err
	}

	for _, node := range nodes_ {
		state := provider.StateCreating

		switch node.Status {
		case "DELETING":
			state = provider.StateDeleting

		case "ACTIVE", "OPERATING":
			state = provider.StateRunning
		}

		srv, ok := servers_[node.PhysicalID]
		if ok {
			// TODO: srv.Status?
			_ = srv
		}

		update(node.ID, state)
	}

	return nil
}

func (g *InstanceGroup) Increase(ctx context.Context, delta int) (succeeded int, err error) {
	actionID, err := clusters.Resize(g.clusteringClient, g.ClusterID, clusters.ResizeOpts{
		AdjustmentType: clusters.ChangeInCapacityAdjustment,
		Number:         &delta,
	}).Extract()
	if err != nil {
		return 0, fmt.Errorf("Failed to resize increase: %w", err)
	}

	action, err := actions.Get(g.clusteringClient, actionID).Extract()
	if err != nil {
		return 0, fmt.Errorf("Failed to get resize action: %w", err)
	}

	g.log.Info("Increase", "delta", delta, "status", action.Status)
	g.size += delta

	return delta, nil
}

func (g *InstanceGroup) Decrease(ctx context.Context, instances []string) (succeeded []string, err error) {
	if len(instances) == 0 {
		return nil, nil
	}

	actionID, err := removeNodes(g.clusteringClient, g.ClusterID,
		ExtRemoveNodesOpts{
			Nodes:                instances,
			DestroyAfterDeletion: true,
		}).Extract()
	if err != nil {
		return nil, fmt.Errorf("Failed to remove nodes: %w", err)
	}

	action, err := actions.Get(g.clusteringClient, actionID).Extract()
	if err != nil {
		return nil, fmt.Errorf("Failed to get remove action: %w", err)
	}

	g.log.Info("Decrease", "instances", instances, "status", action.Status)
	g.size -= len(instances)

	return instances, err
}

func (g *InstanceGroup) getNodes(ctx context.Context, initial bool) ([]nodes.Node, error) {
	page, err := nodes.List(g.clusteringClient, nodes.ListOpts{ClusterID: g.ClusterID}).AllPages()
	if err != nil {
		return nil, fmt.Errorf("Node listing error: %w", err)
	}

	nodes, err := nodes.ExtractNodes(page)
	if err != nil {
		return nil, fmt.Errorf("Node listing extract error: %w", err)
	}

	size := len(nodes)

	if !initial && size != g.size {
		g.log.Error("out-of-sync capacity", "expected", g.size, "actual", size)
	}
	g.size = size

	return nodes, nil
}

func (g *InstanceGroup) getServers(ctx context.Context, nodelist []nodes.Node) (map[string]*servers.Server, error) {
	// Ideally i'd call server list with metadata.cluster_id=id, but we can't.
	// So have to query each server
	var reterr error
	srvs := make(map[string]*servers.Server, len(nodelist))

	for _, n := range nodelist {
		srv, err := servers.Get(g.computeClient, n.PhysicalID).Extract()
		if err != nil {
			reterr = errors.Join(reterr, err)
			g.log.Error("Failed to get server", "server_id", n.PhysicalID, "error", err)
			continue
		}
		srvs[srv.ID] = srv
	}

	return srvs, reterr
}

func (g *InstanceGroup) ConnectInfo(ctx context.Context, instanceID string) (provider.ConnectInfo, error) {
	node, err := nodes.Get(g.clusteringClient, instanceID).Extract()
	if err != nil {
		return provider.ConnectInfo{}, fmt.Errorf("Failed to get node %s: %w", instanceID, err)
	}

	srv, err := servers.Get(g.computeClient, node.PhysicalID).Extract()
	if err != nil {
		return provider.ConnectInfo{}, fmt.Errorf("Failed to get server %s: %w", node.PhysicalID, err)
	}

	if srv.Status != "ACTIVE" {
		return provider.ConnectInfo{}, fmt.Errorf("instance status is not active: %s", srv.Status)
	}

	// TODO: get image metadata and get os_admin_user

	info := provider.ConnectInfo{
		ConnectorConfig: g.settings.ConnectorConfig,
		InternalAddr:    srv.AccessIPv4,
		ExternalAddr:    srv.AccessIPv4,
	}

	// TODO: get from image meta
	info.OS = "linux"
	info.Arch = "amd64"
	if info.Username == "" {
		info.Username = g.SSHUser
	}

	if info.UseStaticCredentials {
		return info, nil
	}

	if info.Protocol == "" {
		info.Protocol = provider.ProtocolSSH
	}

	switch info.Protocol {
	case provider.ProtocolSSH:
		err = g.ssh(ctx, &info)

	case provider.ProtocolWinRM:
		err = fmt.Errorf("winrm not supported")
	}
	if err != nil {
		return provider.ConnectInfo{}, err
	}

	return info, nil
}

func (g *InstanceGroup) Shutdown(ctx context.Context) error {
	return nil
}
