#!/bin/sh
#
# 1.  Verify that `go mod tidy` can be executed successfully
# 2.  Verify that running the above doesn't change go.mod and go.sum
#
# NOTE: This won't work unless the build environment has internet access

set -eux

IS_CONTAINER=${IS_CONTAINER:-false}
CONTAINER_RUNTIME="${CONTAINER_RUNTIME:-podman}"

if [ "${IS_CONTAINER}" != "false" ]; then
    export XDG_CACHE_HOME=/tmp/.cache

    mkdir /tmp/gomod
    cp -r . /tmp/gomod
    cd /tmp/gomod

    STATUS="$(git status --porcelain)"
    if [ -n "${STATUS}" ]; then
        echo "Dirty tree: refusing to continue out of caution"
        exit 1
    fi

    make mod

    STATUS="$(git status --porcelain)"
    if [ -n "${STATUS}" ]; then
        echo "one of the go.mod and/or go.sum files changed"
        echo "${STATUS}"
        echo "Please run 'go mod tidy' and commit the changes"
        exit 1
    fi

else
    "${CONTAINER_RUNTIME}" run --rm \
        --env IS_CONTAINER=TRUE \
        --volume "${PWD}:/workdir:ro,z" \
        --entrypoint sh \
        --workdir /workdir \
        docker.io/golang:1.21 \
        /workdir/hack/gomod.sh "$@"
fi
