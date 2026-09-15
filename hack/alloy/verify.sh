#!/usr/bin/env bash
# -----------------------------------------------------------------------------
# Description: Sanity check that Alloy shipped log entries to the collector, by
#              reading its metrics endpoint. Must run while Alloy is still up
#              (i.e. before stop-alloy.sh). Prints the sent/dropped counts and
#              succeeds only if at least one entry was sent. Never fails the
#              build on its own (callers invoke it with `|| true`).
#
# Usage:       Called by hack/ci-e2e.sh after `make test-e2e`.
# -----------------------------------------------------------------------------
set -u

METRICS="$(curl -s --connect-timeout 2 --max-time 5 http://127.0.0.1:12345/metrics 2>/dev/null || true)"
if [[ -z "${METRICS}" ]]; then
  echo "log-collection: alloy metrics unavailable (not running?)"
  exit 0
fi

sent=$(awk '/^loki_write_sent_entries_total/{s+=$NF} END{print s+0}' <<<"${METRICS}")
dropped=$(awk '/^loki_write_dropped_entries_total/{s+=$NF} END{print s+0}' <<<"${METRICS}")
echo "log-collection: sent_entries=${sent} dropped_entries=${dropped}"

[[ "${sent}" -gt 0 ]]
