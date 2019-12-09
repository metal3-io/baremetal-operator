#!/bin/sh

set -e

sleep ${1}
echo ${2}
if [ -n "${3}" ]; then
  exit ${3}
fi
