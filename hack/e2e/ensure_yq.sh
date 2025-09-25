#!/usr/bin/env bash

set -eu

USR_LOCAL_BIN="/usr/local/bin"
YQ_VERSION="v4.40.5"
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
            echo "yq not found, installing"
            set -x
            curl -LO \
                --create-dirs \
                --output-dir "/tmp" \
                "${YQ_DOWNLOAD_URL}/${YQ_VERSION}/yq_linux_amd64.tar.gz"
            sudo tar -xvf "/tmp/yq_linux_amd64.tar.gz" -C "/tmp"
            sudo install "/tmp/yq_linux_amd64" "${USR_LOCAL_BIN}/yq"
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
