#!/bin/sh

# Ignore the rule that says we should always quote variables, because
# in this script we *do* want globbing.
# shellcheck disable=SC2086,SC2292

set -eux

IS_CONTAINER="${IS_CONTAINER:-false}"
ARTIFACTS="${ARTIFACTS:-/tmp}"
CONTAINER_RUNTIME="${CONTAINER_RUNTIME:-podman}"
WORKDIR="${WORKDIR:-/workdir}"

if [ "${IS_CONTAINER}" != "false" ]; then
    # we need to tell git its OK to use dir owned by someone else
    git config --global safe.directory "${WORKDIR}"
    export XDG_CACHE_HOME="/tmp/.cache"

    INPUT_FILES="$(git ls-files config) $(git ls-files | grep zz_generated)"
    cksum ${INPUT_FILES} > "${ARTIFACTS}/lint.cksums.before"
    export VERBOSE="--verbose"
    make generate manifests
    cksum ${INPUT_FILES} > "${ARTIFACTS}/lint.cksums.after"
    diff "${ARTIFACTS}/lint.cksums.before" "${ARTIFACTS}/lint.cksums.after"

else
    "${CONTAINER_RUNTIME}" run --rm \
        --pull=always \
        --env IS_CONTAINER=TRUE \
        --env DEPLOY_KERNEL_URL=http://172.22.0.1/images/ironic-python-agent.kernel \
        --env DEPLOY_RAMDISK_URL=http://172.22.0.1/images/ironic-python-agent.initramfs \
        --env IRONIC_ENDPOINT=http://localhost:6385/v1/ \
        --volume "${PWD}:${WORKDIR}:rw,z" \
        --entrypoint sh \
        --workdir "${WORKDIR}" \
        quay.io/metal3-io/basic-checks:golang-1.25 \
        "${WORKDIR}"/hack/generate.sh "$@"
fi
