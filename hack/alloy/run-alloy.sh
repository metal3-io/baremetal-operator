#!/usr/bin/env bash
# -----------------------------------------------------------------------------
# Description: Starts Grafana Alloy in the background to stream BMO e2e logs to
#              the CI log collector (Loki) while the test suite runs. Tails the
#              log files the CAPI framework writes live, plus libvirt serial
#              consoles. No-op (exits 0) if the push credential is not set, so
#              local/dev runs and unconfigured jobs are unaffected.
# Usage:       Called by hack/ci-e2e.sh before `make test-e2e`. The pipeline
#              only needs to provide the push credential; everything else has a
#              working default:
#                LOKI_USERNAME        RW push username (required, from credential)
#                LOKI_PASSWORD        RW push password (required, from credential)
#                LOKI_URL             push endpoint (default: staging collector)
#                ALLOY_JOB            'job' label       (default metal3ci)
#                ALLOY_PIPELINE_ID    'pipeline_id'     (default metal3-bmo-e2e[-<protocol>])
#                ALLOY_BUILD_NUMBER   'build_number'    (default $BUILD_NUMBER or 0)
#                BMO_ARTIFACTS_DIR    path to _artifacts (default test/e2e/_artifacts)
# -----------------------------------------------------------------------------
set -eu

REPO_ROOT=$(realpath "$(dirname "${BASH_SOURCE[0]}")/../..")
ALLOY_DIR="${REPO_ROOT}/hack/alloy"

# Streaming is gated on the push credential, which the pipeline provides via
# withCredentials. Without it, no-op so local/dev runs are unaffected.
if [[ -z "${LOKI_USERNAME:-}" || -z "${LOKI_PASSWORD:-}" ]]; then
  echo "log-collection: push credential not set; skipping streaming"
  exit 0
fi

# Install Alloy on demand when the credential is present but the binary is not.
if ! command -v alloy >/dev/null 2>&1; then
  "${REPO_ROOT}/hack/e2e/ensure_alloy.sh" || true
fi
if ! command -v alloy >/dev/null 2>&1; then
  echo "log-collection: alloy unavailable after install attempt; skipping streaming"
  exit 0
fi

# Endpoint defaults to the staging collector; override via env for test/live.
export LOKI_URL="${LOKI_URL:-https://log.apps.staging.metal3.io/store/api/v1/push}"

# Labels default to Jenkins built-ins so no extra pipeline plumbing is required:
# BMC_PROTOCOL is set by the e2e matrix, BUILD_NUMBER by Jenkins.
export ALLOY_JOB="${ALLOY_JOB:-metal3ci}"
export ALLOY_PIPELINE_ID="${ALLOY_PIPELINE_ID:-metal3-bmo-e2e${BMC_PROTOCOL:+-${BMC_PROTOCOL}}}"
export ALLOY_BUILD_NUMBER="${ALLOY_BUILD_NUMBER:-${BUILD_NUMBER:-0}}"
export BMO_ARTIFACTS_DIR="${BMO_ARTIFACTS_DIR:-${REPO_ROOT}/test/e2e/_artifacts}"

# The framework creates this later; make sure it exists so Alloy can watch it.
mkdir -p "${BMO_ARTIFACTS_DIR}"

# Don't start a second instance if one is already running.
if [[ -f "${ALLOY_DIR}/alloy.pid" ]] && kill -0 "$(cat "${ALLOY_DIR}/alloy.pid")" 2>/dev/null; then
  echo "log-collection: alloy already running (pid $(cat "${ALLOY_DIR}/alloy.pid")); skipping start"
  exit 0
fi

alloy run "${ALLOY_DIR}/config.alloy" \
  --storage.path="${ALLOY_DIR}/alloy-data" \
  --server.http.listen-addr=127.0.0.1:12345 \
  > "${BMO_ARTIFACTS_DIR}/alloy.log" 2>&1 &

echo $! > "${ALLOY_DIR}/alloy.pid"
echo "log-collection: alloy started (pid $(cat "${ALLOY_DIR}/alloy.pid")), pipeline_id=${ALLOY_PIPELINE_ID}"
