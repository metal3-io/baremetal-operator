#!/bin/sh

set -eux

IS_CONTAINER=${IS_CONTAINER:-false}
CONTAINER_RUNTIME="${CONTAINER_RUNTIME:-podman}"

if [ "${IS_CONTAINER}" != "false" ]; then
  TOP_DIR="${1:-.}"
  export XDG_CACHE_HOME="/tmp/.cache"
  go vet "${TOP_DIR}"/pkg/... "${TOP_DIR}"/cmd/...
else
  "${CONTAINER_RUNTIME}" run --rm \
    --env IS_CONTAINER=TRUE \
    --volume "${PWD}:/go/src/github.com/metal3-io/baremetal-operator:ro,z" \
    --entrypoint sh \
    --workdir /go/src/github.com/metal3-io/baremetal-operator \
    registry.hub.docker.com/library/golang:1.12 \
    /go/src/github.com/metal3-io/baremetal-operator/hack/govet.sh "${@}"
fi;
