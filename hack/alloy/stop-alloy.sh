#!/usr/bin/env bash
# -----------------------------------------------------------------------------
# Description: Stops the background Alloy process started by run-alloy.sh, after
#              a short grace period so the final log lines are flushed to the
#              collector. No-op if Alloy was never started.
#
#
# Usage:       Called by hack/ci-e2e.sh after the tests (and verify.sh).
# -----------------------------------------------------------------------------
set -u

REPO_ROOT=$(realpath "$(dirname "${BASH_SOURCE[0]}")/../..")
PIDFILE="${REPO_ROOT}/hack/alloy/alloy.pid"

if [[ ! -f "${PIDFILE}" ]]; then
  echo "log-collection: no alloy pidfile; nothing to stop"
  exit 0
fi

PID=$(cat "${PIDFILE}")
# Only signal the PID if it is alive and actually an Alloy process, to avoid
# killing an unrelated process that may have reused a stale PID.
if kill -0 "${PID}" 2>/dev/null && [[ "$(ps -p "${PID}" -o comm= 2>/dev/null)" == *alloy* ]]; then
    # Give Alloy time to tail and push the final lines before we stop it.
    sleep 10
    kill "${PID}" 2>/dev/null || true
    echo "log-collection: alloy stopped (pid ${PID})"
else
    echo "log-collection: no running alloy for pid ${PID}; nothing to stop"
fi
rm -f "${PIDFILE}"
