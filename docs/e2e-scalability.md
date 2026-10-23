# E2E Scalability Tests

The scalability tests measure BMO and Ironic performance under load. They live in
`test/e2e/scalability_test.go` and are labeled `scalability`.
The tests are skipped by default in the CI.

## Tests

| Test | What it measures | Requires real Ironic |
|------|-----------------|---------------------|
| Enrollment | Time to register N hosts (BMH → available) | No (works with fixture) |
| Provisioning | Time to provision N hosts (available → provisioned) | No (works with fixture) |

When Ironic is deployed (`DEPLOY_IRONIC=true`), both tests also measure the
Ironic API latency with their hosts registered. See
[Ironic API latency](#ironic-api-latency) and the limitation described under
[Ironic](#ironic).

## Running

Use `BMO_KUSTOMIZATION` to deploy BMO with the scalability settings. See
[BMO deployment settings](#bmo-deployment-settings).

### Fixture (no Ironic)

```bash
BMO_KUSTOMIZATION=../../config/overlays/fixture-scalability \
SCALABILITY_NUM_HOSTS=100 \
GINKGO_LABEL_FILTER="scalability" \
make test-e2e
```

### Ironic

The tests create their hosts with placeholder BMC addresses
(`redfish+http://192.168.222.1:8000/redfish/v1/Systems/<index>`). The fixture
provisioner never contacts the BMC, but a real Ironic does, so with the
standard e2e lab (`hack/ci-e2e.sh`) the hosts fail registration and never
become available. Running against Ironic needs an environment whose BMC
emulator serves these systems, for example sushy-tools' fake driver combined
with fake-ipa. The `config/overlays/e2e-scalability` overlay is provided for
such environments.

## Configuration

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `SCALABILITY_NUM_HOSTS` | 10 | Number of BMH resources to create |
| `SCALABILITY_API_THRESHOLD_MS` | 5000 | Fail if any Ironic API call exceeds this (ms) |
| `SCALABILITY_API_MEASUREMENTS` | 5 | API calls per measurement point |

## BMO deployment settings

The tests do not modify the BMO deployment. They run against whatever BMO the
suite installed from `BMO_KUSTOMIZATION`, so BMO settings such as the
controller concurrency belong in the kustomization.

The `config/components/scalability` component sets `BMO_CONCURRENCY` in the
`ironic` ConfigMap, which the manager container loads as environment
variables. Two overlays apply it on top of the regular e2e overlays:

| Overlay | Based on |
|---------|----------|
| `config/overlays/fixture-scalability` | `config/overlays/fixture` |
| `config/overlays/e2e-scalability` | `config/overlays/e2e` |

To test a different concurrency, change `BMO_CONCURRENCY` in the component.
For other environments, such as a bare metal lab, include the component in
your own overlay, or set `BMO_CONCURRENCY` there directly.

With namespace-scoped BMO (`NAMESPACE_SCOPED=true`), the tests use the
`scalability` namespace from `config/overlays/e2e/scalability`. BMO only
watches namespaces listed in `WATCH_NAMESPACE`, so a custom overlay must
include that namespace and its RBAC.

## Ironic API latency

The API latency is measured during the enrollment and provisioning tests
rather than in a separate test, so it uses the Ironic nodes that BMO created
for the test hosts. Measurements are taken at these points:

| Test | Point | Phase in results |
|------|-------|------------------|
| Enrollment | All hosts are available | `Enrollment` |
| Provisioning | Right after all hosts were given an image, while BMO is provisioning them | `Provisioning` |
| Provisioning | All hosts are provisioned | `Provisioned` |

At each point the test calls node list `SCALABILITY_API_MEASUREMENTS` times,
and node get the same number of times for both the first and the last host.
Each test logs every snapshot as JSON and fails if the slowest list or get
call at any point exceeded `SCALABILITY_API_THRESHOLD_MS`.
Measurements are skipped when Ironic is not deployed.

## Notes

- All scalability tests are `Serial` and `Ordered` in the test code, so they
   run alone and in order even when Ginkgo uses multiple parallel workers.
- The `scalability` label is excluded by default (the Makefile defaults
   `GINKGO_LABEL_FILTER` to `!scalability`, and the CI jobs select the
   `required`/`optional` tiers, neither of which includes scalability).
   Run them explicitly with `GINKGO_LABEL_FILTER="scalability"`.
