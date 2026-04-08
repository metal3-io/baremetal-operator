#!/usr/bin/env bash

set -euo pipefail

# Verify mode turned off by default
VERIFY_ONLY="${VERIFY_ONLY:-false}"

# Source os-release to get ID variable
# shellcheck source=/dev/null
. /etc/os-release

# Description:
#   Prompt the user for a yes/no confirmation.
#
# Usage:
#   confirm <default> <message>
#   <default> must be "y" or "n" (case-insensitive).
#
# Returns:
#   Exit code 0 (true) if the user confirms; 1 (false) otherwise.
confirm() {
    local default message prompt reply

    # normalize to lowercase
    default="${1,,}"
    if [[ "${default}" == "y" ]]; then
        prompt="[Y/n]"
    else
        prompt="[y/N]"
    fi

    message="${2}"
    while true; do
        read -r -p "${message} Do you want to continue? ${prompt} " reply
        # use default if Enter pressed
        reply="${reply:-${default}}"
        case "${reply,,}" in
            y) return 0 ;;
            n) return 1 ;;
            *) echo "Please enter y or n." ;;
        esac
    done
}

# Description:
#   Install Docker on Ubuntu.
#
# Usage:
#   install_docker_ubuntu download_url
install_docker_ubuntu() {
    local DOCKER_DOWNLOAD_URL

    DOCKER_DOWNLOAD_URL="${1}"

    sudo apt-get install -y ca-certificates curl
    sudo install -m 0755 -d /etc/apt/keyrings
    sudo curl -fsSL \
        -o /etc/apt/keyrings/docker.asc \
        "${DOCKER_DOWNLOAD_URL}/gpg"
    sudo chmod a+r /etc/apt/keyrings/docker.asc
    echo \
        "deb [arch=$(dpkg --print-architecture)" \
        "signed-by=/etc/apt/keyrings/docker.asc]" \
        "${DOCKER_DOWNLOAD_URL}" \
        "${UBUNTU_CODENAME:-$VERSION_CODENAME} stable" | \
        sudo tee /etc/apt/sources.list.d/docker.list > /dev/null
    sudo apt-get update
    sudo apt-get install -y \
        docker-ce \
        docker-ce-cli \
        containerd.io \
        docker-buildx-plugin \
        docker-compose-plugin
    sudo usermod -aG docker "${USER}"

    echo "Docker installed successfully, please log"\
         "out and log back in to apply the new group membership"
}

# Description:
#   Prompt the user for confirmation and execute a function if confirmed.
#
# Usage:
#   confirm_and_run <default> <message> <function> [args...]
#
# Returns:
#   Exit code 0 (true) if confirmed and function ran; 1 (false) if declined.
confirm_and_run() {
    local default fn message

    local default="${1}"
    local message="${2}"
    local fn="${3}"
    shift 3

   if confirm "${default}" "${message}"; then
        set -x
        "${fn}" "$@"
        set +x
    else
        echo "Aborted."
        return 1
    fi
}

# Check if docker is installed and install it if not
verify_docker() {
    local DOCKER URL

    DOCKER="$(command -v docker || true)"
    if ! [[ -x "${DOCKER}" ]]; then
        if [[ "${VERIFY_ONLY}" != "false" ]]; then
            echo "docker is not in PATH"
            return 0
        fi
        echo "docker not found, installing"
        case "${ID}" in
            ubuntu)
                URL="https://download.docker.com/linux/ubuntu"
                confirm_and_run \
                    "n" \
                    "This action will install the latest Docker from ${URL}." \
                    install_docker_ubuntu \
                    "${URL}"
                ;;
            *)
                echo "ERROR: Distribution not supported: ${ID}"
                return 2
                ;;
        esac
    else
        echo "$(${DOCKER} --version) is installed at ${DOCKER}"
    fi
}

verify_docker
