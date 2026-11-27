#!/usr/bin/env bash

# -----------------------------------------------------------------------------
# Description: Generates a sushy-tools config that enables the fake driver and
#              fake-IPA and defines NUM_HOSTS virtual Redfish systems without
#              any backing VM. Used by the scalability e2e tests with
#              BMC_PROTOCOL=fake.
#
#              The UUID and boot MAC address of system N must match what the
#              tests use for BMH N (systemUUID and macFromIndex in
#              test/e2e/scalability_test.go). The fake driver parses system
#              ids as UUIDs, so names are not usable as ids.
#
# Usage:       generate_sushy_fake_config.sh <num-hosts> <listen-ip> <listen-port> <output-file>
# -----------------------------------------------------------------------------

set -o errexit -o nounset -o pipefail

if [[ $# -ne 4 ]]; then
  echo "Usage: $0 <num-hosts> <listen-ip> <listen-port> <output-file>" >&2
  exit 1
fi

num_hosts="$1"
listen_ip="$2"
listen_port="$3"
output="$4"

if ! [[ "${num_hosts}" =~ ^[0-9]+$ ]] || [[ "${num_hosts}" -lt 1 ]]; then
  echo "num-hosts must be a positive integer, got: ${num_hosts}" >&2
  exit 1
fi

{
  cat << EOF
SUSHY_EMULATOR_LISTEN_IP = u"${listen_ip}"
SUSHY_EMULATOR_LISTEN_PORT = ${listen_port}
SUSHY_EMULATOR_FAKE_DRIVER = True
SUSHY_EMULATOR_FAKE_IPA = True
SUSHY_EMULATOR_FAKE_SYSTEMS = [
EOF

  for ((i = 0; i < num_hosts; i++)); do
    uuid="$(printf '00000000-0000-4000-8000-%012x' "${i}")"
    mac="$(printf '02:00:00:%02x:%02x:%02x' $(((i >> 16) & 255)) $(((i >> 8) & 255)) $((i & 255)))"
    # The NIC IP is only reported by fake-ipa in the inventory, it does not
    # need to be reachable. Use 10.0.0.0/8 as there can be more hosts than
    # addresses in the provisioning network.
    n=$((i + 1))
    ip="$(printf '10.%d.%d.%d' $(((n >> 16) & 255)) $(((n >> 8) & 255)) $((n & 255)))"
    cat << EOF
    {
        "uuid": "${uuid}",
        "name": "fake-host-${i}",
        "power_state": "Off",
        "external_notifier": True,
        "nics": [
            {"mac": "${mac}", "ip": "${ip}"},
        ],
    },
EOF
  done

  echo "]"
} > "${output}"
