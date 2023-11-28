#!/usr/bin/env bash

# -----------------------------------------------------------------------------
# Description: This script sets up the environment and runs E2E tests for the
#              BMO project. It uses either vbmc or sushy-tools based on 
#              the BMO_E2E_EMULATOR environment variable.
# Usage:       export BMO_E2E_EMULATOR="vbmc"  # Or "sushy-tools"
#              ./ci-e2e.sh
# -----------------------------------------------------------------------------

set -eux

REPO_ROOT=$(realpath "$(dirname "${BASH_SOURCE[0]}")/..")

cd "${REPO_ROOT}" || exit 1

# BMO_E2E_EMULATOR can be set to either "vbmc" or "sushy-tools"
BMO_E2E_EMULATOR=${BMO_E2E_EMULATOR:-"sushy-tools"}

# Ensure requirements are installed
"${REPO_ROOT}/hack/e2e/ensure_go.sh"
export PATH="${PATH}:/usr/local/go/bin"
"${REPO_ROOT}/hack/e2e/ensure_minikube.sh"
# CAPI test framework uses kubectl in the background
"${REPO_ROOT}/hack/e2e/ensure_kubectl.sh"

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

# Create libvirt domain
VM_NAME="bmo-e2e-0"
export BOOT_MAC_ADDRESS="00:60:2f:31:81:01"

virt-install \
  --connect qemu:///system \
  --name "${VM_NAME}" \
  --description "Virtualized BareMetalHost" \
  --osinfo=ubuntu-lts-latest \
  --ram=4096 \
  --vcpus=2 \
  --disk size=20 \
  --graphics=none \
  --console pty \
  --serial pty \
  --pxe \
  --network network=baremetal-e2e,mac="${BOOT_MAC_ADDRESS}" \
  --noautoconsole

# This IP is defined by the network we created above.
IP_ADDRESS="192.168.222.1"

# These variables are used by the tests. They override variables in the config file.
# These are the VBMC defaults (used since we did not specify anything else for `vbmc add`).
export BMC_USER=admin
export BMC_PASSWORD=password

if [[ "${BMO_E2E_EMULATOR}" == "vbmc" ]]; then
	# VBMC variables
  VBMC_PORT="16230"
  export BMC_ADDRESS="ipmi://${IP_ADDRESS}:${VBMC_PORT}"

  # Start VBMC
  docker run --name vbmc --network host -d \
    -v /var/run/libvirt/libvirt-sock:/var/run/libvirt/libvirt-sock \
    -v /var/run/libvirt/libvirt-sock-ro:/var/run/libvirt/libvirt-sock-ro \
    quay.io/metal3-io/vbmc

  # Add BMH VM to VBMC
  docker exec vbmc vbmc add "${VM_NAME}" --port "${VBMC_PORT}"
  docker exec vbmc vbmc start "${VM_NAME}"
  docker exec vbmc vbmc list

elif [[ "${BMO_E2E_EMULATOR}" == "sushy-tools" ]]; then
  # Sushy-tools variables
  SUSHY_EMULATOR_FILE="${REPO_ROOT}"/test/e2e/sushy-tools/sushy-emulator.conf
  SUSHY_PORT="8000"
  export BMC_ADDRESS="redfish+http://${IP_ADDRESS}:${SUSHY_PORT}/redfish/v1/Systems/${VM_NAME}"

  # Start sushy-tools
  docker run --name sushy-tools -d --network host \
    -v "${SUSHY_EMULATOR_FILE}":/etc/sushy/sushy-emulator.conf:Z \
    -v /var/run/libvirt:/var/run/libvirt:Z \
    -e SUSHY_EMULATOR_CONFIG=/etc/sushy/sushy-emulator.conf \
    quay.io/metal3-io/sushy-tools:latest sushy-emulator

else
  echo "Invalid e2e emulator specified: ${BMO_E2E_EMULATOR}"
  exit 1
fi

# Image server variables
CIRROS_VERSION="0.6.2"
IMAGE_FILE="cirros-${CIRROS_VERSION}-x86_64-disk.img"
export IMAGE_CHECKSUM="c8fc807773e5354afe61636071771906"
export IMAGE_URL="http://${IP_ADDRESS}/${IMAGE_FILE}"
IMAGE_DIR="${REPO_ROOT}/test/e2e/images"

## Download and run image server
mkdir -p "${IMAGE_DIR}"
pushd "${IMAGE_DIR}"
wget --quiet "https://download.cirros-cloud.net/${CIRROS_VERSION}/${IMAGE_FILE}"
popd

docker run --name image-server-e2e -d \
  -p 80:8080 \
  -v "${IMAGE_DIR}:/usr/share/nginx/html" nginxinc/nginx-unprivileged

# We need to gather artifacts/logs before exiting also if there are errors
set +e

# Run the e2e tests
make test-e2e
test_status="$?"

# Collect all artifacts
tar --directory test/e2e/ -czf artifacts.tar.gz _artifacts

exit "${test_status}"
