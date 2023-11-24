#!/usr/bin/env bash

REPO_ROOT=$(dirname "${BASH_SOURCE[0]}")/..
cd "${REPO_ROOT}" || exit 1

minikube delete
docker rm -f vbmc
docker rm -f image-server-e2e
docker rm -f sushy-tools
virsh -c qemu:///system destroy --domain bmo-e2e-0
virsh -c qemu:///system undefine --domain bmo-e2e-0 --remove-all-storage
virsh -c qemu:///system net-destroy baremetal-e2e
virsh -c qemu:///system net-undefine baremetal-e2e

rm -rfv "${REPO_ROOT}/test/e2e/_artifacts"
rm -rfv "${REPO_ROOT}/artifacts.tar.gz"
rm -rfv "${REPO_ROOT}/test/e2e/images"
