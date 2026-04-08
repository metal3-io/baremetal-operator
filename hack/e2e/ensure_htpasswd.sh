#!/usr/bin/env bash

set -eu

# Verify mode turned off by default
VERIFY_ONLY="${VERIFY_ONLY:-false}"

# Check if htpasswd is installed and install it if not
verify_htpasswd() {
    if ! [[ -x "$(command -v htpasswd)" ]]; then
        if [[ "${VERIFY_ONLY}" != "false" ]]; then
            echo "htpasswd is not in PATH"
            return 0
        fi
        echo "htpasswd could not be found, installing..."
        set -x
        sudo apt-get update
        sudo apt-get install -y apache2-utils
        set +x
    else
        echo "htpasswd is installed at $(command -v htpasswd)"
    fi
}

verify_htpasswd
