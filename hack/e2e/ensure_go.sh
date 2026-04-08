#!/usr/bin/env bash

set -eu

MINIMUM_GO_VERSION=go1.25.7

# Verify mode turned off by default
VERIFY_ONLY="${VERIFY_ONLY:-false}"

# Ensure the go tool exists and is a viable version, or installs it
verify_go_version() {
    # If go is not available on the path, get it
    GO="$(command -v go || true)"
    if ! [[ -x "${GO}" ]]; then
        if [[ "${VERIFY_ONLY}" != "false" ]]; then
            echo "go is not in PATH"
            return 0
        fi
        if [[ "${OSTYPE}" == "linux-gnu" ]]; then
            echo "go not found, installing"
            set -x
            curl -sL \
                -o "/tmp/${MINIMUM_GO_VERSION}.linux-amd64.tar.gz" \
                "https://go.dev/dl/${MINIMUM_GO_VERSION}.linux-amd64.tar.gz"
            sudo tar \
                -C /usr/local \
                -xzf "/tmp/${MINIMUM_GO_VERSION}.linux-amd64.tar.gz"
            set +x
            export PATH="${PATH}:/usr/local/go/bin"
            GO="$(command -v go)"
        else
            echo "ERROR: Missing required binary in path: go"
            return 2
        fi
    fi

    local go_version
    IFS=" " read -ra go_version <<< "$("${GO}" version)"
    if [[ "${MINIMUM_GO_VERSION}" != $(
        echo -e "${MINIMUM_GO_VERSION}\n${go_version[2]}" \
        | sort -s -t. -k 1,1 -k 2,2n -k 3,3n \
        | head -n1
    ) ]] && \
       [[ "${go_version[2]}" != "devel" ]]; then
        cat << EOF
Detected go version: ${go_version[2]}.
Requires ${MINIMUM_GO_VERSION} or greater.
Please install ${MINIMUM_GO_VERSION} or later.
EOF
        return 2
    else
        echo "${go_version[2]} is installed at ${GO}"
    fi
}

verify_go_version
