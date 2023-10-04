#!/usr/bin/env bash

set -eux

# Clean up old output
rm -rf ./test/fuzz/bin
mkdir -p ./test/fuzz/bin

# Install go-fuzz-build
go install github.com/dvyukov/go-fuzz/go-fuzz-build@latest

cd ./test/fuzz
files=$(grep -r --include='**_fuzzer.go' --files-with-matches 'func Fuzz' .)

for file in ${files}
do
    funcs=$(grep -o 'func Fuzz\w*' "${file}" | sed 's/func //')

    for func in ${funcs}
    do
        # Build fuzzing targets using go-fuzz-build.
        go-fuzz-build -libfuzzer -o "bin/${func}.a" -func "${func}" ./ &&
        clang -o "bin/${func}" "bin/${func}.a" -fsanitize=fuzzer
        rm "bin/${func}.a"
    done
done
