#!/bin/sh

set -eux

IS_CONTAINER=${IS_CONTAINER:-false}
ARTIFACTS=${ARTIFACTS:-/tmp}
CONTAINER_RUNTIME="${CONTAINER_RUNTIME:-podman}"
TEST_FLAGS="${TEST_FLAGS:--v}"

if [ "${IS_CONTAINER}" != "false" ]; then
  eval "$(go env)"
  cd "${GOPATH}"/src/github.com/metal3-io/baremetal-operator
  export XDG_CACHE_HOME="/tmp/.cache"
  export COVER_PROFILE="${ARTIFACTS}"/cover.out
  TEST_FLAGS=${TEST_FLAGS} make -e unit-cover

else
  "${CONTAINER_RUNTIME}" run --rm \
    --env IS_CONTAINER=TRUE \
    --env TEST_FLAGS="${TEST_FLAGS}" \
    --env DEPLOY_KERNEL_URL=http://172.22.0.1/images/ironic-python-agent.kernel \
    --env DEPLOY_RAMDISK_URL=http://172.22.0.1/images/ironic-python-agent.initramfs \
    --env IRONIC_ENDPOINT=http://localhost:6385/v1/ \
    --volume "${PWD}:/go/src/github.com/metal3-io/baremetal-operator:rw,z" \
    --entrypoint sh \
    --workdir /go/src/github.com/metal3-io/baremetal-operator \
    docker.io/golang:1.21 \
    /go/src/github.com/metal3-io/baremetal-operator/hack/unit.sh "${@}"
fi;
