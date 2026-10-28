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

# IRONIC_USERNAME/IRONIC_PASSWORD must be exported here one way or another:
# the suite's own basic-auth check (VerifyIronicSecurityConfig) compares
# against whatever credentials were baked into the Ironic/BMO kustomizations'
# generated secrets (see hack/ci-e2e.sh's "Generate credentials" step, which
# writes matching ironic-username/ironic-password files into both the BMO
# and IRSO overlays). Without matching env vars exported, the check falls
# back to the e2e config file's placeholder default and fails with HTTP 401
# before any spec runs.
# shellcheck disable=SC1091
source /tmp/fakeipa-run-env
export IRONIC_USERNAME IRONIC_PASSWORD

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
