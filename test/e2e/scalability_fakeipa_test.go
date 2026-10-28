//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"time"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/cluster-api/test/framework"
)

// Env vars controlling the FakeIPA scalability tests. This suite exercises
// the real agent-based deploy path (normal + live-ISO) via BMO + Ironic +
// Fake IPA, and samples Ironic's own memory, without relying on BMO's Fixture
// provisioner, or the fake-hardware driver in Ironic.
const (
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
		if !e2eConfig.GetBoolVariable("DEPLOY_IRONIC") {
			Skip("FakeIPA scalability tests require a real Ironic deployment")
		}
		createdBMHs = nil
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
		// So: clean up the BMHs and leave the namespace alone.
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
			lab.StopFakeIPA(ctx)
		}
		if cancelWatches != nil {
			cancelWatches()
		}
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
			result := runConcurrentProvisioningRound(ctx, clusterProxy, e2eConfig, "provisioning-time-plot", namespace.Name, lab, imageSpec, scale, cfg.roundTimeout, &createdBMHs)
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
