#!/bin/bash

ARCH=amd64
MINIMUM_KUSTOMIZE_VERSION=3.5.5

# Ensure the kustomize tool exists and is a viable version, or installs it
verify_kustomize_version() {

  echo "Verifying kustomize version"
  # If kustomize is available on the path, verify the version
  if [ -x "$(command -v ./bin/kustomize)" ]; then
    local kustomize_version
    kustomize_version=$(./bin/kustomize version | grep -P '(?<=kustomize/v)[0-9]+\.[0-9]+\.[0-9]+' -o)
    if [[ "${MINIMUM_KUSTOMIZE_VERSION}" != $(echo -e "${MINIMUM_KUSTOMIZE_VERSION}\n${kustomize_version}" | sort -s -t. -k 1,1 -k 2,2n -k 3,3n | head -n1) ]]; then
      if [[ "${OSTYPE}" == "linux-gnu" ]]; then
        echo "Kustomize version ${kustomize_version}, expected ${MINIMUM_KUSTOMIZE_VERSION}"
        echo "Updating, removing the old version"
        rm ./bin/kustomize
      else
        cat <<EOF
Detected kustomize version: ${kustomize_version}.
Requires ${MINIMUM_KUSTOMIZE_VERSION} or greater.
Please install ${MINIMUM_KUSTOMIZE_VERSION} or later as $(PWD)bin/kustomize.
EOF
        return 2
      fi
    fi
  fi

  # If kustomize is not available on the path, get it
  if ! [ -x "$(command -v ./bin/kustomize)" ]; then
    if [[ "${OSTYPE}" == "linux-gnu" ]]; then
      echo 'kustomize not found, installing'
      if ! [ -d "./bin" ]; then
        mkdir -p "./bin"
      fi
      curl -L -O "https://github.com/kubernetes-sigs/kustomize/releases/download/kustomize%2Fv${MINIMUM_KUSTOMIZE_VERSION}/kustomize_v${MINIMUM_KUSTOMIZE_VERSION}_linux_${ARCH}.tar.gz"
      tar -xzvf kustomize_v${MINIMUM_KUSTOMIZE_VERSION}_linux_${ARCH}.tar.gz
      mv kustomize ./bin
      rm kustomize_v${MINIMUM_KUSTOMIZE_VERSION}_linux_${ARCH}.tar.gz
    else
      echo "Missing required binary: $(PWD)bin/kustomize"
      return 2
    fi
  fi
}

verify_kustomize_version
