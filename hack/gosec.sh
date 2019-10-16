#!/bin/sh

set -eux

IS_CONTAINER=${IS_CONTAINER:-false}

if [ "${IS_CONTAINER}" != "false" ]; then
  export XDG_CACHE_HOME="/tmp/.cache"
  gosec -severity medium --confidence medium -quiet ./...
else
  podman run --rm \
    --env IS_CONTAINER=TRUE \
    --volume "${PWD}:/go/src/github.com/metal3-io/baremetal-operator:ro,z" \
    --entrypoint sh \
    --workdir /go/src/github.com/metal3-io/baremetal-operator \
    registry.hub.docker.com/securego/gosec:latest \
    /go/src/github.com/metal3-io/baremetal-operator/hack/gosec.sh "${@}"
fi;
