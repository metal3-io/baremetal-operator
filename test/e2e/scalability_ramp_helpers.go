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
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Env vars shared by every "ramp scale until degradation" test. The same
// knobs apply to all four ramps; run specs individually (GINKGO_FOCUS) with
// different env var values if one needs a different step/threshold than
// another.
const (
	envRampStartScale         = "SCALABILITY_FAKEIPA_RAMP_START"
	envRampStepScale          = "SCALABILITY_FAKEIPA_RAMP_STEP"
	envRampMaxScale           = "SCALABILITY_FAKEIPA_RAMP_MAX"
	envRampRoundTimeout       = "SCALABILITY_FAKEIPA_RAMP_ROUND_TIMEOUT"
	envRampTimeMultiplier     = "SCALABILITY_FAKEIPA_RAMP_TIME_MULTIPLIER"
	envRampRetryRateThreshold = "SCALABILITY_FAKEIPA_RETRY_RATE_THRESHOLD"

	defaultRampStart              = 5
	defaultRampStep               = 5
	defaultRampMax                = 25
	defaultRampRoundTimeout       = 10 * time.Minute
	defaultRampTimeMultiplier     = 2.0
	defaultRampRetryRateThreshold = 0.05
)

func getEnvFloatFakeIPA(env string, defaultVal float64) float64 {
	if val, ok := os.LookupEnv(env); ok {
		if f, err := strconv.ParseFloat(val, 64); err == nil && f > 0 {
			return f
		}
	}
	return defaultVal
}

// rampConfig holds the shared tuning knobs for every degradation ramp.
type rampConfig struct {
	start              int
	step               int
	maxScale           int
	roundTimeout       time.Duration
	timeMultiplier     float64
	retryRateThreshold float64
}

func loadRampConfig() rampConfig {
	return rampConfig{
		start:              getEnvIntFakeIPA(envRampStartScale, defaultRampStart),
		step:               getEnvIntFakeIPA(envRampStepScale, defaultRampStep),
		maxScale:           getEnvIntFakeIPA(envRampMaxScale, defaultRampMax),
		roundTimeout:       getEnvDurationFakeIPA(envRampRoundTimeout, defaultRampRoundTimeout),
		timeMultiplier:     getEnvFloatFakeIPA(envRampTimeMultiplier, defaultRampTimeMultiplier),
		retryRateThreshold: getEnvFloatFakeIPA(envRampRetryRateThreshold, defaultRampRetryRateThreshold),
	}
}

// bmhStatusSnapshot is a point-in-time read of the fields the ramps need from
// a BareMetalHost: current state, BMO's own retry/error counters, and
// Ironic/BMO's recorded provisioning duration.
type bmhStatusSnapshot struct {
	Name              string
	State             metal3api.ProvisioningState
	ErrorType         metal3api.ErrorType
	ErrorCount        int
	ProvisionDuration time.Duration
}

// fetchBMHStatuses re-reads each named BMH and returns a snapshot of the
// fields the ramps care about. Hosts that can't be read (e.g. already
// deleted) are silently skipped.
func fetchBMHStatuses(ctx context.Context, cl client.Client, namespace string, names []string) []bmhStatusSnapshot {
	snapshots := make([]bmhStatusSnapshot, 0, len(names))
	for _, name := range names {
		current := &metal3api.BareMetalHost{}
		key := types.NamespacedName{Namespace: namespace, Name: name}
		if err := cl.Get(ctx, key, current); err != nil {
			continue
		}
		snapshots = append(snapshots, bmhStatusSnapshot{
			Name:              current.Name,
			State:             current.Status.Provisioning.State,
			ErrorType:         current.Status.ErrorType,
			ErrorCount:        current.Status.ErrorCount,
			ProvisionDuration: current.Status.OperationHistory.Provision.Duration(),
		})
	}
	return snapshots
}

// statusPollInterval is how often waitForHostsReachState re-reads BMH status.
const statusPollInterval = 5 * time.Second

// waitForHostsReachState polls (rather than using Gomega's Eventually) until
// every named host reaches target or timeout elapses, returning whichever
// comes first without failing the spec: a round timing out is itself a
// degradation signal for these ramps, not a test bug.
func waitForHostsReachState(ctx context.Context, cl client.Client, namespace string, names []string, target metal3api.ProvisioningState, timeout time.Duration) (reachedAll bool, snapshots []bmhStatusSnapshot) {
	deadline := time.Now().Add(timeout)
	for {
		snapshots = fetchBMHStatuses(ctx, cl, namespace, names)
		reached := 0
		for _, s := range snapshots {
			if s.State == target {
				reached++
			}
		}
		if reached == len(names) {
			return true, snapshots
		}
		if time.Now().After(deadline) {
			return false, snapshots
		}
		time.Sleep(statusPollInterval)
	}
}

func avgMaxDuration(durations []time.Duration) (avg, maxDuration time.Duration) {
	if len(durations) == 0 {
		return 0, 0
	}
	var total time.Duration
	for _, d := range durations {
		total += d
		if d > maxDuration {
			maxDuration = d
		}
	}
	return total / time.Duration(len(durations)), maxDuration
}

// cumulativeRampResult summarizes a standing-population ramp.
type cumulativeRampResult struct {
	Mode              string
	MaxCleanScale     int
	DegradedAtScale   int
	DegradationReason string
}

// runCumulativeStateRamp keeps growing a standing population of BMHs (never
// deleted between rounds) by rampConfig.step at a time until "signs of
// instability" - any tracked host (old or new) showing a BMH error, a
// round's wait time blowing past rampConfig.timeMultiplier times the first
// round's baseline, or a round timing out outright - or until
// rampConfig.maxScale (safety cap) is reached. imageSpec nil drives hosts to
// Available; set it to drive them to Provisioned instead. Every created BMH
// is appended to *createdBMHs for the spec's AfterEach to clean up.
func runCumulativeStateRamp(ctx context.Context, mode, namespace string, lab *fakeIPALab, imageSpec *metal3api.Image, targetState metal3api.ProvisioningState, createdBMHs *[]metal3api.BareMetalHost) cumulativeRampResult {
	cfg := loadRampConfig()
	cl := clusterProxy.GetClient()
	systems := lab.Systems()

	var allNames []string
	total := 0
	var baselineDuration time.Duration
	round := 0

	for {
		round++
		batchSize := cfg.step
		if round == 1 {
			batchSize = cfg.start
		}
		if total+batchSize > cfg.maxScale {
			Logf("[%s] Reached configured safety max scale %d without detecting degradation", mode, cfg.maxScale)
			return cumulativeRampResult{Mode: mode, MaxCleanScale: total}
		}
		Expect(len(systems)).To(BeNumerically(">=", total+batchSize),
			"Fake IPA lab only has %d fake systems; run hack/fakeipa-lab-up.sh with at least %d before this ramp", len(systems), total+batchSize)

		By(fmt.Sprintf("[%s] Round %d: growing from %d to %d hosts", mode, round, total, total+batchSize))
		for i := total; i < total+batchSize; i++ {
			hostName := fmt.Sprintf("scale-%s-%04d", mode, i)
			bmh := newFakeIPABMH(ctx, cl, namespace, hostName, i, systems[i], imageSpec)
			Expect(cl.Create(ctx, &bmh)).To(Succeed(), "Failed to create BMH %s", hostName)
			*createdBMHs = append(*createdBMHs, bmh)
			allNames = append(allNames, hostName)
		}

		roundStart := time.Now()
		reachedAll, snapshots := waitForHostsReachState(ctx, cl, namespace, allNames, targetState, cfg.roundTimeout)
		roundDuration := time.Since(roundStart)
		total += batchSize

		errCount := 0
		for _, s := range snapshots {
			if s.ErrorType != "" {
				errCount++
				Logf("WARNING: BMH %s has error: %s", s.Name, s.ErrorType)
			}
		}

		memMB, err := sampleIronicMemoryMB(ctx, ironicNamespace)
		if err != nil {
			Logf("WARNING: failed to sample Ironic memory: %v", err)
		}

		Logf("=== %s ramp round %d: %d hosts, round duration %.1fs, errors %d, memory %.1f MiB ===",
			mode, round, total, roundDuration.Seconds(), errCount, memMB)
		if data, jsonErr := json.Marshal(map[string]any{
			"mode": mode, "round": round, "totalHosts": total,
			"roundDurationSec": roundDuration.Seconds(), "errorCount": errCount,
			"ironicMemoryMb": memMB,
		}); jsonErr == nil {
			Logf("  JSON: %s", data)
		}

		if round == 1 {
			baselineDuration = roundDuration
		}

		if !reachedAll {
			reason := fmt.Sprintf("round timed out after %s before all %d hosts reached %s", cfg.roundTimeout, total, targetState)
			Logf("[%s] DEGRADATION: %s", mode, reason)
			return cumulativeRampResult{Mode: mode, MaxCleanScale: total - batchSize, DegradedAtScale: total, DegradationReason: reason}
		}
		if errCount > 0 {
			reason := fmt.Sprintf("%d of %d hosts show errors", errCount, total)
			Logf("[%s] DEGRADATION: %s", mode, reason)
			return cumulativeRampResult{Mode: mode, MaxCleanScale: total - batchSize, DegradedAtScale: total, DegradationReason: reason}
		}
		if round > 1 && baselineDuration > 0 && float64(roundDuration) > cfg.timeMultiplier*float64(baselineDuration) {
			reason := fmt.Sprintf("round duration %.1fs exceeded %.1fx baseline %.1fs", roundDuration.Seconds(), cfg.timeMultiplier, baselineDuration.Seconds())
			Logf("[%s] DEGRADATION: %s", mode, reason)
			return cumulativeRampResult{Mode: mode, MaxCleanScale: total - batchSize, DegradedAtScale: total, DegradationReason: reason}
		}
	}
}

// percentMultiplier converts a 0-1 fraction (e.g. retryRate) to a percentage
// for logging.
const percentMultiplier = 100

// concurrentRoundResult is one scale point's measurement for a fresh-batch
// concurrent provisioning round.
type concurrentRoundResult struct {
	Scale                   int     `json:"scale"`
	BatchDurationSec        float64 `json:"batchDurationSec"`
	RetryRate               float64 `json:"retryRate"`
	HostsWithRetries        int     `json:"hostsWithRetries"`
	AvgProvisionDurationSec float64 `json:"avgProvisionDurationSec"`
	MaxProvisionDurationSec float64 `json:"maxProvisionDurationSec"`
	IronicMemoryMB          float64 `json:"ironicMemoryMb,omitempty"`
	TimedOut                bool    `json:"timedOut"`
}

// runConcurrentProvisioningRound measures one scale point: create scale BMHs
// that all provision at once, measure BMO's retry counter
// (Status.ErrorCount) and Ironic/BMO's own recorded per-host provisioning
// duration (Status.OperationHistory.Provision), then clean the batch up
// before returning - unlike the cumulative ramp, "at the same time" is the
// variable under test here, not a standing count.
func runConcurrentProvisioningRound(ctx context.Context, mode, namespace string, lab *fakeIPALab, imageSpec *metal3api.Image, scale int, roundTimeout time.Duration, createdBMHs *[]metal3api.BareMetalHost) concurrentRoundResult {
	cl := clusterProxy.GetClient()
	systems := lab.Systems()
	Expect(len(systems)).To(BeNumerically(">=", scale),
		"Fake IPA lab only has %d fake systems; run hack/fakeipa-lab-up.sh with at least %d before this round", len(systems), scale)

	By(fmt.Sprintf("[%s] Provisioning %d hosts concurrently", mode, scale))
	names := make([]string, 0, scale)
	bmhs := make([]metal3api.BareMetalHost, 0, scale)
	for i := range scale {
		hostName := fmt.Sprintf("scale-%s-%04d", mode, i)
		bmh := newFakeIPABMH(ctx, cl, namespace, hostName, i, systems[i], imageSpec)
		Expect(cl.Create(ctx, &bmh)).To(Succeed(), "Failed to create BMH %s", hostName)
		*createdBMHs = append(*createdBMHs, bmh)
		bmhs = append(bmhs, bmh)
		names = append(names, hostName)
	}

	roundStart := time.Now()
	reachedAll, snapshots := waitForHostsReachState(ctx, cl, namespace, names, metal3api.StateProvisioned, roundTimeout)
	roundDuration := time.Since(roundStart)

	retries := 0
	durations := make([]time.Duration, 0, len(snapshots))
	for _, s := range snapshots {
		if s.ErrorCount > 0 {
			retries++
		}
		if s.ProvisionDuration > 0 {
			durations = append(durations, s.ProvisionDuration)
		}
	}
	retryRate := 0.0
	if scale > 0 {
		retryRate = float64(retries) / float64(scale)
	}
	avg, maxDur := avgMaxDuration(durations)

	memMB, err := sampleIronicMemoryMB(ctx, ironicNamespace)
	if err != nil {
		Logf("WARNING: failed to sample Ironic memory: %v", err)
	}

	result := concurrentRoundResult{
		Scale:                   scale,
		BatchDurationSec:        roundDuration.Seconds(),
		RetryRate:               retryRate,
		HostsWithRetries:        retries,
		AvgProvisionDurationSec: avg.Seconds(),
		MaxProvisionDurationSec: maxDur.Seconds(),
		IronicMemoryMB:          memMB,
		TimedOut:                !reachedAll,
	}

	Logf("=== %s round: %d concurrent hosts, batch duration %.1fs, retry rate %.1f%% (%d/%d), avg provision %.1fs, max provision %.1fs, memory %.1f MiB ===",
		mode, scale, result.BatchDurationSec, retryRate*percentMultiplier, retries, scale, avg.Seconds(), maxDur.Seconds(), memMB)
	if data, jsonErr := json.Marshal(result); jsonErr == nil {
		Logf("  JSON: %s", data)
	}

	deleteTimeout := defaultRampRoundTimeout
	if timeouts := e2eConfig.GetIntervals("scalability-fakeipa", "wait-bmh-deleted"); len(timeouts) > 0 {
		if s, ok := timeouts[0].(string); ok {
			if d, err := time.ParseDuration(s); err == nil {
				deleteTimeout = d
			}
		}
	}
	cleanupBMHsParallel(ctx, cl, namespace, bmhs, deleteTimeout)

	return result
}
