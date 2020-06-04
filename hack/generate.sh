#!/bin/sh

# Ignore the rule that says we should always quote variables, because
# in this script we *do* want globbing.
# shellcheck disable=SC2086

set -eux

IS_CONTAINER=${IS_CONTAINER:-false}
ARTIFACTS=${ARTIFACTS:-/tmp}
CONTAINER_RUNTIME="${CONTAINER_RUNTIME:-podman}"

if [ "${IS_CONTAINER}" != "false" ]; then
  eval "$(go env)"
  cd "${GOPATH}"/src/github.com/metal3-io/baremetal-operator
  export XDG_CACHE_HOME="/tmp/.cache"

  INPUT_FILES="deploy/crds/*.yaml pkg/apis/metal3/v1alpha1/zz_generated.*.go"
  cksum $INPUT_FILES > "$ARTIFACTS/lint.cksums.before"
  export VERBOSE="--verbose"
  make generate
  cksum $INPUT_FILES > "$ARTIFACTS/lint.cksums.after"
  diff "$ARTIFACTS/lint.cksums.before" "$ARTIFACTS/lint.cksums.after"

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
    quay.io/metal3-io/operator-sdk:latest \
    /go/src/github.com/metal3-io/baremetal-operator/hack/generate.sh "${@}"
fi;
