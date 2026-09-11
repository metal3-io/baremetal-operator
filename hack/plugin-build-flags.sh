#!/usr/bin/env bash

# Print the go build flags that let an out-of-tree plugin load from any
# directory. Source paths in docs/plugin-provisioners.md says why.
#
# Usage plugin-build-flags.sh <bmo-source-dir> <module-version>
# Run from the module the flags are for.

set -o errexit -o nounset -o pipefail

root="${1:?usage: plugin-build-flags.sh <bmo-source-dir> <module-version>}"
version="${2-}"

# Only a tagged build carries a version a plugin author can pin against.
[[ -n "${version}" ]] || exit 0

mod="github.com/metal3-io/baremetal-operator"

# Read them, they are released on their own version line.
apis="$(go list -m -f '{{.Version}}' "${mod}/apis")"
hwutils="$(go list -m -f '{{.Version}}' "${mod}/pkg/hardwareutils")"

# Most specific first, the compiler takes the first matching rule. The submodule
# rules are needed because the module pattern would otherwise cover them.
printf -- '-trimpath -gcflags=%s/...=-trimpath=%s/apis=>%s/apis@%s;%s/pkg/hardwareutils=>%s/pkg/hardwareutils@%s;%s=>%s@%s\n' \
    "${mod}" \
    "${root}" "${mod}" "${apis}" \
    "${root}" "${mod}" "${hwutils}" \
    "${root}" "${mod}" "${version}"
