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
    docker.io/securego/gosec:2.18.2@sha256:2f9daee1739765788945b79de7f46229f33fda5ed35127393d8a1e459f3a7577 \
    /go/src/github.com/metal3-io/baremetal-operator/hack/gosec.sh "$@"
fi
