# Scalability Test Output Analysis

Full line-by-line explanation of the test run with 700 hosts and 10 concurrent reconciles.

---

## Command

```bash
SCALABILITY_NUM_HOSTS=700 SCALABILITY_MAX_CONCURRENT_RECONCILES=10 \
  GINKGO_FOCUS="Scalability" GINKGO_SKIP_LABELS="" GINKGO_NODES=1 \
  make test-e2e
```

- `SCALABILITY_NUM_HOSTS=700` — create 700 BareMetalHost resources
- `SCALABILITY_MAX_CONCURRENT_RECONCILES=10` — BMO will process 10 BMHs simultaneously
- `GINKGO_FOCUS="Scalability"` — only run tests matching "Scalability"
- `GINKGO_SKIP_LABELS=""` — override the default which skips the `scalability` label
- `GINKGO_NODES=1` — single Ginkgo process (no parallelism)

---

## Phase 1: Suite Setup (`SynchronizedBeforeSuite`)

```
Running Suite: E2e Suite - /home/ubuntu/baremetal-operator/test/e2e
Random Seed: 1786094487
Will run 2 of 32 specs
```

Ginkgo found 32 test specs in the suite but only 2 match the focus filter ("Scalability").
The random seed controls test ordering (reproducible with `--seed=1786094487`).

```
INFO: Creating a kind cluster with name "bmo-e2e"
Creating cluster "bmo-e2e" ...
 ✓ Ensuring node image (kindest/node:v1.36.1)
 ✓ Preparing nodes
 ✓ Writing configuration
 ✓ Starting control-plane
 ✓ Installing CNI
 ✓ Installing StorageClass
```

Creates a single-node Kubernetes cluster using Kind (Kubernetes-in-Docker).
The node image `kindest/node:v1.36.1` contains kubelet, kube-apiserver, etcd,
kube-controller-manager, kube-scheduler — everything needed for a full cluster
running inside one Docker container.

```
INFO: The kubeconfig file for the kind cluster is /tmp/e2e-kind3318282607
```

The test framework saves the kubeconfig to a temp file. All subsequent
`kubectl` / client-go calls use this to connect to the Kind cluster's API server.

```
INFO: Loading image: "quay.io/metal3-io/baremetal-operator:e2e"
INFO: Image quay.io/metal3-io/baremetal-operator:e2e is present in local container image cache
```

The locally-built BMO image (built by `make docker` with `IMG_TAG=e2e`) is loaded
into the Kind node so the Deployment can pull it with `imagePullPolicy: IfNotPresent`.

```
INFO: Loading image: "quay.io/metal3-io/baremetal-operator:release-0.11"
INFO: Image ... is present in local container image cache
```

Older release images are pre-loaded in case upgrade tests need them.
They're not used by the scalability test but the suite loads everything listed in `fixture.yaml`.

```
INFO: [WARNING] Unable to load image "quay.io/jetstack/cert-manager-cainjector:v1.19.2" ...
failed to load image: command "docker exec --privileged -i bmo-e2e-control-plane ctr ..." failed
```

cert-manager images failed to pre-load (likely a multi-arch issue with `ctr import`).
The `loadBehavior: tryLoad` setting means this is non-fatal — the Kind node will
pull them from the registry when cert-manager is deployed.

```
INFO: Configuring provisioning network: adding 192.168.222.2/24 to bmo-e2e-control-plane
INFO: Provisioning network configured successfully
```

Adds a secondary IP to the Kind container's network interface. This is the
provisioning network IP that BMO/Ironic would normally use. Not actually needed
for the fixture provisioner but the suite always configures it.

```
STEP: Installing cert-manager @ 08/07/26 10:14:42.986
STEP: Waiting for cert-manager webhook @ 08/07/26 10:14:48.524
```

cert-manager is deployed because BMO's webhooks (validating/mutating) use TLS
certificates issued by cert-manager. Without it, the webhook server won't start
and BMH creation would be rejected.

```
I0807 10:15:31.871490 ... "spec.privateKey.rotationPolicy: ..."
```

Informational warning from cert-manager about a default change. Harmless.

```
STEP: Installing BMO @ 08/07/26 10:15:31.872
```

Applies `kustomize build config/overlays/fixture` which deploys:
- The BMO Deployment with `--provisioner=fixture` flag
- CRDs (BareMetalHost, HardwareData, etc.)
- RBAC (ClusterRole, ServiceAccount)
- Webhook configurations
- cert-manager Certificate resources for webhook TLS

```
STEP: Waiting for deployment baremetal-operator-system/baremetal-operator-controller-manager to be available
INFO: Creating log watcher for controller ..., pod ...-7b5fbfccf9-v87bh, container manager
```

Waits until the BMO pod is Running and Ready. Then starts streaming its logs
to the artifacts folder for post-mortem debugging.

```
[SynchronizedBeforeSuite] PASSED [238.581 seconds]
```

Total setup time: ~4 minutes (Kind creation + image loading + cert-manager + BMO).

```
SSSSSSSSSSSSSSSSSSSSSS
```

22 test specs were skipped (the S's). They don't match the `--focus="Scalability"` filter.

---

## Phase 2: Enrollment Test

```
Scalability should enroll multiple BMHs within the time window [Serial, scalability]
```

Test name, decorators shown: `Serial` (never run in parallel), `scalability` (label).

```
INFO: Creating namespace scalability-o9v8zb
INFO: Creating event watcher for namespace "scalability-o9v8zb"
```

`BeforeEach` creates a unique namespace with a random suffix. The event watcher
captures Kubernetes events in that namespace and writes them to the artifacts
folder (useful for debugging failures).

```
STEP: Setting BMO max-concurrent-reconciles to 10 @ 08/07/26 12:25:48.307
INFO: Set --max-concurrent-reconciles=10 on BMO deployment
```

The test reads the current BMO Deployment, appends `--max-concurrent-reconciles=10`
to the container args, updates the Deployment, and waits for the new pod to be
Ready. This triggers a rolling update (old pod terminated, new pod created).

```
STEP: Creating 700 BMH resources with credentials @ 08/07/26 12:25:50.342
```

A `for` loop (i=0..699) creates:
- Secret `bmc-creds-XXXX` with username/password
- BareMetalHost `scale-host-XXXX` with:
  - `inspectionMode: disabled` (skip hardware inspection, no IPA needed)
  - `automatedCleaningMode: disabled` (skip disk cleaning step)
  - `bootMACAddress: 02:00:00:XX:XX:XX` (deterministic, locally-administered)
  - `bmc.address: redfish+http://192.168.222.1:8000/redfish/v1/Systems/XXXXXXXX`
    (fixture provisioner doesn't call this, but webhook validates the scheme)

```
INFO: Creating log watcher for controller ..., pod ...-5ffc6b4597-cj7gc, container manager
```

The new pod (from the rolling update) gets a log watcher attached.

```
INFO: Created 700 BMH resources in 2m40.659227539s
```

160 seconds to create 1400 Kubernetes objects (700 secrets + 700 BMHs) sequentially.
That's ~8.75 objects/second — limited by the serial API calls and webhook validation
on each BMH.

```
STEP: Waiting for all 700 BMHs to reach 'available' state @ 08/07/26 12:28:31.001
```

The `Eventually` block starts polling. Every 2 seconds it:
1. Fetches all 700 BMH objects from the API server
2. Counts how many have `.status.provisioning.state == "available"`
3. Asserts count == 700

If the assertion fails, it retries after 2s. If 10 minutes pass without
all 700 reaching `available`, the test fails.

Meanwhile, BMO's reconcile loop is processing these BMHs. With concurrency=10,
it processes 10 BMHs simultaneously. Each BMH goes through:

```
"" → registering → available  (2 reconcile passes with fixture provisioner)
```

Pass 1 (Register): Fixture provisioner returns `provID = "temporary-fake-id"`, marks dirty
Pass 2 (Register): provID already set, returns not dirty → state advances to available

```
INFO: === Enrollment Results ===
INFO:   Hosts:              700
INFO:   Concurrency:        10
INFO:   Duration:           287.7s
INFO:   Throughput:         146.0 hosts/min
INFO:   JSON: {"phase":"Enrollment","numHosts":700,"maxConcurrentReconciles":10,...}
```

**287.7 seconds** from when the `Eventually` loop started until all 700 BMHs
reached `available`. That's **146 hosts/min** or **~2.4 hosts/second**.

With 10 concurrent workers, each BMH takes ~2 reconcile passes at ~5s each
(requeue delay). Theoretical: 10 workers × 60s / 10s per host = 60 hosts/min.
Actual is higher (146) because the fixture provisioner's requeue delays are
short and the controller processes events faster than the worst case.

```
STEP: Restoring BMO deployment to original configuration @ 08/07/26 12:33:18.669
```

`AfterEach` restores the original container args (removing `--max-concurrent-reconciles=10`).
This triggers another rolling update. Now BMO is back to its default concurrency (1).

```
STEP: Deleting test namespace (cascading delete of all resources) @ 08/07/26 12:33:18.705
INFO: Deleting namespace scalability-o9v8zb
```

Deletes the namespace. Kubernetes marks all objects inside for deletion.
But each BMH has a **finalizer** (`baremetalhost.metal3.io`). Kubernetes won't
remove the object until BMO reconciles it, runs cleanup (fixture: instant),
and removes the finalizer. With concurrency restored to 1, this processes
~1 BMH deletion per reconcile cycle.

```
• [685.050 seconds]
```

Total time for the enrollment `It` block including cleanup: ~11.4 minutes.
The test itself (enrollment) took ~4.8 minutes. The rest was setup + cleanup.
First test passed.

---

## Phase 3: Provisioning Test

```
Scalability should provision multiple BMHs within the time window [Serial, scalability]
INFO: Creating namespace scalability-gbwlky
```

New unique namespace for the provisioning test.

```
STEP: Setting BMO max-concurrent-reconciles to 10
```

Same BeforeEach: patches deployment, waits for rollout.

```
STEP: Creating 700 BMH resources with credentials
STEP: Waiting for all 700 BMHs to reach 'available' state
```

Creates 700 BMHs (named `scale-prov-XXXX` this time), waits for all to become
`available`. This is the prerequisite — you can only provision an `available` host.

```
STEP: Patching all 700 BMHs with an image to trigger provisioning @ 08/07/26 12:45:32.643
```

700 goroutines fire simultaneously. Each one:
1. `GET` the BMH (to get current resourceVersion for optimistic locking)
2. Set `spec.image` (URL + checksum) and `spec.rootDeviceHints`
3. `PUT` (update) the modified BMH back

This triggers the state transition: `available → provisioning → provisioned`.

```
I0807 12:45:33.772205 ... "Waited before sending request" delay="1.128335033s"
  reason="client-side throttling, not priority and fairness"
  verb="GET" URL=".../baremetalhosts/scale-prov-0005"
```

**Client-side throttling**. The controller-runtime client has a built-in rate limiter
(default: 20 QPS, burst 30). With 700 goroutines all trying to GET+PUT simultaneously,
they exceed the rate limit. The client queues excess requests internally and logs
this warning when a request had to wait.

The delays grow linearly:
- 1s, 11s, 21s, 31s ... up to 2m20s

This means the last goroutines waited over 2 minutes before their GET request
was even sent. This is **not** a server-side problem — it's the test client
self-limiting. The API server was fine.

After all GETs complete, the PUTs also get throttled:
```
delay="2m19.997631177s" reason="client-side throttling" verb="PUT"
```

Total time to submit all 700 patches: about 5 minutes (from 12:45:32 to 12:50:12).

```
STEP: Waiting for all 700 BMHs to reach 'provisioned' state @ 08/07/26 12:50:12.658
```

Now the `Eventually` loop polls every 2s counting BMHs in `provisioned` state.
BMO's fixture provisioner handles provisioning in 1-2 reconcile passes:
- Pass 1: Sets `image.URL` on internal state, publishes "ProvisioningComplete", marks dirty
- Pass 2: Image already set, state advances to `provisioned`

```
INFO: === Provisioning Results ===
INFO:   Hosts:              700
INFO:   Concurrency:        10
INFO:   Duration:           419.9s
INFO:   Throughput:         100.0 hosts/min
```

**419.9 seconds** (7 minutes) total from patch submission start to all 700 `provisioned`.
But ~5 minutes of that was the test client draining its own throttle queue. The
actual controller processing time was roughly 2 minutes.

**100 hosts/min** is lower than enrollment (146) because the provisioning measurement
includes the client-side throttling overhead. The controller was actually faster —
it was idle waiting for patches to arrive for the first few minutes.

```
STEP: Restoring BMO deployment to original configuration @ 08/07/26 12:52:32.573
```

Concurrency restored to 1. Rolling update.

```
STEP: Deleting test namespace (cascading delete of all resources) @ 08/07/26 12:52:32.586
INFO: Deleting namespace scalability-gbwlky
```

Same cascading namespace delete. 700 BMHs with finalizers, but now concurrency is 1.

```
[FAILED] Timed out after 600.000s.
Expected <bool>: false to be true
```

`WaitForNamespaceDeleted` polled for 600 seconds (10 minutes) and the namespace
still existed. With concurrency=1, BMO processes ~1 finalizer removal per second.
700 BMHs × ~1s each = ~700 seconds minimum. The 600s timeout was too short.

The fix is to **delete the namespace before restoring concurrency** so the high
concurrency (10) is still active during finalizer processing, or increase the timeout.

---

## Phase 4: Teardown

```
[SynchronizedAfterSuite]
STEP: Tearing down the management cluster
INFO: Error getting pod ... dial tcp 127.0.0.1:41865: connect: connection refused
```

The Kind cluster is being deleted. The log watcher tries one last fetch and
gets "connection refused" because the API server is gone. This is expected and
doesn't affect the test result.

```
[SynchronizedAfterSuite] PASSED [2.696 seconds]
```

Kind cluster destroyed.

---

## Final Summary

```
Summarizing 1 Failure:
  [FAIL] Scalability [AfterEach] should provision multiple BMHs within the time window

Ran 2 of 32 Specs in 2464.398 seconds
FAIL! -- 1 Passed | 1 Failed | 0 Pending | 30 Skipped
```

- **1 Passed**: Enrollment test (including its cleanup)
- **1 Failed**: Provisioning test passed, but its AfterEach cleanup timed out
- **30 Skipped**: Other E2E tests that didn't match the focus filter
- **Total wall time**: 41 minutes

---

## Key Metrics

| Phase | Hosts | Concurrency | Duration | Throughput |
|-------|-------|-------------|----------|------------|
| Enrollment | 700 | 10 | 287.7s | 146.0 hosts/min |
| Provisioning | 700 | 10 | 419.9s | 100.0 hosts/min |

---

## Root Cause of Failure

The test restores BMO concurrency to 1 **before** deleting the namespace.
Namespace deletion requires BMO to process finalizer removal on all 700 BMHs.
At concurrency=1, this takes ~700+ seconds. The timeout was 600 seconds.

**Fix**: Move `restoreBMOArgs` to after namespace deletion, so cleanup benefits
from the high concurrency setting.
