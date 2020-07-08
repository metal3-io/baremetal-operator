#!/bin/sh

set -eux

IS_CONTAINER=${IS_CONTAINER:-false}
ARTIFACTS=${ARTIFACTS:-/tmp}
CONTAINER_RUNTIME="${CONTAINER_RUNTIME:-podman}"

if [ "${IS_CONTAINER}" != "false" ]; then
  eval "$(go env)"
  cd "${GOPATH}"/src/github.com/metal3-io/baremetal-operator

  export XDG_CACHE_HOME="/tmp/.cache"
  export DEPLOY_KERNEL_URL=http://172.22.0.1/images/ironic-python-agent.kernel \
  export DEPLOY_RAMDISK_URL=http://172.22.0.1/images/ironic-python-agent.initramfs \
  export IRONIC_ENDPOINT=http://localhost:6385/v1/ \
  export IRONIC_INSPECTOR_ENDPOINT=http://localhost:5050/v1/ \

  # download kubebuilder and extract it to tmp
  curl -L https://go.kubebuilder.io/dl/2.3.1/${GOOS}/${GOARCH} | tar -xz -C /tmp/
  mv /tmp/kubebuilder_2.3.1_${GOOS}_${GOARCH} /usr/local/kubebuilder

  go test ./... -coverprofile "${ARTIFACTS}"/cover.out
else
  "${CONTAINER_RUNTIME}" run --rm \
    --env IS_CONTAINER=TRUE \
    --volume "${PWD}:/go/src/github.com/metal3-io/baremetal-operator:rw,z" \
    --entrypoint sh \
    --workdir /go/src/github.com/metal3-io/baremetal-operator \
    registry.hub.docker.com/library/golang:1.14 \
    /go/src/github.com/metal3-io/baremetal-operator/hack/unit.sh "${@}"
fi;
