//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Env vars controlling the FakeIPA scalability tests. This suite exercises
// the real agent-based deploy path (normal + live-ISO) via BMO + Ironic +
// Fake IPA, and samples Ironic's own memory, without relying on BMO's Fixture
// provisioner, or the fake-hardware driver in Ironic.
const (
	// envScalabilityFakeIPAEnabled opts in to this suite. It needs a lab
	// (hack/fakeipa-lab-up.sh) that the stock real-Ironic e2e setup doesn't
	// create, so an unfiltered `make test-e2e` against ironic.yaml must skip it
	// rather than fail. Set by hack/fakeipa-test-e2e.sh.
	envScalabilityFakeIPAEnabled     = "SCALABILITY_FAKEIPA_ENABLED"
	envScalabilityFakeIPASystemsFile = "SCALABILITY_FAKEIPA_SYSTEMS_FILE"
	envScalabilityFakeIPAFakeImage   = "SCALABILITY_FAKEIPA_FAKEIPA_IMAGE"

	defaultFakeImage = "quay.io/metal3-io/fake-ipa:latest"
)

var _ = Describe("Scalability (FakeIPA)", Serial, Label("scalability", "fakeipa"), func() {
	var (
		specName      = "scalability-fakeipa"
		namespace     *corev1.Namespace
		cancelWatches context.CancelFunc
		lab           *fakeIPALab
		createdBMHs   []metal3api.BareMetalHost
	)

	BeforeEach(func() {
		// Reset before anything can Skip: AfterEach runs even for skipped
		// specs and must not act on a previous spec's state (e.g. stop a Fake
		// IPA container a previous spec deliberately left running).
		namespace, cancelWatches, lab, createdBMHs = nil, nil, nil, nil
		if os.Getenv(envScalabilityFakeIPAEnabled) != "true" {
			Skip("FakeIPA scalability tests are opt-in: run them via hack/fakeipa-test-e2e.sh (sets " + envScalabilityFakeIPAEnabled + "=true)")
		}
		if !e2eConfig.GetBoolVariable("DEPLOY_IRONIC") {
			Skip("FakeIPA scalability tests require a real Ironic deployment")
		}
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

		// A previous spec whose cleanup timed out leaves hosts still
		// deprovisioning in this shared namespace; measuring on top of them
		// would silently skew this spec's results.
		By("Checking that no BMHs are left over from a previous spec")
		var leftover metal3api.BareMetalHostList
		Expect(clusterProxy.GetClient().List(ctx, &leftover, client.InNamespace(namespace.Name))).To(Succeed())
		Expect(leftover.Items).To(BeEmpty(),
			"%d BMH(s) from a previous spec are still in %s; refusing to measure on top of them", len(leftover.Items), namespace.Name)

		By("Starting Fake IPA against the lab created by hack/fakeipa-lab-up.sh")
		var err error
		lab, err = startFakeIPA(ctx, systemsFile, fakeIPAImage, ironicURL)
		Expect(err).NotTo(HaveOccurred(), "Failed to start Fake IPA")
	})

	AfterEach(func() {
		// Nothing was set up (the spec skipped itself in BeforeEach).
		if namespace == nil {
			return
		}
		// scalability-fakeipa is a namespace-scoped (NAMESPACE_SCOPED=true)
		// BMO watch namespace, like basic-ops/provisioning-ops/etc - it's
		// created once by the BMO kustomization and reused across runs, not
		// torn down per spec. Deleting it here would race Kubernetes' own
		// namespace-content garbage collection: our namespace's RoleBinding/
		// Role (no finalizer) get GC'd before the BMHs (which do have a
		// finalizer) are gone, permanently stripping BMO's RBAC for this
		// namespace mid-cleanup and deadlocking the BMH finalizers forever.
		// So: clean up the BMHs and leave the namespace alone.
		var cleanupErr error
		if !skipCleanup {
			deleteTimeout := defaultRampRoundTimeout
			if timeouts := e2eConfig.GetIntervals(specName, "wait-bmh-deleted"); len(timeouts) > 0 {
				if s, ok := timeouts[0].(string); ok {
					if d, err := time.ParseDuration(s); err == nil {
						deleteTimeout = d
					}
				}
			}
			cleanupErr = cleanupBMHsParallel(ctx, clusterProxy.GetClient(), namespace.Name, createdBMHs, deleteTimeout)
		}
		// Fake IPA must stay up until every BMH above is confirmed deleted:
		// deprovisioning a BMH makes Ironic call back to sushy-tools, which
		// in turn tries to notify Fake IPA. Killing Fake IPA first causes
		// that notification to fail ("connection refused"), leaving the BMH
		// stuck in an error state with its finalizer never cleared. If cleanup
		// timed out, leave it running so the remaining hosts can still finish;
		// the next spec's startFakeIPA replaces the container.
		if lab != nil && cleanupErr == nil {
			lab.StopFakeIPA(ctx)
		}
		if cancelWatches != nil {
			cancelWatches()
		}
		Expect(cleanupErr).NotTo(HaveOccurred(),
			"BMH cleanup incomplete; Fake IPA left running so the remaining hosts can finish deprovisioning")
	})

	// Every spec below ramps scale by a configurable step (see
	// loadRampConfig) until a test-specific degradation signal fires,
	// reporting the last clean scale as the ceiling. Every round of every test
	// here samples Ironic's memory, so Ironic memory vs. node count data is
	// available for every test run.

	It("should find the max available-state BMH count before signs of instability", func() {
		// No Image spec: hosts register, skip inspection, and stay at
		// Available - this never needs Fake IPA to boot anything.
		result := runCumulativeStateRamp(ctx, clusterProxy, "available-scaling", namespace.Name, lab, nil, metal3api.StateAvailable, &createdBMHs)
		Logf("=== Ceiling for available-state scaling: %d hosts clean (degraded at %d: %s) ===",
			result.MaxCleanScale, result.DegradedAtScale, result.DegradationReason)
	})

	It("should find the max provisioned-state BMH count before signs of instability (normal mode)", func() {
		// Same ramp as the available-state test, but driven all the way to
		// Provisioned via a real Fake IPA deploy, and the population is left
		// standing (not deprovisioned) between rounds.
		imageSpec := &metal3api.Image{
			URL:          e2eConfig.GetVariable("IMAGE_URL"),
			Checksum:     e2eConfig.GetVariable("IMAGE_CHECKSUM"),
			ChecksumType: metal3api.MD5,
		}
		result := runCumulativeStateRamp(ctx, clusterProxy, "provisioned-scaling-normal", namespace.Name, lab, imageSpec, metal3api.StateProvisioned, &createdBMHs)
		Logf("=== Ceiling for provisioned-state scaling (normal mode): %d hosts clean (degraded at %d: %s) ===",
			result.MaxCleanScale, result.DegradedAtScale, result.DegradationReason)
	})

	It("should find the max provisioned-state BMH count before signs of instability (live-ISO mode)", func() {
		// Same ramp, deployed via live-ISO instead of a normal image - the
		// only mechanical difference from the normal-mode spec above.
		imageSpec := &metal3api.Image{
			URL:        e2eConfig.GetVariable("ISO_IMAGE_URL"),
			DiskFormat: ptr.To("live-iso"),
		}
		result := runCumulativeStateRamp(ctx, clusterProxy, "provisioned-scaling-live-iso", namespace.Name, lab, imageSpec, metal3api.StateProvisioned, &createdBMHs)
		Logf("=== Ceiling for provisioned-state scaling (live-ISO mode): %d hosts clean (degraded at %d: %s) ===",
			result.MaxCleanScale, result.DegradedAtScale, result.DegradationReason)
	})

	It("should find the max concurrent provisioning count before retry rate exceeds threshold", func() {
		// Unlike the cumulative-state tests above, each scale point here is
		// a fresh batch (created, measured, and cleaned up before the
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
			result := runConcurrentProvisioningRound(ctx, clusterProxy, e2eConfig, "concurrent-retry", namespace.Name, lab, imageSpec, scale, cfg.roundTimeout, &createdBMHs)
			if result.TimedOut || result.RetryRate >= cfg.retryRateThreshold {
				var reasons []string
				if result.TimedOut {
					reasons = append(reasons, fmt.Sprintf("round timed out after %s", cfg.roundTimeout))
				}
				if result.RetryRate >= cfg.retryRateThreshold {
					reasons = append(reasons, fmt.Sprintf("retry rate %.1f%% >= %.0f%%",
						result.RetryRate*percentMultiplier, cfg.retryRateThreshold*percentMultiplier))
				}
				Logf("=== Ceiling for concurrent provisioning: %d hosts clean, stopped at %d: %s ===",
					maxClean, scale, strings.Join(reasons, ", "))
				return
			}
			maxClean = scale
		}
		Logf("=== Reached configured safety max scale %d without exceeding retry rate threshold ===", cfg.maxScale)
	})

	It("should plot provisioning time as a function of concurrent host count", func() {
		// Same round mechanics as the retry-rate test above, but every scale
		// point is kept and, once the ramp ends (configured max reached,
		// degradation signal, or a spec failure mid-ramp), rendered as an SVG
		// line chart of per-host avg/max provisioning time and batch
		// wall-clock time against concurrent host count, alongside the raw
		// series as CSV, under artifactFolder/<specName>/.
		cfg := loadRampConfig()
		imageSpec := &metal3api.Image{
			URL:          e2eConfig.GetVariable("IMAGE_URL"),
			Checksum:     e2eConfig.GetVariable("IMAGE_CHECKSUM"),
			ChecksumType: metal3api.MD5,
		}
		var results []concurrentRoundResult
		degradedAtScale := 0
		defer func() {
			if len(results) == 0 {
				Logf("WARNING: no provisioning rounds completed; skipping provisioning-time plot")
				return
			}
			svgPath, csvPath, err := writeProvisioningTimePlot(filepath.Join(artifactFolder, specName), results, degradedAtScale)
			if err != nil {
				Logf("WARNING: failed to write provisioning-time plot: %v", err)
				return
			}
			Logf("=== Provisioning time vs. concurrent host count plot: %s (data: %s) ===", svgPath, csvPath)
		}()

		var baselineAvg time.Duration
		for scale := cfg.start; scale <= cfg.maxScale; scale += cfg.step {
			result := runConcurrentProvisioningRound(ctx, clusterProxy, e2eConfig, "provisioning-time-plot", namespace.Name, lab, imageSpec, scale, cfg.roundTimeout, &createdBMHs)
			results = append(results, result)
			avgDur := time.Duration(result.AvgProvisionDurationSec * float64(time.Second))
			if scale == cfg.start {
				baselineAvg = avgDur
			}
			degraded := result.TimedOut || result.RetryRate >= cfg.retryRateThreshold ||
				(baselineAvg > 0 && float64(avgDur) > cfg.timeMultiplier*float64(baselineAvg))
			if degraded {
				degradedAtScale = scale
				Logf("=== Plot data collection stopped at %d hosts (degradation signal) ===", scale)
				return
			}
		}
		Logf("=== Reached configured safety max scale %d ===", cfg.maxScale)
	})
})
