#!/bin/sh

set -eux

IS_CONTAINER=${IS_CONTAINER:-false}
CONTAINER_RUNTIME="${CONTAINER_RUNTIME:-podman}"

if [ "${IS_CONTAINER}" != "false" ]; then
  TOP_DIR="${1:-.}"
  find "${TOP_DIR}" \
    -type f -name '*.sh' -exec shellcheck -s bash {} \+
else
  "${CONTAINER_RUNTIME}" run --rm \
    --env IS_CONTAINER=TRUE \
    --volume "${PWD}:/workdir:ro,z" \
    --entrypoint sh \
    --workdir /workdir \
    registry.hub.docker.com/koalaman/shellcheck-alpine:stable \
    /workdir/hack/shellcheck.sh "${@}"
fi;
