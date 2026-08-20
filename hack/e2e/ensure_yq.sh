#!/usr/bin/env bash

set -eu

# shellcheck source=hack/e2e/checksums.sh
source "$(dirname "${BASH_SOURCE[0]}")/checksums.sh"

USR_LOCAL_BIN="/usr/local/bin"
YQ_DOWNLOAD_URL="https://github.com/mikefarah/yq/releases/download"

# Verify mode turned off by default
VERIFY_ONLY="${VERIFY_ONLY:-false}"

# Check if yq tool is installed and install it if not
verify_yq()
{
    YQ="$(command -v yq || true)"
    if ! [[ -x "${YQ}" ]]; then
        if [[ "${VERIFY_ONLY}" != "false" ]]; then
            echo "yq is not in PATH"
            return 0
        fi
        if [[ "${OSTYPE}" == "linux-gnu" ]]; then
            if ! command -v sha256sum &>/dev/null; then
                echo "ERROR: sha256sum not found" >&2
                return 1
            fi

            echo "yq not found, installing"
            set -x
            local tmp_dir
            tmp_dir="$(mktemp -d)"
            trap 'rm -rf "${tmp_dir}"; trap - RETURN' RETURN

            local URL="${YQ_DOWNLOAD_URL}/${YQ_VERSION}/yq_linux_amd64.tar.gz"

            # Download binary
            safe_curl -o "${tmp_dir}/yq_linux_amd64.tar.gz" "${URL}"

            # Verify checksum before extraction
            local checksum
            checksum="$(sha256sum "${tmp_dir}/yq_linux_amd64.tar.gz" | awk '{print $1;}')"
            if [[ "${checksum}" != "${YQ_SHA256}" ]]; then
                echo >&2 "fatal: ${URL} checksum '${checksum}' differs from expected '${YQ_SHA256}'"
                return 1
            fi

            tar -xvf "${tmp_dir}/yq_linux_amd64.tar.gz" -C "${tmp_dir}"
            sudo install "${tmp_dir}/yq_linux_amd64" "${USR_LOCAL_BIN}/yq"
            set +x
        else
            echo "ERROR: Missing required binary in path: yq"
            return 2
        fi
    else
        echo "$(yq --version) is installed at ${YQ}"
    fi
}

verify_yq
