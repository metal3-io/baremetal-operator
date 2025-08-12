#!/bin/bash

set -eu

CONTAINER_RUNTIME="${CONTAINER_RUNTIME:-podman}"
CRDOC_VERSION="0.6.2@sha256:355ef777a45021ee864e613b2234b4f2c6193762e3e0de94a26b66d06cec81c3"

"${CONTAINER_RUNTIME}" run --rm -v "${PWD}":/src:ro,z \
    ghcr.io/fybrik/crdoc:"${CRDOC_VERSION}" \
    --resources /src/config/base/crds/bases/ --output /dev/stdout \
    > docs/api.md