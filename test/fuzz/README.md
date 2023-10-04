# Fuzzing

Fuzzing or fuzz testing is an automated software testing technique that involves
providing invalid, unexpected, or random data as inputs to a computer program. A
fuzzing target is defined for running the tests. These targets are defined in
the test/fuzz directory.

There are scripts and make targets (that run the scripts) for building and
running the fuzzers. They are `make build-fuzzers` or `test/fuzz/build.sh` for
building and `make fuzz` or `test/fuzz/run.sh` for running.

The build script will compile binaries and put them in the `test/fuzz/bin`
folder. The run script will run the binaries and create corpus folders under
`test/fuzz/corpus`. Logs will be stored in `test/fuzz/output`.
