#!/usr/bin/env bash

set -eux

USR_LOCAL_BIN="/usr/local/bin"
OS=$(go env GOOS)
ARCH=$(go env GOARCH)
MINIMUM_MINIKUBE_VERSION=v1.31.2

verify_minikube_version() {
  if ! [ -x "$(command -v minikube)" ]; then
      if [[ "${OSTYPE}" == "linux-gnu" ]]; then
          echo "minikube not found, installing"
          curl -LO "https://storage.googleapis.com/minikube/releases/${MINIMUM_MINIKUBE_VERSION}/minikube-${OS}-${ARCH}"
          sudo install minikube-linux-amd64 "${USR_LOCAL_BIN}/minikube"
      else
          echo "Missing required binary in path: minikube"
          return 2
      fi
  fi
  local minikube_version
  minikube_version="$(minikube version --short)"
  if [[ "${MINIMUM_MINIKUBE_VERSION}" != $(echo -e "${MINIMUM_MINIKUBE_VERSION}\n${minikube_version}" | sort -V | head -n1) ]]; then
      cat << EOF
Detected minikube version: ${minikube_version}.
Requires ${MINIMUM_MINIKUBE_VERSION} or greater.
Please install ${MINIMUM_MINIKUBE_VERSION} or later.
EOF
      return 2
  fi
}

verify_minikube_version
