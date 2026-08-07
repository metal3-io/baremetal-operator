#!/usr/bin/env bash

# -----------------------------------------------------------------------------
# Scalability Test for Baremetal Operator (BMO)
#
# Measures how many BareMetalHost resources BMO can enroll within a given time window.
#
# Architecture:
#   - Kind cluster with BMO + Ironic deployed
#   - sushy-tools BMC emulator responding to Redfish API for N fake systems
#     (no real VMs needed — sushy-tools uses its "fake" libvirt driver)
#   - Inspection disabled on BMHs (no IPA boot required)
#   - Measures time for all BMHs to reach target states
#
# Usage:
#   NUM_HOSTS=50 ./hack/scalability-tests/run.sh
#   NUM_HOSTS=100 SKIP_PROVISIONING=true ./hack/scalability-tests/run.sh
#   NUM_HOSTS=200 MAX_CONCURRENT_RECONCILES=10 ./hack/scalability-tests/run.sh
# -----------------------------------------------------------------------------

set -o errexit
set -o nounset
set -o pipefail

REPO_ROOT=$(realpath "$(dirname "${BASH_SOURCE[0]}")/../..")

# ==============================================================================
# Configuration (override via environment)
# ==============================================================================

# Number of BareMetalHost resources to create
NUM_HOSTS="${NUM_HOSTS:-50}"

# Whether to skip the provisioning phase (only measure enrollment)
SKIP_PROVISIONING="${SKIP_PROVISIONING:-false}"

# Maximum time to wait for enrollment (all hosts → available)
ENROLLMENT_TIMEOUT="${ENROLLMENT_TIMEOUT:-600}"

# Maximum time to wait for provisioning (all hosts → provisioned)
PROVISIONING_TIMEOUT="${PROVISIONING_TIMEOUT:-900}"

# BMO controller concurrency setting
MAX_CONCURRENT_RECONCILES="${MAX_CONCURRENT_RECONCILES:-3}"

# Polling interval for state checks (seconds)
POLL_INTERVAL="${POLL_INTERVAL:-2}"

# Namespace for test BMHs
TEST_NAMESPACE="${TEST_NAMESPACE:-scalability-test}"

# sushy-tools configuration
SUSHY_TOOLS_IMAGE="${SUSHY_TOOLS_IMAGE:-quay.io/metal3-io/sushy-tools:latest}"
SUSHY_TOOLS_PORT="${SUSHY_TOOLS_PORT:-8000}"
SUSHY_TOOLS_CONTAINER="${SUSHY_TOOLS_CONTAINER:-scalability-sushy-tools}"

# Network configuration
BMC_ADDRESS="${BMC_ADDRESS:-127.0.0.1}"
# BMO deployment
DEPLOY_BMO="${DEPLOY_BMO:-true}"

# Artifacts directory for results
ARTIFACTS="${ARTIFACTS:-${REPO_ROOT}/hack/scalability-tests/_artifacts}"

# Kind cluster name
CLUSTER_NAME="${CLUSTER_NAME:-bmo-scalability}"

# Use existing cluster instead of creating a new one
USE_EXISTING_CLUSTER="${USE_EXISTING_CLUSTER:-false}"

# Image for BMO (set to "e2e" to use locally built image)
BMO_IMAGE_TAG="${BMO_IMAGE_TAG:-e2e}"

# Fake image URL for provisioning (not actually downloaded)
IMAGE_URL="${IMAGE_URL:-http://${BMC_ADDRESS}/fake-image.qcow2}"
IMAGE_CHECKSUM="${IMAGE_CHECKSUM:-e3b0c44298fc1c149afbf4c8996fb924}"

# ==============================================================================
# Helper Functions
# ==============================================================================

log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*"
}

log_section() {
    echo ""
    echo "======================================================================"
    echo "  $*"
    echo "======================================================================"
    echo ""
}

cleanup() {
    local exit_code=$?
    log "Cleaning up..."

    # Save results even on failure
    if [[ -f "${ARTIFACTS}/results.json" ]]; then
        log "Results saved to ${ARTIFACTS}/results.json"
    fi

    if kubectl get namespace "${TEST_NAMESPACE}" &>/dev/null 2>&1; then
        kubectl logs -n baremetal-operator-system deployment/controller-manager \
            > "${ARTIFACTS}/bmo-controller.log" 2>&1 || true
    fi

    # Stop sushy-tools
    docker rm -f "${SUSHY_TOOLS_CONTAINER}" 2>/dev/null || true

    # Delete the kind cluster if we created it
    if [[ "${USE_EXISTING_CLUSTER}" != "true" ]]; then
        kind delete cluster --name "${CLUSTER_NAME}" 2>/dev/null || true
    fi

    exit "${exit_code}"
}

# Wait for all BMHs to reach a target provisioning state.
# Returns 0 on success, 1 on timeout.
# Outputs elapsed time in seconds to stdout.
wait_for_state() {
    local target_state="$1"
    local timeout="$2"
    local start_time
    local elapsed
    local count
    start_time=$(date +%s)

    log "Waiting for ${NUM_HOSTS} BMH(s) to reach state '${target_state}' (timeout: ${timeout}s)..." >&2
    while true; do
        elapsed=$(( $(date +%s) - start_time ))

        if (( elapsed >= timeout )); then
            log "TIMEOUT: ${elapsed}s elapsed, not all hosts reached '${target_state}'" >&2

            count=$(kubectl get bmh -n "${TEST_NAMESPACE}" \
                -o jsonpath='{range .items[*]}{.status.provisioning.state}{"\n"}{end}' 2>/dev/null \
                | grep -c "^${target_state}$" || true)

            count=${count:-0}

            log "  ${count}/${NUM_HOSTS} hosts reached '${target_state}'" >&2
            echo "${elapsed}"
            return 1
        fi

        # Count hosts in the target state
        count=$(kubectl get bmh -n "${TEST_NAMESPACE}" \
            -o jsonpath='{range .items[*]}{.status.provisioning.state}{"\n"}{end}' 2>/dev/null \
            | grep -c "^${target_state}$" || true)

        count=${count:-0}

        if (( count >= NUM_HOSTS )); then
            log "All ${NUM_HOSTS} BMH(s) reached state '${target_state}' in ${elapsed}s" >&2
            echo "${elapsed}"
            return 0
        fi

        # Progress update every 10 polls
        if (( (elapsed / POLL_INTERVAL) % 10 == 0 )) && (( elapsed > 0 )); then
            log "  Progress: ${count}/${NUM_HOSTS} in '${target_state}' (${elapsed}s elapsed)" >&2
        fi

        sleep "${POLL_INTERVAL}"
    done
}

# Collect per-host timing data from BMH conditions/events.
collect_per_host_timing() {
    local phase="$1"
    local output_file="${ARTIFACTS}/timing-${phase}.csv"

    log "Collecting per-host timing data for phase: ${phase}..."

    echo "host,created,state_reached,duration_seconds" > "${output_file}"

    local bmh_json
    bmh_json=$(kubectl get bmh -n "${TEST_NAMESPACE}" -o json 2>/dev/null)

    echo "${bmh_json}" | jq -r --arg phase "${phase}" '
        .items[] |
        .metadata.name as $name |
        .metadata.creationTimestamp as $created |
        .status.lastUpdated as $updated |
        "\($name),\($created),\($updated // "N/A")"
    ' >> "${output_file}" 2>/dev/null || true

    log "  Timing data written to ${output_file}"
}

# Calculate and report statistics from timing data.
report_statistics() {
    local phase="$1"
    local total_time="$2"
    local success="$3"

    local throughput
    if (( total_time > 0 )); then
        throughput=$(echo "scale=2; ${NUM_HOSTS} * 60 / ${total_time}" | bc 2>/dev/null || echo "N/A")
    else
        throughput="N/A"
    fi

    log_section "Results: ${phase}"
    log "  Hosts:              ${NUM_HOSTS}"
    log "  Total time:         ${total_time}s"
    log "  Throughput:         ${throughput} hosts/min"
    log "  Success:            ${success}"
    log "  Concurrent reconciles: ${MAX_CONCURRENT_RECONCILES}"

    # Append to JSON results
    jq --arg phase "${phase}" \
       --argjson hosts "${NUM_HOSTS}" \
       --argjson time "${total_time}" \
       --arg throughput "${throughput}" \
       --arg success "${success}" \
       --argjson concurrency "${MAX_CONCURRENT_RECONCILES}" \
       '. + {($phase): {hosts: $hosts, total_time_seconds: $time, throughput_hosts_per_min: $throughput, success: ($success == "true"), max_concurrent_reconciles: $concurrency}}' \
       "${ARTIFACTS}/results.json" > "${ARTIFACTS}/results.json.tmp" \
       && mv "${ARTIFACTS}/results.json.tmp" "${ARTIFACTS}/results.json"
}

# ==============================================================================
# Main
# ==============================================================================

trap cleanup EXIT

mkdir -p "${ARTIFACTS}"

# Initialize results JSON
echo '{"test": "scalability", "timestamp": "'"$(date -Iseconds)"'", "num_hosts": '"${NUM_HOSTS}"'}' > "${ARTIFACTS}/results.json"

log_section "BMO Scalability Test"
log "Configuration:"
log "  NUM_HOSTS:                  ${NUM_HOSTS}"
log "  SKIP_PROVISIONING:          ${SKIP_PROVISIONING}"
log "  MAX_CONCURRENT_RECONCILES:  ${MAX_CONCURRENT_RECONCILES}"
log "  ENROLLMENT_TIMEOUT:         ${ENROLLMENT_TIMEOUT}s"
log "  PROVISIONING_TIMEOUT:       ${PROVISIONING_TIMEOUT}s"
log "  USE_EXISTING_CLUSTER:       ${USE_EXISTING_CLUSTER}"
log "  ARTIFACTS:                  ${ARTIFACTS}"

# --------------------------------------------------------------------------
# Step 1: Cluster Setup
# --------------------------------------------------------------------------
log_section "Step 1: Cluster Setup"

if [[ "${USE_EXISTING_CLUSTER}" != "true" ]]; then
    log "Creating Kind cluster '${CLUSTER_NAME}'..."
    if kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
        log "  Cluster already exists, deleting..."
        kind delete cluster --name "${CLUSTER_NAME}"
    fi

    kind create cluster --name "${CLUSTER_NAME}" --wait 120s
    log "  Kind cluster created."
else
    log "Using existing cluster."
fi

# Verify connectivity
kubectl cluster-info --context "kind-${CLUSTER_NAME}" || kubectl cluster-info

# --------------------------------------------------------------------------
# Step 2: Deploy sushy-tools BMC Emulator
# --------------------------------------------------------------------------
log_section "Step 2: Deploy sushy-tools BMC Emulator (fake mode)"

# Stop any existing instance
docker rm -f "${SUSHY_TOOLS_CONTAINER}" 2>/dev/null || true

# Create a sushy-tools config that uses the "fake" libvirt driver.
# This makes sushy-tools respond to Redfish API calls without needing
# actual libvirt VMs — it just returns synthetic data for any system ID.
SUSHY_CONFIG_DIR="${ARTIFACTS}/sushy-config"
mkdir -p "${SUSHY_CONFIG_DIR}"

cat > "${SUSHY_CONFIG_DIR}/sushy-emulator.conf" << 'EOF'
# sushy-tools configuration for scalability testing.
# Uses the "fake" driver: responds to all Redfish calls with synthetic data.
# No real libvirt VMs are needed.
SUSHY_EMULATOR_LISTEN_IP = '0.0.0.0'
SUSHY_EMULATOR_LISTEN_PORT = 8000
SUSHY_EMULATOR_SSL_CERT = None
SUSHY_EMULATOR_SSL_KEY = None

# The "fake" driver creates N fake systems without libvirt.
# Each system is addressable as /redfish/v1/Systems/<uuid>
SUSHY_EMULATOR_BACKEND = 'fake'

# Number of fake systems to create.
# Must be >= NUM_HOSTS so each BMH has a unique system to talk to.
SUSHY_EMULATOR_FAKE_SYSTEMS_COUNT = 1000

# Fake driver settings
SUSHY_EMULATOR_FAKE_DRIVER = True
EOF

# Override the fake systems count to match our test
sed -i "s/SUSHY_EMULATOR_FAKE_SYSTEMS_COUNT = 1000/SUSHY_EMULATOR_FAKE_SYSTEMS_COUNT = ${NUM_HOSTS}/" \
    "${SUSHY_CONFIG_DIR}/sushy-emulator.conf"

log "Starting sushy-tools container with fake backend (${NUM_HOSTS} systems)..."
docker run -d \
    --name "${SUSHY_TOOLS_CONTAINER}" \
    --network host \
    -v "${SUSHY_CONFIG_DIR}/sushy-emulator.conf:/etc/sushy/sushy-emulator.conf:ro" \
    "${SUSHY_TOOLS_IMAGE}" \
    sushy-emulator --config /etc/sushy/sushy-emulator.conf

# Wait for sushy-tools to become ready
log "Waiting for sushy-tools to be ready..."
for (( i=1; i<=30; i++ )); do
    if curl -sf "http://${BMC_ADDRESS}:${SUSHY_TOOLS_PORT}/redfish/v1/" > /dev/null 2>&1; then
        log "  sushy-tools is ready."
        break
    fi
    if (( i == 30 )); then
        log "ERROR: sushy-tools did not become ready"
        docker logs "${SUSHY_TOOLS_CONTAINER}" 2>&1 | tail -20
        exit 1
    fi
    sleep 2
done

SYSTEM_COUNT=$(curl -sf "http://${BMC_ADDRESS}:${SUSHY_TOOLS_PORT}/redfish/v1/Systems" \
    | jq '.Members | length')

log "  Found ${SYSTEM_COUNT} fake systems available."

# --------------------------------------------------------------------------
# Step 3: Deploy BMO (with Ironic or fixture)
# --------------------------------------------------------------------------
log_section "Step 3: Deploy BMO"

if [[ "${DEPLOY_BMO}" == "true" ]]; then
    # Build BMO image
    log "Building BMO container image..."
    IMG=quay.io/metal3-io/baremetal-operator IMG_TAG="${BMO_IMAGE_TAG}" make -C "${REPO_ROOT}" docker 2>&1 | tail -5

    # Load into kind
    log "Loading BMO image into Kind cluster..."
    kind load docker-image "quay.io/metal3-io/baremetal-operator:${BMO_IMAGE_TAG}" --name "${CLUSTER_NAME}"

    # Install cert-manager (required for webhooks)
    log "Installing cert-manager..."
    kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.19.2/cert-manager.yaml
    kubectl wait --for=condition=Available deployment/cert-manager-webhook -n cert-manager --timeout=120s

    # Deploy BMO using the fixture overlay (no real Ironic needed for scalability)
    # The fixture provisioner simulates the entire lifecycle instantly.
    log "Deploying BMO with fixture provisioner..."
    log "  (Using fixture provisioner: simulates registration, inspection, and provisioning)"
    log "  (This measures pure controller throughput without Ironic API latency)"

    # Apply CRDs first
    kubectl apply -k "${REPO_ROOT}/config/base/crds"

    # Wait for CRDs to be established
    kubectl wait --for=condition=Established crd/baremetalhosts.metal3.io --timeout=60s

    # Create the namespace
    kubectl create namespace baremetal-operator-system 2>/dev/null || true

    # Create a configmap for ironic settings (required even with fixture)
    kubectl create configmap ironic \
        -n baremetal-operator-system \
        --from-literal=IRONIC_NETWORKING_ENABLED=false \
        2>/dev/null || true

    # Deploy BMO with the fixture provisioner and custom concurrency
    # We patch the deployment to set max-concurrent-reconciles
    kubectl apply -k "${REPO_ROOT}/config/overlays/fixture"

    # Patch the deployment to set concurrency
    kubectl patch deployment controller-manager \
        -n baremetal-operator-system \
        --type='json' \
        -p='[{"op": "add", "path": "/spec/template/spec/containers/0/args/-", "value": "--max-concurrent-reconciles='"${MAX_CONCURRENT_RECONCILES}"'"}]' \
        2>/dev/null || true

    # Wait for BMO to be ready
    log "Waiting for BMO deployment to be ready..."
    kubectl wait --for=condition=Available deployment/baremetal-operator-controller-manager \
        -n baremetal-operator-system --timeout=180s

    log "  BMO deployed and ready."
fi

# --------------------------------------------------------------------------
# Step 4: Create Test Namespace and BMH Resources
# --------------------------------------------------------------------------
log_section "Step 4: Create ${NUM_HOSTS} BareMetalHost Resources"

# Generate BMH manifests using the Go generator
BMH_MANIFEST="${ARTIFACTS}/bmh-manifests.yaml"

log "Building generate-bmhs tool..."
go build -o "${ARTIFACTS}/generate-bmhs" "${REPO_ROOT}/hack/scalability-tests/generate-bmhs/"

log "Generating ${NUM_HOSTS} BMH manifests..."
"${ARTIFACTS}/generate-bmhs" \
    -num-hosts="${NUM_HOSTS}" \
    -namespace="${TEST_NAMESPACE}" \
    -bmc-address="${BMC_ADDRESS}" \
    -bmc-port="${SUSHY_TOOLS_PORT}" \
    -output="${BMH_MANIFEST}"

log "  Generated ${NUM_HOSTS} BMH manifests (${BMH_MANIFEST})"

# Apply in a single batch for maximum burst pressure
log "Applying all BMH manifests..."
APPLY_START=$(date +%s)
kubectl apply -f "${BMH_MANIFEST}"
APPLY_DURATION=$(( $(date +%s) - APPLY_START ))
log "  kubectl apply completed in ${APPLY_DURATION}s"

# --------------------------------------------------------------------------
# Step 5: Measure Enrollment (→ available)
# --------------------------------------------------------------------------
log_section "Step 5: Measure Enrollment (registering → available)"

ENROLLMENT_TIME=$(wait_for_state "available" "${ENROLLMENT_TIMEOUT}") && ENROLLMENT_SUCCESS=true || ENROLLMENT_SUCCESS=false
collect_per_host_timing "enrollment"
report_statistics "enrollment" "${ENROLLMENT_TIME}" "${ENROLLMENT_SUCCESS}"

# --------------------------------------------------------------------------
# Step 6: Measure Provisioning (→ provisioned) [optional]
# --------------------------------------------------------------------------
if [[ "${SKIP_PROVISIONING}" != "true" ]]; then
    log_section "Step 6: Measure Provisioning (available → provisioned)"

    log "Patching all BMHs with image to trigger provisioning..."
    PATCH_START=$(date +%s)

    # Patch all BMHs in parallel using kubectl
    while IFS= read -r bmh_ref; do
        kubectl patch "${bmh_ref}" -n "${TEST_NAMESPACE}" --type=merge \
            -p '{"spec":{"image":{"url":"'"${IMAGE_URL}"'","checksum":"'"${IMAGE_CHECKSUM}"'","checksumType":"md5"},"rootDeviceHints":{"deviceName":"/dev/vda"}}}' &
    done < <(kubectl get bmh -n "${TEST_NAMESPACE}" -o name)
    wait

    PATCH_DURATION=$(( $(date +%s) - PATCH_START ))
    log "  Patching completed in ${PATCH_DURATION}s"

    PROVISIONING_TIME=$(wait_for_state "provisioned" "${PROVISIONING_TIMEOUT}") && PROVISIONING_SUCCESS=true || PROVISIONING_SUCCESS=false
    collect_per_host_timing "provisioning"
    report_statistics "provisioning" "${PROVISIONING_TIME}" "${PROVISIONING_SUCCESS}"
else
    log_section "Step 6: Provisioning skipped (SKIP_PROVISIONING=true)"
fi

# --------------------------------------------------------------------------
# Step 7: Final Report
# --------------------------------------------------------------------------
log_section "Final Report"

# Collect BMH state distribution
log "BMH state distribution:"
kubectl get bmh -n "${TEST_NAMESPACE}" -o jsonpath='{range .items[*]}{.status.provisioning.state}{"\n"}{end}' \
    | sort | uniq -c | sort -rn | while IFS= read -r line; do
    log "  ${line}"
done

# Collect error states
ERROR_HOSTS=$(kubectl get bmh -n "${TEST_NAMESPACE}" \
    -o jsonpath='{range .items[*]}{.status.operationalStatus}{" "}{.metadata.name}{"\n"}{end}' \
    | grep -c "^error" || true)

ERROR_HOSTS=${ERROR_HOSTS:-0}
if (( ERROR_HOSTS > 0 )); then
    log ""
    log "WARNING: ${ERROR_HOSTS} host(s) in error state:"
    kubectl get bmh -n "${TEST_NAMESPACE}" \
        -o jsonpath='{range .items[*]}{.status.operationalStatus}{" "}{.metadata.name}{" "}{.status.errorMessage}{"\n"}{end}' \
        | grep "^error" | head -5 | while IFS= read -r line; do
        log "  ${line}"
    done
fi

log ""
log "Full results: ${ARTIFACTS}/results.json"
jq . "${ARTIFACTS}/results.json"

log ""
log "Scalability test complete."
