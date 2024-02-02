#!/usr/bin/env bash

set -ux

BMH_NAME_REGEX="${1:-^bmh-test-}"
# Get a list of all virtual machines
VM_LIST=$(virsh -c qemu:///system list --all --name | grep "${BMH_NAME_REGEX}")

if [[ -n "${VM_LIST}" ]]; then
    # Loop through the list and delete each virtual machine
    for vm_name in ${VM_LIST}; do
        virsh -c qemu:///system destroy --domain "${vm_name}"
        virsh -c qemu:///system undefine --domain "${vm_name}" --remove-all-storage
        kubectl delete baremetalhost "${vm_name}"
    done
else
    echo "No virtual machines found. Skipping..."
fi

# Clear vbmc
docker rm -f vbmc 

# Clear network
virsh -c qemu:///system net-destroy baremetal-e2e
virsh -c qemu:///system net-undefine baremetal-e2e
