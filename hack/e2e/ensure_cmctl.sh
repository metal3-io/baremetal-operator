#!/usr/bin/env bash

set -eux

USR_LOCAL_BIN="/usr/local/bin"
OS=$(go env GOOS)
ARCH=$(go env GOARCH)
MINIMUM_CMCTL_VERSION=v1.13.0

verify_cmctl_version() {
  if ! [ -x "$(command -v cmctl)" ]; then
      if [[ "${OSTYPE}" == "linux-gnu" ]]; then
          echo "cmctl not found, installing"
          curl -fsSL -o cmctl.tar.gz "https://github.com/cert-manager/cert-manager/releases/download/${MINIMUM_CMCTL_VERSION}/cmctl-${OS}-${ARCH}.tar.gz"
          tar xzf cmctl.tar.gz
          sudo mv cmctl "${USR_LOCAL_BIN}/cmctl"
          rm cmctl.tar.gz
      else
          echo "Missing required binary in path: cmctl"
          return 2
      fi
  fi

  local cmctl_version
  IFS=" " read -ra cmctl_version <<< "$(cmctl version --client --short)"
  if [[ "${MINIMUM_CMCTL_VERSION}" != $(echo -e "${MINIMUM_CMCTL_VERSION}\n${cmctl_version[2]}" | sort -s -t. -k 1,1 -k 2,2n -k 3,3n | head -n1) ]]; then
      cat << EOF
Detected cmctl version: ${cmctl_version[2]}.
Requires ${MINIMUM_CMCTL_VERSION} or greater.
Please install ${MINIMUM_CMCTL_VERSION} or later.
EOF
      return 2
  fi
}

verify_cmctl_version
