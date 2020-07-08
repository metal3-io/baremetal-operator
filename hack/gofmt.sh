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

  dirs="${TOP_DIR}/{api,bmc,cmd,controllers,hardware,provisioner,utils,version}"

  if [ -n "$(gofmt -l ${dirs})" ]; then
      gofmt -d ${dirs}
      exit 1
  fi
else
  "${CONTAINER_RUNTIME}" run --rm \
    --env IS_CONTAINER=TRUE \
    --volume "${PWD}:/workdir:rw,z" \
    --entrypoint sh \
    --workdir /workdir \
    registry.hub.docker.com/library/golang:1.12 \
    /workdir/hack/gofmt.sh "${@}"
fi;
