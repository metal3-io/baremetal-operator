#!/bin/sh

set -eux

IS_CONTAINER=${IS_CONTAINER:-false}
CONTAINER_RUNTIME="${CONTAINER_RUNTIME:-podman}"
K8S_VERSION="${K8S_VERSION:-master}"

# --strict: Disallow additional properties not in schema.
# --ignore-missing-schemas: Skip validation for resource
# definitions without a schema. This will skip the checks
# for the Custom Resource Definitions(CRDs).
# --ignore-filename-pattern string: ignore pattern, can give multiple
# We are skipping validation for the files that
# matches our regexp pattern (i.e. kustom, patch).
# --output string: The format of the output of this script.
# --kubernetes-version string: which k8s version schema to test against

# KUBECONFORM_PATH is needed as kubeconform binary in the official image
# is at the root /kubeconform, but it is not at default path, while
# in non-container run, it is on go bin path and can't have leading /

if [ "${IS_CONTAINER}" != "false" ]; then
    { set +x; } 2>/dev/null
    echo "<-------------------------STARTING MANIFESTS VALIDATION CHECKS------------------------->"
    "${KUBECONFORM_PATH:-}"kubeconform --strict --ignore-missing-schemas \
    --kubernetes-version "${K8S_VERSION}" \
    --ignore-filename-pattern kustom --ignore-filename-pattern patch \
    --ignore-filename-pattern controller_manager_config \
    --output tap \
    config/ examples/
    echo "<-------------------------COMPLETED MANIFESTS VALIDATION CHECKS------------------------>"
else
  "${CONTAINER_RUNTIME}" run --rm \
    --env IS_CONTAINER=TRUE \
    --env KUBECONFORM_PATH="/" \
    --volume "${PWD}:/workdir:ro,z" \
    --entrypoint sh \
    --workdir /workdir \
    ghcr.io/yannh/kubeconform:v0.5.0-alpine@sha256:ae1583859c7c9683b2f044a9cc911d9427030cd48ec33eeff6c0f23396780da9 \
    /workdir/hack/manifestlint.sh "${@}"
fi;
