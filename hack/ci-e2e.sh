#!/usr/bin/env bash

set -eux

REPO_ROOT=$(dirname "${BASH_SOURCE[0]}")/..
cd "${REPO_ROOT}" || exit 1

# Ensure requirements are installed
"${REPO_ROOT}/hack/e2e/ensure_go.sh"
export PATH="${PATH}:/usr/local/go/bin"
"${REPO_ROOT}/hack/e2e/ensure_minikube.sh"
"${REPO_ROOT}/hack/e2e/ensure_kubectl.sh"
"${REPO_ROOT}/hack/e2e/ensure_cmctl.sh"

# Build the container image with e2e tag (used in tests)
IMG=quay.io/metal3-io/baremetal-operator:e2e make docker

# Set up minikube
minikube start --driver=kvm2

virsh -c qemu:///system net-define "${REPO_ROOT}/hack/e2e/net.xml"
virsh -c qemu:///system net-start baremetal-e2e
# Attach baremetal-e2e interface to minikube with specific mac.
# This will give minikube a known reserved IP address that we can use for Ironic
virsh -c qemu:///system attach-interface --domain minikube --mac="52:54:00:6c:3c:01" \
  --model virtio --source baremetal-e2e --type network --config

# Restart minikube to apply the changes
minikube stop
minikube start

# Load the BMO e2e image into it
minikube image load quay.io/metal3-io/baremetal-operator:e2e

# Start VBMC
docker run --name vbmc --network host -d \
  -v /var/run/libvirt/libvirt-sock:/var/run/libvirt/libvirt-sock \
  -v /var/run/libvirt/libvirt-sock-ro:/var/run/libvirt/libvirt-sock-ro \
  quay.io/metal3-io/vbmc

# Create libvirt domain
NAME="bmo-e2e-0"
BOOT_MAC_ADDRESS="00:60:2f:31:81:01"
VBMC_PORT="16230"
virt-install --connect qemu:///system -n "${NAME}" --description "Virtualized BareMetalHost" --osinfo=ubuntu-lts-latest \
  --ram=4096 --vcpus=2 --disk size=20 --graphics=none --console pty --serial pty --pxe \
  --network network=baremetal-e2e,mac="${BOOT_MAC_ADDRESS}" --noautoconsole

# Add BMH VM to VBMC
docker exec vbmc vbmc add "${NAME}" --port "${VBMC_PORT}"
docker exec vbmc vbmc start "${NAME}"
docker exec vbmc vbmc list

# These variables are used by the tests. They override variables in the config file.
# This IP is defined by the network we created above.
# Together with the VBMC_PORT this becomes the BMC_ADDRESS used by the BMH in the test.
export BMC_ADDRESS="ipmi://192.168.222.1:${VBMC_PORT}"
export BOOT_MAC_ADDRESS
# These are the VBMC defaults (used since we did not specify anything else for `vbmc add`).
export BMC_USER=admin
export BMC_PASSWORD=password

# We need to gather artifacts/logs before exiting also if there are errors
set +e

# Run the e2e tests
make test-e2e
test_status="$?"

# Collect all artifacts
tar --directory test/e2e/ -czf artifacts.tar.gz _artifacts

exit "${test_status}"
