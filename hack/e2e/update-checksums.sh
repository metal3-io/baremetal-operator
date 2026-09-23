#!/usr/bin/env bash

# Helper script to refresh SHA-256 checksums in hack/e2e/checksums.sh.
# Run this after bumping any version in checksums.sh to update the corresponding
# checksum. It fetches the official checksum for each pinned version and updates
# the file in place.
#
# When run outside a container (the default), this script re-executes itself
# inside a minimal container with only hack/e2e/checksums.sh mounted read-write.
# This limits the blast radius: the container has network access to fetch
# checksums but can only modify the hack/e2e/checksums.sh.
#
# Usage:
#   ./hack/e2e/update-checksums.sh [--sysrescue-commit COMMIT]
#
# Options:
#   --sysrescue-commit COMMIT   Pin sysrescue-customize to a specific commit.
#                                If omitted, the latest commit on main is used.

set -eu -o pipefail

IS_CONTAINER="${IS_CONTAINER:-false}"
CONTAINER_RUNTIME="${CONTAINER_RUNTIME:-podman}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
CHECKSUMS_FILE="${CHECKSUMS_FILE:-${SCRIPT_DIR}/checksums.sh}"

if [[ "${IS_CONTAINER}" == "false" ]]; then
    # Digest: https://hub.docker.com/layers/library/python/3.13-alpine/images
    "${CONTAINER_RUNTIME}" run --rm \
        --env IS_CONTAINER=TRUE \
        --volume "${SCRIPT_DIR}/checksums.sh:/workdir/checksums.sh:z" \
        --volume "${SCRIPT_DIR}/update-checksums.sh:/workdir/update-checksums.sh:ro,z" \
        --workdir /workdir \
        docker.io/python:3.13-alpine@sha256:399babc8b49529dabfd9c922f2b5eea81d611e4512e3ed250d75bd2e7683f4b0 \
        sh -c 'apk add --no-cache curl bash >/dev/null 2>&1 && bash /workdir/update-checksums.sh "$@"' -- "$@"
    exit $?
fi

# --- Running inside the container ---

# sed -i does not work on bind-mounted files (atomic rename fails).
# Use a helper that writes to a temp buffer and truncates the original in place.
sed_inplace() {
    local file="$1"
    shift
    local tmp
    tmp="$(sed "$@" "${file}")"
    printf '%s\n' "${tmp}" > "${file}"
}


# shellcheck source=hack/e2e/checksums.sh
source "${CHECKSUMS_FILE}"

# Parse arguments
SYSRESCUE_COMMIT_ARG=""
while [[ $# -gt 0 ]]; do
    case "$1" in
        --sysrescue-commit)
            SYSRESCUE_COMMIT_ARG="$2"
            shift 2
            ;;
        *)
            echo "Unknown option: $1" >&2
            exit 1
            ;;
    esac
done

echo "Updating checksums in ${CHECKSUMS_FILE}..."

tmp_dir="$(mktemp -d)"
trap 'rm -rf "${tmp_dir}"' EXIT

# --- Go ---
GO_TARBALL="${MINIMUM_GO_VERSION}.linux-amd64.tar.gz"
NEW_GO_SHA256="$(safe_curl "https://dl.google.com/go/${GO_TARBALL}.sha256")"
if [[ -z "${NEW_GO_SHA256}" ]]; then
    echo "ERROR: Failed to fetch Go checksum" >&2
    exit 1
fi
echo "Go ${MINIMUM_GO_VERSION}: ${NEW_GO_SHA256}"
sed_inplace "${CHECKSUMS_FILE}" "s/^GO_SHA256=\".*\"/GO_SHA256=\"${NEW_GO_SHA256}\"/"

# --- kubectl ---
NEW_KUBECTL_SHA256="$(safe_curl "https://dl.k8s.io/release/${MINIMUM_KUBECTL_VERSION}/bin/linux/amd64/kubectl.sha256")"
if [[ -z "${NEW_KUBECTL_SHA256}" ]]; then
    echo "ERROR: Failed to fetch kubectl checksum" >&2
    exit 1
fi
echo "kubectl ${MINIMUM_KUBECTL_VERSION}: ${NEW_KUBECTL_SHA256}"
sed_inplace "${CHECKSUMS_FILE}" "s/^KUBECTL_SHA256=\".*\"/KUBECTL_SHA256=\"${NEW_KUBECTL_SHA256}\"/"

# --- yq ---
RELEASE_URL="https://github.com/mikefarah/yq/releases/download/${YQ_VERSION}"
safe_curl -o "${tmp_dir}/checksums_hashes_order" \
    "${RELEASE_URL}/checksums_hashes_order"
safe_curl -o "${tmp_dir}/checksums" \
    "${RELEASE_URL}/checksums"

sha256_line="$(grep -n "^SHA-256$" "${tmp_dir}/checksums_hashes_order" | cut -d: -f1)"
if [[ -z "${sha256_line}" ]]; then
    echo "ERROR: SHA-256 not found in yq checksums_hashes_order" >&2
    exit 1
fi
sha256_col=$((sha256_line + 1))
NEW_YQ_SHA256="$(grep -F "yq_linux_amd64.tar.gz" "${tmp_dir}/checksums" | awk "{print \$${sha256_col}}")"
if [[ -z "${NEW_YQ_SHA256}" ]]; then
    echo "ERROR: Failed to extract yq checksum" >&2
    exit 1
fi
echo "yq ${YQ_VERSION}: ${NEW_YQ_SHA256}"
sed_inplace "${CHECKSUMS_FILE}" "s/^YQ_SHA256=\".*\"/YQ_SHA256=\"${NEW_YQ_SHA256}\"/"

# --- cirros ---
NEW_CIRROS_SHA256="$(safe_curl "https://artifactory.nordix.org/artifactory/metal3/images/iso/cirros-${CIRROS_VERSION}-x86_64-disk.img.sha256")"
if [[ -z "${NEW_CIRROS_SHA256}" ]]; then
    echo "ERROR: Failed to fetch cirros checksum" >&2
    exit 1
fi
echo "cirros ${CIRROS_VERSION}: ${NEW_CIRROS_SHA256}"
sed_inplace "${CHECKSUMS_FILE}" "s/^CIRROS_SHA256=\".*\"/CIRROS_SHA256=\"${NEW_CIRROS_SHA256}\"/"

# --- SystemRescue ISO ---
NEW_SYSRESCUE_ISO_SHA256="$(safe_curl "https://artifactory.nordix.org/artifactory/metal3/images/sysrescue/systemrescue-${SYSRESCUE_VERSION}-amd64.iso.sha256")"
if [[ -z "${NEW_SYSRESCUE_ISO_SHA256}" ]]; then
    echo "ERROR: Failed to fetch systemrescue ISO checksum" >&2
    exit 1
fi
echo "systemrescue ${SYSRESCUE_VERSION}: ${NEW_SYSRESCUE_ISO_SHA256}"
sed_inplace "${CHECKSUMS_FILE}" "s/^SYSRESCUE_ISO_SHA256=\".*\"/SYSRESCUE_ISO_SHA256=\"${NEW_SYSRESCUE_ISO_SHA256}\"/"

# --- sysrescue-customize ---
SYSRESCUE_SCRIPT_PATH="airootfs/usr/share/sysrescue/bin/sysrescue-customize"
SYSRESCUE_PROJECT_URL="https://gitlab.com/systemrescue/systemrescue-sources"

COMMIT="${SYSRESCUE_COMMIT_ARG:-}"
if [[ -z "${COMMIT}" ]]; then
    echo "Fetching latest sysrescue-customize commit from main..." >&2
    COMMIT="$(safe_curl "https://gitlab.com/api/v4/projects/systemrescue%2Fsystemrescue-sources/repository/files/${SYSRESCUE_SCRIPT_PATH//\//%2F}?ref=main" \
        | python3 -c "import json,sys; print(json.load(sys.stdin)['last_commit_id'])")"
fi
echo "sysrescue-customize commit: ${COMMIT}"

NEW_SYSRESCUE_CUSTOMIZE_SHA256="$(safe_curl "${SYSRESCUE_PROJECT_URL}/-/raw/${COMMIT}/${SYSRESCUE_SCRIPT_PATH}" \
    | sha256sum | awk '{print $1;}')"
if [[ -z "${NEW_SYSRESCUE_CUSTOMIZE_SHA256}" ]]; then
    echo "ERROR: Failed to compute sysrescue-customize checksum" >&2
    exit 1
fi
echo "sysrescue-customize: ${NEW_SYSRESCUE_CUSTOMIZE_SHA256}"
sed_inplace "${CHECKSUMS_FILE}" "s/^SYSRESCUE_CUSTOMIZE_COMMIT=\".*\"/SYSRESCUE_CUSTOMIZE_COMMIT=\"${COMMIT}\"/"
sed_inplace "${CHECKSUMS_FILE}" "s/^SYSRESCUE_CUSTOMIZE_SHA256=\".*\"/SYSRESCUE_CUSTOMIZE_SHA256=\"${NEW_SYSRESCUE_CUSTOMIZE_SHA256}\"/"

echo ""
echo "Done. All checksums updated in ${CHECKSUMS_FILE}."
