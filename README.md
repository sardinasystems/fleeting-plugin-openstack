fleeting-plugin-openstack
=========================

GitLab fleeting plugin for OpenStack.

**WIP**

Plugin Configuration
--------------------

The following parameters are supported:

| Parameter             | Type   | Description |
|-----------------------|--------|-------------|
| `cloud` | string | Name of the cloud config from clouds.yaml to use |
| `clouds_config` | string | Optional. Path to clouds.yaml |
| `name`                | string | Name of the Auto Scaling Group |
| `cluster_id` | string | Optional. UUID of the Senlin cluster. Overrides search by name. |
| `boot_time` | string | Optional. Wait some time since instance creation to complete boot up process. |

### Default connector config

| Parameter                | Default  |
|--------------------------|----------|
| `os`                     | `linux`  |
| `protocol`               | `ssh` |
| `username`               | `unset` |
| `use_static_credentials` | `true`  |

Cluster setup
-------------

OpenStack Senlin cluster requred. Example configuration you may find in etc/.

```
openstack cluster profile create --spec-file etc/sample_profile.yaml runner-profile
openstack cluster policy create --spec-file etc/sample_affinity_policy.yaml runner-aa-policy
openstack cluster create --profile runner-profile gitlab-runners
openstack cluster policy attach --policy runner-aa-policy gitlab-runners
```

Example runner config
---------------------
```
[[runners]]
name = "manager"
url = "https://gitlab.com"
token = "token"
executor = "docker-autoscaler"
shell = "bash"

[runners.cache]
Type = "s3"
Shared = true

[runners.cache.s3]
ServerAddress = "s3.foo.bar"
AccessKey = "access"
SecretKey = "secret"
BucketName = "cache"

[runners.docker]
disable_entrypoint_overwrite = false
oom_kill_disable = false
disable_cache = false
shm_size = 0
network_mtu = 0
host = "unix:///run/user/1000/podman/podman.sock"
tls_verify = false
image = "quay.io/podman/stable"
privileged = true
#privileged = false
pull_policy = ["always", "always"]

[runners.autoscaler]
capacity_per_instance = 1
max_use_count = 10
max_instances = 10
plugin = "fleeting-plugin-openstack"

[runners.autoscaler.plugin_config]
cloud = "runner"
# clouds_file = "/etc/openstack/clouds.yaml"
name = "senlin-cluster"
# cluster_id = {{ cluster_id|to_json }}
boot_time = "1m"

[runners.autoscaler.connector_config]
username = "fedora"
password = ""
key_path = "/etc/gitlab-runner/id_rsa"
use_static_credentials = true
keepalive = "30s"
timeout = "5m"
use_external_addr = false

[[runners.autoscaler.policy]]
idle_count = 2
idle_time = "15m0s"
scale_factor = 0.0
scale_factor_limit = 0
```
