fleeting-plugin-openstack
=========================

GitLab fleeting plugin for OpenStack.

https://docs.gitlab.com/runner/executors/docker_autoscaler.html


Plugin Configuration
--------------------

The following parameters are supported:

| Parameter             | Type   | Description |
|-----------------------|--------|-------------|
| `cloud`               | string | Name of the cloud config from clouds.yaml to use |
| `clouds_config`       | string | Optional. Path to clouds.yaml |
| `auth_from_env`       | bool   | Optional. Use environment variables for authentication |
| `name`                | string | Name of the Auto Scaling Group (unique string that used to find instances) |
| `nova_microversion`   | string | Optional. Microversion for the Openstack Nova client. Default 2.79 (which should be ok for Train+) |
| `boot_time`           | string | Optional. Maximum wait time for instance to boot up. During that time plugin check Cloud-Init signatures. |
| `use_ignition`        | string | Enable Fedora CoreOS / Flatcar Linux Ignition support |
| `server_spec`         | object | Server spec used to create instances. See: [Compute API](https://docs.openstack.org/api-ref/compute/#create-server) |


### Default connector config

| Parameter                | Default  |
|--------------------------|----------|
| `os`                     | `linux`  |
| `protocol`               | `ssh`    |
| `username`               | `unset`  |
| `use_static_credentials` | `false`  |


OpenStack setup
---------------

1. You should create a special user (recommended) and project (optional),
   then export clouds.yaml with credentials for that cloud.

  1. Optional: You can also use OS\_\* environment variables to authenticate.

2. You may create a tenant network for workers, in that case don't forget to add a router.
   In that case manager VM should have two ports: external and that tenant network,
   so it will be able to connect to the worker instances.

3. You should upload a special image with container runtime installed in it.
   For example we use [Flatcar Linux](https://stable.release.flatcar-linux.net/amd64-usr/current/)

4. *(Optional)* You should generate SSH keypair which will be used by manager instance to connect to workers.
   Public key must be added to Nova from the user.

   Note: that key required only for Cloud-Init based images. For a Flatcar plugin can generate dynamic ssh key and pass it via Ignition script.

Preparation of the resources could be done by Heat using [heat/stack.yaml](heat/stack.yaml).
But consider it as an example.


Example runner config
---------------------
```toml
concurrent = 16
check_interval = 0
shutdown_timeout = 0
log_level = "info"

[session_server]
session_timeout = 1800
listen_address = ":8093"
advertise_address = "mgr.scalingrunner.cloud:8093"

[[runners]]
name = "manager"
url = "https://gitlab.com"
token = "token"
executor = "docker-autoscaler"
output_limit = 10240
shell = "bash"
environment = [
  "FF_NETWORK_PER_BUILD=1",
  "FF_USE_FASTZIP=1",
  "ARTIFACT_COMPRESSION_LEVEL=default",
  "CACHE_COMPRESSION_LEVEL=fastest",
  "FASTZIP_ARCHIVER_BUFFER_SIZE=67108864"
  ]

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
disable_cache = true
shm_size = 0
network_mtu = 0
# host = "unix:///run/user/1000/podman/podman.sock"
# tls_verify = false
# image = "quay.io/podman/stable"
image = "almalinux:9"
privileged = true
pull_policy = ["always", "always"]

[runners.autoscaler]
capacity_per_instance = 1
max_use_count = 10
max_instances = 16
# NOTE: If you manually download plugin and place it into your PATH:
# plugin = "fleeting-plugin-openstack"
# Or just run `gitlab-runner fleeting install` and it'll download OCI image automatically.
plugin = "ghcr.io/sardinasystems/fleeting-plugin-openstack:latest"

[runners.autoscaler.plugin_config]
cloud = "runner"
clouds_config = "/etc/gitlab-runner/clouds.yaml"
name = "scaling-runner-stack-id"
nova_microversion = "2.79" # train+
boot_time = "10m"
use_ignition = true  # enable injection of dynamic SSH key into Ignition config

[runners.autoscaler.plugin_config.server_spec]
name = "scaling-runner-%d"                                               # %d replaced with instance index
description = "GitLab CI Docker runners with autoscaling"
tags = ["GitLab", "CI", "Docker", "Scaling"]
imageRef = "d5460af5-83f3-47d7-9c4f-80294c66b267"                       # Flatcar Linux
flavorRef = "4e9d4fa4-a703-4850-8bc1-58b5e139ab57"                      # xlarge flavor
# key_name = "ci-admin"                                                 # SSH public key for worker nodes
networks = [ { uuid = "f05e7f64-9e0f-4c5c-acb0-b636000d7301" } ]        # tenant network
security_groups = [ "cee22d91-bb9a-455d-be88-e911d3cb066a" ]            # allow SSH ingress from tenant network
scheduler_hints = { group = "a9c941cb-5b34-46e0-8fc6-7471e3b77c75" }    # [Soft-]Anti-Affinity group
# May be used to pass #cloud-config or ignition scripts.
# If use_ignition == true, plugin will try parse existing script to inject passwd.users entry.
# Example: disable OS auto-updates
user_data = '''
{
  "ignition": {
    "version": "3.4.0"
  },
  "storage": {
    "files": [
      {
        "overwrite": true,
        "path": "/etc/flatcar/update.conf",
        "contents": {
          "compression": "",
          "source": "data:,SERVER%3Ddisabled%0AREBOOT_STRATEGY%3Doff%0A"
        },
        "mode": 272
      }
    ]
  }
}
'''

[runners.autoscaler.connector_config]
# username = "fedora"                    # Can be extracted from Image metadata os_admin_user
# password = ""                          # not used
# key_path = "/etc/gitlab-runner/id_rsa" # private key passed to server_spec.key_name. Required in cloud-init mode, optional for Ignition.
# use_static_credentials = true          # Tells to use key provided above.
keepalive = "30s"
timeout = "0m"
use_external_addr = false

[[runners.autoscaler.policy]]
idle_count = 2
idle_time = "30m0s"
scale_factor = 0.0
scale_factor_limit = 0
```
