//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

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
)

// getNumHosts returns the number of hosts to create for the scalability test.
func getNumHosts() int {
	if val, ok := os.LookupEnv(envScalabilityHosts); ok {
		n, err := strconv.Atoi(val)
		if err == nil && n > 0 {
			return n
		}
	}
	return defaultScalabilityHosts
}

// getMaxConcurrentReconciles returns the desired max-concurrent-reconciles value.
func getMaxConcurrentReconciles() int {
	if val, ok := os.LookupEnv(envMaxConcurrentReconciles); ok {
		n, err := strconv.Atoi(val)
		if err == nil && n > 0 {
			return n
		}
	}
	return defaultMaxConcurrentReconciles
}

// generateScalabilityMAC produces a deterministic, locally-administered MAC address from an index.
func generateScalabilityMAC(index int) string {
	return fmt.Sprintf("02:00:00:%02x:%02x:%02x",
		(index>>16)&0xFF,
		(index>>8)&0xFF,
		index&0xFF,
	)
}

// setConcurrentReconciles patches the BMO deployment to set --max-concurrent-reconciles
// and waits for the rollout to complete. It returns the original args so they can be
// restored in cleanup.
func setConcurrentReconciles(ctx context.Context, cl client.Client, concurrency int) []string {
	var originalArgs []string

	// Use retry loop to handle conflicts from concurrent Deployment updates
	Eventually(func(g Gomega) {
		deploy := &appsv1.Deployment{}
		key := types.NamespacedName{Name: bmoDeploymentName, Namespace: bmoNamespace}
		g.Expect(cl.Get(ctx, key, deploy)).To(Succeed(), "Failed to get BMO deployment")

		// Save original args on first successful Get
		if originalArgs == nil {
			container := &deploy.Spec.Template.Spec.Containers[0]
			originalArgs = make([]string, len(container.Args))
			copy(originalArgs, container.Args)
		}

		// Remove any existing --max-concurrent-reconciles arg
		container := &deploy.Spec.Template.Spec.Containers[0]
		newArgs := make([]string, 0, len(container.Args)+1)
		for _, arg := range container.Args {
			if len(arg) > 28 && arg[:28] == "--max-concurrent-reconciles=" {
				continue
			}
			if arg == "--max-concurrent-reconciles" {
				continue
			}
			newArgs = append(newArgs, arg)
		}
		newArgs = append(newArgs, fmt.Sprintf("--max-concurrent-reconciles=%d", concurrency))
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

	Logf("Set --max-concurrent-reconciles=%d on BMO deployment", concurrency)
	return originalArgs
}

// restoreBMOArgs restores the original container args on the BMO deployment.
func restoreBMOArgs(ctx context.Context, cl client.Client, originalArgs []string) {
	deploy := &appsv1.Deployment{}
	key := types.NamespacedName{Name: bmoDeploymentName, Namespace: bmoNamespace}
	if err := cl.Get(ctx, key, deploy); err != nil {
		Logf("WARNING: Failed to get BMO deployment for arg restoration: %v", err)
		return
	}

	deploy.Spec.Template.Spec.Containers[0].Args = originalArgs
	if err := cl.Update(ctx, deploy); err != nil {
		Logf("WARNING: Failed to restore BMO deployment args: %v", err)
	}
}

// removeFinalizers removes the BMO finalizer from all BMHs in the given namespace
// in parallel. This allows namespace deletion to proceed immediately without
// waiting for BMO to reconcile each host individually.
func removeFinalizers(ctx context.Context, cl client.Client, ns string) {
	bmhList := &metal3api.BareMetalHostList{}
	if err := cl.List(ctx, bmhList, client.InNamespace(ns)); err != nil {
		Logf("WARNING: Failed to list BMHs for finalizer removal: %v", err)
		return
	}

	var wg sync.WaitGroup
	sem := make(chan struct{}, 20)
	for i := range bmhList.Items {
		bmh := &bmhList.Items[i]
		if len(bmh.Finalizers) == 0 {
			continue
		}
		wg.Add(1)
		go func(b *metal3api.BareMetalHost) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			b.Finalizers = nil
			if err := cl.Update(ctx, b); err != nil {
				Logf("WARNING: Failed to remove finalizer from %s: %v", b.Name, err)
			}
		}(bmh)
	}
	wg.Wait()
	Logf("Removed finalizers from %d BMHs", len(bmhList.Items))
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
		numHosts = getNumHosts()
		concurrency = getMaxConcurrentReconciles()

		// Use a unique namespace per test run to avoid conflicts
		namespace, cancelWatches = framework.CreateNamespaceAndWatchEvents(ctx, framework.CreateNamespaceAndWatchEventsInput{
			Creator:   clusterProxy.GetClient(),
			ClientSet: clusterProxy.GetClientSet(),
			Name:      fmt.Sprintf("%s-%s", specName, util.RandomString(6)),
			LogFolder: artifactFolder,
		})

		// Set the desired concurrency on the BMO controller
		By(fmt.Sprintf("Setting BMO max-concurrent-reconciles to %d", concurrency))
		originalArgs = setConcurrentReconciles(ctx, clusterProxy.GetClient(), concurrency)
	})

	It("should enroll multiple BMHs within the time window", func() {
		cl := clusterProxy.GetClient()

		By(fmt.Sprintf("Creating %d BMH resources with credentials", numHosts))
		bmhs := make([]metal3api.BareMetalHost, numHosts)
		startTime := time.Now()

		for i := range numHosts {
			secretName := fmt.Sprintf("bmc-creds-%04d", i)
			hostName := fmt.Sprintf("scale-host-%04d", i)
			mac := generateScalabilityMAC(i)

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
			mac := generateScalabilityMAC(numHosts + i)

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

		// Submit all patches with limited concurrency to avoid client-side throttling.
		// Use concurrency/2 since each goroutine does GET+PUT (2 requests).
		patchWorkers := concurrency / 2
		if patchWorkers < 1 {
			patchWorkers = 1
		}
		var wg sync.WaitGroup
		sem := make(chan struct{}, patchWorkers)
		for i := range bmhs {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				defer GinkgoRecover()

				sem <- struct{}{}
				defer func() { <-sem }()

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

		// Start measuring provisioning duration AFTER all patches are submitted.
		// This isolates the controller processing time from the patch submission overhead.
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

	AfterEach(func() {
		if !skipCleanup {
			// Remove finalizers in bulk so namespace deletion doesn't have to wait
			// for BMO to reconcile each BMH individually. This cuts cleanup from
			// ~10 minutes to seconds.
			By("Removing finalizers from all BMHs in bulk")
			removeFinalizers(ctx, clusterProxy.GetClient(), namespace.Name)

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
		// Restore original BMO args after cleanup
		if originalArgs != nil {
			By("Restoring BMO deployment to original configuration")
			restoreBMOArgs(ctx, clusterProxy.GetClient(), originalArgs)
		}
	})
})
