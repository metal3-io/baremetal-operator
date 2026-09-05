#!/usr/bin/env bash

# The root go.mod requires apis and pkg/hardwareutils at a version that the
# local replace directives hide, so a stale requirement is invisible here and
# breaks every consumer. Nothing in the release process bumps them, which is how
# they drifted to v0.5.1 while pkg/provisioner had long stopped compiling
# against it.
#
# Build pkg/provisioner with the replaces dropped, which is what a consumer
# resolves, so the drift fails here instead of in somebody else's build.
#
# Lint with shellcheck. Run from the repository root.

set -o errexit -o nounset -o pipefail

mod="github.com/metal3-io/baremetal-operator"
check="go.submodule-check.mod"
checksum="go.submodule-check.sum"

cleanup() {
    rm -f "${check}" "${checksum}"
}
trap cleanup EXIT

cp go.mod "${check}"
cp go.sum "${checksum}"

go mod edit -modfile="${check}" \
    -dropreplace "${mod}/apis" \
    -dropreplace "${mod}/pkg/hardwareutils"

required() {
    go list -modfile="${check}" -m -f '{{.Version}}' "$1"
}

# Only the package a plugin imports. The ironic subtree is not part of the
# plugin contract, and ./... would drag in the buildmode=plugin main packages,
# which never build on their own.
# -mod=mod so the checksums for the now unreplaced modules can be recorded.
if ! GOFLAGS=-mod=mod go build -modfile="${check}" ./pkg/provisioner 2>&1; then
    cat >&2 <<EOF
error: pkg/provisioner does not build against the submodule versions go.mod requires

    ${mod}/apis $(required "${mod}/apis")
    ${mod}/pkg/hardwareutils $(required "${mod}/pkg/hardwareutils")

The replace directives hide this locally, but consumers get no replace and
resolve exactly these versions. Bump both requirements in go.mod and
test/go.mod to the released versions of the current line.
EOF
    exit 1
fi

echo "pkg/provisioner builds against the required submodule versions"
