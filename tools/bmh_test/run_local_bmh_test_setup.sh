#!/usr/bin/env bash

set -eux

REPO_ROOT=$(realpath "$(dirname "${BASH_SOURCE[0]}")/../..")

cd "${REPO_ROOT}" || exit 1

# List of packages to check
commands=("virt-install" "virsh")

# Check each package
for cmd in "${commands[@]}"; do
    if ! command -v "${cmd}" &> /dev/null; then
        echo "ERROR: ${cmd} not found. Please install it."
        exit 1
    fi
done

# Define and start a virtual network
virsh -c qemu:///system net-define "${REPO_ROOT}/hack/e2e/net.xml"
virsh -c qemu:///system net-start baremetal-e2e

# Start VBMC
docker run --name vbmc --network host -d \
    -v /var/run/libvirt/libvirt-sock:/var/run/libvirt/libvirt-sock \
    -v /var/run/libvirt/libvirt-sock-ro:/var/run/libvirt/libvirt-sock-ro \
    quay.io/metal3-io/vbmc
