#!/usr/bin/env bash

set -eu

MINIMUM_GO_VERSION=go1.26.6

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
            local GO_TARBALL="${MINIMUM_GO_VERSION}.linux-amd64.tar.gz"
            for attempt in 1 2 3; do
                echo "Downloading Go (attempt ${attempt}/3)"
                if curl -fsSL -o "/tmp/${GO_TARBALL}" "https://go.dev/dl/${GO_TARBALL}" && \
                    curl -fsSL -o "/tmp/${GO_TARBALL}.sha256" "https://dl.google.com/go/${GO_TARBALL}.sha256" && \
                    echo "$(cat "/tmp/${GO_TARBALL}.sha256")  /tmp/${GO_TARBALL}" | sha256sum --check --quiet; then
                    break
                fi
                rm -f "/tmp/${GO_TARBALL}" "/tmp/${GO_TARBALL}.sha256"
            done
            [[ -f "/tmp/${GO_TARBALL}" ]] || { echo "ERROR: failed to download valid Go archive"; return 2; }
            set -x
            sudo tar -C /usr/local -xzf "/tmp/${GO_TARBALL}"
            rm -f "/tmp/${GO_TARBALL}" "/tmp/${GO_TARBALL}.sha256"
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
