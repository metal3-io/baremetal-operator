#!/usr/bin/env bash
#
# This script reads BMC information in a config file and prepare VMs 
# whose info match those config
#
set -eux

REPO_ROOT=$(realpath "$(dirname "${BASH_SOURCE[0]}")/..")
cd "${REPO_ROOT}"

CONFIG_FILE=$1
NETWORK=${2:-"baremetal-e2e"}

readarray -t BMCS < <(yq e -o=j -I=0 '.[]' "${CONFIG_FILE}")

for bmc in "${BMCS[@]}"; do
  address=$(echo "${bmc}" | jq -r '.address')
  bootMacAddress=$(echo "${bmc}" | jq -r '.bootMacAddress')
  hostName=$(echo "${bmc}" | jq -r '.hostName')
  ipAddress=$(echo "${bmc}" | jq -r '.ipAddress')

  # Add the the VM to the network host list
  virsh -c qemu:///system net-update "${NETWORK}" add-last ip-dhcp-host \
    "<host mac='${bootMacAddress}' name='${hostName}' ip='${ipAddress}' />" \
    --live --config --parent-index 0

  # Create VM
  "${REPO_ROOT}/tools/bmh_test/create_vm.sh" "${hostName}" "${bootMacAddress}"

  # Add BMH VM to VBMC
  if [[ "${address}" =~ "ipmi://" ]]; then
    vbmc_port="${address##*:}"
    "${REPO_ROOT}/tools/bmh_test/vm2vbmc.sh" "${hostName}" "${vbmc_port}"
  fi
done
