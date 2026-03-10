#!/usr/bin/env bash

set -eu -o pipefail

REPO_ROOT=$(realpath "$(dirname "${BASH_SOURCE[0]}")/..")
cd "${REPO_ROOT}" || exit 1

WATCH_NS="$(grep -A1 WATCH_NAMESPACE config/overlays/e2e/namespaced-manager-patch.yaml | grep 'value:' | awk '{ print $2; }')"
EXCEPTIONS=upgrade

EXITCODE=0
for spec in $(grep -E --no-filename ' *specName +:?=' test/e2e/*_test.go | grep -o '".*"' | tr -d '"'); do
    # shellcheck disable=SC2076
    if ! [[ ",${WATCH_NS},${EXCEPTIONS}," =~ ",${spec}," ]]; then
        echo "ERROR: namespace ${spec} is declared in e2e test but not added to config/overlays/e2e/namespaced-manager-patch.yaml or EXCEPTIONS"
        EXITCODE=1
    fi

    # shellcheck disable=SC2076
    if ! [[ ",${EXCEPTIONS}," =~ ",${spec}," ]] && ! [[ -f "config/overlays/e2e/${spec}/namespace.yaml" ]]; then
        echo "ERROR: namespace ${spec} is declared in e2e test but there is no namespace kustomization in config/overlays/e2e/"
        EXITCODE=1
    fi
done
exit ${EXITCODE}
