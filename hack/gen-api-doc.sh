#!/bin/bash

set -eux

IS_CONTAINER="${IS_CONTAINER:-false}"
CONTAINER_RUNTIME="${CONTAINER_RUNTIME:-podman}"
CRDOC_VERSION="0.6.2@sha256:355ef777a45021ee864e613b2234b4f2c6193762e3e0de94a26b66d06cec81c3"
WORKDIR="${WORKDIR:-/workdir}"

if [ "${IS_CONTAINER}" != "false" ]; then
    # Running inside a container (e.g., from generate.sh)
    # Skip API doc generation as it's not part of the validation checksums
    # and would require nested containers which may not be supported
    echo "Skipping API doc generation inside container (not part of validation)"
    exit 0
fi

# Run crdoc in its own container
"${CONTAINER_RUNTIME}" run --rm \
    --volume "${PWD}:${WORKDIR}:ro,z" \
    --workdir "${WORKDIR}" \
    ghcr.io/fybrik/crdoc:"${CRDOC_VERSION}" \
    --resources /workdir/config/base/crds/bases \
    --output /dev/stdout \
    > "${PWD}"/docs/api.md
