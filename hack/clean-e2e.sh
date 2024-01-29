#!/usr/bin/env bash

REPO_ROOT=$(dirname "${BASH_SOURCE[0]}")/..
cd "${REPO_ROOT}" || exit 1

minikube delete
docker rm -f vbmc
docker rm -f image-server-e2e
docker rm -f sushy-tools

virsh_vms=$(virsh list --name --all)

for vm in ${virsh_vms}; do
  if [[ "${vm}" =~ "bmo-e2e-" ]]; then
    virsh -c qemu:///system destroy --domain "${vm}"
    virsh -c qemu:///system undefine --domain "${vm}" --nvram --remove-all-storage
  fi
done

virsh -c qemu:///system net-destroy baremetal-e2e
virsh -c qemu:///system net-undefine baremetal-e2e

rm -rf "${REPO_ROOT}/test/e2e/_artifacts"
rm -rf "${REPO_ROOT}"/artifacts-*
rm -rf "${REPO_ROOT}/test/e2e/images"
