#!/usr/bin/env bash
# shellcheck disable=SC1091

set -eux

# Check if docker is installed and install it if not.
# This is optimized to install on Ubuntu.
# The `newgrp` will stop a non-interactive script run.
verify_docker()
{
    if ! [[ -x "$(command -v docker)" ]]; then
        if [[ "${OSTYPE}" == "linux-gnu" ]] && [[ "$(. /etc/os-release && echo "${ID}")" == "ubuntu" ]]; then
            echo "docker not found, installing"
            sudo apt-get install ca-certificates
            sudo install -m 0755 -d /etc/apt/keyrings
            sudo curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc
            sudo chmod a+r /etc/apt/keyrings/docker.asc
            echo \
              "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/ubuntu \
              $(. /etc/os-release && echo "${UBUNTU_CODENAME:-$VERSION_CODENAME}") stable" | \
            sudo tee /etc/apt/sources.list.d/docker.list > /dev/null
            sudo apt-get update
            sudo apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
            sudo usermod -aG docker "${USER}"
            newgrp docker
        else
            echo "Missing required binary in path: docker"
            return 2
        fi
    fi
}

verify_docker
