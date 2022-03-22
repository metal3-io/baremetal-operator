#!/bin/bash

set -eux

IS_CONTAINER=${IS_CONTAINER:-false}
CONTAINER_RUNTIME="${CONTAINER_RUNTIME:-podman}"

if [ "${IS_CONTAINER}" != "false" ]; then
  TOP_DIR="${1:-.}"
  find "${TOP_DIR}" \
       \( -path ./vendor -prune \) \
      -o -name '*.md' -exec \
      mdl --style all --warnings \
      --rules "MD001,MD002,MD003,MD004,MD005,MD006,MD007,MD009,MD010,MD011,MD012,MD014,MD018,MD019,MD020,MD021,MD022,MD023,MD024,MD025,MD026,MD027,MD028,MD030,MD031,MD032,MD033,MD034,MD035,MD036,MD037,MD038,MD039,MD040,MD041" \
      {} \+
else
  "${CONTAINER_RUNTIME}" run --rm \
    --env IS_CONTAINER=TRUE \
    --volume "${PWD}:/workdir:ro,z" \
    --entrypoint sh \
    --workdir /workdir \
    registry.hub.docker.com/pipelinecomponents/markdownlint:latest \
    /workdir/hack/markdownlint.sh "${@}"
fi;

# $ mdl --style all -l
# MD001 - Header levels should only increment by one level at a time
# MD002 - First header should be a top level header
# MD003 - Header style
# MD004 - Unordered list style
# MD005 - Inconsistent indentation for list items at the same level
# MD006 - Consider starting bulleted lists at the beginning of the line
# MD007 - Unordered list indentation
# MD009 - Trailing spaces
# MD010 - Hard tabs
# MD011 - Reversed link syntax
# MD012 - Multiple consecutive blank lines
# MD014 - Dollar signs used before commands without showing output
# MD018 - No space after hash on atx style header
# MD019 - Multiple spaces after hash on atx style header
# MD020 - No space inside hashes on closed atx style header
# MD021 - Multiple spaces inside hashes on closed atx style header
# MD022 - Headers should be surrounded by blank lines
# MD023 - Headers must start at the beginning of the line
# MD024 - Multiple headers with the same content
# MD025 - Multiple top level headers in the same document
# MD026 - Trailing punctuation in header
# MD027 - Multiple spaces after blockquote symbol
# MD028 - Blank line inside blockquote
# MD029 - Ordered list item prefix - DISABLED
# MD030 - Spaces after list markers
# MD031 - Fenced code blocks should be surrounded by blank lines
# MD032 - Lists should be surrounded by blank lines
# MD033 - Inline HTML
# MD034 - Bare URL used
# MD035 - Horizontal rule style
# MD036 - Emphasis used instead of a header
# MD037 - Spaces inside emphasis markers
# MD038 - Spaces inside code span elements
# MD039 - Spaces inside link text
# MD040 - Fenced code blocks should have a language specified
# MD041 - First line in file should be a top level header
# MD046 - Code block style - DISABLED
