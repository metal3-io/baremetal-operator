#!/bin/sh

# Ignore the rule that says we should always quote variables, because
# in this script we *do* want globbing.
# shellcheck disable=SC2086

set -eux

IS_CONTAINER=${IS_CONTAINER:-false}
CONTAINER_RUNTIME="${CONTAINER_RUNTIME:-podman}"

if [ "${IS_CONTAINER}" != "false" ]; then
  export XDG_CACHE_HOME="/tmp/.cache"
  gosec -severity medium --confidence medium -exclude-dir=tools ./...
else
  "${CONTAINER_RUNTIME}" run --rm \
    --env IS_CONTAINER=TRUE \
    --volume "${PWD}:/go/src/github.com/metal3-io/baremetal-operator:ro,z" \
    --entrypoint sh \
    --workdir /go/src/github.com/metal3-io/baremetal-operator \
    registry.hub.docker.com/securego/gosec:latest \
    /go/src/github.com/metal3-io/baremetal-operator/hack/gosec.sh "${@}"
fi;
