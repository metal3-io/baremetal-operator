#!/usr/bin/env bash

set -eux

USR_LOCAL_BIN="/usr/local/bin"
MINIMUM_KIND_VERSION=v0.26.0

# Ensure the kind tool exists and is a viable version, or installs it
verify_kind_version()
{
    # If kind is not available on the path, get it
    if ! [ -x "$(command -v kind)" ]; then
        if [[ "${OSTYPE}" == "linux-gnu" ]]; then
            echo "kind not found, installing"
            curl -LO "https://kind.sigs.k8s.io/dl/${MINIMUM_KIND_VERSION}/kind-linux-amd64"
            sudo install kind-linux-amd64 "${USR_LOCAL_BIN}/kind"
        else
            echo "Missing required binary in path: kind"
            return 2
        fi
    fi

    local kind_version
    IFS=" " read -ra kind_version <<< "$(kind version)"
    if [[ "${MINIMUM_KIND_VERSION}" != $(echo -e "${MINIMUM_KIND_VERSION}\n${kind_version[1]}" | sort -s -t. -k 1,1 -k 2,2n -k 3,3n | head -n1) ]]; then
        cat << EOF
Detected kind version: ${kind_version[2]}.
Requires ${MINIMUM_KIND_VERSION} or greater.
Please install ${MINIMUM_KIND_VERSION} or later.
EOF
        return 2
    fi
}

verify_kind_version
