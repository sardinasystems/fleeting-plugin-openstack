package fpoc

import (
	"maps"
	"regexp"
	"strings"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
	"github.com/mitchellh/mapstructure"
)

// BlockDeviceMappingV2 see https://docs.openstack.org/api-ref/compute/#create-server
type BlockDeviceMappingV2 struct {
	BootIndex           int    `json:"boot_index,omitempty"`
	DeleteOnTermination bool   `json:"delete_on_termination,omitempty"`
	DestinationType     string `json:"destination_type,omitempty"`
	DeviceName          string `json:"device_name,omitempty"`
	DeviceType          string `json:"device_type,omitempty"`
	DiskBus             string `json:"disk_bus,omitempty"`
	GuestFromat         string `json:"guest_format,omitempty"`
	NoDevice            bool   `json:"no_device,omitempty"`
	SourceType          string `json:"source_type,omitempty"`
	UUID                string `json:"uuid,omitempty"`
	VolumeSize          int    `json:"volume_size,omitempty"`
}

// SchedulerHints
type SchedulerHints struct {
	Group string `json:"group,omitempty"`
}

// ExtCreateOpts extended version of servers.CreateOpts
type ExtCreateOpts struct {
	servers.CreateOpts

	Description          string                 `json:"description,omitempty"`
	KeyName              string                 `json:"key_name,omitempty"`
	BlockDeviceMappingV2 []BlockDeviceMappingV2 `json:"block_device_mapping_v2,omitempty"`
	Networks             any                    `json:"networks,omitempty"`
	SecurityGroups       []string               `json:"security_groups,omitempty"`
	UserData             string                 `json:"user_data,omitempty"`
	SchedulerHints       *SchedulerHints        `json:"os:scheduler_hints,omitempty"`
}

// ToServerCreateMap for extended opts
func (opts ExtCreateOpts) ToServerCreateMap() (map[string]interface{}, error) {
	opts.CreateOpts.SecurityGroups = opts.SecurityGroups
	opts.CreateOpts.UserData = []byte(opts.UserData)

	ob, err := opts.CreateOpts.ToServerCreateMap()
	if err != nil {
		return nil, err
	}

	b, err := gophercloud.BuildRequestBody(opts, "")
	if err != nil {
		return nil, err
	}

	delete(b, "user_data")
	delete(b, "security_groups")

	sob := ob["server"].(map[string]any)
	maps.Copy(sob, b)

	return ob, nil
}

type Address struct {
	Version int    `json:"version"`
	Address string `json:"addr"`
	MACAddr string `json:"OS-EXT-IPS-MAC:mac_addr,omitempty"`
	Type    string `json:"OS-EXT-IPS:type,omitempty"`
}

func extractAddresses(srv *servers.Server) (map[string][]Address, error) {
	ret := make(map[string][]Address, len(srv.Addresses))

	for net, isv := range srv.Addresses {
		ism := isv.([]interface{})
		items := make([]Address, 0, len(ism))

		for _, iv := range ism {
			var out Address

			cfg := &mapstructure.DecoderConfig{
				Metadata: nil,
				Result:   &out,
				TagName:  "json",
			}
			decoder, _ := mapstructure.NewDecoder(cfg)
			err := decoder.Decode(iv)
			if err != nil {
				return nil, err
			}

			items = append(items, out)
		}

		ret[net] = items
	}

	return ret, nil
}

var initFinishedRe = regexp.MustCompile(`^.*Cloud-init\ v\.\ \d+\.\d+\.\d+\ finished\ at.*$`)

func IsCloudInitFinished(log string) bool {
	lines := strings.Split(log, "\n")

	for _, line := range lines {
		if initFinishedRe.MatchString(line) {
			return true
		}
	}
	return false
}
