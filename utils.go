package fpoc

import (
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/clustering/v1/clusters"
)

// ExtRemoveNodesOpts is an extended version of clusters.RemoveNodesOpts
// We need to destroy removed nodes, which is unavailable in vanilla
type ExtRemoveNodesOpts struct {
	Nodes                []string `json:"nodes" required:"true"`
	DestroyAfterDeletion bool     `json:"destroy_after_deletion"`
}

func (opts ExtRemoveNodesOpts) ToClusterRemoveNodeMap() (map[string]interface{}, error) {
	return gophercloud.BuildRequestBody(opts, "del_nodes")
}

func removeNodes(client *gophercloud.ServiceClient, clusterID string, opts ExtRemoveNodesOpts) (r clusters.ActionResult) {
	b, err := opts.ToClusterRemoveNodeMap()
	if err != nil {
		r.Err = err
		return
	}
	resp, err := client.Post(actionsURL(client, clusterID), b, &r.Body, &gophercloud.RequestOpts{
		OkCodes: []int{202},
	})
	_, r.Header, r.Err = gophercloud.ParseResponse(resp, err)
	return
}

func actionsURL(client *gophercloud.ServiceClient, id string) string {
	return client.ServiceURL("v1", "clusters", id, "actions")
}
