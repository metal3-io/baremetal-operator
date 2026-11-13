# E2E Scalability Tests (Fake IPA)

Other scalability suites in this repo may use the Fixture provider (no real
Ironic) or the `fake-hardware` driver (no agent, no real deploy) - neither
exercises a real agent-based deploy, live-ISO mode, or measure Ironic's own
memory usage under load.

This suite (`test/e2e/scalability_fakeipa_test.go`) instead drives the
**real** stack — BMO + Ironic +
[Fake IPA](https://github.com/metal3-io/utility-images/tree/main/fake-ipa) —
using sushy-tools' `FakeDriver` as the BMC emulator (no VMs, no libvirt
domains). BMO and Ironic never know the BMC or the agent are fake, thanks to the
real Redfish calls, real agent lookup/heartbeat/deploy-step traffic, real
`ramdisk`/live-ISO state machine.

## Tests

| Test | What it measures | Shape |
|------|-------------------|-------|
| Max available-state BMH count | How many BMHs can sit at `Available` before signs of instability | cumulative ramp (`loadRampConfig`) |
| Max provisioned-state BMH count (normal mode) | Same, but at `Provisioned` via a real Fake IPA deploy of a normal image | cumulative ramp |
| Max provisioned-state BMH count (live-ISO mode) | Same, but deployed via live-ISO instead of a normal image | cumulative ramp |
| Max concurrent provisioning count | How many BMHs can provision *at the same time* before the retry rate hits a threshold | fresh-batch ramp per scale point |
| Provisioning time vs. concurrent host count | `Status.OperationHistory.Provision` duration distribution at each concurrency level, for plotting | fresh-batch ramp per scale point |

Ironic memory vs. node count isn't a separate test — every round of every
test above samples Ironic's own process memory (cgroup
`memory.current`/`memory.usage_in_bytes`) and logs it alongside the host
count, so the memory-vs-nodes curve falls out of all five specs together.

### The two ramp styles

- **Cumulative ramp** (`runCumulativeStateRamp`, used by the available-state
  test and both provisioned-state tests): the population of BMHs keeps
  growing by `SCALABILITY_FAKEIPA_RAMP_STEP` each round and is *never
  cleaned up* until the whole spec finishes — this matches "how many BMHs
  can we **have**" as a standing-count question. Degradation = any tracked
  host (old or new) shows a BMH error, a round's wait time exceeds
  `SCALABILITY_FAKEIPA_RAMP_TIME_MULTIPLIER`× the first round's baseline, or
  a round times out outright. The result is the last round's count that
  stayed clean.
- **Fresh-batch ramp** (`runConcurrentProvisioningRound`, used by the
  concurrent-provisioning and provisioning-time tests): each scale point
  creates a brand new batch, measures it, and deletes it before trying the
  next (larger) scale — this matches "how many **at the same time**", not a
  standing count. Retry rate is the fraction of the batch with
  `Status.ErrorCount > 0` (BMO's own retry/backoff counter).

Both ramps have a hard `SCALABILITY_FAKEIPA_RAMP_MAX` safety cap so a
misconfigured threshold can't run forever, and both are logged as JSON lines
per round/scale point for extraction.

## Prerequisites

- Docker (the Fake IPA container and vbmctl's BMC emulator container both run
  with `--network host`)
- `./bin/vbmctl` built (`make build-vbmctl`) and capable of running
  `create`/`delete bml` (needs `libvirtd` reachable even though no VMs are
  created — see `test/vbmctl/cmd/vbmctl/create.go`)
- The `quay.io/metal3-io/sushy-tools` and `quay.io/metal3-io/fake-ipa` images

## Running

The lab (baremetal-e2e bridge/veth pair, image server, sushy-tools FakeDriver
container) must be created **before** the suite runs, the same way
`hack/ci-e2e.sh` creates the stock VM-based lab before invoking ginkgo:
Ironic's own health check runs once in the suite's `SynchronizedBeforeSuite`,
before any spec's `BeforeEach` - too early for anything started from inside
this test package to help. `hack/fakeipa-lab-up.sh` fills that gap, and
`hack/fakeipa-test-e2e.sh` runs the suite itself:

```bash
./hack/fakeipa-lab-up.sh 25   # must be >= SCALABILITY_FAKEIPA_RAMP_MAX (25 by default)

./hack/fakeipa-test-e2e.sh

./hack/fakeipa-lab-down.sh
```

Use `./hack/fakeipa-test-e2e.sh` rather than calling `make test-e2e`
directly: this suite needs `E2E_CONF_FILE`, `E2E_BMCS_CONF_FILE`, and
`IRONIC_KUSTOMIZATION` all pointed at non-default values (`make test-e2e` on
its own defaults to the Fixture provider, with no real Ironic at all), and
forgetting `IRONIC_KUSTOMIZATION` in particular fails silently at first -
Ironic comes up fine and batches start, only failing minutes later with
`SSLCertVerificationError: self-signed certificate` once a BMH tries to
deploy through Fake IPA, because the default Ironic overlay leaves
`agent.verify_ca` at its default (`True`). `fakeipa-test-e2e.sh` sets
sensible defaults for all three (see Configuration below) and forwards
everything else - including `GINKGO_FOCUS` if you pass it as an argument, to
run a single test - straight to `make test-e2e`.

For a quick smoke test of a single spec, pass a smaller
`SCALABILITY_FAKEIPA_RAMP_MAX` to match a smaller lab instead of waiting for
every ramp to run to its default ceiling:

```bash
./hack/fakeipa-lab-up.sh 5

SCALABILITY_FAKEIPA_RAMP_MAX=5 \
./hack/fakeipa-test-e2e.sh 'should find the max available-state BMH count before signs of instability'

./hack/fakeipa-lab-down.sh
```

Start small to confirm the FakeDriver/Fake IPA wiring reaches `Provisioned`
before ramping both `fakeipa-lab-up.sh`'s system count and
`SCALABILITY_FAKEIPA_RAMP_MAX` up together (100 → 500 → 1000+) to find where
timeouts or errors start appearing, which is the actual scale-ceiling data
point.

## Configuration

| Environment variable | Default | Description |
|---|---|---|
| `SCALABILITY_FAKEIPA_SYSTEMS_FILE` | `/tmp/fakeipa-lab/systems.json` | Fake systems list written by `fakeipa-lab-up.sh` |
| `SCALABILITY_FAKEIPA_FAKEIPA_IMAGE` | `quay.io/metal3-io/fake-ipa:latest` | Fake IPA image |

`fakeipa-lab-up.sh`/`fakeipa-lab-down.sh` also respect `FAKEIPA_LAB_DIR`
(default `/tmp/fakeipa-lab`), `FAKEIPA_IMAGE_DIR` (default
`/tmp/fakeipa-image-cache`, kept across runs to avoid re-downloading),
`SCALABILITY_FAKEIPA_SUSHY_IMAGE`, and `SCALABILITY_FAKEIPA_VBMCTL_BIN`.

`fakeipa-test-e2e.sh` only sets a variable if it isn't already set, so any of
these can still be overridden:

| Environment variable | Default set by `fakeipa-test-e2e.sh` | Description |
|---|---|---|
| `E2E_CONF_FILE` | `test/e2e/config/ironic.yaml` | Selects the real-Ironic e2e profile instead of `make test-e2e`'s own Fixture-provider default |
| `E2E_BMCS_CONF_FILE` | `test/e2e/config/bmcs-redfish-virtualmedia.yaml` | BMC config matching the Redfish FakeDriver this suite uses |
| `IRONIC_KUSTOMIZATION` | `data/ironic-standalone-operator/ironic/overlays/e2e-fakeipa` | Adds `agent.verify_ca=False` so Ironic accepts Fake IPA's self-signed cert - required by every test except the available-state ramp, which never deploys |

### Ramp tests

One shared set of knobs drives all five specs (`loadRampConfig`). Run specs
individually with `GINKGO_FOCUS` if one needs different tuning than
another:

| Environment variable | Default | Description |
|---|---|---|
| `SCALABILITY_FAKEIPA_RAMP_START` | 5 | First round's scale |
| `SCALABILITY_FAKEIPA_RAMP_STEP` | 5 | How much to grow by each round |
| `SCALABILITY_FAKEIPA_RAMP_MAX` | 25 | Hard safety cap - stop regardless of degradation |
| `SCALABILITY_FAKEIPA_RAMP_ROUND_TIMEOUT` | 10m | Per-round timeout; a round timing out counts as degradation |
| `SCALABILITY_FAKEIPA_RAMP_TIME_MULTIPLIER` | 2.0 | Cumulative-ramp and plot tests: round time vs. baseline multiplier that counts as degradation |
| `SCALABILITY_FAKEIPA_RETRY_RATE_THRESHOLD` | 0.05 | Fresh-batch ramp tests: retry rate (fraction) that counts as degradation |

**The fake-systems pool must be at least `SCALABILITY_FAKEIPA_RAMP_MAX`
large** - run `hack/fakeipa-lab-up.sh <N>` with a big enough `N` before any
of these specs, or they'll fail fast with a clear "lab only has N fake
systems" message rather than silently under-testing.

## Known limitations

- The Ironic pod is located by a `strings.Contains(pod.Name, "ironic")`
heuristic rather than a confirmed label selector — if `sampleIronicMemoryMB`
logs a warning, check the actual pod name/labels in the
`baremetal-operator-system` namespace and adjust.
- sushy-tools' `FakeDriver` behind a BMH-driven live-ISO deploy is new,
unproven wiring — if the live-ISO provisioned-state spec fails early, check
whether Ironic's live-ISO code path requires BMC/boot capabilities
`FakeDriver` doesn't fully emulate. A failure there is isolated to that one
spec and doesn't affect the normal-mode provisioned-state spec or any other
spec in the suite.
