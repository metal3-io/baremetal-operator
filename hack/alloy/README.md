# BMO e2e log collection (Alloy → Loki)

Streams the logs produced during the BMO e2e suite to the Metal³ CI log
collector (Grafana Loki), so they are searchable in Grafana alongside the
Jenkins console output instead of only living inside the `artifacts-*.tar.gz`
archive.

## How it works

The CAPI test framework already streams the deployment logs to files *live*
during the run (`WatchDeploymentLogs` in `test/e2e/e2e_suite_test.go`), and
libvirt writes the BMH serial consoles live under `/var/log/libvirt/qemu/`.
Grafana Alloy simply tails those files as they grow and pushes each line to
Loki. Because the files are written live, this is real-time streaming - logs
arrive as they happen and survive a mid-run crash - without Alloy needing any
Kubernetes API access or pod discovery.

```text
CAPI framework ─┐                         ┌─ loki.process (labels + metadata)
libvirt        ─┴─▶ *.log files ─▶ Alloy ─┤
                                          └─ loki.write ─▶ Loki (push API)
```

`hack/ci-e2e.sh` starts Alloy before `make test-e2e` and stops it afterwards:

```text
run-alloy.sh   # installs Alloy on demand, starts it in the background
make test-e2e  # logs are tailed and shipped while the suite runs
verify.sh      # asserts Alloy actually shipped entries (reads its metrics)
stop-alloy.sh  # flushes and stops Alloy
```

All of this is opt-in: if the push endpoint/credential is not configured, or
Alloy cannot be installed, the scripts print a message and exit 0, leaving the
e2e run unchanged.

## Files

| File                     | Purpose                                                        |
|--------------------------|----------------------------------------------------------------|
| `config.alloy`           | Alloy pipeline: tail log files, label them, push to Loki.      |
| `run-alloy.sh`           | Start Alloy in the background (installs it on demand).          |
| `verify.sh`              | Confirm Alloy shipped entries, via its local metrics endpoint.  |
| `stop-alloy.sh`          | Flush and stop the background Alloy process.                    |
| `../e2e/ensure_alloy.sh` | Install the Alloy binary if it is not already on `PATH`.        |

## Configuration (environment variables)

Set by the Jenkins pipeline; the `ALLOY_*` / `BMO_ARTIFACTS_DIR` values fall
back to sensible defaults for local runs. The variable names match the CAPM3
Alloy config so a single pipeline wiring feeds both jobs.

| Variable             | Required | Meaning                                                    |
|----------------------|----------|------------------------------------------------------------|
| `LOKI_URL`           | yes      | Loki push endpoint, e.g. `.../store/api/v1/push`.          |
| `LOKI_USERNAME`      | yes      | Read/write push username.                                  |
| `LOKI_PASSWORD`      | yes      | Read/write push password.                                  |
| `ALLOY_JOB`          | no       | `job` label (default `metal3ci`).                          |
| `ALLOY_PIPELINE_ID`  | no       | `pipeline_id` label (default `metal3-bmo-e2e`).            |
| `ALLOY_BUILD_NUMBER` | no       | `build_number` label (default `0`).                        |
| `BMO_ARTIFACTS_DIR`  | no       | Path to `_artifacts` (default `test/e2e/_artifacts`).      |
| `ALLOY_VERSION`      | no       | Alloy release to install (default `v1.18.1`).               |

## Labels and metadata

Streams carry a small set of low-cardinality **labels** for dashboard
drill-down, and richer per-line **structured metadata** derived from the file
path:

- Labels: `job`, `pipeline_id`, `build_number`, `source="artifact"` (set in
  `loki.write.external_labels`), plus `collector` (`container` or `serial`).
- Structured metadata: `component`, `pod`, `container` for container logs;
  `machine` for serial consoles.

To appear in the same dashboard context as a build's Jenkins output,
`pipeline_id` and `build_number` must match the values that build reports.

Only `*.log` files are shipped. Manifests, resource dumps and `*-log-metadata`
files are left in the artifact archive, not pushed to Loki.

## CI wiring

The push credential and `ALLOY_*` values are supplied to `ci-e2e.sh` by the BMO
e2e Jenkins job via the `metal3-ci-log-collector-push` credential; that pipeline
wiring lives in `project-infra`. Without it the log-collection steps no-op and
the e2e run is unaffected.
