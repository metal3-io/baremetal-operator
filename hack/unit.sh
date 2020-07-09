#!/bin/sh

set -eux

IS_CONTAINER=${IS_CONTAINER:-false}
ARTIFACTS=${ARTIFACTS:-/tmp}
CONTAINER_RUNTIME="${CONTAINER_RUNTIME:-podman}"

if [ "${IS_CONTAINER}" != "false" ]; then
  export XDG_CACHE_HOME="/tmp/.cache"
  eval "$(go env)"

  os=$(go env GOOS)
  arch=$(go env GOARCH)

  # download kubebuilder and extract it to tmp
  KUBEBUILDER_VERSION="2.3.1"
  curl -L "https://go.kubebuilder.io/dl/${KUBEBUILDER_VERSION}/${os}/${arch}" | tar -xz -C /tmp/
  mv "/tmp/kubebuilder_${KUBEBUILDER_VERSION}_${os}_${arch}" /usr/local/kubebuilder
  export PATH=$PATH:/usr/local/kubebuilder/bin

  cd "${GOPATH}"/src/github.com/metal3-io/baremetal-operator
  make unit
else
  "${CONTAINER_RUNTIME}" run --rm \
    --env IS_CONTAINER=TRUE \
    --env DEPLOY_KERNEL_URL=http://172.22.0.1/images/ironic-python-agent.kernel \
    --env DEPLOY_RAMDISK_URL=http://172.22.0.1/images/ironic-python-agent.initramfs \
    --env IRONIC_ENDPOINT=http://localhost:6385/v1/ \
    --env IRONIC_INSPECTOR_ENDPOINT=http://localhost:5050/v1/ \
    --volume "${PWD}:/go/src/github.com/metal3-io/baremetal-operator:rw,z" \
    --entrypoint sh \
    --workdir /go/src/github.com/metal3-io/baremetal-operator \
    registry.hub.docker.com/library/golang:1.14 \
    /go/src/github.com/metal3-io/baremetal-operator/hack/unit.sh "${@}"
fi;
