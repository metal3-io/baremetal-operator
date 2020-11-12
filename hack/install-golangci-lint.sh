#!/bin/bash

set -ex

OUTPUT=bin/golangci-lint

# Check for a vendor directory if any downstream forks use that dependency
# tracking method
if [ -d "vendor" ]
then
    go build -o "${OUTPUT}" ./vendor/github.com/golangci/golangci-lint/cmd/golangci-lint
    exit 0;
fi


CMDPATH="$GOPATH/pkg/mod/github.com/golangci/golangci-lint@v1.32.0/cmd/golangci-lint"

if [ ! -f "$CMDPATH" ]
then
    go mod download
fi

go build -o "${OUTPUT}" "github.com/golangci/golangci-lint/cmd/golangci-lint"
