#!/bin/bash

set -eux

IS_CONTAINER="${IS_CONTAINER:-false}"
CONTAINER_RUNTIME="${CONTAINER_RUNTIME:-podman}"
CRDOC_VERSION="0.6.2@sha256:355ef777a45021ee864e613b2234b4f2c6193762e3e0de94a26b66d06cec81c3"

if [ "${IS_CONTAINER}" != "false" ]; then
    # we need to tell git its OK to use dir owned by someone else
    git config --global safe.directory "${WORKDIR}"
    diff "${WORKDIR}"/docs/gen-api-doc/api.md" "${WORKDIR}"/docs/api.md"
    
else
    "${CONTAINER_RUNTIME}" run --rm \ 
        -v "${PWD}":/workdir:ro,z \
        ghcr.io/fybrik/crdoc:"${CRDOC_VERSION}" \
        --resources /workdir/config/crd/bases/ \
        --output /dev/stdout \
        > docs/gen-api-doc/api.md
fi