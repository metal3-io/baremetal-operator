# Baremetal Operator - AI Coding Assistant Instructions

## Project Overview

The Baremetal Operator (BMO) is a Kubernetes controller that manages bare
metal hosts through the `BareMetalHost` custom resource. It integrates with
OpenStack Ironic to provision, inspect, and deprovision physical servers.
This is part of the Metal³ project for bare metal provisioning in Kubernetes.

## Architecture

### Core Components

1. **Custom Resources (CRDs)** - Located in `apis/metal3.io/v1alpha1/`:
   - `BareMetalHost` - Primary resource representing a physical host
   - `HostFirmwareSettings` - BIOS/firmware configuration
     (1:1 with BareMetalHost)
   - `FirmwareSchema` - Vendor-specific firmware setting constraints
   - `HardwareData` - Hardware specs from Ironic inspection
   - `PreprovisioningImage` - ISO images for provisioning

2. **Controller** -
   `internal/controller/metal3.io/baremetalhost_controller.go`:
   - Main reconciliation loop driven by state machine pattern
   - State transitions: `unmanaged → registering → inspecting → available →
     provisioning → provisioned`
   - Uses `newHostStateMachine()` to manage state transitions
   - Publishes Kubernetes events for major state changes

3. **Provisioner Interface** - `pkg/provisioner/`:
   - `provisioner.go` defines the `Provisioner` interface for BMC
     interactions
   - `ironic/` - Production implementation using Ironic API (gophercloud)
   - `fixture/` - Test mode implementation for development without Ironic
   - `demo/` - Demo mode for showcasing without real hardware
   - Factory pattern creates appropriate provisioner based on flags

### State Machine

The operator uses explicit state tracking through
`host.Status.Provisioning.State`:

- States are defined as `ProvisioningState` constants in
  `baremetalhost_types.go`
- Each state has associated actions in the controller's state machine
- Failed operations set `host.Status.ErrorMessage` and error conditions
- Annotations control behavior: `baremetalhost.metal3.io/paused`,
  `baremetalhost.metal3.io/detached`

### Configuration

Environment variables (set in `config/default/ironic.env` and referenced in
`main.go`):

- `IRONIC_ENDPOINT` - Ironic API URL (required for production)
- `DEPLOY_KERNEL_URL`, `DEPLOY_RAMDISK_URL` - IPA deployment images
- `BMO_CONCURRENCY` - Number of concurrent reconciles (default: CPU count,
  2-8 range)
- `PROVISIONING_LIMIT` - Max simultaneous (de)provisioning operations
  (default: 20)

## Development Workflows

### Build and Test

```bash
# Generate manifests after API changes - ALWAYS run this after modifying
# config/ or APIs
make manifests

# Run unit tests (includes apis/, pkg/hardwareutils)
make test

# Run linters (golangci-lint across multiple modules)
make lint

# Build operator binary
make manager

# Build container image
IMG=quay.io/metal3-io/baremetal-operator IMG_TAG=dev make docker
```

### Local Development

**Without Ironic (test/fixture mode):**

```bash
make run-test-mode  # Uses -test-mode flag, no real BMC operations
```

**With local Ironic:**

```bash
tools/run_local_ironic.sh  # Starts Ironic in podman containers
make run  # Runs operator locally against k8s cluster
```

**Using Tilt for iterative development:**

- Tilt watches for changes and auto-rebuilds/redeploys
- Configure via `tilt-settings.json`
- Default context: `kind-bmo`

### E2E Tests

Located in `test/e2e/`, using Ginkgo framework:

```bash
# Run main e2e test suite (creates VMs via libvirt, uses VBMC/sushy-tools)
./hack/ci-e2e.sh

# Run specific tests
GINKGO_FOCUS="power cycle" ./hack/ci-e2e.sh

# Run optional tests (upgrades)
GINKGO_FOCUS="upgrade" GINKGO_SKIP="" ./hack/ci-e2e.sh
```

```text

E2E setup creates:


- Kind cluster with exposed ports for Ironic
- Libvirt VMs as BareMetalHosts
- VBMC (for IPMI) or sushy-tools (for Redfish) as BMC emulators
- Configuration in `test/e2e/config/`

## Status Management

- Use Condition types: `ConditionRegistrationError`,
  `ConditionInspectionError`, `ConditionProvisioningError`
- Status changes flow through reconciler.Status() methods
- Never directly update Status fields; use helper methods

## Code Patterns and Conventions


### API Modifications

1. Edit types in `apis/metal3.io/v1alpha1/*_types.go`
2. Run `make manifests` to regenerate CRDs, RBAC, webhooks
3. This updates:

   - `config/base/crds/bases/*.yaml`
   - `config/base/rbac/*.yaml`
   - `config/render/capm3.yaml` (rendered Kustomize output)

### Controller Patterns

**Reconciliation Structure:**


```go
// Controllers use reconcileInfo to pass context
type reconcileInfo struct {
    ctx context.Context
    log logr.Logger
    host *metal3api.BareMetalHost
    bmcCredsSecret *corev1.Secret
    events []corev1.Event
    postSaveCallbacks []func()
}

// State actions return actionResult (from state machine pattern)
// Common returns: actionComplete{}, actionContinue{},
// actionFailed{}, actionDelayed{}
```

```go
// actionDelayed{}
```

**Error Handling:**

- Use `recordActionFailure(info, errorType, message)` for recoverable errors

- Set error types: `RegistrationError`, `InspectionError`,
  `ProvisioningError`, etc.
- Errors are reflected in host status and metrics

**Credentials:**

- BMC credentials resolved from Secrets via
  `buildAndValidateBMCCredentials()`
- Credentials validated through `pkg/hardwareutils/bmc` package
- Special handling for unmanaged/deleting hosts (empty credentials)

### Testing

- Unit tests must pass for `./...`, `apis/`, `pkg/hardwareutils/`,
  `test/`
- Use `gomega` for assertions in tests
- E2E tests use CAPI test framework patterns
- Tag e2e code with `//go:build e2e`

### Multi-Module Structure

The repo uses Go workspaces:

- Main module: `github.com/metal3-io/baremetal-operator`
- `apis/` - Separate module for API types
- `pkg/hardwareutils/` - Separate module for BMC utilities
- Test each module independently during CI

## Key Files Reference

- `main.go` - Entrypoint, flag parsing, manager setup
- `PROJECT` - Kubebuilder project configuration
- `Makefile` - Primary build/test interface
- `config/default/kustomization.yaml` - Default deployment configuration
- `docs/api.md` - API documentation (update when changing APIs)
- `hack/ci-e2e.sh` - E2E test orchestration script

## CI/PR Integration

- E2E tests triggered via `/test metal3-bmo-e2e-test-pull` (required)
- Optional tests via `/test metal3-bmo-e2e-test-optional-pull`
- CAPM3 integration tests verify BMO changes don't break
  cluster-api-provider-metal3
- Sign commits with `-s` flag (DCO required)

## Common Pitfalls and Solutions

- Don't modify status outside reconciler's Status() method
- Use baremetal.SetError() for host errors, not manual StatusError creation
- Always check HasBMCDetails() before accessing BMC credentials

- Use SetOperationalStatus() for state transitions, not direct field writes
