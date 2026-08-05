# BMO Scalability Tests

Measures how many BareMetalHost resources BMO can enroll and provision
within a given time window.

## Quick Start

```bash

# 50 hosts, enrollment only (fastest)
NUM_HOSTS=50 SKIP_PROVISIONING=true ./hack/scalability-tests/run.sh
# 100 hosts, enrollment + provisioning
NUM_HOSTS=100 ./hack/scalability-tests/run.sh
```

## Prerequisites

- `docker`
- `kind`
- `kubectl`
- `go` (1.22+)
- `jq`, `curl`, `bc`

## Architecture

The test uses the **fixture provisioner** by default. This means:

- No Ironic process runs
- Registration, inspection, and provisioning are simulated in-memory

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `NUM_HOSTS` | 50 | Number of BMH resources to create |
| `SKIP_PROVISIONING` | false | Only measure enrollment (skip provisioning phase) |
| `MAX_CONCURRENT_RECONCILES` | 3 | BMO controller worker count |
| `ENROLLMENT_TIMEOUT` | 600 | Seconds to wait for all hosts to reach available |
| `PROVISIONING_TIMEOUT` | 900 | Seconds to wait for all hosts to reach provisioned |
| `POLL_INTERVAL` | 2 | Seconds between state checks |
| `TEST_NAMESPACE` | scalability-test | Namespace for test resources |
| `USE_EXISTING_CLUSTER` | false | Use current kubectl context instead of creating Kind |
| `CLUSTER_NAME` | bmo-scalability | Kind cluster name |
| `BMC_ADDRESS` | 192.168.222.1 | IP for sushy-tools BMC emulator |
| `SUSHY_TOOLS_PORT` | 8000 | Port for sushy-tools |
| `DEPLOY_BMO` | true | Set to false to skip BMO deployment |
| `BMO_IMAGE_TAG` | e2e | Tag for the locally built BMO image |

## Output

Results are written to `hack/scalability-tests/_artifacts/`:

- `results.json` — throughput and timing data (machine-readable)
- `timing-enrollment.csv` — per-host enrollment timestamps
- `timing-provisioning.csv` — per-host provisioning timestamps
- `bmo-controller.log` — controller logs for debugging
- `bmh-manifests.yaml` — the generated BMH manifests

## Cleanup

The script cleans up automatically on exit (trap). To force cleanup:

```bash
docker rm -f scalability-sushy-tools
kind delete cluster --name bmo-scalability
```
