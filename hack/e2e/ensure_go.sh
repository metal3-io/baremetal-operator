#!/usr/bin/env bash

set -eu

# shellcheck source=hack/e2e/checksums.sh
source "$(dirname "${BASH_SOURCE[0]}")/checksums.sh"

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
            if ! command -v sha256sum &>/dev/null; then
                echo "ERROR: sha256sum not found" >&2
                return 1
            fi

            echo "go not found, installing"
            local tmp_dir
            tmp_dir="$(mktemp -d)"
            trap 'rm -rf "${tmp_dir}"; trap - RETURN' RETURN

            local BINARY_FILE="${MINIMUM_GO_VERSION}.linux-amd64.tar.gz"
            local URL="https://go.dev/dl/${BINARY_FILE}"

            local checksum
            local success=false
            for attempt in 1 2 3; do
                echo "Downloading Go (attempt ${attempt}/3)"
                if safe_curl -o "${tmp_dir}/${BINARY_FILE}" "${URL}"; then
                    checksum="$(sha256sum "${tmp_dir}/${BINARY_FILE}" | awk '{print $1;}')"
                    if [[ "${checksum}" == "${GO_SHA256}" ]]; then
                        success=true
                        break
                    fi
                    echo >&2 "fatal: ${URL} checksum '${checksum}' differs from expected '${GO_SHA256}'"
                fi
            done
            if [[ "${success}" != "true" ]]; then
                echo "ERROR: failed to download valid Go archive" >&2
                return 2
            fi

            set -x
            sudo tar -C /usr/local -xzf "${tmp_dir}/${BINARY_FILE}"
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
