#!/bin/bash

set -ex

CONTROLLER_TOOLS_VERSION=${1:-v0.4.0}
OUTPUT=${2:-${ORIG_DIR}/bin/controller-gen}

WORK_DIR=$(mktemp --tmpdir -d controller-gen-XXX)
trap cleanup EXIT

function cleanup() {
    rm -rf "$WORK_DIR"
}

cd "$WORK_DIR"

go mod init tmp
go get -d "sigs.k8s.io/controller-tools/cmd/controller-gen@${CONTROLLER_TOOLS_VERSION}"
go build -o "${OUTPUT}" sigs.k8s.io/controller-tools/cmd/controller-gen
