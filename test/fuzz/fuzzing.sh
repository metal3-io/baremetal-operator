#!/bin/bash

set -e
chmod u=rwx ./test/fuzz/fuzzing.sh
rm -rf ./test/fuzz/output
files=$(grep -r --include='**_fuzzer.go' --files-with-matches 'func Fuzz' .)
mkdir -p ./test/fuzz/output
#Install go-fuzz-build
go install github.com/dvyukov/go-fuzz/go-fuzz-build@latest
## Add dependency for go-fuzz-build compilation.
go get github.com/dvyukov/go-fuzz/go-fuzz-dep

for file in ${files}
do
    funcs=$(grep -o 'func Fuzz\w*' "$file" | sed 's/func //')
    cd ./test/fuzz/output

    for func in ${funcs}
    do
        # Build fuzzing targets using go-fuzz-build.
        go-fuzz-build -libfuzzer -o yaml_"$func".a -func "$func" ../ &&
        clang -o "$func" yaml_"$func".a -fsanitize=fuzzer
    done

    for func in ${funcs}
    do
        # Running fuzzing targets. Refer to https://llvm.org/docs/LibFuzzer.html for other options
        ./"$func" -max_total_time=10
    done
done
