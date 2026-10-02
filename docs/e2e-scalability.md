# E2E Scalability Tests

The scalability tests measure BMO and Ironic performance under load. They live in
`test/e2e/scalability_test.go` and are labeled `scalability`.
The tests are skipped by default in the CI.

## Tests

| Test | What it measures | Requires real Ironic |
|------|-----------------|---------------------|
| Enrollment | Time to register N hosts (BMH → available) | No (works with fixture) |
| Provisioning | Time to provision N hosts (available → provisioned) | No (works with fixture) |
| API Latency | Ironic API response time as node count grows | Yes |

## Running

### Enrollment and Provisioning (fixture, no Ironic)

```bash
SCALABILITY_NUM_HOSTS=100 \
SCALABILITY_MAX_CONCURRENT_RECONCILES=5 \
GINKGO_FOCUS="Scalability" \
GINKGO_SKIP_LABELS="" \
GINKGO_NODES=1 \
make test-e2e
```

### API Latency (requires real Ironic)

```bash
SCALABILITY_API_BATCH_SIZE=100 \
SCALABILITY_API_MAX_HOSTS=5000 \
SCALABILITY_API_THRESHOLD_MS=30000 \
SCALABILITY_API_MEASUREMENTS=10 \
GINKGO_SKIP_LABELS="none" \
GINKGO_FOCUS="API response times" \
GINKGO_NODES=1 \
./hack/ci-e2e.sh
```

## Configuration

### Enrollment / Provisioning

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `SCALABILITY_NUM_HOSTS` | 10 | Number of BMH resources to create |
| `SCALABILITY_MAX_CONCURRENT_RECONCILES` | 3 | BMO controller concurrency |

### API Latency

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `SCALABILITY_API_BATCH_SIZE` | 10 | Nodes added per measurement iteration |
| `SCALABILITY_API_MAX_HOSTS` | 100 | Maximum nodes to register in Ironic |
| `SCALABILITY_API_THRESHOLD_MS` | 5000 | Fail if any API call exceeds this (ms) |
| `SCALABILITY_API_MEASUREMENTS` | 5 | API calls per measurement point |

## Notes

- All scalability tests are `Serial` — always use `GINKGO_NODES=1`.
- The scalability label is skipped by default. Override with
   `GINKGO_SKIP_LABELS=""` (for make) or
   `GINKGO_SKIP_LABELS="none"` (for ci-e2e.sh).
- The API latency test creates nodes directly in Ironic using the
   `fake-hardware` driver. It does not need VMs or a BMC emulator
   for the nodes it creates.
