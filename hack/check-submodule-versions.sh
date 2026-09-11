#!/usr/bin/env bash

# Consumers get no replace directives, so they resolve whatever versions of
# apis and pkg/hardwareutils go.mod requires, and a stale one breaks them.
# Build pkg/provisioner the way they resolve it, from the repository root.

set -o errexit -o nounset -o pipefail

mod="github.com/metal3-io/baremetal-operator"
check="go.submodule-check.mod"
checksum="go.submodule-check.sum"

submodules=("${mod}/apis" "${mod}/pkg/hardwareutils")

cleanup() {
    rm -f "${check}" "${checksum}"
}
trap cleanup EXIT

cp go.mod "${check}"
cp go.sum "${checksum}"

go mod edit -modfile="${check}" \
    -dropreplace "${mod}/apis" \
    -dropreplace "${mod}/pkg/hardwareutils"

# -e so a version with no tag yet is reported instead of failing the graph.
required() {
    go list -modfile="${check}" -m -e -f '{{.Version}}' "$1"
}

# A release tags the root and the submodules at once, so a requirement naming
# that release resolves only once the tags are pushed.
pending=""
report=""

for submodule in "${submodules[@]}"; do
    version="$(required "${submodule}")"
    report+="    ${submodule} ${version}"$'\n'
    test_version="$(cd test && go list -m -e -f '{{.Version}}' "${submodule}")"

    if [[ "${version}" != "${test_version}" ]]; then
        cat >&2 <<EOF
error: go.mod and test/go.mod require different versions of ${submodule}

    go.mod      ${version}
    test/go.mod ${test_version}

Only consumers see this, so bump the two files together.
EOF
        exit 1
    fi

    if ! query="$(GOFLAGS=-mod=mod go list -modfile="${check}" -m "${submodule}@${version}" 2>&1)"; then
        case "${query}" in
        *"unknown revision"* | *"invalid version"*)
            pending="${submodule} ${version}"
            ;;
        *)
            echo "${query}" >&2
            exit 1
            ;;
        esac
    fi
done

if [[ -n "${pending}" ]]; then
    echo "${pending} is not tagged yet, skipping the build check"
    exit 0
fi

# Only the package a plugin imports, ./... would drag in the plugin main
# packages. -mod=mod to record checksums for the unreplaced modules.
if ! GOFLAGS=-mod=mod go build -modfile="${check}" ./pkg/provisioner 2>&1; then
    cat >&2 <<EOF
error: pkg/provisioner does not build against the versions go.mod requires

${report}
Consumers resolve exactly these. Bump go.mod and test/go.mod to the
released versions of the current line.
EOF
    exit 1
fi

echo "pkg/provisioner builds against the required submodule versions"
