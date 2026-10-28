# E2E Scalability Tests (Fake IPA)

Other scalability suites in this repo may use the Fixture provider (no real
Ironic) or the `fake-hardware` driver (no agent, no real deploy) - neither
exercises a real agent-based deploy, live-ISO mode, or Ironic's own memory
usage under load.

This suite (`test/e2e/scalability_fakeipa_test.go`) instead drives the
**real** stack — BMO + Ironic +
[Fake IPA](https://github.com/metal3-io/utility-images/tree/main/fake-ipa) —
using sushy-tools' `FakeDriver` as the BMC emulator (no VMs, no libvirt
domains). BMO and Ironic never know the BMC or the agent are fake: real
Redfish calls, real agent lookup/heartbeat/deploy-step traffic, real
`ramdisk`/live-ISO state machine.

## Tests

| Test | What it measures | Shape |
|------|-------------------|-------|
| Normal-mode deploy scaling | BMH→Provisioned throughput and Ironic memory as host count grows, via a normal image deploy | fixed-size ramp, `IMAGE_URL`/`IMAGE_CHECKSUM` |
| Live-ISO deploy scaling | Same, but via a live-ISO deploy | fixed-size ramp, `ISO_IMAGE_URL`, `DiskFormat: live-iso` |
| Max available-state BMH count | How many BMHs can sit at `Available` before signs of instability | cumulative ramp (`loadRampConfig`) |
| Max provisioned-state BMH count | Same, but at `Provisioned` (via a real Fake IPA deploy) | cumulative ramp |
| Max concurrent provisioning count | How many BMHs can provision *at the same time* before the retry rate hits a threshold | fresh-batch ramp per scale point |
| Provisioning time vs. concurrent host count | `Status.OperationHistory.Provision` duration distribution at each concurrency level, for plotting | fresh-batch ramp per scale point |

Ironic memory vs. node count isn't a separate test — every round of every
test above samples Ironic's own process memory (cgroup
`memory.current`/`memory.usage_in_bytes`) and logs it alongside the host
count, so the memory-vs-nodes curve falls out of all six specs together.

### The two ramp styles

- **Cumulative ramp** (`runCumulativeStateRamp`, used by the available-state
  and provisioned-state tests): the population of BMHs keeps growing by
  `SCALABILITY_FAKEIPA_RAMP_STEP` each round and is *never cleaned up* until
  the whole spec finishes — this matches "how many BMHs can we **have**" as
  a standing-count question. Degradation = any tracked host (old or new)
  shows a BMH error, a round's wait time exceeds
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
container) must be created **before** `make test-e2e` runs, the same way
`hack/ci-e2e.sh` creates the stock VM-based lab before invoking ginkgo:
Ironic's own health check runs once in the suite's `SynchronizedBeforeSuite`,
before any spec's `BeforeEach` - too early for anything started from inside
this test package to help. `hack/fakeipa-lab-up.sh` fills that gap:

```bash
./hack/fakeipa-lab-up.sh 5   # number of fake systems to pre-register, matching SCALABILITY_FAKEIPA_MAX_HOSTS

SCALABILITY_FAKEIPA_MAX_HOSTS=5 \
SCALABILITY_FAKEIPA_BATCH_SIZE=5 \
GINKGO_FOCUS="Scalability \(FakeIPA\)" \
GINKGO_SKIP_LABELS="" \
make test-e2e

./hack/fakeipa-lab-down.sh
```

Start small (5 hosts) to confirm the FakeDriver/Fake IPA wiring reaches
`Provisioned` before ramping both `fakeipa-lab-up.sh`'s system count and
`SCALABILITY_FAKEIPA_MAX_HOSTS` up together (100 → 500 → 1000+) to find
where timeouts or errors start appearing, which is the actual scale-ceiling
data point.

## Configuration

| Environment variable | Default | Description |
|---|---|---|
| `SCALABILITY_FAKEIPA_MAX_HOSTS` | 100 | Total fake hosts to ramp up to (must be ≤ the count passed to `fakeipa-lab-up.sh`) |
| `SCALABILITY_FAKEIPA_BATCH_SIZE` | 10 | Hosts created per batch |
| `SCALABILITY_FAKEIPA_TIMEOUT` | 5m | Per-batch timeout waiting for `Provisioned` |
| `SCALABILITY_FAKEIPA_SYSTEMS_FILE` | `/tmp/fakeipa-lab/systems.json` | Fake systems list written by `fakeipa-lab-up.sh` |
| `SCALABILITY_FAKEIPA_FAKEIPA_IMAGE` | `quay.io/metal3-io/fake-ipa:latest` | Fake IPA image |

`fakeipa-lab-up.sh`/`fakeipa-lab-down.sh` also respect `FAKEIPA_LAB_DIR`
(default `/tmp/fakeipa-lab`), `FAKEIPA_IMAGE_DIR` (default
`/tmp/fakeipa-image-cache`, kept across runs to avoid re-downloading),
`SCALABILITY_FAKEIPA_SUSHY_IMAGE`, and `SCALABILITY_FAKEIPA_VBMCTL_BIN`.

### Ramp tests

One shared set of knobs drives all four ramp tests (`loadRampConfig`). Run
specs individually with `GINKGO_FOCUS` if one needs different tuning than
another:

| Environment variable | Default | Description |
|---|---|---|
| `SCALABILITY_FAKEIPA_RAMP_START` | 5 | First round's scale |
| `SCALABILITY_FAKEIPA_RAMP_STEP` | 5 | How much to grow by each round |
| `SCALABILITY_FAKEIPA_RAMP_MAX` | 100 | Hard safety cap - stop regardless of degradation |
| `SCALABILITY_FAKEIPA_RAMP_ROUND_TIMEOUT` | 10m | Per-round timeout; a round timing out counts as degradation |
| `SCALABILITY_FAKEIPA_RAMP_TIME_MULTIPLIER` | 3.0 | Cumulative-ramp and plot tests: round time vs. baseline multiplier that counts as degradation |
| `SCALABILITY_FAKEIPA_RETRY_RATE_THRESHOLD` | 0.05 | Fresh-batch ramp tests: retry rate (fraction) that counts as degradation |

**The fake-systems pool must be at least `SCALABILITY_FAKEIPA_RAMP_MAX`
large** (and at least `SCALABILITY_FAKEIPA_MAX_HOSTS` for the normal/live-ISO
deploy tests) - run `hack/fakeipa-lab-up.sh <N>` with a big enough `N` before
any of these specs, or they'll fail fast with a clear "lab only has N fake
systems" message rather than silently under-testing.

## Known limitations

- The Ironic pod is located by a `strings.Contains(pod.Name, "ironic")`
heuristic rather than a confirmed label selector — if `sampleIronicMemoryMB`
logs a warning, check the actual pod name/labels in the
`baremetal-operator-system` namespace and adjust.
- sushy-tools' `FakeDriver` behind a BMH-driven live-ISO deploy is new,
unproven wiring — if the live-ISO test fails early, check whether Ironic's
live-ISO code path requires BMC/boot capabilities `FakeDriver` doesn't fully
emulate.
