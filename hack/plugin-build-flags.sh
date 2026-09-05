#!/usr/bin/env bash

# Print the go build flags that let an out-of-tree plugin load from any
# directory. -trimpath records the main module under a bare import path but a
# dependency under module@version, and BMO is the main module here and a
# dependency in every plugin, so its own packages are recorded in the dependency
# form. The submodule rules only restate what -trimpath already does, because
# the pattern covers them too and would otherwise overwrite them.
#
# Usage: plugin-build-flags.sh <bmo-source-dir> <module-version>
# Run from the module the flags are for. Lint with shellcheck.

set -o errexit -o nounset -o pipefail

root="${1:?usage: plugin-build-flags.sh <bmo-source-dir> <module-version>}"
version="${2-}"

# No version, no rewrite. A build that is not a release has no version a plugin
# author could pin, and this keeps debug builds pointing at real source files.
[[ -n "${version}" ]] || exit 0

mod="github.com/metal3-io/baremetal-operator"

# Read from go.mod rather than assuming. apis and pkg/hardwareutils are released
# on their own version line, so they never carry BMO's version.
apis="$(go list -m -f '{{.Version}}' "${mod}/apis")"
hwutils="$(go list -m -f '{{.Version}}' "${mod}/pkg/hardwareutils")"

# Most specific first, the compiler takes the first matching rule.
printf -- '-trimpath -gcflags=%s/...=-trimpath=%s/apis=>%s/apis@%s;%s/pkg/hardwareutils=>%s/pkg/hardwareutils@%s;%s=>%s@%s\n' \
    "${mod}" \
    "${root}" "${mod}" "${apis}" \
    "${root}" "${mod}" "${hwutils}" \
    "${root}" "${mod}" "${version}"
