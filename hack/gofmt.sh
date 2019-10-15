#!/bin/sh

set -eux

IS_CONTAINER=${IS_CONTAINER:-false}

if [ "${IS_CONTAINER}" != "false" ]; then
  TOP_DIR="${1:-.}"
  go fmt "${TOP_DIR}"/pkg/... "${TOP_DIR}"/cmd/...
else
  podman run --rm \
    --env IS_CONTAINER=TRUE \
    --volume "${PWD}:/workdir:rw,z" \
    --entrypoint sh \
    --workdir /workdir \
    registry.hub.docker.com/library/golang:1.12 \
    /workdir/hack/gofmt.sh "${@}"
fi;
