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

  INPUT_FILES=$(find pkg/clientset -type f)

  cksum $INPUT_FILES | tee "$ARTIFACTS/client-generate.cksums.before"

  ./hack/run-client-generate.sh

  cksum $INPUT_FILES | tee "$ARTIFACTS/client-generate.cksums.after"
  diff "$ARTIFACTS/client-generate.cksums.before" "$ARTIFACTS/client-generate.cksums.after"

else
  "${CONTAINER_RUNTIME}" run --rm \
    --env IS_CONTAINER=TRUE \
    --volume "${PWD}:/go/src/github.com/metal3-io/baremetal-operator:rw,z" \
    --entrypoint sh \
    --workdir /go/src/github.com/metal3-io/baremetal-operator \
    quay.io/metal3-io/client-code-generator:latest \
    /go/src/github.com/metal3-io/baremetal-operator/hack/"$(basename "$0")" "${@}"
fi;
