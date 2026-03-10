# Fuzzing

## Running Fuzz Tests

### Quick Start with Makefile

The easiest way to run fuzz tests is using the Makefile targets:

```bash
# Run fuzz tests as regression tests (using seed corpus only, fast)
make fuzz

# Run all fuzz tests sequentially with fuzzing enabled (default: 30 seconds each)
make fuzz-run

# Run all fuzz tests for custom duration (e.g., 5 minutes each)
make fuzz-run FUZZ_TIME=5m
```

The `fuzz-run` target automatically discovers and runs all fuzz tests by
iteration, dedicating the specified time to each test.

### Crash Corpus and Regression Testing

When fuzzing discovers a crash, Go automatically saves the failing input to
`testdata/fuzz/<FuzzTestName>/` in the test directory. These crash files should
be committed to the repository:

```bash
git add test/fuzz/testdata/
git commit -m "Add fuzz crash corpus"
```

Once committed, these crashes are automatically replayed as regression tests
when running `make fuzz` (or `go test` without `-fuzz`).

## Resources

- [Go Fuzzing Documentation](https://go.dev/doc/fuzz/)
- [Go Fuzzing Tutorial](https://go.dev/security/fuzz/)
