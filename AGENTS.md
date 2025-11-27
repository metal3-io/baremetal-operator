# Metal3 Baremetal Operator (BMO) - AI Coding Agent Instructions

This file provides comprehensive instructions for AI coding agents working on
the Metal3 Baremetal Operator project. It covers architecture, conventions,
tooling, CI/CD, and behavioral guidelines.

## Table of Contents

- [Project Overview](#project-overview)
- [Architecture](#architecture)
- [Development Workflows](#development-workflows)
- [Makefile Reference](#makefile-reference)
- [Hack Scripts Reference](#hack-scripts-reference)
- [CI/CD and GitHub Workflows](#cicd-and-github-workflows)
- [Code Patterns and Conventions](#code-patterns-and-conventions)
- [Testing Guidelines](#testing-guidelines)
- [Integration Points](#integration-points)
- [Common Pitfalls](#common-pitfalls)
- [AI Agent Behavioral Guidelines](#ai-agent-behavioral-guidelines)

---

## Project Overview

The Baremetal Operator (BMO) is a Kubernetes controller that manages physical
bare metal servers through the `BareMetalHost` custom resource. It integrates
tightly with OpenStack Ironic for hardware provisioning, inspection, and
lifecycle management. BMO is the core component of the Metal³ project for
bare metal provisioning in Kubernetes environments.

**Key URLs:**

- Repository: <https://github.com/metal3-io/baremetal-operator>
- Container Image: `quay.io/metal3-io/baremetal-operator`
- Documentation: <https://book.metal3.io/bmo/introduction>

### Project Goals

BMO aims to provide a Kubernetes-native interface for managing bare metal
infrastructure:

1. **Hardware Lifecycle Management** - Full lifecycle control of physical
   servers from discovery through decommissioning, using a declarative API.

2. **Ironic Integration** - Tight coupling with OpenStack Ironic for hardware
   provisioning via IPA (Ironic Python Agent). Supports Ironic Standalone
   Operator (IRSO) for deployment.

3. **Multi-Protocol BMC Support** - Supports multiple BMC protocols:
   - IPMI (via virtualbmc for testing)
   - Redfish (via sushy-tools for testing)
   - Redfish Virtual Media
   - iDRAC, iRMC, and other vendor-specific implementations

4. **CAPI Integration** - Works with Cluster API Provider Metal3 (CAPM3) to
   enable Kubernetes cluster provisioning on bare metal.

5. **Production Ready** - Mature API (v1alpha1, moving to v1beta1) with
   validation webhooks, status conditions, and comprehensive metrics.

---

## Architecture

### Core Components

1. **Custom Resources (CRDs)** - Located in `apis/metal3.io/v1alpha1/`:
   - `BareMetalHost` - Primary resource representing a physical host
   - `HostFirmwareSettings` - BIOS/firmware configuration (1:1 with BMH)
   - `HostFirmwareComponents` - Firmware component versions
   - `FirmwareSchema` - Vendor-specific firmware setting constraints
   - `HardwareData` - Hardware specs from Ironic inspection
   - `PreprovisioningImage` - ISO/ramdisk images for provisioning
   - `DataImage` - Custom data images to attach to hosts
   - `BMCEventSubscription` - BMC event subscription management
   - `HostUpdatePolicy` - Controls firmware update behavior

2. **Controllers** - Located in `internal/controller/metal3.io/`:
   - `BareMetalHostReconciler` - Main reconciliation loop with state machine
   - `HostFirmwareSettingsReconciler` - BIOS configuration management
   - `HostFirmwareComponentsReconciler` - Firmware updates
   - `PreprovisioningImageReconciler` - Image lifecycle
   - `DataImageReconciler` - Data image attachment
   - `BMCEventSubscriptionReconciler` - Event subscription handling

3. **Provisioner Interface** - Located in `pkg/provisioner/`:
   - `provisioner.go` - Defines the `Provisioner` interface
   - `ironic/` - Production implementation using gophercloud
   - `fixture/` - Test mode (no real BMC operations)
   - `demo/` - Demo mode for showcasing workflows
   - Factory pattern selects provisioner based on flags

4. **Webhooks** - Located in `internal/webhooks/metal3.io/v1alpha1/`:
   - Validation webhooks for BareMetalHost, BMCEventSubscription
   - Immutability checks for critical fields
   - BMC address and credential validation

5. **Hardware Utilities** - Located in `pkg/hardwareutils/bmc/`:
   - BMC protocol detection and parsing
   - Credential validation
   - Driver-specific implementations (IPMI, Redfish, iDRAC, iRMC)

### State Machine

The operator uses a finite state machine for host lifecycle management.
States are tracked in `host.Status.Provisioning.State`:

```text
┌──────────────────────────────────────────────────────────────────────────┐
│                           State Transitions                               │
├──────────────────────────────────────────────────────────────────────────┤
│  (none) ──► unmanaged ──► registering ──► inspecting ──► preparing       │
│                                                              │            │
│                                                              ▼            │
│  deleting ◄── powering off before delete ◄── deprovisioning ◄── provisioned │
│                                                              ▲            │
│                                                              │            │
│                                         provisioning ◄── available       │
│                                                                          │
│  externally provisioned (separate path, not managed by BMO)              │
└──────────────────────────────────────────────────────────────────────────┘
```

**State Definitions:**

| State | Description |
|-------|-------------|
| `unmanaged` | Insufficient info to register (missing BMC details) |
| `registering` | Registering host with Ironic |
| `inspecting` | Running hardware inspection via IPA |
| `preparing` | Applying RAID/firmware configuration |
| `available` | Ready for provisioning |
| `provisioning` | Writing image to disk |
| `provisioned` | Image deployed, host in use |
| `deprovisioning` | Removing image from disk |
| `powering off before delete` | Powering off before cleanup |
| `deleting` | Cleaning up Ironic resources |
| `externally provisioned` | Managed externally, BMO only monitors |

**Key Annotations:**

- `baremetalhost.metal3.io/paused` - Pauses reconciliation
- `baremetalhost.metal3.io/detached` - Detaches from provisioner
- `baremetalhost.metal3.io/status` - Status backup for pivot operations
- `inspect.metal3.io` - Controls inspection behavior
- `reboot.metal3.io` - Triggers reboots (hard/soft)

### Resource Relationships

```text
BareMetalHost (physical server representation)
    ├── Secret (BMC credentials)
    ├── HostFirmwareSettings (BIOS config, 1:1)
    ├── HostFirmwareComponents (firmware versions, 1:1)
    ├── HardwareData (inspection results, 1:1)
    ├── PreprovisioningImage (boot image)
    ├── DataImage (optional data attachment)
    └── BMCEventSubscription (event notifications)

Ironic (via IRSO)
    └── Manages actual hardware operations
```

### Directory Structure

```text
baremetal-operator/
├── apis/metal3.io/v1alpha1/  # CRD type definitions (separate Go module)
├── cmd/                       # CLI tools (get-hardware-details, make-bm-worker)
├── config/                    # Kustomize manifests
│   ├── base/                 # Base CRDs, RBAC, webhooks
│   ├── default/              # Default deployment with Ironic env
│   ├── components/           # Optional components
│   ├── overlays/             # Environment-specific overlays
│   ├── render/               # Pre-rendered manifests
│   └── samples/              # Example resources
├── docs/                      # Documentation and diagrams
├── hack/                      # Build, test, and CI scripts
│   ├── e2e/                  # E2E test setup scripts
│   └── tools/                # Tool dependencies (separate Go module)
├── internal/
│   ├── controller/metal3.io/ # Controller implementations
│   └── webhooks/metal3.io/   # Webhook implementations
├── ironic-deployment/         # Ironic deployment manifests
├── pkg/
│   ├── hardwareutils/        # BMC utilities (separate Go module)
│   ├── imageprovider/        # Image URL handling
│   ├── provisioner/          # Provisioner interface and implementations
│   ├── secretutils/          # Secret handling utilities
│   ├── utils/                # General utilities
│   └── version/              # Version information
├── test/
│   ├── e2e/                  # E2E tests (Ginkgo)
│   └── vbmctl/               # VM creation tool for e2e
└── tools/                     # Helper scripts
```

### Configuration

Environment variables (set in `config/default/ironic.env` and `main.go`):

| Variable | Description | Default |
|----------|-------------|---------|
| `IRONIC_ENDPOINT` | Ironic API URL | Required for production |
| `DEPLOY_KERNEL_URL` | IPA kernel URL | Required |
| `DEPLOY_RAMDISK_URL` | IPA ramdisk URL | Required |
| `IRONIC_USERNAME` | Ironic auth username | Optional |
| `IRONIC_PASSWORD` | Ironic auth password | Optional |
| `WATCH_NAMESPACE` | Namespace(s) to watch | All (cluster-scoped) |
| `POD_NAMESPACE` | Pod's namespace | Auto-detected |
| `IRONIC_NAME` | IRSO Ironic CR name | Optional |
| `IRONIC_NAMESPACE` | IRSO Ironic CR namespace | Optional |

**Controller Flags:**

| Flag | Description | Default |
|------|-------------|---------|
| `-namespace` | Namespace to watch | All |
| `-metrics-addr` | Metrics endpoint | `:8443` |
| `-health-addr` | Health probe endpoint | `:9440` |
| `-webhook-port` | Webhook server port (0 disables) | `9443` |
| `-test-mode` | Disable Ironic communication | `false` |
| `-demo-mode` | Use demo provisioner | `false` |
| `-build-preprov-image` | Enable PreprovisioningImage API | `false` |
| `-controller-concurrency` | Concurrent reconciles per CR type | Auto |
| `-kube-api-qps` | K8s API QPS limit | `20` |
| `-kube-api-burst` | K8s API burst limit | `30` |

---

## Development Workflows

### Quick Start Commands

```bash
# Full verification (generate + lint + test)
make test

# Generate code and manifests after API changes
make generate manifests

# Run unit tests only
make unit

# Run linters only
make lint

# Build manager binary
make manager

# Build all binaries (manager + tools)
make build

# Verify go modules are tidy
make mod
```

### Local Development

**Without Ironic (test/fixture mode):**

```bash
make run-test-mode  # Uses -test-mode flag, no real BMC operations
```

**Demo mode (simulated state transitions):**

```bash
make demo  # Uses -demo-mode flag
```

**With local Ironic (requires podman):**

```bash
tools/run_local_ironic.sh  # Starts Ironic containers
make run                    # Runs operator locally
```

### Local Development with Tilt

This repository supports Tilt for iterative development with live reload:

```bash
make kind-create  # Create kind cluster with registry
make tilt-up      # Start Tilt (watches for changes)
```

Configuration via `tilt-settings.json`:

```json
{
  "default_registry": "localhost:5000",
  "deploy_cert_manager": true,
  "deploy_ironic": true
}
```

### Docker Build

```bash
# Build container image
make docker IMG=quay.io/metal3-io/baremetal-operator IMG_TAG=dev

# Build with debug symbols
make docker-debug IMG=quay.io/metal3-io/baremetal-operator IMG_TAG=debug
```

---

## Makefile Reference

### Testing Targets

| Target | Description |
|--------|-------------|
| `make test` | Run generate + lint + manifests + unit |
| `make unit` | Run unit tests for all modules |
| `make unit-cover` | Run unit tests with coverage report |
| `make unit-verbose` | Run unit tests with verbose output |
| `make test-e2e` | Run E2E tests (requires setup) |

### Build Targets

| Target | Description |
|--------|-------------|
| `make build` | Build everything (manager + tools + e2e) |
| `make manager` | Build manager binary to `bin/baremetal-operator` |
| `make tools` | Build CLI tools (get-hardware-details, etc.) |
| `make build-e2e` | Build E2E test binary |
| `make build-vbmctl` | Build VM creation tool for E2E |

### Code Generation Targets

| Target | Description |
|--------|-------------|
| `make generate` | Generate Go code (deepcopy functions) |
| `make manifests` | Generate CRDs, RBAC, webhooks, and render |
| `make manifests-generate` | Generate CRDs and RBAC only |
| `make manifests-kustomize` | Render Kustomize output to `config/render/` |

### Linting Targets

| Target | Description |
|--------|-------------|
| `make lint` | Run golangci-lint on all modules |
| `make lint-fix` | Run linter with auto-fix |
| `make linters` | Run lint + generate-check |
| `make manifest-lint` | Validate Kubernetes manifests |

### Verification Targets

| Target | Description |
|--------|-------------|
| `make generate-check` | Verify generated code is up-to-date |
| `make generate-check-local` | Same as above, run locally |

### Module Management

| Target | Description |
|--------|-------------|
| `make mod` | Run go mod tidy/verify on all modules |

### Deployment Targets

| Target | Description |
|--------|-------------|
| `make install` | Install CRDs into cluster |
| `make uninstall` | Remove CRDs from cluster |
| `make deploy` | Deploy controller to cluster |
| `make run` | Run controller locally against cluster |
| `make run-test-mode` | Run controller in test mode |
| `make demo` | Run controller in demo mode |

### Release Targets

| Target | Description |
|--------|-------------|
| `make release` | Full release (requires RELEASE_TAG) |
| `make release-manifests` | Build release manifests to `out/` |
| `make release-notes` | Generate release notes |

### Tilt/Kind Targets

| Target | Description |
|--------|-------------|
| `make kind-create` | Create kind cluster with registry |
| `make kind-reset` | Delete kind cluster |
| `make tilt-up` | Start Tilt development environment |

### Cleanup Targets

| Target | Description |
|--------|-------------|
| `make clean` | Remove temp files and tools |
| `make clean-e2e` | Clean E2E test artifacts |

### Utility Targets

| Target | Description |
|--------|-------------|
| `make go-version` | Print Go version used |
| `make help` | Display all available targets |
| `make docs` | Generate documentation diagrams |

---

## Hack Scripts Reference

Scripts in `hack/` support both local and containerized execution.
**For local development, prefer Make targets** which handle setup.

### CI Scripts

| Script | Purpose | Make Target |
|--------|---------|-------------|
| `generate.sh` | Verify generated code | `make generate-check` |
| `gomod.sh` | Verify go.mod is tidy | `make mod` |
| `ci-e2e.sh` | Run E2E test suite | `make test-e2e` |
| `clean-e2e.sh` | Clean E2E artifacts | `make clean-e2e` |

### Linting Scripts (containerized)

| Script | Purpose |
|--------|---------|
| `shellcheck.sh` | Shell script linting |
| `markdownlint.sh` | Markdown linting |
| `manifestlint.sh` | Kubernetes manifest validation |

Run directly: `./hack/shellcheck.sh`

### E2E Setup Scripts (in `hack/e2e/`)

| Script | Purpose |
|--------|---------|
| `ensure_go.sh` | Install correct Go version |
| `ensure_kubectl.sh` | Install kubectl |
| `ensure_yq.sh` | Install yq |
| `ensure_htpasswd.sh` | Install htpasswd |

### Container Execution Pattern

Scripts support containerized execution:

```bash
IS_CONTAINER="${IS_CONTAINER:-false}"
CONTAINER_RUNTIME="${CONTAINER_RUNTIME:-podman}"

if [ "${IS_CONTAINER}" != "false" ]; then
    # Run the actual logic
else
    "${CONTAINER_RUNTIME}" run --rm \
        --env IS_CONTAINER=TRUE \
        --volume "${PWD}:/workdir:ro,z" \
        <image> \
        /workdir/hack/<script>.sh "$@"
fi
```

---

## CI/CD and GitHub Workflows

BMO uses a **dual CI system**:

1. **GitHub Actions** - Primary CI for mandatory checks (linting, unit tests,
   BMO-specific E2E tests)
2. **Prow + Jenkins** - Optional CAPM3 integration tests via project-infra

Workflows are defined in `.github/workflows/` and Prow jobs in
[project-infra](https://github.com/metal3-io/project-infra/blob/main/prow/config/jobs/metal3-io/baremetal-operator.yaml).

### GitHub Actions Pipeline (`pipeline.yml`)

Triggered on PRs to `main` and `release-*` branches:

```text
┌─────────────────┐     ┌─────────────────┐
│  golangci-lint  │     │      unit       │
└────────┬────────┘     └────────┬────────┘
         │                       │
         └───────────┬───────────┘
                     ▼
         ┌───────────────────────┐
         │   e2e-fixture-test    │
         └───────────┬───────────┘
                     │
         ┌───────────┴───────────┐
         ▼                       ▼
┌─────────────────┐     ┌─────────────────┐
│ e2e-test (rfvm) │     │ e2e-test (ipmi) │
└─────────────────┘     └─────────────────┘
```

### GitHub Actions Workflow Files

| Workflow | Trigger | Description |
|----------|---------|-------------|
| `pipeline.yml` | PR to main/release-* | Main CI pipeline |
| `unit.yml` | Called by pipeline | Unit tests |
| `golangci-lint.yml` | Called by pipeline | Go linting |
| `e2e-fixture-test.yml` | Called by pipeline | E2E with fixture provisioner |
| `e2e-test.yml` | Called by pipeline | Full E2E with libvirt VMs |
| `e2e-test-periodic-*.yml` | Scheduled | Periodic E2E for branches |
| `e2e-test-optional-periodic.yml` | Scheduled | Optional tests (upgrades) |
| `release.yaml` | Tag push | Release automation |

### Prow Jobs (project-infra)

Prow jobs run containerized checks and trigger Jenkins for E2E tests.
Jobs are defined in `prow/config/jobs/metal3-io/baremetal-operator.yaml`.

**Presubmit Jobs (automatic on PRs):**

| Job | Description | Skip Conditions |
|-----|-------------|-----------------|
| `gomod` | Verify go.mod is tidy | OWNERS, *.md changes |
| `generate` | Verify generated code | OWNERS, *.md changes |
| `manifestlint` | Kubernetes manifest validation | OWNERS, *.md changes |
| `shellcheck` | Shell script linting | Only runs on *.sh, Makefile |
| `markdownlint` | Markdown linting | Only runs on *.md |
| `test` | Makefile/hack script verification | Only runs on Makefile, hack/* |

**Optional Jenkins E2E Tests (triggered via `/test` comments):**

| Job | Trigger |
|-----|---------|
| `metal3-centos-e2e-basic-test-main` | `/test <job-name>` |
| `metal3-ubuntu-e2e-basic-test-main` | `/test <job-name>` |
| `metal3-centos-e2e-integration-test-main` | `/test <job-name>` |
| `metal3-ubuntu-e2e-integration-test-main` | `/test <job-name>` |
| `metal3-*-e2e-feature-test-main-pivoting` | `/test <job-name>` |
| `metal3-*-e2e-feature-test-main-remediation` | `/test <job-name>` |
| `metal3-*-e2e-feature-test-main-features` | `/test <job-name>` |
| `metal3-e2e-clusterctl-upgrade-test-main` | `/test <job-name>` |
| `metal3-e2e-*-upgrade-test-main` | `/test <job-name>` |
| `metal3-bmo-e2e-test-optional-pull` | `/test <job-name>` |
| `metal3-dev-env-integration-test-*-main` | `/test <job-name>` |

**Note:** All Jenkins E2E jobs are `optional: true` and `always_run: false`.
They test CAPM3 integration and must be triggered manually with `/test`.

### E2E Test Infrastructure

E2E tests create a complete bare metal simulation:

1. **Kind Cluster** - With exposed ports for Ironic services
2. **Libvirt VMs** - Act as BareMetalHosts (created by `vbmctl`)
3. **BMC Emulators**:
   - VBMC (VirtualBMC) for IPMI protocol
   - sushy-tools for Redfish protocols
4. **Image Server** - Nginx serving test images (cirros, systemrescue)
5. **Ironic** - Deployed via IRSO (Ironic Standalone Operator)

**BMC Protocol Selection:**

```bash
# Environment variable selects protocol
export BMC_PROTOCOL="redfish-virtualmedia"  # or "redfish" or "ipmi"
./hack/ci-e2e.sh
```

### E2E Test Categories

| Test Suite | Description | Required |
|------------|-------------|----------|
| Basic operations | Provision, deprovision, power cycle | Yes |
| Inspection | Hardware discovery | Yes |
| Live ISO | Boot from ISO image | Yes |
| Externally provisioned | External management | Yes |
| Automated cleaning | Disk cleaning | Yes |
| Upgrade tests | BMO/Ironic upgrades | Optional |

### Skip Patterns

GitHub Actions pipeline skips for documentation-only changes:

- `**/*.md`, `docs/**`
- `LICENSE`, `DCO`, `OWNERS`
- `.gitignore`, `SECURITY_CONTACTS`

---

## Code Patterns and Conventions

### Go Code Style

Go code is formatted with `gofmt`. The linter (golangci-lint v2) enforces
rules defined in `.golangci.yaml`. Key conventions:

- Go version: 1.24 (see `Makefile` `GO_VERSION`)
- Import aliasing enforced:
   - `corev1` for `k8s.io/api/core/v1`
   - `metav1` for `k8s.io/apimachinery/pkg/apis/meta/v1`
   - `metal3api` for BMO APIs
   - `ctrl` for `sigs.k8s.io/controller-runtime`
- License headers required (see `hack/boilerplate.go.txt`)
- Run `make lint` to check, `make lint-fix` to auto-fix

### YAML Conventions

- Kubernetes manifests: 2-space indentation
- Kustomize overlays in `config/overlays/`

### Shell Script Conventions

**Required settings at script start:**

```bash
#!/usr/bin/env bash
set -eux
# or
set -o errexit
set -o nounset
set -o pipefail
```

### Markdown Conventions

Configuration in `.markdownlint-cli2.yaml`:

- Unordered list indent: 3 spaces
- No auto-fix (lint only)

### API Modifications Workflow

1. Edit types in `apis/metal3.io/v1alpha1/*_types.go`
2. Run `make generate` to regenerate deepcopy functions
3. Run `make manifests` to regenerate:
   - CRDs in `config/base/crds/bases/`
   - RBAC in `config/base/rbac/`
   - Webhooks in `config/base/webhook/`
   - Rendered output in `config/render/capm3.yaml`
4. Update webhooks if validation changes
5. Update `docs/api.md` if behavior changes
6. Run `make test` to verify

### Controller Patterns

**State Machine Structure:**

```go
// hostStateMachine manages state transitions
type hostStateMachine struct {
    Host        *metal3api.BareMetalHost
    NextState   metal3api.ProvisioningState
    Reconciler  *BareMetalHostReconciler
    Provisioner provisioner.Provisioner
    haveCreds   bool
}

// State handlers map states to actions
func (hsm *hostStateMachine) handlers() stateHandlerMap {
    return stateHandlerMap{
        metal3api.StateNone:        hsm.handleNone,
        metal3api.StateRegistering: hsm.handleRegistering,
        // ... other states
    }
}
```

**Reconciliation Context:**

```go
// reconcileInfo carries context through reconciliation
type reconcileInfo struct {
    ctx                context.Context
    log                logr.Logger
    host               *metal3api.BareMetalHost
    bmcCredsSecret     *corev1.Secret
    events             []corev1.Event
    postSaveCallbacks  []func()
}
```

**Action Results:**

```go
// State actions return actionResult
type actionResult interface {
    // ...
}

// Common results:
actionComplete{}   // State complete, move to next
actionContinue{}   // Requeue immediately
actionFailed{}     // Error occurred
actionDelayed{}    // Capacity limit, delay
```

**Error Handling:**

```go
// Use recordActionFailure for recoverable errors
recordActionFailure(info, metal3api.RegistrationError, "failed to register host")

// Error types:
// - RegistrationError
// - InspectionError
// - PreparationError
// - ProvisioningError
// - PowerManagementError
// - DetachError
// - ServicingError
```

**Credentials Handling:**

```go
// BMC credentials resolved from Secrets
creds, err := r.buildAndValidateBMCCredentials(info, host)
if err != nil {
    // Handle missing/invalid credentials
}

// Validate through bmc package
accessDetails, err := bmc.NewAccessDetails(host.Spec.BMC.Address, creds)
```

### Provisioner Interface

```go
type Provisioner interface {
    // Registration
    ValidateManagementAccess(data ManagementAccessData, ...) (result Result, ...)
    
    // Inspection
    InspectHardware(data InspectData, ...) (result Result, ...)
    
    // Provisioning
    Provision(data ProvisionData, ...) (result Result, ...)
    Deprovision(...) (result Result, ...)
    
    // Power management
    PowerOn(...) (result Result, ...)
    PowerOff(...) (result Result, ...)
    
    // Capacity management
    HasCapacity() (bool, error)
}
```

### Multi-Module Structure

The repo uses Go workspaces with separate modules:

| Module | Path | Purpose |
|--------|------|---------|
| Main | `/` | Operator and controllers |
| APIs | `apis/` | CRD type definitions |
| Hardware Utils | `pkg/hardwareutils/` | BMC protocol handling |
| Tools | `hack/tools/` | Build tool dependencies |
| E2E Tests | `test/` | E2E test code |

Each module is linted and tested independently:

```bash
# Lint all modules
make lint  # Runs golangci-lint on /, apis/, test/, pkg/hardwareutils/, hack/tools/

# Test all modules
make unit  # Tests ./..., apis/..., pkg/hardwareutils/...
```

---

## Testing Guidelines

### Unit Test Framework

Tests use standard Go testing + Gomega matchers:

```go
func TestSomething(t *testing.T) {
    g := NewWithT(t)
    
    result := doSomething()
    
    g.Expect(result).To(Equal(expected))
    g.Expect(err).ToNot(HaveOccurred())
}
```

Controller tests use envtest for a real API server:

```go
var _ = BeforeSuite(func() {
    testEnv = &envtest.Environment{
        CRDDirectoryPaths: []string{
            filepath.Join("..", "..", "..", "config", "base", "crds", "bases"),
        },
    }
    cfg, err := testEnv.Start()
    // ...
})
```

### Running Tests

```bash
# All unit tests (recommended)
make unit

# With coverage
make unit-cover

# Verbose output
make unit-verbose

# Specific package
go test ./pkg/provisioner/ironic/... -v
```

### E2E Test Framework

E2E tests use Ginkgo and the CAPI test framework:

```go
//go:build e2e

var _ = Describe("Basic Operations", func() {
    Context("when provisioning a host", func() {
        It("should complete successfully", func() {
            // Test implementation
        })
    })
})
```

**Running E2E tests:**

```bash
# Full E2E suite (requires libvirt, docker)
./hack/ci-e2e.sh

# Specific tests
GINKGO_FOCUS="inspection" ./hack/ci-e2e.sh

# Skip upgrade tests (default)
GINKGO_SKIP="upgrade" ./hack/ci-e2e.sh

# Run upgrade tests only
GINKGO_FOCUS="upgrade" GINKGO_SKIP="" ./hack/ci-e2e.sh
```

**E2E Configuration Files** (in `test/e2e/config/`):

| File | Purpose |
|------|---------|
| `ironic.yaml` | Main E2E config with Ironic |
| `fixture.yaml` | Fixture mode config |
| `bmcs-ipmi.yaml` | IPMI BMC definitions |
| `bmcs-redfish.yaml` | Redfish BMC definitions |
| `bmcs-redfish-virtualmedia.yaml` | Redfish VM BMC definitions |

---

## Integration Points

### With Ironic Standalone Operator (IRSO)

BMO integrates with IRSO for Ironic lifecycle management:

```yaml
# BMO watches Ironic CR for endpoint info
env:
- name: IRONIC_NAME
  value: "ironic"
- name: IRONIC_NAMESPACE
  value: "baremetal-operator-system"
```

IRSO deploys and manages:

- Ironic API service
- Ironic conductor
- IPA (Ironic Python Agent) images
- Optional MariaDB, keepalived

### With CAPM3 (Cluster API Provider Metal3)

CAPM3 creates BareMetalHost resources and manages:

- Metal3Machine to BareMetalHost binding
- Network data via Metal3DataTemplate
- IP allocation via Metal3 IPAM

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: Metal3Machine
spec:
  hostSelector:
    matchLabels:
      cluster.x-k8s.io/cluster-name: my-cluster
```

### E2E Testing with CAPM3

**Important:** BMO has its own standalone E2E tests using GitHub Actions.
CAPM3 also runs integration tests that exercise BMO.

BMO E2E tests:

1. Create libvirt VMs using `vbmctl`
2. Start BMC emulators (VBMC or sushy-tools)
3. Deploy Ironic via IRSO
4. Test BareMetalHost lifecycle operations

---

## Common Pitfalls

1. **Status Updates** - Never modify status fields directly; use controller
   helper methods and the state machine pattern.

2. **Credentials Validation** - Always check `HasBMCDetails()` before
   accessing BMC credentials to handle unmanaged hosts.

3. **Generate After API Changes** - Always run `make generate manifests`
   after modifying `apis/metal3.io/v1alpha1/*_types.go`.

4. **Multi-Module Linting** - Run `make lint` (not just `golangci-lint run`)
   to lint all modules.

5. **E2E Test Dependencies** - E2E tests require libvirt, docker, and
   specific kernel modules. Use CI for reliable E2E testing.

6. **Provisioner Selection** - Test mode (`-test-mode`) bypasses Ironic
   entirely. Use for unit tests, not integration testing.

7. **State Machine Transitions** - State changes must go through
   `hostStateMachine`. Direct status field updates break the workflow.

8. **Annotation Handling** - Check for pause/detach annotations before
   performing operations that modify host state.

---

## AI Agent Behavioral Guidelines

### Critical Rules

1. **Run verification first** - Before making changes, run `make lint` and
   `make unit` to understand the baseline state.

2. **Be strategic with output filtering** - Use `head`, `tail`, or `grep`
   for large outputs, but prefer full output for smaller results.

3. **Challenge assumptions** - Verify claims against code. If you have
   evidence against an assumption, present it with references.

4. **Security first** - Review code for:
   - Hardcoded credentials
   - Insecure defaults
   - Missing input validation
   - Privilege escalation risks

5. **Pin dependencies by SHA** - Container images, GitHub Actions, and
   downloaded binaries should be SHA-pinned for security.

6. **Follow existing conventions** - Match the style of existing code:
   - Shell scripts: use patterns from `hack/` scripts
   - Go code: follow `.golangci.yaml` rules
   - Markdown: follow `.markdownlint-cli2.yaml` rules
   - License headers: use `hack/boilerplate.go.txt`

### Before Making Changes

1. Run `make lint` to understand current linting rules
2. Run `make unit` to verify baseline test status
3. Check existing patterns in similar files
4. Verify Go version matches `Makefile` (currently 1.24)

### When Modifying Code

1. Make minimal, surgical changes
2. Run `make generate manifests` after API changes
3. Run `make test` before submitting
4. Update `docs/api.md` if API behavior changes
5. Add tests for new functionality

### When Debugging CI Failures

1. Check GitHub Actions workflow definitions in `.github/workflows/`
2. Run the same hack script locally with `IS_CONTAINER=TRUE`
3. Use the exact container images from CI
4. Check if failure is flaky vs consistent
5. Review E2E logs in uploaded artifacts

### Commit Guidelines

- Sign commits with `-s` flag (DCO required)
- Use conventional commit prefixes:
   - ✨ `:sparkles:` - New feature
   - 🐛 `:bug:` - Bug fix
   - 📖 `:book:` - Documentation
   - 🌱 `:seedling:` - Other changes
   - ⚠️ `:warning:` - Breaking changes

---

## Git and Release Information

- **Branches**: `main` (development), `release-X.Y` (stable releases)
- **DCO Required**: All commits must be signed off (`git commit -s`)
- **PR Labels**: ⚠️ breaking, ✨ feature, 🐛 bug, 📖 docs, 🌱 other
- **Release Process**: See `docs/releasing.md`

---

## Additional Resources

### Related Projects

- [IRSO](https://github.com/metal3-io/ironic-standalone-operator) - Ironic
  Standalone Operator (manages Ironic deployment)
- [CAPM3](https://github.com/metal3-io/cluster-api-provider-metal3) - Cluster
  API Provider Metal3 (Kubernetes cluster provisioning)
- [IPAM](https://github.com/metal3-io/ip-address-manager) - IP Address Manager
- [Metal3 Docs](https://book.metal3.io) - Project documentation
- [Ironic](https://docs.openstack.org/ironic/latest/) - OpenStack Ironic

### Documentation

- `docs/api.md` - API reference
- `docs/configuration.md` - Configuration options
- `docs/dev-setup.md` - Development environment setup
- `docs/testing.md` - Testing guide
- `docs/baremetalhost-states.md` - State machine documentation

### Issue Tracking

- Issues: <https://github.com/metal3-io/baremetal-operator/issues>
- Good first issues: [good first issue label](https://github.com/metal3-io/baremetal-operator/issues?q=is%3Aopen+is%3Aissue+label%3A%22good+first+issue%22)

---

## Quick Reference Card

```bash
# Most common commands
make test              # Full verification
make unit              # Unit tests only
make lint              # Linting only
make generate          # Regenerate Go code
make manifests         # Regenerate CRDs/RBAC/webhooks
make mod               # Tidy go modules
make manager           # Build binary
make docker            # Build container

# Local development
make kind-create       # Create test cluster
make install           # Deploy CRDs
make run               # Run controller locally
make run-test-mode     # Run without Ironic
make demo              # Run in demo mode
make tilt-up           # Start Tilt dev environment

# E2E testing
./hack/ci-e2e.sh                          # Full E2E
GINKGO_FOCUS="inspection" ./hack/ci-e2e.sh # Specific test
BMC_PROTOCOL=ipmi ./hack/ci-e2e.sh        # IPMI protocol

# Hack scripts (containerized)
./hack/shellcheck.sh   # Shell linting
./hack/markdownlint.sh # Markdown linting
./hack/manifestlint.sh # K8s manifest linting
./hack/generate.sh     # Verify codegen
./hack/gomod.sh        # Verify modules

# Release verification
RELEASE_TAG=v0.x.y make release
```
