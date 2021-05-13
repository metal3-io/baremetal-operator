#!/bin/sh

set -eux

IS_CONTAINER=${IS_CONTAINER:-false}
CONTAINER_RUNTIME="${CONTAINER_RUNTIME:-podman}"

GOLANGCI_LINT=bin/golangci-lint
if [ ! -f "${GOLANGCI_LINT}" ]; then
    curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s v1.39.0
fi

if [ "${IS_CONTAINER}" != "false" ]; then
  export XDG_CACHE_HOME="/tmp/.cache"

  $GOLANGCI_LINT run
  cd apis
  ./../$GOLANGCI_LINT run
  cd ..
else
  "${CONTAINER_RUNTIME}" run --rm \
    --env IS_CONTAINER=TRUE \
    --volume "${PWD}:/go/src/github.com/metal3-io/baremetal-operator:ro,z" \
    --entrypoint sh \
    --workdir /go/src/github.com/metal3-io/baremetal-operator \
    quay.io/metal3-io/golint:latest \
    /go/src/github.com/metal3-io/baremetal-operator/hack/golint.sh "${@}"
fi;
