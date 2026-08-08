#!/usr/bin/env bash

# Configure the module to build the Starlark plugin against a BMO git ref.
# It fetches BMO at the ref and pins go.mod, the Dockerfile, and the Makefile.

set -o errexit -o nounset -o pipefail

ref="${1:?usage: retarget-bmo.sh <bmo-git-ref> [bmo-repo-url] [bmo-src-dir]}"
repo="${2:-https://github.com/metal3-io/baremetal-operator.git}"
src="${3:-}"

module_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${module_dir}"

cloned=false
if [[ -z "${src}" ]]; then
    cloned=true
    src="${module_dir}/.bmo-src"
    rm -rf "${src}"
    echo "cloning ${repo} at ${ref} into ${src}"
    if ! git clone --depth 1 --branch "${ref}" "${repo}" "${src}" 2>/dev/null; then
        git clone "${repo}" "${src}"
        git -C "${src}" checkout "${ref}"
    fi
fi

if [[ ! -f "${src}/go.mod" ]]; then
    echo "error: ${src} is not a BMO checkout (no go.mod)" >&2
    exit 1
fi

bmo="github.com/metal3-io/baremetal-operator"
ipmi="github.com/bougou/go-ipmi"
gofish="github.com/stmcginnis/gofish"
starlark="go.starlark.net"

echo "pinning go.mod to BMO ${ref} via ${src}"

# For a version tag, record it in every require line. For a branch or commit the
# replace to the source is authoritative and the require version is cosmetic.
if [[ "${ref}" =~ ^v[0-9] ]]; then
    go mod edit \
        -require "${bmo}@${ref}" \
        -require "${bmo}/apis@${ref}" \
        -require "${bmo}/pkg/hardwareutils@${ref}"
fi

go mod edit \
    -replace "${bmo}=${src}" \
    -replace "${bmo}/apis=${src}/apis" \
    -replace "${bmo}/pkg/hardwareutils=${src}/pkg/hardwareutils"

# Pin the plugin dependencies outside BMO to the versions already in go.mod, never @latest, for reproducible builds.
for mod in "${starlark}" "${ipmi}" "${gofish}"; do
    ver="$(go list -m -f '{{.Version}}' "${mod}" 2>/dev/null || true)"
    if [[ -n "${ver}" ]]; then
        go get "${mod}@${ver}"
    fi
done

go mod tidy

# Only a host retarget rewrites the build files. Inside the image build a source
# dir is passed and the Dockerfile already has its images fixed for that build.
if [[ "${cloned}" == true ]]; then
    dockerfile="${module_dir}/Dockerfile"
    makefile="${module_dir}/Makefile"

    # Match the toolchain and base images the release used. Go plugin loading
    # needs the exact same Go version to load into the official image.
    for arg in BUILD_IMAGE BASE_IMAGE; do
        line="$(grep -m1 "^ARG ${arg}=" "${src}/Dockerfile" || true)"
        if [[ -n "${line}" ]]; then
            sed -i "s|^ARG ${arg}=.*|${line}|" "${dockerfile}"
        fi
    done
    sed -i "s|^ARG BMO_VERSION=.*|ARG BMO_VERSION=${ref}|" "${dockerfile}"
    sed -i "s|^BMO_VERSION[[:space:]]*[?]=.*|BMO_VERSION ?= ${ref}|" "${makefile}"
fi

echo "done. build with: make plugin  or  make image"
