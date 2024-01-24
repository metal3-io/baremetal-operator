#!/usr/bin/env bash

set -eux

# Check if htpasswd is installed and install it if not
verify_htpasswd()
{
    if ! [ -x "$(command -v htpasswd)" ]; then
        echo "htpasswd could not be found, installing..."
        sudo apt-get update
        sudo apt-get install -y apache2-utils
    fi
}

verify_htpasswd
