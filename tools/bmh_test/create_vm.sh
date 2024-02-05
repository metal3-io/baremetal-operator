#!/usr/bin/env bash

set -eux

VM_NAME="${1:?}"
MAC_ADDRESS="${2:?}"
SERIAL_LOG_PATH="/var/log/libvirt/qemu/${VM_NAME}-serial0.log"

# Create a virtual machine
virt-install \
  --connect qemu:///system \
  --name "${VM_NAME}" \
  --description "Virtualized BareMetalHost" \
  --osinfo=ubuntu-lts-latest \
  --ram=4096 \
  --vcpus=2 \
  --disk size=20 \
  --graphics=none \
  --console pty,target_type=serial \
  --serial file,path="${SERIAL_LOG_PATH}" \
  --xml "./devices/serial/@type=pty" \
  --xml "./devices/serial/log/@file=${SERIAL_LOG_PATH}" \
  --xml "./devices/serial/log/@append=on" \
  --pxe \
  --network network=baremetal-e2e,mac="${MAC_ADDRESS}" \
  --noautoconsole
