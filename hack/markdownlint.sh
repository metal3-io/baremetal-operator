#!/bin/sh
# markdownlint-cli2 has config file(s) named .markdownlint-cli2.yaml in the repo
# shellcheck disable=SC2292

set -eux

IS_CONTAINER="${IS_CONTAINER:-false}"
CONTAINER_RUNTIME="${CONTAINER_RUNTIME:-podman}"
WORKDIR="${WORKDIR:-/workdir}"

# all md files, but ignore .github
if [ "${IS_CONTAINER}" != "false" ]; then
    markdownlint-cli2 "**/*.md" "#.github"
else
    "${CONTAINER_RUNTIME}" run --rm \
        --env IS_CONTAINER=TRUE \
        --volume "${PWD}:${WORKDIR}:ro,z" \
        --entrypoint sh \
        --workdir "${WORKDIR}" \
        docker.io/pipelinecomponents/markdownlint-cli2:0.12.0@sha256:a3977fba9814f10d33a1d69ae607dc808e7a6470b2ba03e84c17193c0791aac0 \
        "${WORKDIR}"/hack/markdownlint.sh "$@"
fi
