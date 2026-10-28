#!/usr/bin/env bash

# -----------------------------------------------------------------------------
# Description: Runs the FakeIPA scalability e2e suite with the env vars it
#              requires already wired in (E2E_CONF_FILE, E2E_BMCS_CONF_FILE,
#              IRONIC_KUSTOMIZATION) - `make test-e2e` on its own defaults to
#              the Fixture provider (no real Ironic), and even pointed at the
#              real ironic.yaml config, the default Ironic kustomization
#              leaves agent.verify_ca at its default (True). That failure is
#              silent at first: Ironic comes up fine and batches start, only
#              failing minutes later with "SSLCertVerificationError:
#              self-signed certificate" once a BMH tries to deploy through
#              Fake IPA. Run hack/fakeipa-lab-up.sh first (see that script).
#
# Usage:       ./hack/fakeipa-test-e2e.sh ['ginkgo focus regex']
# -----------------------------------------------------------------------------

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${REPO_ROOT}"

# Bootstrap the gitignored artifacts hack/ci-e2e.sh normally generates before
# `make test-e2e` (see its "Generate credentials" step):
#
# - IRONIC_USERNAME/IRONIC_PASSWORD must be exported AND match the
#   ironic-username/ironic-password files baked into both the BMO overlay
#   (BMO_KUSTOMIZATION in test/e2e/config/ironic.yaml) and the IRSO basic-auth
#   component. The suite's basic-auth check (VerifyIronicSecurityConfig)
#   otherwise falls back to the e2e config's placeholder default and fails
#   with HTTP 401 before any spec runs.
# - ironic/base/ironic.yaml must be rendered from its template, since the
#   e2e-fakeipa overlay builds on ../e2e -> ../../base.
#
# Credentials are taken from the environment if exported, otherwise reused
# from previously generated files (so they keep matching an already-deployed
# lab), otherwise freshly generated.
BMO_OVERLAY_DIR="${REPO_ROOT}/config/overlays/e2e"
IRSO_IRONIC_AUTH_DIR="${REPO_ROOT}/test/e2e/data/ironic-standalone-operator/components/basic-auth"
IRSO_IRONIC_BASE_DIR="${REPO_ROOT}/test/e2e/data/ironic-standalone-operator/ironic/base"

if [[ -z "${IRONIC_USERNAME:-}" ]] && [[ -s "${BMO_OVERLAY_DIR}/ironic-username" ]]; then
  IRONIC_USERNAME="$(<"${BMO_OVERLAY_DIR}/ironic-username")"
fi
if [[ -z "${IRONIC_PASSWORD:-}" ]] && [[ -s "${BMO_OVERLAY_DIR}/ironic-password" ]]; then
  IRONIC_PASSWORD="$(<"${BMO_OVERLAY_DIR}/ironic-password")"
fi
IRONIC_USERNAME="${IRONIC_USERNAME:-$(uuidgen)}"
IRONIC_PASSWORD="${IRONIC_PASSWORD:-$(uuidgen)}"
export IRONIC_USERNAME IRONIC_PASSWORD

for dir in "${BMO_OVERLAY_DIR}" "${IRSO_IRONIC_AUTH_DIR}"; do
  echo "${IRONIC_USERNAME}" > "${dir}/ironic-username"
  echo "${IRONIC_PASSWORD}" > "${dir}/ironic-password"
done

# Fake IPA never boots the deploy ramdisk, so no SSH key needs to be baked in.
# shellcheck disable=SC2016
SSH_PUB_KEY_CONTENT="" envsubst '${SSH_PUB_KEY_CONTENT}' < \
  "${IRSO_IRONIC_BASE_DIR}/ironic.yaml.tmpl" > \
  "${IRSO_IRONIC_BASE_DIR}/ironic.yaml"

# Opt in to the FakeIPA specs, which otherwise skip themselves so an
# unfiltered `make test-e2e` against the stock real-Ironic lab isn't broken.
export SCALABILITY_FAKEIPA_ENABLED=true

export E2E_CONF_FILE="${E2E_CONF_FILE:-${REPO_ROOT}/test/e2e/config/ironic.yaml}"
export E2E_BMCS_CONF_FILE="${E2E_BMCS_CONF_FILE:-${REPO_ROOT}/test/e2e/config/bmcs-redfish-virtualmedia.yaml}"
export IRONIC_KUSTOMIZATION="${IRONIC_KUSTOMIZATION:-data/ironic-standalone-operator/ironic/overlays/e2e-fakeipa}"
export GINKGO_FOCUS="${1:-${GINKGO_FOCUS:-Scalability \(FakeIPA\)}}"

echo "Running FakeIPA e2e suite with:"
echo "  E2E_CONF_FILE=${E2E_CONF_FILE}"
echo "  E2E_BMCS_CONF_FILE=${E2E_BMCS_CONF_FILE}"
echo "  IRONIC_KUSTOMIZATION=${IRONIC_KUSTOMIZATION}"
echo "  GINKGO_FOCUS=${GINKGO_FOCUS}"

make test-e2e
