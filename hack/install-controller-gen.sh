#!/bin/bash

set -ex

CONTROLLER_TOOLS_VERSION=${1:-v0.4.0}
OUTPUT=bin/controller-gen

# Check for a vendor directory if any downstream forks use that dependency
# tracking method
if [ -d "vendor" ]
then
    go build -o "${OUTPUT}" ./vendor/sigs.k8s.io/controller-tools/cmd/controller-gen
    exit 0;
fi


CMDPATH="$GOPATH/pkg/mod/sigs.k8s.io/controller-tools@${CONTROLLER_TOOLS_VERSION}/cmd/controller-gen"

if [ ! -f "$CMDPATH" ]
then
    go mod download
fi

go build -o "${OUTPUT}" "sigs.k8s.io/controller-tools/cmd/controller-gen"
