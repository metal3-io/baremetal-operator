#!/usr/bin/env bash

set -eux

JOBS="${JOBS:-8}"
MAX_TIME="${MAX_TIME:-3}"

cd ./test/fuzz/bin

files=(./*)
for file in "${files[@]}"
do
    mkdir -p "../corpus/${file}"
    "${file}" "../corpus/${file}" -jobs="${JOBS}" -max_total_time="${MAX_TIME}"
    mkdir -p "../output/${file}"
    mv fuzz-*.log "../output/${file}/"
done
