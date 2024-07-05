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
        docker.io/pipelinecomponents/markdownlint-cli2:0.9.0@sha256:71370df6c967bae548b0bfd0ae313ddf44bfad87da76f88180eff55c6264098c \
        "${WORKDIR}"/hack/markdownlint.sh "$@"
fi
