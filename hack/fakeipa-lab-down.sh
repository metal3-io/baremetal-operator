#!/usr/bin/env bash

# -----------------------------------------------------------------------------
# Description: Tears down the FakeIPA scalability test lab created by
#              hack/fakeipa-lab-up.sh (network, veth pair, image server,
#              sushy-tools FakeDriver container). Run after `make test-e2e`
#              completes. Does not remove the downloaded image-server cache
#              (FAKEIPA_IMAGE_DIR) so repeated runs don't re-download it.
# Usage:       ./hack/fakeipa-lab-down.sh
# -----------------------------------------------------------------------------

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
LAB_DIR="${FAKEIPA_LAB_DIR:-/tmp/fakeipa-lab}"
VBMCTL_BIN="${SCALABILITY_FAKEIPA_VBMCTL_BIN:-${REPO_ROOT}/bin/vbmctl}"

if [[ ! -f "${LAB_DIR}/vbmctl-fakeipa.yaml" ]]; then
  echo "No lab config found at ${LAB_DIR}/vbmctl-fakeipa.yaml, nothing to tear down."
  exit 0
fi

"${VBMCTL_BIN}" -c "${LAB_DIR}/vbmctl-fakeipa.yaml" delete bml
