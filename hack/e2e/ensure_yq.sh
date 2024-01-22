#!/usr/bin/env bash

set -eux

USR_LOCAL_BIN="/usr/local/bin"
YQ_VERSION="v4.40.5"

# Check if yq tool is installed and install it if not
verify_yq()
{
    if ! [[ -x "$(command -v yq)" ]]; then
        if [[ "${OSTYPE}" == "linux-gnu" ]]; then
            echo "yq not found, installing"
            curl -LO "https://github.com/mikefarah/yq/releases/download/${YQ_VERSION}/yq_linux_amd64.tar.gz"
            tar xvf yq_linux_amd64.tar.gz
            sudo install yq_linux_amd64 "${USR_LOCAL_BIN}/yq"
        else
            echo "Missing required binary in path: yq"
            return 2
        fi
    fi
}

verify_yq
