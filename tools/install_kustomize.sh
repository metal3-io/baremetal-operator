#!/bin/bash

set -ex

OUTPUT=bin/kustomize

# Check for a vendor directory if any downstream forks use that dependency
# tracking method
if [ -d "vendor" ]
then
    go build -o "${OUTPUT}" ./vendor/sigs.k8s.io/kustomize/kustomize/v3
    exit 0;
fi

CMDPATH="$GOPATH/pkg/mod/sigs.k8s.io/kustomize/kustomize/v3@v3.8.5"

if [ ! -f "$CMDPATH" ]
then
    go mod download
fi

go build -o "${OUTPUT}" "sigs.k8s.io/kustomize/kustomize/v3"
