#!/usr/bin/env bash
#
# This script reads BMC information in a config file and prepare VMs 
# whose info match those config
#
set -x

REPO_ROOT=$(realpath "$(dirname "${BASH_SOURCE[0]}")")

virsh pool-destroy default || true
virsh pool-delete default || true
virsh pool-undefine default || true

CONFIG_FILE=$1
NETWORK=${2:-"baremetal-e2e"}

readarray -t BMCS < <(yq e -o=j -I=0 '.[]' "${CONFIG_FILE}")

for bmc in "${BMCS[@]}"; do
  bootMacAddress=$(echo "${bmc}" | jq -r '.bootMacAddress')
  ipAddress=$(echo "${bmc}" | jq -r '.ipAddress')
  virsh -c qemu:///system net-update "${NETWORK}" delete ip-dhcp-host "<host mac='${bootMacAddress}' ip='${ipAddress}'/>" --live --config
done
"${REPO_ROOT}/tools/bmh_test/clean_local_bmh_test_setup.sh" "^bmo-e2e-"
rm -rf /tmp/bmo-e2e-*
rm -rf /tmp/pool_oo
