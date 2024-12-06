#!/bin/bash

set -x

copybin() {
	local gorel=$1
	local plugarch=$2
	mkdir -p "dist/$plugarch/plugin"
	cp -p "dist/$gorel/bin"/* "dist/$plugarch/plugin/"
}

# move binaries to places expected by fleetine-artifact
copybin fleeting-plugin-openstack_linux_386_sse2 linux/386
copybin fleeting-plugin-openstack_linux_amd64_v1 linux/amd64
copybin fleeting-plugin-openstack_linux_arm64_v8.0 linux/arm64
copybin fleeting-plugin-openstack_linux_arm_7 linux/armv7

tag=${GITHUB_REF#refs/*/}

go install gitlab.com/gitlab-org/fleeting/fleeting-artifact/...@latest

# fleeting-artifact release ghcr.io/sardinasystems/fleeting-plugin-openstack:
fleeting-artifact release "ghcr.io/$GITHUB_REPOSITORY:$tag"
