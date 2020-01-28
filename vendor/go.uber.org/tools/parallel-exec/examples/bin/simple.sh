#!/bin/sh

set -e

sleep ${1}
echo ${2}
if [ -n "${3}" ]; then
  echo "exiting with status ${3}" >&2
  exit ${3}
fi
