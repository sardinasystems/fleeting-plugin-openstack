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

Not working yet
---------------
```
Preparing instance...                               job=332687 project=179 runner=ksN2v_jPn
Dialing instance                                    external-address=10.0.42.231 instance-id=08de3734-bd6c-4e9e-bec2-70ebbc38fa22 internal-address=10.0.42.231 job=332687 project=179 runner=ksN2v_jPn use-external-address=true
Dialing instance 08de3734-bd6c-4e9e-bec2-70ebbc38fa22...  job=332687 project=179 runner=ksN2v_jPn
Instance 08de3734-bd6c-4e9e-bec2-70ebbc38fa22 connected  job=332687 project=179 runner=ksN2v_jPn
Volumes manager is empty, skipping volumes cleanup  job=332687 project=179 runner=ksN2v_jPn
ERROR: Failed to remove network for build           error=networksManager is undefined job=332687 network= project=179 runner=ksN2v_jPn
WARNING: Preparation failed: error during connect: Get "http://internel.tunnel.invalid/v1.24/info": dialing environment connection: ssh: rejected: connect failed (open failed) (docker.go:826:0s)  job=332687 project=179 runner=ksN2v_jPn
Will be retried in 3s ...                           job=332687 project=179 runner=ksN2v_jPn
```

While i can do a `ssh -i /etc/gitlab-runner/id_rsa fedora@10.0.42.231` without any problem.

```
# gitlab-runner version
Runtime platform                                    arch=amd64 os=linux pid=1751 revision=853330f9 version=16.5.0
```
