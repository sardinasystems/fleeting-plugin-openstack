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
[runners.autoscaler]
plugin = "fleeting-plugin-openstack"
capacity_per_instance = 1
max_use_count = 1
max_instances = 25

[runners.autoscaler.plugin_config]
  name             = "gitlab-runners"
  cloud            = "mycloud"

[runners.autoscaler.connector_config]
  username          = "fedora"
  key_file          = "/etc/gitlab-runner/id_rsa"

[[runners.autoscaler.policy]]
  idle_count = 4
  idle_time = "15m0s"
```
