#!/usr/bin/env bash

# -----------------------------------------------------------------------------
# Description: This script sets up the environment and runs E2E tests for the
#              BMO project. It uses either vbmc or sushy-tools based on
#              the BMC_PROTOCOL environment variable.
#              Supported protocols are: ipmi, redfish and redfish-virtualmedia.
#              VBMC is used for ipmi and sushy-tools for both redfish protocols.
#              By default, redfish-virtualmedia will be used.
# Usage:       export BMC_PROTOCOL="redfish"  # Or "ipmi" or "redfish-virtualmedia"
#              ./ci-e2e.sh
# -----------------------------------------------------------------------------

set -eux

REPO_ROOT=$(realpath "$(dirname "${BASH_SOURCE[0]}")/..")

cd "${REPO_ROOT}" || exit 1

BMC_PROTOCOL="${BMC_PROTOCOL:-"redfish-virtualmedia"}"
if [[ "${BMC_PROTOCOL}" == "redfish" ]] || [[ "${BMC_PROTOCOL}" == "redfish-virtualmedia" ]]; then
  BMO_E2E_EMULATOR="sushy-tools"
elif [[ "${BMC_PROTOCOL}" == "ipmi" ]]; then
  BMO_E2E_EMULATOR="vbmc"
else
  echo "FATAL: Invalid BMC protocol specified: ${BMC_PROTOCOL}"
  exit 1
fi

echo "BMC_PROTOCOL=${BMC_PROTOCOL}"
echo "BMO_E2E_EMULATOR=${BMO_E2E_EMULATOR}"

export E2E_CONF_FILE="${REPO_ROOT}/test/e2e/config/ironic.yaml"
export E2E_BMCS_CONF_FILE="${REPO_ROOT}/test/e2e/config/bmcs-${BMC_PROTOCOL}.yaml"

case "${GINKGO_FOCUS:-}" in
  *upgrade*)
    export DEPLOY_IRONIC="false"
    export DEPLOY_BMO="false"
    export DEPLOY_CERT_MANAGER="false"
    export GINKGO_NODES=1
    ;;
  *)
    export GINKGO_SKIP="${GINKGO_SKIP:-upgrade}"
    ;;
esac

# Ensure requirements are installed
export PATH="/usr/local/go/bin:${PATH}"
"${REPO_ROOT}/hack/e2e/ensure_go.sh"
"${REPO_ROOT}/hack/e2e/ensure_htpasswd.sh"
# CAPI test framework uses kubectl in the background
"${REPO_ROOT}/hack/e2e/ensure_kubectl.sh"
"${REPO_ROOT}/hack/e2e/ensure_yq.sh"

# Build the container image with e2e tag (used in tests)
IMG=quay.io/metal3-io/baremetal-operator:e2e make docker

virsh -c qemu:///system net-define "${REPO_ROOT}/hack/e2e/net.xml"
virsh -c qemu:///system net-start baremetal-e2e

# Allow traffic between docker bridges and the metal3 interface
sudo iptables -I FORWARD -i br-+ -o metal3 -j ACCEPT
sudo iptables -I FORWARD -i metal3 -o br-+ -j ACCEPT
sudo iptables -L FORWARD -n -v

# This IP is defined by the network we created above.
IP_ADDRESS="192.168.222.1"

# This IP is also defined by the network above, and is used consistently in all of
# our e2e overlays
export IRONIC_PROVISIONING_IP="${IP_ADDRESS}"

# Build vbmctl
make build-vbmctl
# Create VMs to act as BMHs in the tests.
./bin/vbmctl --yaml-source-file "${E2E_BMCS_CONF_FILE}"

if [[ "${BMO_E2E_EMULATOR}" == "vbmc" ]]; then
  # Start VBMC
  docker run --name vbmc --network host -d \
    -v /var/run/libvirt/libvirt-sock:/var/run/libvirt/libvirt-sock \
    -v /var/run/libvirt/libvirt-sock-ro:/var/run/libvirt/libvirt-sock-ro \
    quay.io/metal3-io/vbmc

  readarray -t BMCS < <(yq e -o=j -I=0 '.[]' "${E2E_BMCS_CONF_FILE}")
  for bmc in "${BMCS[@]}"; do
    address=$(echo "${bmc}" | jq -r '.address')
    hostName=$(echo "${bmc}" | jq -r '.name')
    vbmc_port="${address##*:}"
    "${REPO_ROOT}/tools/bmh_test/vm2vbmc.sh" "${hostName}" "${vbmc_port}" "${IP_ADDRESS}"
  done

elif [[ "${BMO_E2E_EMULATOR}" == "sushy-tools" ]]; then
  # Sushy-tools variables
  SUSHY_EMULATOR_FILE="${REPO_ROOT}"/test/e2e/sushy-tools/sushy-emulator.conf
  # Start sushy-tools
  docker run --name sushy-tools -d --network host \
    -v "${SUSHY_EMULATOR_FILE}":/etc/sushy/sushy-emulator.conf:Z \
    -v /var/run/libvirt:/var/run/libvirt:Z \
    -e SUSHY_EMULATOR_CONFIG=/etc/sushy/sushy-emulator.conf \
    quay.io/metal3-io/sushy-tools:latest sushy-emulator

else
  echo "FATAL: Invalid e2e emulator specified: ${BMO_E2E_EMULATOR}"
  exit 1
fi

# Image server variables
CIRROS_VERSION="0.6.2"
IMAGE_FILE="cirros-${CIRROS_VERSION}-x86_64-disk.img"
export IMAGE_CHECKSUM="c8fc807773e5354afe61636071771906"
export IMAGE_URL="http://${IP_ADDRESS}/${IMAGE_FILE}"
IMAGE_DIR="${REPO_ROOT}/test/e2e/images"
mkdir -p "${IMAGE_DIR}"

## Download disk images
wget --quiet -P "${IMAGE_DIR}/" https://artifactory.nordix.org/artifactory/metal3/images/iso/"${IMAGE_FILE}"
wget --quiet -P "${IMAGE_DIR}/" https://artifactory.nordix.org/artifactory/metal3/images/sysrescue/systemrescue-11.00-amd64.iso

## Start the image server
docker run --name image-server-e2e -d \
  -p 80:8080 \
  -v "${IMAGE_DIR}:/usr/share/nginx/html" nginxinc/nginx-unprivileged

# Generate ssh key pair for verifying provisioned BMHs
ssh-keygen -t ed25519 -f "${IMAGE_DIR}/ssh_testkey" -q -N ""

# Build an ISO image with baked ssh key
# See https://www.system-rescue.org/scripts/sysrescue-customize/
# We use the systemrescue ISO and their script for customizing it.
pushd "${IMAGE_DIR}"
wget -O sysrescue-customize https://gitlab.com/systemrescue/systemrescue-sources/-/raw/main/airootfs/usr/share/sysrescue/bin/sysrescue-customize?inline=false
chmod +x sysrescue-customize

pub_ssh_key=$(cut -d " " -f "1,2" "ssh_testkey.pub")

mkdir -p recipe/iso_add/sysrescue.d
# Reference: https://www.system-rescue.org/manual/Configuring_SystemRescue/
cat << EOF > recipe/iso_add/sysrescue.d/90-config.yaml
---
global:
    nofirewall: true
sysconfig:
    authorized_keys:
        "test@example.com": "${pub_ssh_key}"
EOF

./sysrescue-customize --auto --recipe-dir recipe --source systemrescue-11.00-amd64.iso --dest=sysrescue-out.iso
export ISO_IMAGE_URL="http://${IP_ADDRESS}/sysrescue-out.iso"
popd

# Generate credentials
BMO_OVERLAYS=(
  "${REPO_ROOT}/config/overlays/e2e"
  "${REPO_ROOT}/config/overlays/e2e-release-0.6"
  "${REPO_ROOT}/config/overlays/e2e-release-0.8"
  "${REPO_ROOT}/config/overlays/e2e-release-0.9"
)
IRONIC_OVERLAYS=(
  "${REPO_ROOT}/ironic-deployment/overlays/e2e"
  "${REPO_ROOT}/ironic-deployment/overlays/e2e-release-24.1"
  "${REPO_ROOT}/ironic-deployment/overlays/e2e-release-25.0"
  "${REPO_ROOT}/ironic-deployment/overlays/e2e-release-26.0"
)

IRONIC_USERNAME="$(uuidgen)"
IRONIC_PASSWORD="$(uuidgen)"

# These must be exported so that envsubst can pick them up below
export IRONIC_USERNAME
export IRONIC_PASSWORD

for overlay in "${BMO_OVERLAYS[@]}"; do
  echo "${IRONIC_USERNAME}" > "${overlay}/ironic-username"
  echo "${IRONIC_PASSWORD}" > "${overlay}/ironic-password"
done

for overlay in "${IRONIC_OVERLAYS[@]}"; do
  echo "IRONIC_HTPASSWD=$(htpasswd -n -b -B "${IRONIC_USERNAME}" "${IRONIC_PASSWORD}")" > \
    "${overlay}/ironic-htpasswd"
  envsubst < "${REPO_ROOT}/ironic-deployment/components/basic-auth/ironic-auth-config-tpl" > \
  "${overlay}/ironic-auth-config"
done

# We need to gather artifacts/logs before exiting also if there are errors
set +e

# Run the e2e tests
make test-e2e
test_status="$?"

LOGS_DIR="${REPO_ROOT}/test/e2e/_artifacts/logs"
mkdir -p "${LOGS_DIR}/qemu"
sudo sh -c "cp -r /var/log/libvirt/qemu/* ${LOGS_DIR}/qemu/"
sudo chown -R "${USER}:${USER}" "${LOGS_DIR}/qemu"

# Collect all artifacts
tar --directory test/e2e/ -czf "artifacts-e2e-${BMO_E2E_EMULATOR}-${BMC_PROTOCOL}.tar.gz" _artifacts

exit "${test_status}"
