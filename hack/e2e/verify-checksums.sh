#!/bin/sh
#
# 1.  Verify that `make update-e2e-checksums` can be executed successfully
# 2.  Verify that running the above doesn't change hack/e2e/checksums.sh
#
# NOTE: This won't work unless the build environment has internet access
# shellcheck disable=SC2292

set -eux

IS_CONTAINER=${IS_CONTAINER:-false}
CONTAINER_RUNTIME="${CONTAINER_RUNTIME:-podman}"

if [ "${IS_CONTAINER}" != "false" ]; then
    export XDG_CACHE_HOME=/tmp/.cache

    mkdir /tmp/verify-checksums
    cp -r . /tmp/verify-checksums
    cd /tmp/verify-checksums

    STATUS="$(git status --porcelain)"
    if [ -n "${STATUS}" ]; then
        echo "Dirty tree: refusing to continue out of caution"
        exit 1
    fi

    make update-e2e-checksums

    STATUS="$(git status --porcelain)"
    if [ -n "${STATUS}" ]; then
        echo "hack/e2e/checksums.sh is out of date"
        echo "${STATUS}"
        git diff
        echo "Please run 'make update-e2e-checksums' and commit the changes"
        exit 1
    fi

else
    "${CONTAINER_RUNTIME}" run --rm \
        --pull=always \
        --env IS_CONTAINER=TRUE \
        --volume "${PWD}:/workdir:ro,z" \
        --entrypoint sh \
        --workdir /workdir \
        quay.io/metal3-io/basic-checks:golang-1.26 \
        /workdir/hack/e2e/verify-checksums.sh "$@"
fi
