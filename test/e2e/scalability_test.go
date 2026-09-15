//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/nodes"
	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	defaultScalabilityHosts        = 10
	defaultMaxConcurrentReconciles = 3
	envScalabilityHosts            = "SCALABILITY_NUM_HOSTS"
	envMaxConcurrentReconciles     = "SCALABILITY_MAX_CONCURRENT_RECONCILES"
	bmoDeploymentName              = "baremetal-operator-controller-manager"
	bmoNamespace                   = "baremetal-operator-system"

	// API latency test constants.
	defaultAPILatencyBatchSize    = 10
	defaultAPILatencyMaxHosts     = 100
	defaultAPILatencyThresholdMs  = 5000
	defaultAPILatencyMeasurements = 5
	envAPILatencyBatchSize        = "SCALABILITY_API_BATCH_SIZE"
	envAPILatencyMaxHosts         = "SCALABILITY_API_MAX_HOSTS"
	envAPILatencyThresholdMs      = "SCALABILITY_API_THRESHOLD_MS"
	envAPILatencyMeasurements     = "SCALABILITY_API_MEASUREMENTS"
)

// getEnvInt reads an environment variable as a positive integer, returning
// the given default if unset, empty, or invalid.
func getEnvInt(env string, defaultVal int) int {
	if val, ok := os.LookupEnv(env); ok {
		n, err := strconv.Atoi(val)
		if err == nil && n > 0 {
			return n
		}
	}
	return defaultVal
}

// macFromIndex returns a locally-administered MAC address (02:00:00:xx:xx:xx)
// deterministically derived from the given index.
func macFromIndex(index int) string {
	return fmt.Sprintf("02:00:00:%02x:%02x:%02x",
		(index>>16)&0xFF,
		(index>>8)&0xFF,
		index&0xFF,
	)
}

// setConcurrentReconciles patches the BMO deployment to set --controller-concurrency
// and waits for the rollout to complete.
func setConcurrentReconciles(ctx context.Context, cl client.Client, concurrency int) {
	// Use retry loop to handle conflicts from concurrent Deployment updates
	Eventually(func(g Gomega) {
		deploy := &appsv1.Deployment{}
		key := types.NamespacedName{Name: bmoDeploymentName, Namespace: bmoNamespace}
		g.Expect(cl.Get(ctx, key, deploy)).To(Succeed(), "Failed to get BMO deployment")

		// Remove any existing --controller-concurrency arg
		container := &deploy.Spec.Template.Spec.Containers[0]
		newArgs := make([]string, 0, len(container.Args)+1)
		skipNext := false
		for _, arg := range container.Args {
			if skipNext {
				skipNext = false
				continue
			}
			if strings.HasPrefix(arg, "--controller-concurrency=") {
				continue
			}
			if arg == "--controller-concurrency" {
				skipNext = true
				continue
			}
			newArgs = append(newArgs, arg)
		}
		newArgs = append(newArgs, fmt.Sprintf("--controller-concurrency=%d", concurrency))
		container.Args = newArgs

		g.Expect(cl.Update(ctx, deploy)).To(Succeed(), "Failed to patch BMO deployment with concurrency=%d", concurrency)
	}, "30s", "2s").Should(Succeed(), "Failed to set concurrency on BMO deployment after retries")

	// Wait for rollout to complete
	key := types.NamespacedName{Name: bmoDeploymentName, Namespace: bmoNamespace}
	Eventually(func(g Gomega) {
		current := &appsv1.Deployment{}
		g.Expect(cl.Get(ctx, key, current)).To(Succeed())
		g.Expect(current.Status.UpdatedReplicas).To(Equal(*current.Spec.Replicas))
		g.Expect(current.Status.ReadyReplicas).To(Equal(*current.Spec.Replicas))
		g.Expect(current.Status.ObservedGeneration).To(Equal(current.Generation))
	}, "2m", "2s").Should(Succeed(), "BMO deployment rollout did not complete")

	Logf("Set --controller-concurrency=%d on BMO deployment", concurrency)
}

// scalabilityResults holds the test results for structured logging.
type scalabilityResults struct {
	Phase       string  `json:"phase"`
	NumHosts    int     `json:"numHosts"`
	Concurrency int     `json:"maxConcurrentReconciles"`
	DurationSec float64 `json:"durationSeconds"`
	Throughput  float64 `json:"throughputHostsPerMin"`
}

func logResults(r scalabilityResults) {
	Logf("=== %s Results ===", r.Phase)
	Logf("  Hosts:              %d", r.NumHosts)
	Logf("  Concurrency:        %d", r.Concurrency)
	Logf("  Duration:           %.1fs", r.DurationSec)
	Logf("  Throughput:         %.1f hosts/min", r.Throughput)
	jsonBytes, err := json.Marshal(r)
	if err == nil {
		Logf("  JSON: %s", string(jsonBytes))
	}
}

// avgDuration returns the mean of a slice of durations.
func avgDuration(durations []time.Duration) time.Duration {
	if len(durations) == 0 {
		return 0
	}
	var total time.Duration
	for _, d := range durations {
		total += d
	}
	return total / time.Duration(len(durations))
}

// maxDuration returns the maximum value from a slice of durations.
func maxDuration(durations []time.Duration) time.Duration {
	if len(durations) == 0 {
		return 0
	}
	m := durations[0]
	for _, d := range durations[1:] {
		if d > m {
			m = d
		}
	}
	return m
}

// Scalability tests are Serial to avoid namespace conflicts when Ginkgo
// runs multiple parallel nodes. They create many resources and should
// not compete for controller capacity with other tests.
var _ = Describe("Scalability", Serial, Label("scalability"), func() {
	var (
		specName      = "scalability"
		namespace     *corev1.Namespace
		cancelWatches context.CancelFunc
		numHosts      int
		concurrency   int
		originalArgs  []string
	)

	BeforeEach(func() {
		numHosts = getEnvInt(envScalabilityHosts, defaultScalabilityHosts)
		concurrency = getEnvInt(envMaxConcurrentReconciles, defaultMaxConcurrentReconciles)

		namespace, cancelWatches = framework.CreateNamespaceAndWatchEvents(ctx, framework.CreateNamespaceAndWatchEventsInput{
			Creator:   clusterProxy.GetClient(),
			ClientSet: clusterProxy.GetClientSet(),
			Name:      fmt.Sprintf("%s-%s", specName, util.RandomString(6)),
			LogFolder: artifactFolder,
		})

		// Save original BMO deployment args before modifying concurrency
		deploy := &appsv1.Deployment{}
		key := types.NamespacedName{Name: bmoDeploymentName, Namespace: bmoNamespace}
		Expect(clusterProxy.GetClient().Get(ctx, key, deploy)).To(Succeed())
		originalArgs = make([]string, len(deploy.Spec.Template.Spec.Containers[0].Args))
		copy(originalArgs, deploy.Spec.Template.Spec.Containers[0].Args)

		By(fmt.Sprintf("Setting BMO controller-concurrency to %d", concurrency))
		setConcurrentReconciles(ctx, clusterProxy.GetClient(), concurrency)
	})

	It("should enroll multiple BMHs within the time window", func() {
		cl := clusterProxy.GetClient()

		By(fmt.Sprintf("Creating %d BMH resources with credentials", numHosts))
		bmhs := make([]metal3api.BareMetalHost, numHosts)
		startTime := time.Now()

		for i := range numHosts {
			secretName := fmt.Sprintf("bmc-creds-%04d", i)
			hostName := fmt.Sprintf("scale-host-%04d", i)
			mac := macFromIndex(i)

			CreateSecret(ctx, cl, namespace.Name, secretName, map[string]string{
				"username": "admin",
				"password": "password",
			})

			bmh := metal3api.BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      hostName,
					Namespace: namespace.Name,
				},
				Spec: metal3api.BareMetalHostSpec{
					Online: true,
					BMC: metal3api.BMCDetails{
						Address:                        fmt.Sprintf("redfish+http://192.168.222.1:8000/redfish/v1/Systems/%08d", i),
						CredentialsName:                secretName,
						DisableCertificateVerification: true,
					},
					BootMACAddress:        mac,
					BootMode:              metal3api.UEFI,
					AutomatedCleaningMode: metal3api.CleaningModeDisabled,
					InspectionMode:        metal3api.InspectionModeDisabled,
				},
			}
			Expect(cl.Create(ctx, &bmh)).To(Succeed(), "Failed to create BMH %s", hostName)
			bmhs[i] = bmh
		}

		createDuration := time.Since(startTime)
		Logf("Created %d BMH resources in %s", numHosts, createDuration)

		By(fmt.Sprintf("Waiting for all %d BMHs to reach 'available' state", numHosts))
		enrollStart := time.Now()

		Eventually(func(g Gomega) {
			availableCount := 0
			for i := range bmhs {
				current := &metal3api.BareMetalHost{}
				key := types.NamespacedName{Namespace: bmhs[i].Namespace, Name: bmhs[i].Name}
				g.Expect(cl.Get(ctx, key, current)).To(Succeed())

				state := current.Status.Provisioning.State
				if state == metal3api.StateAvailable {
					availableCount++
				}
				if current.Status.ErrorType != "" {
					Logf("WARNING: BMH %s has error: %s - %s", current.Name, current.Status.ErrorType, current.Status.ErrorMessage)
				}
			}
			g.Expect(availableCount).To(Equal(numHosts),
				fmt.Sprintf("Expected %d hosts available, got %d", numHosts, availableCount))
		}, e2eConfig.GetIntervals(specName, "wait-enrollment")...).Should(Succeed())

		enrollDuration := time.Since(enrollStart)
		logResults(scalabilityResults{
			Phase:       "Enrollment",
			NumHosts:    numHosts,
			Concurrency: concurrency,
			DurationSec: enrollDuration.Seconds(),
			Throughput:  float64(numHosts) / enrollDuration.Seconds() * 60,
		})
	})

	It("should provision multiple BMHs within the time window", func() {
		cl := clusterProxy.GetClient()

		By(fmt.Sprintf("Creating %d BMH resources with credentials", numHosts))
		bmhs := make([]metal3api.BareMetalHost, numHosts)

		for i := range numHosts {
			secretName := fmt.Sprintf("bmc-creds-%04d", i)
			hostName := fmt.Sprintf("scale-prov-%04d", i)
			mac := macFromIndex(numHosts + i)

			CreateSecret(ctx, cl, namespace.Name, secretName, map[string]string{
				"username": "admin",
				"password": "password",
			})

			bmh := metal3api.BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      hostName,
					Namespace: namespace.Name,
				},
				Spec: metal3api.BareMetalHostSpec{
					Online: true,
					BMC: metal3api.BMCDetails{
						Address:                        fmt.Sprintf("redfish+http://192.168.222.1:8000/redfish/v1/Systems/%08d", numHosts+i),
						CredentialsName:                secretName,
						DisableCertificateVerification: true,
					},
					BootMACAddress:        mac,
					BootMode:              metal3api.UEFI,
					AutomatedCleaningMode: metal3api.CleaningModeDisabled,
					InspectionMode:        metal3api.InspectionModeDisabled,
				},
			}
			Expect(cl.Create(ctx, &bmh)).To(Succeed(), "Failed to create BMH %s", hostName)
			bmhs[i] = bmh
		}

		By(fmt.Sprintf("Waiting for all %d BMHs to reach 'available' state", numHosts))
		Eventually(func(g Gomega) {
			availableCount := 0
			for i := range bmhs {
				current := &metal3api.BareMetalHost{}
				key := types.NamespacedName{Namespace: bmhs[i].Namespace, Name: bmhs[i].Name}
				g.Expect(cl.Get(ctx, key, current)).To(Succeed())
				if current.Status.Provisioning.State == metal3api.StateAvailable {
					availableCount++
				}
			}
			g.Expect(availableCount).To(Equal(numHosts))
		}, e2eConfig.GetIntervals(specName, "wait-enrollment")...).Should(Succeed())

		By(fmt.Sprintf("Patching all %d BMHs with an image to trigger provisioning", numHosts))

		imageURL := e2eConfig.GetVariable("IMAGE_URL")
		imageChecksum := e2eConfig.GetVariable("IMAGE_CHECKSUM")

		// Submit all patches concurrently.
		var wg sync.WaitGroup
		for i := range bmhs {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				defer GinkgoRecover()

				current := &metal3api.BareMetalHost{}
				key := types.NamespacedName{Namespace: bmhs[idx].Namespace, Name: bmhs[idx].Name}
				Expect(cl.Get(ctx, key, current)).To(Succeed())

				current.Spec.Image = &metal3api.Image{
					URL:          imageURL,
					Checksum:     imageChecksum,
					ChecksumType: metal3api.MD5,
				}
				current.Spec.RootDeviceHints = &metal3api.RootDeviceHints{
					DeviceName: "/dev/vda",
				}
				Expect(cl.Update(ctx, current)).To(Succeed())
			}(i)
		}
		wg.Wait()

		By(fmt.Sprintf("Waiting for all %d BMHs to reach 'provisioned' state", numHosts))
		provisionStart := time.Now()

		Eventually(func(g Gomega) {
			provisionedCount := 0
			for i := range bmhs {
				current := &metal3api.BareMetalHost{}
				key := types.NamespacedName{Namespace: bmhs[i].Namespace, Name: bmhs[i].Name}
				g.Expect(cl.Get(ctx, key, current)).To(Succeed())
				if current.Status.Provisioning.State == metal3api.StateProvisioned {
					provisionedCount++
				}
			}
			g.Expect(provisionedCount).To(Equal(numHosts),
				fmt.Sprintf("Expected %d hosts provisioned, got %d", numHosts, provisionedCount))
		}, e2eConfig.GetIntervals(specName, "wait-provisioning")...).Should(Succeed())

		provisionDuration := time.Since(provisionStart)
		logResults(scalabilityResults{
			Phase:       "Provisioning",
			NumHosts:    numHosts,
			Concurrency: concurrency,
			DurationSec: provisionDuration.Seconds(),
			Throughput:  float64(numHosts) / provisionDuration.Seconds() * 60,
		})
	})

	It("should keep Ironic API response times below threshold as hosts scale up", Label("ironic"), func() {
		batchSize := getEnvInt(envAPILatencyBatchSize, defaultAPILatencyBatchSize)
		maxHosts := getEnvInt(envAPILatencyMaxHosts, defaultAPILatencyMaxHosts)
		thresholdMs := getEnvInt(envAPILatencyThresholdMs, defaultAPILatencyThresholdMs)
		measurements := getEnvInt(envAPILatencyMeasurements, defaultAPILatencyMeasurements)
		threshold := time.Duration(thresholdMs) * time.Millisecond

		ironicClient := CreateIronicClient(e2eConfig)

		Logf("API Latency Test Configuration:")
		Logf("  Batch size:        %d hosts", batchSize)
		Logf("  Max hosts:         %d", maxHosts)
		Logf("  Threshold:         %s", threshold)
		Logf("  Measurements/step: %d", measurements)

		type latencySnapshot struct {
			TotalHosts  int     `json:"totalHosts"`
			ListNodeAvg float64 `json:"listNodeAvgSeconds"`
			ListNodeMax float64 `json:"listNodeMaxSeconds"`
			GetNodeAvg  float64 `json:"getNodeAvgSeconds"`
			GetNodeMax  float64 `json:"getNodeMaxSeconds"`
		}

		var allSnapshots []latencySnapshot
		var createdNodeUUIDs []string
		totalCreated := 0
		thresholdBreached := false
		var breachSnapshot latencySnapshot

		defer func() {
			// Clean up: delete all nodes we created directly in Ironic
			By(fmt.Sprintf("Cleaning up %d Ironic nodes", len(createdNodeUUIDs)))
			for _, uuid := range createdNodeUUIDs {
				_ = nodes.Delete(ctx, ironicClient, uuid).ExtractErr()
			}
		}()

		for batch := 0; !thresholdBreached && totalCreated < maxHosts; batch++ {
			batchStart := totalCreated
			batchEnd := totalCreated + batchSize
			if batchEnd > maxHosts {
				batchEnd = maxHosts
			}

			By(fmt.Sprintf("Creating batch %d: nodes %d-%d directly in Ironic", batch+1, batchStart+1, batchEnd))
			for i := batchStart; i < batchEnd; i++ {
				nodeName := fmt.Sprintf("scale-api-node-%04d", i)
				createOpts := nodes.CreateOpts{
					Driver: "fake-hardware",
					Name:   nodeName,
					Properties: map[string]any{
						"cpu_arch": "x86_64",
					},
				}
				node, err := nodes.Create(ctx, ironicClient, createOpts).Extract()
				Expect(err).NotTo(HaveOccurred(), "Failed to create Ironic node %s", nodeName)
				createdNodeUUIDs = append(createdNodeUUIDs, node.UUID)
			}
			totalCreated = batchEnd

			By(fmt.Sprintf("Measuring Ironic API latency with %d nodes registered", totalCreated))

			// Measure node list latency
			listDurations := make([]time.Duration, measurements)
			for m := range measurements {
				start := time.Now()
				pager := nodes.List(ironicClient, nodes.ListOpts{
					Fields: []string{"uuid,name,provision_state"},
				})
				_, err := pager.AllPages(ctx)
				listDurations[m] = time.Since(start)
				Expect(err).NotTo(HaveOccurred(), "Failed to list Ironic nodes")
			}

			// Measure single node get latency (first and last node)
			getDurations := make([]time.Duration, 0, measurements*2)
			sampleNodes := []string{
				"scale-api-node-0000",
				fmt.Sprintf("scale-api-node-%04d", totalCreated-1),
			}
			for _, nodeName := range sampleNodes {
				for range measurements {
					start := time.Now()
					_, err := nodes.Get(ctx, ironicClient, nodeName).Extract()
					getDurations = append(getDurations, time.Since(start))
					Expect(err).NotTo(HaveOccurred(), "Failed to get Ironic node %s", nodeName)
				}
			}

			snapshot := latencySnapshot{
				TotalHosts:  totalCreated,
				ListNodeAvg: avgDuration(listDurations).Seconds(),
				ListNodeMax: maxDuration(listDurations).Seconds(),
				GetNodeAvg:  avgDuration(getDurations).Seconds(),
				GetNodeMax:  maxDuration(getDurations).Seconds(),
			}
			allSnapshots = append(allSnapshots, snapshot)

			Logf("--- Hosts: %d | List avg: %.3fs max: %.3fs | Get avg: %.3fs max: %.3fs",
				snapshot.TotalHosts,
				snapshot.ListNodeAvg, snapshot.ListNodeMax,
				snapshot.GetNodeAvg, snapshot.GetNodeMax)

			// Check if threshold is breached
			if snapshot.ListNodeMax > threshold.Seconds() || snapshot.GetNodeMax > threshold.Seconds() {
				thresholdBreached = true
				breachSnapshot = snapshot
			}
		}

		// Log final summary
		By("Logging API latency scalability results")
		jsonBytes, err := json.MarshalIndent(allSnapshots, "", "  ")
		if err == nil {
			Logf("API Latency Snapshots:\n%s", string(jsonBytes))
		}

		if thresholdBreached {
			Fail(fmt.Sprintf(
				"Ironic API response time exceeded threshold of %s at %d hosts. "+
					"List max: %.3fs, Get max: %.3fs",
				threshold, breachSnapshot.TotalHosts,
				breachSnapshot.ListNodeMax, breachSnapshot.GetNodeMax))
		}

		Logf("SUCCESS: Ironic API stayed below %s threshold with %d hosts registered", threshold, totalCreated)
	})

	AfterEach(func() {
		if !skipCleanup {
			By("Deleting all BMHs before deleting the namespace")
			DeleteBmhsInNamespace(ctx, clusterProxy.GetClient(), namespace.Name)

			By("Deleting test namespace (cascading delete of all resources)")
			framework.DeleteNamespace(ctx, framework.DeleteNamespaceInput{
				Deleter: clusterProxy.GetClient(),
				Name:    namespace.Name,
			})
			WaitForNamespaceDeleted(ctx, WaitForNamespaceDeletedInput{
				Getter:    clusterProxy.GetClient(),
				Namespace: *namespace,
			}, e2eConfig.GetIntervals(specName, "wait-namespace-deleted")...)
			if cancelWatches != nil {
				cancelWatches()
			}
		}

		By("Restoring original BMO deployment args")
		Eventually(func(g Gomega) {
			deploy := &appsv1.Deployment{}
			key := types.NamespacedName{Name: bmoDeploymentName, Namespace: bmoNamespace}
			g.Expect(clusterProxy.GetClient().Get(ctx, key, deploy)).To(Succeed())
			deploy.Spec.Template.Spec.Containers[0].Args = originalArgs
			g.Expect(clusterProxy.GetClient().Update(ctx, deploy)).To(Succeed())
		}, "30s", "2s").Should(Succeed(), "Failed to restore BMO deployment args")
	})
})
