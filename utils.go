package fpoc

import (
	"encoding/json"
	"fmt"
	"maps"
	"regexp"
	"strings"

	igncfg "github.com/coreos/ignition/v2/config/v3_4"
	igntyp "github.com/coreos/ignition/v2/config/v3_4/types"
	"github.com/coreos/vcontext/report"
	"github.com/go-viper/mapstructure/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/servers"
)

// ExtCreateOpts extended version of servers.CreateOpts
// nolint:revive
type ExtCreateOpts struct {
	servers.CreateOpts

	// fields absent in gophercloud
	Description string `json:"description,omitempty"`
	KeyName     string `json:"key_name,omitempty"`

	// search for imageRef by name each time
	ImageName string `json:"image_name,omitempty"`

	// annotation overrides
	Networks       []servers.Network          `json:"networks,omitempty"`
	SecurityGroups []string                   `json:"security_groups,omitempty"`
	UserData       string                     `json:"user_data,omitempty"`
	SchedulerHints *servers.SchedulerHintOpts `json:"scheduler_hints,omitempty"`
}

// ToServerCreateMap for extended opts
func (opts ExtCreateOpts) ToServerCreateMap() (map[string]interface{}, error) {
	if opts.Networks != nil {
		opts.CreateOpts.Networks = opts.Networks
	}

	if opts.SecurityGroups != nil {
		opts.CreateOpts.SecurityGroups = opts.SecurityGroups
	}

	if opts.UserData != "" {
		opts.CreateOpts.UserData = []byte(opts.UserData)
	}

	ob, err := opts.CreateOpts.ToServerCreateMap()
	if err != nil {
		return nil, err
	}

	b := map[string]any{}
	if opts.Description != "" {
		b["description"] = opts.Description
	}
	if opts.KeyName != "" {
		b["key_name"] = opts.KeyName
	}

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

var (
	initFinishedRe   = regexp.MustCompile(`^.*Cloud-init\ v\.\ \S+\ finished\ at.*$`)
	initSSHHostKeyRe = regexp.MustCompile(`^SSH\ host\ key:\ \S+:\S+\ (\S+)$`)
	initLoginRe      = regexp.MustCompile(`^\S+\ login:\ .*$`)
)

func IsCloudInitFinished(log string) bool {
	lines := strings.Split(log, "\n")

	for _, line := range lines {
		if initFinishedRe.MatchString(line) {
			return true
		}
	}
	return false
}

func IsIgnitionFinished(log string) bool {
	lines := strings.Split(log, "\n")

	// Flatcar do not have any meaningful line,
	// so instead we first check that there ssh host key message
	// followed with login prompt
	searchKeys := true

	for _, line := range lines {

		if searchKeys {
			if initSSHHostKeyRe.MatchString(line) {
				searchKeys = false
			}
		} else {
			if initLoginRe.MatchString(line) {
				return true
			}
		}
	}
	return false
}

func InsertSSHKeyIgn(spec *ExtCreateOpts, username, pubKey string) error {
	var cfg igntyp.Config
	var err error

	if spec.UserData != "" {
		var rpt report.Report

		cfg, rpt, err = igncfg.ParseCompatibleVersion([]byte(spec.UserData))
		if err != nil {
			return fmt.Errorf("failed to parse ignition: %w", err)
		}

		_ = rpt
	}

	if cfg.Ignition.Version == "" {
		cfg.Ignition.Version = igntyp.MaxVersion.String()
	}

	var user *igntyp.PasswdUser
	if cfg.Passwd.Users == nil {
		cfg.Passwd.Users = make([]igntyp.PasswdUser, 0)
	}

	for idx, lu := range cfg.Passwd.Users {
		if lu.Name == username {
			user = &cfg.Passwd.Users[idx]
			break
		}
	}
	if user == nil {
		cfg.Passwd.Users = append(cfg.Passwd.Users, igntyp.PasswdUser{Name: username})
		user = &cfg.Passwd.Users[len(cfg.Passwd.Users)-1]
	}

	if user.SSHAuthorizedKeys == nil {
		user.SSHAuthorizedKeys = make([]igntyp.SSHAuthorizedKey, 0)
	}

	user.SSHAuthorizedKeys = append(user.SSHAuthorizedKeys, igntyp.SSHAuthorizedKey(pubKey))

	buf, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal ignition: %w", err)
	}

	spec.UserData = string(buf)
	return nil
}
