#!/bin/bash

# TODO:
# Fix these two failures in future commit, and remove the ignore rules
#
# Further documentation is available for these failures:
# - MD029: https://github.com/markdownlint/markdownlint/blob/main/docs/RULES.md#md029---ordered-list-item-prefix
# - MD013: https://github.com/markdownlint/markdownlint/blob/main/docs/RULES.md#md013---line-length

set -eux

IS_CONTAINER=${IS_CONTAINER:-false}
CONTAINER_RUNTIME="${CONTAINER_RUNTIME:-podman}"

if [ "${IS_CONTAINER}" != "false" ]; then
  TOP_DIR="${1:-.}"
  find "${TOP_DIR}" \
       \( -path ./vendor -o -path ./.github \) \
      -prune -o -name '*.md' -exec \
      mdl --style all --warnings \
      --rules "~MD013,~MD029" \
      {} \+
else
  "${CONTAINER_RUNTIME}" run --rm \
    --env IS_CONTAINER=TRUE \
    --volume "${PWD}:/workdir:ro,z" \
    --entrypoint sh \
    --workdir /workdir \
    docker.io/pipelinecomponents/markdownlint:0.13.0@sha256:9c0cdfb64fd3f1d3bdc5181629b39c2e43b6a52fc9fdc146611e1860845bbae0 \
    /workdir/hack/markdownlint.sh "$@"
fi
