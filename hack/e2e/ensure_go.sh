#!/usr/bin/env bash

set -eux

MINIMUM_GO_VERSION=go1.25.7

# Ensure the go tool exists and is a viable version, or installs it
verify_go_version()
{
    # If go is not available on the path, get it
    if ! [ -x "$(command -v go)" ]; then
        if [[ "${OSTYPE}" == "linux-gnu" ]]; then
            echo 'go not found, installing'
            curl -sLo "/tmp/${MINIMUM_GO_VERSION}.linux-amd64.tar.gz" "https://go.dev/dl/${MINIMUM_GO_VERSION}.linux-amd64.tar.gz"
            sudo tar -C /usr/local -xzf "/tmp/${MINIMUM_GO_VERSION}.linux-amd64.tar.gz"
            export PATH=/usr/local/go/bin:$PATH
        else
            echo "Missing required binary in path: go"
            return 2
        fi
    fi

    local go_version
    IFS=" " read -ra go_version <<< "$(go version)"
    if [[ "${MINIMUM_GO_VERSION}" != $(echo -e "${MINIMUM_GO_VERSION}\n${go_version[2]}" | sort -s -t. -k 1,1 -k 2,2n -k 3,3n | head -n1) ]] && [[ "${go_version[2]}" != "devel" ]]; then
        cat << EOF
Detected go version: ${go_version[2]}.
Requires ${MINIMUM_GO_VERSION} or greater.
Please install ${MINIMUM_GO_VERSION} or later.
EOF
        return 2
    fi
}

verify_go_version
