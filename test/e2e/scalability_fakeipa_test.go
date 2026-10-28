//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/cluster-api/test/framework"
)

// Env vars controlling the FakeIPA scalability tests. This suite exercises
// the real agent-based deploy path (normal + live-ISO) via BMO + Ironic +
// Fake IPA, and samples Ironic's own memory, without relying on the Fixture
// provider or the fake-hardware driver.
const (
	envScalabilityFakeIPAMaxHosts    = "SCALABILITY_FAKEIPA_MAX_HOSTS"
	envScalabilityFakeIPABatchSize   = "SCALABILITY_FAKEIPA_BATCH_SIZE"
	envScalabilityFakeIPATimeout     = "SCALABILITY_FAKEIPA_TIMEOUT"
	envScalabilityFakeIPASystemsFile = "SCALABILITY_FAKEIPA_SYSTEMS_FILE"
	envScalabilityFakeIPAFakeImage   = "SCALABILITY_FAKEIPA_FAKEIPA_IMAGE"

	defaultScalabilityFakeIPAMaxHosts  = 100
	defaultScalabilityFakeIPABatchSize = 10
	defaultScalabilityFakeIPATimeout   = 6 * time.Minute

	defaultFakeImage = "quay.io/metal3-io/fake-ipa:latest"

	ironicNamespace = "baremetal-operator-system"
)

func getEnvIntFakeIPA(env string, defaultVal int) int {
	if val, ok := os.LookupEnv(env); ok {
		if n, err := strconv.Atoi(val); err == nil && n > 0 {
			return n
		}
	}
	return defaultVal
}

func getEnvDurationFakeIPA(env string, defaultVal time.Duration) time.Duration {
	if val, ok := os.LookupEnv(env); ok {
		if d, err := time.ParseDuration(val); err == nil && d > 0 {
			return d
		}
	}
	return defaultVal
}

func getEnvStringFakeIPA(env, defaultVal string) string {
	if val, ok := os.LookupEnv(env); ok && val != "" {
		return val
	}
	return defaultVal
}

// fakeIPAScalabilityResult is a single batch's measurement, logged as JSON so
// results can be extracted and plotted from CI/test logs.
type fakeIPAScalabilityResult struct {
	Mode                   string  `json:"mode"`
	TotalHosts             int     `json:"totalHosts"`
	BatchDurationSec       float64 `json:"batchDurationSec"`
	ThroughtputHostsPerMin float64 `json:"throughputHostsPerMin"`
	IronicMemoryMB         float64 `json:"ironicMemoryMb,omitempty"`
}

func logFakeIPAResult(r fakeIPAScalabilityResult) {
	Logf("=== FakeIPA Scalability (%s) — %d hosts ===", r.Mode, r.TotalHosts)
	Logf("  Batch duration: %.1fs", r.BatchDurationSec)
	Logf("  Throughput:     %.1f hosts/min", r.ThroughtputHostsPerMin)
	if r.IronicMemoryMB > 0 {
		Logf("  Ironic memory:  %.1f MiB", r.IronicMemoryMB)
	}
	if data, err := json.Marshal(r); err == nil {
		Logf("  JSON: %s", string(data))
	}
}

// runFakeIPABatches ramps up BMHs in batches (using the fake Redfish systems
// provided by lab) until either maxHosts is reached or a batch fails to reach
// Provisioned within perBatchTimeout. imageSpec is applied to every BMH
// created, letting the caller select normal vs. live-ISO deploy mode. Every
// created BMH is appended to *createdBMHs so the caller's AfterEach can clean
// them up individually (see the Describe block below for why).
func runFakeIPABatches(ctx context.Context, mode string, lab *fakeIPALab, namespace string, batchSize, maxHosts int, perBatchTimeout time.Duration, imageSpec *metal3api.Image, createdBMHs *[]metal3api.BareMetalHost) {
	cl := clusterProxy.GetClient()
	systems := lab.Systems()
	Expect(len(systems)).To(BeNumerically(">=", maxHosts),
		"Fake IPA lab was not provisioned with enough fake systems for maxHosts=%d", maxHosts)

	created := 0
	for created < maxHosts {
		batchEnd := created + batchSize
		if batchEnd > maxHosts {
			batchEnd = maxHosts
		}

		By(fmt.Sprintf("[%s] Creating BMHs %d-%d", mode, created, batchEnd))
		batchStart := time.Now()
		var bmhNames []string
		for i := created; i < batchEnd; i++ {
			hostName := fmt.Sprintf("scale-%s-%04d", mode, i)
			bmh := newFakeIPABMH(ctx, cl, namespace, hostName, i, systems[i], imageSpec)
			Expect(cl.Create(ctx, &bmh)).To(Succeed(), "Failed to create BMH %s", hostName)
			bmhNames = append(bmhNames, hostName)
			*createdBMHs = append(*createdBMHs, bmh)
		}

		Eventually(func(g Gomega) {
			provisioned := 0
			for _, name := range bmhNames {
				current := &metal3api.BareMetalHost{}
				key := types.NamespacedName{Namespace: namespace, Name: name}
				g.Expect(cl.Get(ctx, key, current)).To(Succeed())
				if current.Status.Provisioning.State == metal3api.StateProvisioned {
					provisioned++
				}
				if current.Status.ErrorType != "" {
					Logf("WARNING: BMH %s has error: %s - %s", current.Name, current.Status.ErrorType, current.Status.ErrorMessage)
				}
			}
			g.Expect(provisioned).To(Equal(len(bmhNames)),
				"expected %d hosts provisioned in this batch, got %d", len(bmhNames), provisioned)
		}, perBatchTimeout, 5*time.Second).Should(Succeed(),
			"Batch ending at %d hosts did not reach Provisioned within %s", batchEnd, perBatchTimeout)

		batchDuration := time.Since(batchStart)
		batchHostCount := batchEnd - created
		created = batchEnd

		memMB, err := sampleIronicMemoryMB(ctx, ironicNamespace)
		if err != nil {
			Logf("WARNING: failed to sample Ironic memory: %v", err)
		}

		logFakeIPAResult(fakeIPAScalabilityResult{
			Mode:                   mode,
			TotalHosts:             created,
			BatchDurationSec:       batchDuration.Seconds(),
			ThroughtputHostsPerMin: (float64(batchHostCount) / batchDuration.Seconds()) * 60,
			IronicMemoryMB:         memMB,
		})
	}
}

var _ = Describe("Scalability (FakeIPA)", Serial, Label("scalability", "fakeipa"), func() {
	var (
		specName      = "scalability-fakeipa"
		namespace     *corev1.Namespace
		cancelWatches context.CancelFunc
		lab           *fakeIPALab
		maxHosts      int
		batchSize     int
		timeout       time.Duration
		createdBMHs   []metal3api.BareMetalHost
	)

	BeforeEach(func() {
		createdBMHs = nil
		maxHosts = getEnvIntFakeIPA(envScalabilityFakeIPAMaxHosts, defaultScalabilityFakeIPAMaxHosts)
		batchSize = getEnvIntFakeIPA(envScalabilityFakeIPABatchSize, defaultScalabilityFakeIPABatchSize)
		timeout = getEnvDurationFakeIPA(envScalabilityFakeIPATimeout, defaultScalabilityFakeIPATimeout)

		systemsFile := getEnvStringFakeIPA(envScalabilityFakeIPASystemsFile, defaultFakeIPASystemsFile)
		fakeIPAImage := getEnvStringFakeIPA(envScalabilityFakeIPAFakeImage, defaultFakeImage)

		ironicPort := e2eConfig.GetVariable("IRONIC_PROVISIONING_PORT")
		// Fake IPA runs with --network host, so (unlike kind-internal pods)
		// it reaches Ironic the same way a host process would: via the
		// loopback address the kindExtraPortMappings publish Ironic's ports
		// to.
		ironicURL := "https://127.0.0.1:" + ironicPort

		namespace, cancelWatches = framework.CreateNamespaceAndWatchEvents(ctx, framework.CreateNamespaceAndWatchEventsInput{
			Creator:             clusterProxy.GetClient(),
			ClientSet:           clusterProxy.GetClientSet(),
			Name:                specName,
			LogFolder:           artifactFolder,
			IgnoreAlreadyExists: true,
		})

		By("Starting Fake IPA against the lab created by hack/fakeipa-lab-up.sh")
		var err error
		lab, err = startFakeIPA(ctx, systemsFile, fakeIPAImage, ironicURL)
		Expect(err).NotTo(HaveOccurred(), "Failed to start Fake IPA")
	})

	AfterEach(func() {
		// scalability-fakeipa is a namespace-scoped (NAMESPACE_SCOPED=true)
		// BMO watch namespace, like basic-ops/provisioning-ops/etc - it's
		// created once by the BMO kustomization and reused across runs, not
		// torn down per spec. Deleting it here would race Kubernetes' own
		// namespace-content garbage collection: our namespace's RoleBinding/
		// Role (no finalizer) get GC'd before the BMHs (which do have a
		// finalizer) are gone, permanently stripping BMO's RBAC for this
		// namespace mid-cleanup and deadlocking the BMH finalizers forever.
		// So: clean up each BMH individually (CleanupBMH deletes and waits
		// for it to actually be gone) and leave the namespace alone.
		if !skipCleanup {
			deleteTimeout := defaultRampRoundTimeout
			if timeouts := e2eConfig.GetIntervals(specName, "wait-bmh-deleted"); len(timeouts) > 0 {
				if s, ok := timeouts[0].(string); ok {
					if d, err := time.ParseDuration(s); err == nil {
						deleteTimeout = d
					}
				}
			}
			cleanupBMHsParallel(ctx, clusterProxy.GetClient(), namespace.Name, createdBMHs, deleteTimeout)
		}
		// Fake IPA must stay up until every BMH above is confirmed deleted:
		// deprovisioning a BMH makes Ironic call back to sushy-tools, which
		// in turn tries to notify Fake IPA. Killing Fake IPA first causes
		// that notification to fail ("connection refused"), leaving the BMH
		// stuck in an error state with its finalizer never cleared.
		if lab != nil {
			lab.Cleanup(ctx)
		}
		if cancelWatches != nil {
			cancelWatches()
		}
	})

	It("should scale normal-mode deploys using Fake IPA", func() {
		imageSpec := &metal3api.Image{
			URL:          e2eConfig.GetVariable("IMAGE_URL"),
			Checksum:     e2eConfig.GetVariable("IMAGE_CHECKSUM"),
			ChecksumType: metal3api.MD5,
		}
		runFakeIPABatches(ctx, "normal", lab, namespace.Name, batchSize, maxHosts, timeout, imageSpec, &createdBMHs)
	})

	It("should scale live-ISO deploys using Fake IPA", func() {
		imageSpec := &metal3api.Image{
			URL:        e2eConfig.GetVariable("ISO_IMAGE_URL"),
			DiskFormat: ptr.To("live-iso"),
		}
		runFakeIPABatches(ctx, "live-iso", lab, namespace.Name, batchSize, maxHosts, timeout, imageSpec, &createdBMHs)
	})

	// The following four specs each ramp scale by a configurable step (see
	// loadRampConfig) until a test-specific degradation signal fires,
	// reporting the last clean scale as the ceiling. There's no separate
	// spec for Ironic memory vs. node count - every round of every test here
	// samples Ironic's memory, so that data falls out of all six specs
	// together.

	It("should find the max available-state BMH count before signs of instability", func() {
		// No Image spec: hosts register, skip inspection, and stay at
		// Available - this never needs Fake IPA to boot anything.
		result := runCumulativeStateRamp(ctx, "available-scaling", namespace.Name, lab, nil, metal3api.StateAvailable, &createdBMHs)
		Logf("=== Ceiling for available-state scaling: %d hosts clean (degraded at %d: %s) ===",
			result.MaxCleanScale, result.DegradedAtScale, result.DegradationReason)
	})

	It("should find the max provisioned-state BMH count before signs of instability", func() {
		// Same ramp as the available-state test, but driven all the way to
		// Provisioned via a real Fake IPA deploy, and the population is left
		// standing (not deprovisioned) between rounds.
		imageSpec := &metal3api.Image{
			URL:          e2eConfig.GetVariable("IMAGE_URL"),
			Checksum:     e2eConfig.GetVariable("IMAGE_CHECKSUM"),
			ChecksumType: metal3api.MD5,
		}
		result := runCumulativeStateRamp(ctx, "provisioned-scaling", namespace.Name, lab, imageSpec, metal3api.StateProvisioned, &createdBMHs)
		Logf("=== Ceiling for provisioned-state scaling: %d hosts clean (degraded at %d: %s) ===",
			result.MaxCleanScale, result.DegradedAtScale, result.DegradationReason)
	})

	It("should find the max concurrent provisioning count before retry rate exceeds threshold", func() {
		// Unlike the two cumulative-state tests above, each scale point here
		// is a fresh batch (created, measured, and cleaned up before the
		// next), since "at the same time" - not a standing count - is the
		// variable under test.
		cfg := loadRampConfig()
		imageSpec := &metal3api.Image{
			URL:          e2eConfig.GetVariable("IMAGE_URL"),
			Checksum:     e2eConfig.GetVariable("IMAGE_CHECKSUM"),
			ChecksumType: metal3api.MD5,
		}
		maxClean := 0
		for scale := cfg.start; scale <= cfg.maxScale; scale += cfg.step {
			result := runConcurrentProvisioningRound(ctx, "concurrent-retry", namespace.Name, lab, imageSpec, scale, cfg.roundTimeout, &createdBMHs)
			if result.TimedOut || result.RetryRate >= cfg.retryRateThreshold {
				Logf("=== Ceiling for concurrent provisioning: %d hosts clean, retry rate >= %.0f%% at %d ===",
					maxClean, cfg.retryRateThreshold*100, scale)
				return
			}
			maxClean = scale
		}
		Logf("=== Reached configured safety max scale %d without exceeding retry rate threshold ===", cfg.maxScale)
	})

	It("should plot provisioning time as a function of concurrent host count", func() {
		// Same round mechanics as the retry-rate test above, but the
		// deliverable is the full (hosts, provisioning time) series logged
		// as JSON at every scale point, not just the stopping point - collect
		// those log lines to plot the curve.
		cfg := loadRampConfig()
		imageSpec := &metal3api.Image{
			URL:          e2eConfig.GetVariable("IMAGE_URL"),
			Checksum:     e2eConfig.GetVariable("IMAGE_CHECKSUM"),
			ChecksumType: metal3api.MD5,
		}
		var baselineAvg time.Duration
		for scale := cfg.start; scale <= cfg.maxScale; scale += cfg.step {
			result := runConcurrentProvisioningRound(ctx, "provisioning-time-plot", namespace.Name, lab, imageSpec, scale, cfg.roundTimeout, &createdBMHs)
			avgDur := time.Duration(result.AvgProvisionDurationSec * float64(time.Second))
			if scale == cfg.start {
				baselineAvg = avgDur
			}
			degraded := result.TimedOut || result.RetryRate >= cfg.retryRateThreshold ||
				(baselineAvg > 0 && float64(avgDur) > cfg.timeMultiplier*float64(baselineAvg))
			if degraded {
				Logf("=== Plot data collection stopped at %d hosts (degradation signal) - see JSON log lines above for the full series ===", scale)
				return
			}
		}
		Logf("=== Reached configured safety max scale %d - see JSON log lines above for the full series ===", cfg.maxScale)
	})
})
