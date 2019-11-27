#!/bin/sh

set -eux

# Required operator-sdk version
REQUIRED_OPERATOR_SDK_VERSION="v0.12.0"

if [ ! "$(command -v operator-sdk)" ]; then 
    LOCAL_OPERATOR_SDK_VERSION=""
else 
    LOCAL_OPERATOR_SDK_VERSION="$(operator-sdk version | head -n1 | cut -d" " -f3 | tr -d '",')"
fi

# If local operator-sdk version is not required version
# or operator-sdk is not installed, install operator-sdk v0.12.0
if [ "${LOCAL_OPERATOR_SDK_VERSION}" != "${REQUIRED_OPERATOR_SDK_VERSION}" ] || [ -z "${LOCAL_OPERATOR_SDK_VERSION}" ]; then
    mkdir -p ./bin
    curl -L https://github.com/operator-framework/operator-sdk/releases/download/${REQUIRED_OPERATOR_SDK_VERSION}/operator-sdk-${REQUIRED_OPERATOR_SDK_VERSION}-x86_64-linux-gnu -o ./bin/operator-sdk
    chmod +x ./bin/operator-sdk
fi
