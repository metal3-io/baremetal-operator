#!/bin/sh

set -eux

IS_CONTAINER=${IS_CONTAINER:-false}
CONTAINER_RUNTIME="${CONTAINER_RUNTIME:-podman}"

if [ "${IS_CONTAINER}" != "false" ]; then
  export XDG_CACHE_HOME="/tmp/.cache"

  # It seems like gosec does not handle submodules well. Therefore we skip them and run separately.
  gosec -severity medium --confidence medium -quiet -exclude-dir=apis -exclude-dir=hack/tools -exclude-dir=test ./...
  (cd apis && gosec -severity medium --confidence medium -quiet ./...)
  (cd hack/tools && gosec -severity medium --confidence medium -quiet ./...)
  (cd test && gosec -severity medium --confidence medium -quiet ./...)
else
  "${CONTAINER_RUNTIME}" run --rm \
    --env IS_CONTAINER=TRUE \
    --volume "${PWD}:/go/src/github.com/metal3-io/baremetal-operator:ro,z" \
    --entrypoint sh \
    --workdir /go/src/github.com/metal3-io/baremetal-operator \
    docker.io/securego/gosec:2.14.0@sha256:73858f8b1b9b7372917677151ec6deeceeaa40c5b02753080bd647dede14e213 \
    /go/src/github.com/metal3-io/baremetal-operator/hack/gosec.sh "${@}"
fi;
