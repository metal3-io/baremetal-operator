#!/usr/bin/env bash

set -eu

USR_LOCAL_BIN="/usr/local/bin"
# Pin a release by exporting ALLOY_VERSION=vX.Y.Z; defaults to v1.18.1.
ALLOY_VERSION="${ALLOY_VERSION:-v1.18.1}"
ALLOY_RELEASES_URL="https://github.com/grafana/alloy/releases"

# Verify mode turned off by default
VERIFY_ONLY="${VERIFY_ONLY:-false}"

# Check if the alloy tool is installed and install it if not
verify_alloy()
{
    ALLOY="$(command -v alloy || true)"
    if ! [[ -x "${ALLOY}" ]]; then
        if [[ "${VERIFY_ONLY}" != "false" ]]; then
            echo "alloy is not in PATH"
            return 0
        fi
        if [[ "${OSTYPE}" == "linux-gnu" ]]; then
            echo "alloy not found, installing"
            # Alloy ships the binary as a zip; make sure we can extract it.
            if ! command -v unzip >/dev/null 2>&1; then
                sudo apt-get update && sudo apt-get install -y unzip
            fi
            ALLOY_ZIP="alloy-linux-amd64.zip"
            if [[ "${ALLOY_VERSION}" == "latest" ]]; then
                ALLOY_BASE_URL="${ALLOY_RELEASES_URL}/latest/download"
            else
                ALLOY_BASE_URL="${ALLOY_RELEASES_URL}/download/${ALLOY_VERSION}"
            fi
            set -x
            curl -LO --create-dirs --output-dir "/tmp" "${ALLOY_BASE_URL}/${ALLOY_ZIP}"
            curl -LO --create-dirs --output-dir "/tmp" "${ALLOY_BASE_URL}/SHA256SUMS"
            # Verify the archive against the release checksums before installing.
            (cd "/tmp" && sha256sum --ignore-missing -c SHA256SUMS)
            unzip -o "/tmp/${ALLOY_ZIP}" -d "/tmp"
            sudo install "/tmp/alloy-linux-amd64" "${USR_LOCAL_BIN}/alloy"
            set +x
        else
            echo "ERROR: Missing required binary in path: alloy"
            return 2
        fi
    else
        echo "$(alloy --version | head -n1) is installed at ${ALLOY}"
    fi
}

verify_alloy
