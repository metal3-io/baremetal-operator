#!/usr/bin/env bash

REPO_ROOT=$(realpath "$(dirname "${BASH_SOURCE[0]}")/..")
cd "${REPO_ROOT}" || exit 1

if [[ ! -r "${REPO_ROOT}/test/e2e/config/vbmctl.yaml" ]]; then
    echo "vbmctl.yaml not found, skipping e2e cleanup with vbmctl"
else
    ./bin/vbmctl -c "${REPO_ROOT}/test/e2e/config/vbmctl.yaml" delete bml
fi

# Keeping BMH cleanup for legacy reasons.
"${REPO_ROOT}/tools/bmh_test/clean_local_bmh_test_setup.sh" "^bmo-e2e-"

rm -rf "${REPO_ROOT}/test/e2e/_artifacts"
rm -rf "${REPO_ROOT}"/artifacts-*
KEEP_E2E_IMAGES="${KEEP_E2E_IMAGES:-false}"
if [[ "${KEEP_E2E_IMAGES,,}" != "true" ]]; then
    rm -rf "${REPO_ROOT}/test/e2e/images"
fi
