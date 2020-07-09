#!/bin/sh

# Ignore the rule that says we should always quote variables, because
# in this script we *do* want globbing.
# shellcheck disable=SC2086

set -eux

IS_CONTAINER=${IS_CONTAINER:-false}
CONTAINER_RUNTIME="${CONTAINER_RUNTIME:-podman}"

if [ "${IS_CONTAINER}" != "false" ]; then
  TOP_DIR="${1:-.}"
  export XDG_CACHE_HOME="/tmp/.cache"

  cd "${TOP_DIR}"
  packages="*.go $(go list ./... | sed -e 's|github.com/metal3-io/baremetal-operator||' -e 's|^/||')"

  if [ -n "$(gofmt -l ${packages})" ]; then
      gofmt -d ${packages}
      exit 1
  fi
else
  "${CONTAINER_RUNTIME}" run --rm \
    --env IS_CONTAINER=TRUE \
    --volume "${PWD}:/workdir:rw,z" \
    --entrypoint sh \
    --workdir /workdir \
    registry.hub.docker.com/library/golang:1.14 \
    /workdir/hack/gofmt.sh "${@}"
fi;
