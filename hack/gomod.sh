#!/bin/sh
#
# 1.  Verify that `go mod tidy` can be executed successfully
# 2.  Verify that running the above doesn't change go.mod and go.sum
#
# NOTE: This won't work unless the build environment has internet access

set -ux

IS_CONTAINER=${IS_CONTAINER:-false}
CONTAINER_RUNTIME="${CONTAINER_RUNTIME:-podman}"

if [ "${IS_CONTAINER}" != "false" ]; then
  export XDG_CACHE_HOME="/tmp/.cache"

  STATUS=$(git status --porcelain)
  if [ -n "$STATUS" ]; then
      echo "Dirty tree: refusing to continue out of caution"
      exit 1
  fi

  go mod tidy
  rc=$?

  if [ $rc -ne 0 ]; then
      echo "'go mod tidy' failed"
      exit 1;
  fi

  STATUS=$(git status --porcelain go.mod go.sum)
  if [ -n "$STATUS" ]; then
      echo "go.mod and go.sum changed"
      echo "Please run 'go mod tidy' and commit the changes to go.mod & go.sum."
      echo "Abort"
      exit 1
  fi

  exit 0;

else
  "${CONTAINER_RUNTIME}" run --rm \
    --env IS_CONTAINER=TRUE \
    --volume "${PWD}:/go/src/github.com/metal3-io/baremetal-operator:rw,z" \
    --entrypoint sh \
    --workdir /go/src/github.com/metal3-io/baremetal-operator \
    registry.hub.docker.com/library/golang:1.18 \
    /go/src/github.com/metal3-io/baremetal-operator/hack/gomod.sh "${@}"
fi;
