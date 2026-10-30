# Baremetal Operator (BMO)

Kubernetes operator that manages bare metal hosts through the
`BareMetalHost` API, with Ironic as the default provisioner backend.
Project overview: [README.md](README.md). Contribution rules:
[CONTRIBUTING.md](CONTRIBUTING.md) and the
[Metal3 community guide](https://github.com/metal3-io/community/blob/main/CONTRIBUTING.md).

This file holds only what you cannot cheaply infer from the code. Where it
disagrees with the code or the Makefile, trust those and report the
mismatch.

## AI-assisted contributions

Metal3 follows the CNCF and
[Linux Foundation generative AI policy](https://www.linuxfoundation.org/legal/generative-ai).

- AI is a tool. The human who opens a PR is its author and is responsible
  for every line of it, including licensing and third-party content.
- Review is a conversation between humans. A PR where an agent or bot
  answers reviewers will be closed. Agents may draft changes; the
  contributor reads them, understands them, and replies personally.

- Keep changes surgical. The smallest complete change that solves the
  stated problem wins. Every added file, function, flag, dependency or
  test helper must trace to that problem; anything that does not is
  bloat and gets removed. Prefer reusing existing code over new code.
  Report unrelated findings instead of fixing them in the same PR.

## Repository map

Five Go modules (root, `apis/`, `pkg/hardwareutils/`, `test/`,
`hack/tools/`), wired together with `replace` directives. Root
`go test ./...` does not cover the other four.

- Host lifecycle: `internal/controller/metal3.io/host_state_machine.go`,
  documented in [docs/baremetalhost-states.md](docs/baremetalhost-states.md).
- Provisioner interface: `pkg/provisioner/provisioner.go`. Implementations:
  `ironic/`, `demo/` (built as Go plugin `.so` files) and `fixture/`
  (compiled in, used by unit and fixture E2E tests).
- Feature gates: `pkg/features/features.go`.

## Testing

- Targets: `make help` and [docs/testing.md](docs/testing.md). CI jobs:
  [Prow](https://github.com/metal3-io/project-infra/blob/main/prow/config/jobs/metal3-io/baremetal-operator.yaml)
  and `.github/workflows/`.
- Conventions are whatever the existing code, tests, and lint configs do.
  Read neighboring files and `_test.go` files before writing new ones.
- Before finishing, add or update tests for every behavior change, then
  run `make test` and the `hack/` checks for the file types you touched.
  All must pass.
- Never hand-edit generated output (`zz_generated.*.go`, CRDs, RBAC
  role and webhook manifests under `config/`). Change the source and run
  `make generate manifests`. The `generate` CI job fails on any diff.
- Do not run `hack/ci-e2e.sh` or `hack/e2e/ensure_*.sh` unless the user
  asks: they install packages with `sudo apt-get`, install Go into
  `/usr/local/go`, and create libvirt VMs and networks on the host. CI
  runs E2E on every PR that changes code.

## Invariants

These apply to writing and to reviewing changes.

- **API versions.** `BareMetalHost` and `HardwareData` exist in `v1alpha1`
  (storage version, conversion spoke) and `v1beta1` (served, conversion
  hub). A field change needs both types, the conversion functions in
  `apis/metal3.io/v1alpha1/conversion.go`, and a passing
  `TestFuzzyConversion` round-trip. Fields with no `v1alpha1` equivalent
  survive the round-trip through the conversion-data annotation
  (`conversion_data.go`). Validation and defaulting webhooks exist only
  for `v1alpha1` (`internal/webhooks/metal3.io/v1alpha1/`).
- **API compatibility.** Never remove or rename a served field, and never
  tighten validation on existing fields, without a deprecation path.
  Mark such PRs as breaking.
- **Provisioner plugins.** Out-of-tree plugins load into the manager
  process and must match its Go toolchain and every shared module version
  exactly. A change to `pkg/provisioner/provisioner.go` or to the plugin
  symbols breaks every existing plugin: mark it breaking and update
  [docs/plugin-provisioners.md](docs/plugin-provisioners.md).
- **State machine.** A change to host state transitions must update
  `docs/baremetalhost-states.md` and `docs/BaremetalHost_ProvisioningState.dot`
  (then `make docs`).
- **RBAC.** Grant permissions with `+kubebuilder:rbac` markers on the
  controller that needs them, then `make manifests`.
- **Security.** Never weaken security to make something work: no
  disabled TLS verification, broadened RBAC, secrets in code, logs or
  fixtures, or skipped validation. Treat resource specs, BMC and Ironic
  responses, and image URLs as untrusted input. Call out any change to
  authentication, TLS, RBAC, Secrets or webhooks in the PR description.
- **Tenancy and isolation.** One BMO serves many namespaces, watching
  all of them or a `WATCH_NAMESPACE` list
  ([docs/namespace-scoped-setup.md](docs/namespace-scoped-setup.md)).
  Review these areas with extra care:
   - Cross-namespace access. A reference must never let one namespace
     read or modify another namespace's hosts or Secrets. Secrets
     resolve in the referencing object's own namespace (see
     `CredentialsKey()`). `HostClaim` binding to a `BareMetalHost` in
     another namespace is allowed only through a `HostDeployPolicy`
     (`pkg/hostclaim/`).
   - Role isolation. Tenant-facing roles (`*_editor_role.yaml`,
     `*_viewer_role.yaml`) must not gain access to other kinds,
     Secrets or cluster-scoped resources. Manager RBAC must also work
     with namespace-scoped Roles.
   - Multi-tenancy. Status, events, logs and metrics must not leak one
     tenant's data, such as BMC addresses or credentials, to another.
- **Pinning.** Pin new external references: GitHub Actions by commit SHA,
  container images by digest, downloaded binaries by checksum.

## External references

- [Metal3 Book, BMO user guide](https://book.metal3.io/bmo/introduction)
- [BMO design proposals](https://github.com/metal3-io/metal3-docs/tree/main/design/baremetal-operator)
- [Metal3 API design principles](https://github.com/metal3-io/metal3-docs/blob/main/design/api-design-principles.md)
