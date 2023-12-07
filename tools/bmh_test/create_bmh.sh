#!/usr/bin/env bash

# -------------------------------------------------------------------------------------------
# Description:    This script creates a virtual machine using virt-install,
#                 adds the virtual machine to VBMC (Virtual BMC) for out-of-band management,
#                 and applies the configuration to Kubernetes
#
# Usage:          make tilt-up -> press button in the right upper corner to create bmhs
#                 /tools/bmh_test/create_bmh.sh
# 
# Prerequecites:  a network with ip address of 192.168.222.1 named baremetal-e2e and
#                 VBMC runing
# -------------------------------------------------------------------------------------------

set -euxo pipefail

REPO_ROOT=$(realpath "$(dirname "${BASH_SOURCE[0]}")/../..")

cd "${REPO_ROOT}" || exit 1

# Set default values
NAME="bmh-test-${1:?}"
VBMC_PORT="${2:?}"
CONSUMER="${3:-}"
CONSUMER_NAMESPACE="${4:-}"

# Generate a random MAC address for the VM's network interface
MAC_ADDRESS="$(printf '00:60:2F:%02X:%02X:%02X\n' $((RANDOM%256)) $((RANDOM%256)) $((RANDOM%256)))"

# Create a virtual machine and connect it to vbmc
"${REPO_ROOT}/tools/bmh_test/create_vm.sh" "${NAME}" "${MAC_ADDRESS}"
"${REPO_ROOT}/tools/bmh_test/vm2vbmc.sh" "${NAME}" "${VBMC_PORT}"

# Create a YAML file to generate Kubernetes configuration for the VM
# Apply the generated YAML file to the cluster
if [[ -n "${CONSUMER}" ]] && [[ -n "${CONSUMER_NAMESPACE}" ]]; then
  echo "Applying YAML for controlplane host..."
  cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Secret
metadata:
  name: ${NAME}-bmc-secret
type: Opaque
stringData:
  username: admin
  password: password

---
apiVersion: metal3.io/v1alpha1
kind: BareMetalHost
metadata:
  name: ${NAME}
spec:
  online: true
  bmc:
    address: libvirt://192.168.122.1:${VBMC_PORT}/
    credentialsName: ${NAME}-bmc-secret
  bootMACAddress: ${MAC_ADDRESS}
  consumerRef:
    name: ${CONSUMER}
    namespace: ${CONSUMER_NAMESPACE}
EOF
else 
  echo "Applying YAML for host..."
  cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Secret
metadata:
  name: ${NAME}-bmc-secret
type: Opaque
stringData:
  username: admin
  password: password

---
apiVersion: metal3.io/v1alpha1
kind: BareMetalHost
metadata:
  name: ${NAME}
spec:
  online: true
  bmc:
    address: libvirt://192.168.122.1:${VBMC_PORT}/
    credentialsName: ${NAME}-bmc-secret
  bootMACAddress: ${MAC_ADDRESS}
EOF
fi
