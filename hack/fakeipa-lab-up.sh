#!/usr/bin/env bash

# -----------------------------------------------------------------------------
# Description: Stands up the FakeIPA scalability test lab - the
#              baremetal-e2e bridge/veth pair to kind, an image server (for
#              Ironic's own ramdisk-downloader init container, which needs it
#              regardless of Fake IPA), and sushy-tools running with its
#              FakeDriver (no VMs, no libvirt domains) instead of the
#              libvirt-backed driver the stock e2e lab uses.
#
#              This must run BEFORE `make test-e2e`, the same way
#              hack/ci-e2e.sh runs the stock `vbmctl create bml` before
#              ginkgo starts: Ironic's own health check (in the Go suite's
#              SynchronizedBeforeSuite, which runs once before any spec) needs
#              this bridge and image server to already exist, and nothing
#              inside a spec's BeforeEach runs early enough to provide them.
#
# Usage:       ./hack/fakeipa-lab-up.sh [num-systems]
# -----------------------------------------------------------------------------

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
NUM_SYSTEMS="${1:-100}"

LAB_DIR="${FAKEIPA_LAB_DIR:-/tmp/fakeipa-lab}"
IMAGE_DIR="${FAKEIPA_IMAGE_DIR:-/tmp/fakeipa-image-cache}"
SUSHY_IMAGE="${SCALABILITY_FAKEIPA_SUSHY_IMAGE:-quay.io/metal3-io/sushy-tools:latest}"
VBMCTL_BIN="${SCALABILITY_FAKEIPA_VBMCTL_BIN:-${REPO_ROOT}/bin/vbmctl}"

mkdir -p "${LAB_DIR}" "${IMAGE_DIR}"

echo "Preparing image server content in ${IMAGE_DIR}..."

# Required for Ironic's ramdisk-downloader init container to succeed, even
# though Fake IPA never actually uses the ramdisk it fetches - matches
# hack/ci-e2e.sh's IPA_FILE.
if [[ ! -f "${IMAGE_DIR}/ipa-centos9-master.tar.gz" ]]; then
  wget --no-verbose -O "${IMAGE_DIR}/ipa-centos9-master.tar.gz" \
    https://tarballs.opendev.org/openstack/ironic-python-agent/dib/ipa-centos9-master.tar.gz
fi

# Matches test/e2e/config/ironic.yaml's IMAGE_URL/IMAGE_CHECKSUM.
if [[ ! -f "${IMAGE_DIR}/cirros-0.6.2-x86_64-disk.img" ]]; then
  wget --no-verbose -O "${IMAGE_DIR}/cirros-0.6.2-x86_64-disk.img" \
    https://download.cirros-cloud.net/0.6.2/cirros-0.6.2-x86_64-disk.img
fi

# test/e2e/config/ironic.yaml's ISO_IMAGE_URL default (minimal_linux_live-v2.iso)
# has no real upstream source and Fake IPA never actually boots or writes it,
# so a placeholder is enough to avoid a 404 if anything checks reachability.
if [[ ! -f "${IMAGE_DIR}/minimal_linux_live-v2.iso" ]]; then
  echo "placeholder live-iso content for Fake IPA testing" > "${IMAGE_DIR}/minimal_linux_live-v2.iso"
fi

echo "Generating ${NUM_SYSTEMS} fake Redfish systems..."

SYSTEMS_JSON="${LAB_DIR}/systems.json"
SUSHY_CONFIG="${LAB_DIR}/sushy-fakedriver.conf"

python3 - "${NUM_SYSTEMS}" "${SYSTEMS_JSON}" "${SUSHY_CONFIG}" <<'PYEOF'
import json
import sys
import uuid

num_systems = int(sys.argv[1])
systems_path = sys.argv[2]
sushy_config_path = sys.argv[3]

systems = []
fake_systems = []
for i in range(num_systems):
    system_uuid = str(uuid.uuid4())
    name = f"scale-fakeipa-{i:04d}"
    mac = "02:00:00:%02x:%02x:%02x" % ((i >> 16) & 0xFF, (i >> 8) & 0xFF, i & 0xFF)
    systems.append({"uuid": system_uuid, "name": name})
    fake_systems.append({
        "uuid": system_uuid,
        "name": name,
        "power_state": "Off",
        "external_notifier": True,
        "nics": [{"mac": mac, "ip": ""}],
    })

with open(systems_path, "w") as f:
    json.dump(systems, f)

# Fake IPA's default listen port is 9999, which is also FakeDriver's default
# EXTERNAL_NOTIFICATION_URL target - both sushy-tools and Fake IPA run with
# --network host, so they share the loopback interface directly.
with open(sushy_config_path, "w") as f:
    f.write("SUSHY_EMULATOR_FAKE_DRIVER = True\n")
    f.write('SUSHY_EMULATOR_LISTEN_IP = "0.0.0.0"\n')
    f.write("SUSHY_EMULATOR_LISTEN_PORT = 8000\n")
    # Fake IPA serves its agent API over HTTPS (self-signed cert, see
    # startFakeIPA in test/e2e/fakeipa_helpers.go) because Ironic's agent
    # API rejects heartbeats from an agent that announces a plain-http
    # callback URL ("TLS is required by configuration"). sushy-tools'
    # external notifier defaults to verify=False, so the self-signed cert
    # doesn't need to be trusted explicitly.
    f.write('EXTERNAL_NOTIFICATION_URL = "https://127.0.0.1:9999/"\n')
    f.write(f"SUSHY_EMULATOR_FAKE_SYSTEMS = {fake_systems!r}\n")
PYEOF

echo "Writing vbmctl lab config..."

# NOTE: BMCEmulatorConfig uses a flat YAML shape (see
# test/vbmctl/pkg/api/types.go UnmarshalYAML/MarshalYAML) - configFile is a
# direct child of bmcEmulator, not nested under a sushyToolsConfig key.
#
# The network/veth-pair/docker-network sections mirror the stock
# test/e2e/config/vbmctl.yaml.tmpl exactly: Ironic's own IRSO CR hardcodes
# networking.ipAddress 192.168.222.2 on this bridge independently of whether
# any BMH VMs exist on it.
cat > "${LAB_DIR}/vbmctl-fakeipa.yaml" <<EOF
apiVersion: vbmctl.metal3.io/v1alpha1
kind: Config
spec:
  libvirt:
    uri: qemu:///system
  pool:
    name: baremetal-e2e-fakeipa
    path: ${LAB_DIR}/pool
  vms: []
  networks:
  - name: baremetal-e2e
    bridge: metal3
    address: 192.168.222.1
    netmask: 24
  vethPairs:
  - link1: metal3
    link2: kind-bridge
    veth1: metalend
    veth2: kindend
  dockerNetworks:
  - name: "kind"
    bridgeName: "kind-bridge"
    subnet: "fc00:f853:ccd:e793::/64"
    driverMtu: 1500
    ipv6: true
  bmcEmulator:
    type: "sushy-tools"
    image: "${SUSHY_IMAGE}"
    configFile: "${SUSHY_CONFIG}"
  imageServer:
    port: 80
    dataDir: "${IMAGE_DIR}"
    containerName: "fakeipa-image-server"
EOF

echo "Running vbmctl create bml..."
"${VBMCTL_BIN}" -c "${LAB_DIR}/vbmctl-fakeipa.yaml" create bml

echo "FakeIPA lab is up. Systems list: ${SYSTEMS_JSON}"
