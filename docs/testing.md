# Running the tests

## Make targets

Use these targets from the repository root. They match CI more closely
than invoking `go test` or linters directly.

```bash
make unit
make lint
make test
```

- `make unit` runs unit tests (requires envtest)
- `make lint` runs golangci-lint across modules
- `make test` runs generate, lint, manifests, and unit tests

End-to-end tests are documented in [test/e2e/README.md](../test/e2e/README.md)
and can be launched with `./hack/ci-e2e.sh`.

The user running in-cluster tests must be able to create CRDs.

## Hack scripts

The `hack` directory contains containerized checks that match CI.
All of them honor `CONTAINER_RUNTIME` (default `podman`; set to
`docker` if needed).

```bash
./hack/markdownlint.sh
./hack/shellcheck.sh
./hack/manifestlint.sh
```

Run them from the repository root so volume mounts and the Makefile
resolve correctly.
