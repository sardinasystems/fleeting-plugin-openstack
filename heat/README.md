Runner heat stack
=================

This stack create most of resources needed to run autoscaler: a manager instance, network, DNS, anti-affinity group.

Stack is based on Flatcar Linux image, which runs `gitlab-runner` on manager node, and provides Docker on workers.
Flatcar is small, starts fast, usually under 30 sec. That makes it pretty useful for CI application.

Most likely you only need to provide `key_name` (to setup access into manager instance) and `public_net` (to setup external facing network).

You can also provide `dns_zone`, to register that zone in the Designate and add external records for manager and router gateway.
That might be useful to SSH to manager node.

Once manager is online you should `rsync` your `clouds.yaml` and `config.toml` into `/etc/gitlab-runner`.
Then `systemctl restart gitlab-runner.service` (or you can simply reboot the node).

Unfortunately I do not know a good way to pass this two files securely during ignition.
It's impossible to mount any Brabican secrets into a VM;
And there no easy way of providing temporary URL which is accessible only by your manager.
At least not with Heat...

So that steps automated with `ansible.builtin.raw`, as Flatcar do not have a Python.
Unfortunately that part consist lots of secrets, so i won't be able to opensource it.
