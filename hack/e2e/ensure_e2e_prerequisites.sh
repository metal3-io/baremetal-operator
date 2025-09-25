#!/usr/bin/env bash

# -----------------------------------------------------------------------------
# Description: This script sets up the environment for E2E tests. It can also
#              be used in verify mode to check if the required tools are installed
#              without making any changes to the system.
# Usage:       To set up the environment: ./ensure_e2e_prerequisites.sh
#              To verify prerequisites without installing:
#              VERIFY_ONLY=1 ./ensure_e2e_prerequisites.sh
# -----------------------------------------------------------------------------

set -eu

REPO_ROOT="$(realpath "$(dirname "${BASH_SOURCE[0]}")/../..")"
VERIFY_ONLY="${VERIFY_ONLY:-false}"

# Source os-release to get ID variable
# shellcheck source=/dev/null
. /etc/os-release

declare -A PACKAGES=(
  [build-essential]=""
  [tar]=""
  [libvirt-daemon-system]=""
  [qemu-kvm]="qemu-system-x86"
  [virt-manager]=""
)

declare -A TOOLS=(
  [ubuntu]="install_pkg_ubuntu dpkg"
)

# Ensure required tools are available
ensure_tools() {
  local missing_tools cmd

  # Ensure required low-level tools are present
  missing_tools=()
  for cmd in ${TOOLS[${ID}]}; do
    if ! command -v "${cmd}" >/dev/null; then
      missing_tools+=("${cmd}")
    fi
  done

  if ((${#missing_tools[@]})); then
    echo "ERROR: Missing required tools: ${missing_tools[*]}"
    return 2
  fi

  return 0
}

# Install packages using apt-get on Ubuntu
install_pkg_ubuntu() {
  local packages

  packages=("$@")

  sudo apt-get update
  sudo apt-get install -y "${packages[@]}"
}

# Ensure required packages are installed, or install them if not#
ensure_packages() {
  local missing installed packages_to_check alt check_tool install_tool
  local version tools

  read -ra tools <<< "${TOOLS[${ID}]}"

  install_tool="${tools[0]}"
  check_tool="${tools[1]}"

  # Collect packages that are not satisfied by any alias
  missing=()
  for pkg in "${!PACKAGES[@]}"; do
    # check any alias is installed
    installed=""
    packages_to_check=("${pkg}")
    if [[ -n "${PACKAGES[${pkg}]}" ]]; then
      read -ra aliases <<< "${PACKAGES[${pkg}]}"
      packages_to_check+=("${aliases[@]}")
    fi
    for alt in "${packages_to_check[@]}"; do
      version="$("${check_tool}" -s "${alt}" 2>/dev/null \
                  | grep '^Version:' \
                  || true)"
      if [[ -n "${version}" ]]; then
        installed="${alt} (${version})"
        break
      fi
    done

    if [[ -n "${installed}" ]]; then
      echo "Package ${pkg} satisfied by installed package: ${installed}"
      continue
    fi

    if [[ "${VERIFY_ONLY}" != "false" ]]; then
      if [[ -n "${PACKAGES[${pkg}]}" ]]; then
        echo "Package ${pkg} (aliases: ${PACKAGES[${pkg}]}) is not installed"
      else
        echo "Package ${pkg} is not installed"
      fi
      continue
    fi

    missing+=("${pkg}")
  done

  if [[ "${VERIFY_ONLY}" == "false" && ${#missing[@]} -gt 0 ]]; then
    echo "Installing missing packages: ${missing[*]}"
    set -x
    "${install_tool}" "${missing[@]}"
    set +x
  fi
}

# Ensure all prerequisites are installed
ensure_components() {
  export PATH="${PATH}:/usr/local/go/bin"
  VERIFY_ONLY="${VERIFY_ONLY}" "${REPO_ROOT}/hack/e2e/ensure_kubectl.sh"
  VERIFY_ONLY="${VERIFY_ONLY}" "${REPO_ROOT}/hack/e2e/ensure_yq.sh"
  VERIFY_ONLY="${VERIFY_ONLY}" "${REPO_ROOT}/hack/e2e/ensure_go.sh"
  VERIFY_ONLY="${VERIFY_ONLY}" "${REPO_ROOT}/hack/e2e/ensure_htpasswd.sh"
  VERIFY_ONLY="${VERIFY_ONLY}" "${REPO_ROOT}/hack/e2e/ensure_docker.sh"
}

# Main execution
case "${ID}" in
  ubuntu)
    ;;
  *)
    echo "ERROR: Distribution not supported: ${ID}"
    return 2
    ;;
esac

ensure_tools
ensure_packages
ensure_components
