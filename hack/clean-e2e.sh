#!/usr/bin/env bash

REPO_ROOT=$(dirname "${BASH_SOURCE[0]}")/..
cd "${REPO_ROOT}" || exit 1

kind delete cluster
docker rm -f vbmc
docker rm -f image-server-e2e
docker rm -f sushy-tools

"${REPO_ROOT}/tools/bmh_test/clean_local_bmh_test_setup.sh" "^bmo-e2e-"

rm -rf "${REPO_ROOT}/test/e2e/_artifacts"
rm -rf "${REPO_ROOT}"/artifacts-*
rm -rf "${REPO_ROOT}/test/e2e/images"

# Clear network
virsh -c qemu:///system net-destroy baremetal-e2e
virsh -c qemu:///system net-undefine baremetal-e2e

# Clean volume pool directory
rm -rf /tmp/pool_oo/*

# Clean volume pool
virsh pool-destroy default || true
virsh pool-delete default || true
virsh pool-undefine default || true
