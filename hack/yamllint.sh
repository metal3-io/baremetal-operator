#!/bin/sh
# yamllint config file is .yamllint.yaml in the repo
# shellcheck disable=SC2292

set -eux

IS_CONTAINER="${IS_CONTAINER:-false}"
CONTAINER_RUNTIME="${CONTAINER_RUNTIME:-podman}"

if [ "${IS_CONTAINER}" != "false" ]; then
    yamllint -c .yamllint.yaml .

    SNIPPET_DIR="$(mktemp -d)"
    trap 'rm -rf "${SNIPPET_DIR}"' EXIT

    find . -name "*.md" -not -path "./.git/*" | while IFS= read -r md_file; do
        # shellcheck disable=SC2001
        safe_name=$(echo "${md_file}" | sed 's#[/.]#_#g')
        awk -v base="${safe_name}" -v dir="${SNIPPET_DIR}" '
            BEGIN { count = 0 }
            /^```(yaml|yml)/ { in_block = 1; next }
            /^```/ && in_block {
                if (block != "") {
                    count++
                    file = sprintf("%s/%s_snippet_%03d.yaml", dir, base, count)
                    print block > file
                    close(file)
                    block = ""
                }
                in_block = 0
                next
            }
            in_block { block = (block == "" ? $0 : block "\n" $0) }
        ' "${md_file}"
    done

    if [ -n "$(ls -A "${SNIPPET_DIR}" 2>/dev/null)" ]; then
        yamllint -c .yamllint.yaml -s "${SNIPPET_DIR}"
    fi
else
    "${CONTAINER_RUNTIME}" run --rm \
        --env IS_CONTAINER=TRUE \
        --volume "${PWD}:/workdir:ro,z" \
        --entrypoint sh \
        --workdir /workdir \
        docker.io/pipelinecomponents/yamllint:0.35.13@sha256:5ab5eb7da0ed5e606b07c1723fc8b275e925189f70ac259b26b7329cb5f8f44d \
        /workdir/hack/yamllint.sh "$@"
fi
