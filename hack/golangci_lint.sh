#!/bin/sh

set -eux

# If this is run from container, golangci-lint command should exist.
# If not, that means it is run from local machine.
if ! hash golangci-lint 2>/dev/null;
then
  GOLANGCI_LINT=bin/golangci-lint
  if [ ! -f "${GOLANGCI_LINT}" ]; then
    curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s v1.41.1
  fi
  ${GOLANGCI_LINT} run
else
  golangci-lint run
fi
