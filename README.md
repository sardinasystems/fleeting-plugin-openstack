fleeting-plugin-openstack
=========================

GitLab fleeting plugin for OpenStack.


Plugin Configuration
--------------------

The following parameters are supported:

| Parameter             | Type   | Description |
|-----------------------|--------|-------------|
| `cloud` | string | Name of the cloud config from clouds.yaml to use |
| `clouds_config` | string | Optional. Path to clouds.yaml |
| `name`                | string | Name of the Auto Scaling Group |
| `cluster_id` | string | Optional. UUID of the Senlin cluster. Overrides search by name. |
| `ssh_file` | string | Required. Private key of the ssh keypair given in senlin profile. |
| `ssh_user` | string | Optional. Set instance ssh username. |

### Default connector config

| Parameter                | Default  |
|--------------------------|----------|
| `os`                     | `linux`  |
| `protocol`               | `ssh` |
| `username`               | `ssh_user` |
| `use_static_credentials` | `false`  |

Cluster setup
-------------

OpenStack Senlin cluster requred. Example configuration you may find in etc/.

```
openstack cluster profile create --spec-file etc/sample_profile.yaml runner-profile
openstack cluster policy create --spec-file etc/sample_affinity_policy.yaml runner-aa-policy
openstack cluster create --profile runner-profile gitlab-runners
openstack cluster policy attach --policy runner-aa-policy gitlab-runners
```
