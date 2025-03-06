module github.com/sardinasystems/fleeting-plugin-openstack

go 1.23.4

// https://gist.github.com/mitchellh/90029601268e59a29e64e55bab1c5bdc
replace github.com/mitchellh/mapstructure => github.com/go-viper/mapstructure v1.6.0

require (
	github.com/caarlos0/env/v11 v11.3.1
	github.com/coreos/ignition/v2 v2.20.0
	github.com/coreos/vcontext v0.0.0-20231102161604-685dc7299dc5
	github.com/go-viper/mapstructure/v2 v2.2.1
	github.com/gophercloud/gophercloud/v2 v2.6.0
	github.com/gophercloud/utils/v2 v2.0.0-20250212084022-725b94822eeb
	github.com/hashicorp/go-hclog v1.6.3
	github.com/jinzhu/copier v0.4.0
	github.com/stretchr/testify v1.10.0
	gitlab.com/gitlab-org/fleeting/fleeting v0.0.0-20250116074924-5d69933ca3b8
	golang.org/x/crypto v0.36.0
)

require (
	github.com/aws/aws-sdk-go v1.55.6 // indirect
	github.com/coreos/go-json v0.0.0-20231102161613-e49c8866685a // indirect
	github.com/coreos/go-semver v0.3.1 // indirect
	github.com/coreos/go-systemd/v22 v22.5.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/fatih/color v1.18.0 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/hashicorp/go-plugin v1.6.3 // indirect
	github.com/hashicorp/yamux v0.1.2 // indirect
	github.com/kr/pretty v0.1.0 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/oklog/run v1.1.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/vincent-petithory/dataurl v1.0.0 // indirect
	golang.org/x/mod v0.23.0 // indirect
	golang.org/x/net v0.35.0 // indirect
	golang.org/x/sys v0.31.0 // indirect
	golang.org/x/text v0.23.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250219182151-9fdb1cabc7b2 // indirect
	google.golang.org/grpc v1.70.0 // indirect
	google.golang.org/protobuf v1.36.5 // indirect
	gopkg.in/check.v1 v1.0.0-20180628173108-788fd7840127 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
