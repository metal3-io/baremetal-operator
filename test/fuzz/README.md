# Fuzzing

## Available Fuzz Tests

- `FuzzGetParsedURL` (`bmc_fuzz_test.go`): Parses BMC access URLs across
  supported protocols (IPMI, Redfish, iDRAC, iLO, and their
  virtualmedia/http/https variants) to ensure parsing never panics.
- `FuzzCredentialsValidate` (`credential_fuzz_test.go`): Validates BMC
  username/password combinations, checking that empty fields are rejected and
  non-empty credentials are accepted.
- `FuzzMakeHintMap` (`devicehint_fuzz_test.go`): Converts `RootDeviceHints`
  into the Ironic hint map, verifying device-name, size, and rotational
  mapping invariants.
- `TestMakeHintMapNil` (`devicehint_fuzz_test.go`): Regular regression test
  ensuring `MakeHintMap` handles nil `RootDeviceHints` input by returning an
  empty map without panicking.

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
