#!/bin/sh

set -eux

IS_CONTAINER=${IS_CONTAINER:-false}
CONTAINER_RUNTIME="${CONTAINER_RUNTIME:-podman}"

if [ "${IS_CONTAINER}" != "false" ]; then
  export XDG_CACHE_HOME="/tmp/.cache"

  make lint
else
  "${CONTAINER_RUNTIME}" run -u "$(id -u)" --rm \
    --env IS_CONTAINER=TRUE \
    --volume "${PWD}:/go/src/github.com/metal3-io/baremetal-operator:rw,z" \
    --entrypoint sh \
    --workdir /go/src/github.com/metal3-io/baremetal-operator \
    docker.io/golang:1.21 \
    /go/src/github.com/metal3-io/baremetal-operator/hack/golint.sh "${@}"
fi;
