package main

import (
	osplugin "github.com/sardinasystems/fleeting-plugin-openstack"
	"gitlab.com/gitlab-org/fleeting/fleeting/plugin"
)

func main() {
	plugin.Serve(&osplugin.InstanceGroup{})
}
