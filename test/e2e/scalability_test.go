//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strconv"
	"sync"
	"time"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/nodes"
	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	defaultScalabilityHosts = 10
	envScalabilityHosts     = "SCALABILITY_NUM_HOSTS"

	defaultAPILatencyThresholdMs  = 5000
	defaultAPILatencyMeasurements = 5
	envAPILatencyThresholdMs      = "SCALABILITY_API_THRESHOLD_MS"
	envAPILatencyMeasurements     = "SCALABILITY_API_MEASUREMENTS"

	// scalabilityBMCEndpoint is the BMC emulator (sushy-tools) address as seen
	// from the management cluster.
	scalabilityBMCEndpoint = "192.168.222.1:8000"

	// patchConcurrency bounds the number of BMH patches in flight at once.
	patchConcurrency = 50
)

// systemUUID returns the UUID of the sushy-tools fake Redfish system with the
// given index. The fake driver requires system ids to be real UUIDs. They are
// derived from the index so that hack/ci-e2e.sh, which generates the fake
// systems, and the tests agree without sharing a file. Keep the format in sync
// with hack/e2e/sushy-fake-config.sh.
func systemUUID(index int) string {
	return fmt.Sprintf("00000000-0000-4000-8000-%012x", index)
}

// getEnvInt reads a variable from the e2e config (or environment) as a positive
// integer, returning defaultVal if it isn't set.
func getEnvInt(varName string, defaultVal int) int {
	if !e2eConfig.HasVariable(varName) {
		return defaultVal
	}
	value, err := strconv.Atoi(e2eConfig.GetVariable(varName))
	Expect(err).NotTo(HaveOccurred(), "variable %s must be an integer", varName)
	Expect(value).To(BeNumerically(">", 0), "variable %s must be positive", varName)
	return value
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

// scalabilityResults holds the test results for structured logging.
type scalabilityResults struct {
	Phase       string  `json:"phase"`
	NumHosts    int     `json:"numHosts"`
	DurationSec float64 `json:"durationSeconds"`
	Throughput  float64 `json:"throughputHostsPerMin"`
}

func logResults(r scalabilityResults) {
	Logf("=== %s Results ===", r.Phase)
	Logf("  Hosts:              %d", r.NumHosts)
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

type latencySnapshot struct {
	Phase       string  `json:"phase"`
	TotalHosts  int     `json:"totalHosts"`
	ListNodeAvg float64 `json:"listNodeAvgSeconds"`
	ListNodeMax float64 `json:"listNodeMaxSeconds"`
	GetNodeAvg  float64 `json:"getNodeAvgSeconds"`
	GetNodeMax  float64 `json:"getNodeMaxSeconds"`
}

// measureIronicLatency times node list and node get calls against Ironic
// and appends the result to snapshots.
func measureIronicLatency(ctx context.Context, ironicClient *gophercloud.ServiceClient, snapshots []latencySnapshot,
	phase string, bmhs []metal3api.BareMetalHost, measurements int) []latencySnapshot {
	if ironicClient == nil {
		return snapshots
	}

	By(fmt.Sprintf("Measuring Ironic API latency (%s, %d hosts)", phase, len(bmhs)))

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

	getDurations := make([]time.Duration, 0, measurements*2)
	sampleNodes := []string{
		IronicNodeName(bmhs[0].Namespace, bmhs[0].Name),
		IronicNodeName(bmhs[len(bmhs)-1].Namespace, bmhs[len(bmhs)-1].Name),
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
		Phase:       phase,
		TotalHosts:  len(bmhs),
		ListNodeAvg: avgDuration(listDurations).Seconds(),
		ListNodeMax: maxDuration(listDurations).Seconds(),
		GetNodeAvg:  avgDuration(getDurations).Seconds(),
		GetNodeMax:  maxDuration(getDurations).Seconds(),
	}

	Logf("--- %s | Hosts: %d | List avg: %.3fs max: %.3fs | Get avg: %.3fs max: %.3fs",
		snapshot.Phase, snapshot.TotalHosts,
		snapshot.ListNodeAvg, snapshot.ListNodeMax,
		snapshot.GetNodeAvg, snapshot.GetNodeMax)

	return append(snapshots, snapshot)
}

// checkIronicLatency logs all snapshots and fails the test if any of them
// exceeded the threshold.
func checkIronicLatency(snapshots []latencySnapshot, threshold time.Duration) {
	if len(snapshots) == 0 {
		return
	}

	By("Logging API latency scalability results")
	jsonBytes, err := json.MarshalIndent(snapshots, "", "  ")
	if err == nil {
		Logf("API Latency Snapshots:\n%s", string(jsonBytes))
	}

	for _, snapshot := range snapshots {
		if snapshot.ListNodeMax > threshold.Seconds() || snapshot.GetNodeMax > threshold.Seconds() {
			Fail(fmt.Sprintf(
				"Ironic API response time exceeded threshold of %s during %s with %d hosts. "+
					"List max: %.3fs, Get max: %.3fs",
				threshold, snapshot.Phase, snapshot.TotalHosts,
				snapshot.ListNodeMax, snapshot.GetNodeMax))
		}
	}

	Logf("SUCCESS: Ironic API stayed below %s threshold", threshold)
}

// newUnthrottledClient returns a client to the management cluster without
// client-side rate limiting. The suite client uses client-go's default of 5
// QPS, which would make every create, patch and poll of hundreds of hosts
// take numHosts/5 seconds and turn the measured durations into a measurement
// of the rate limiter instead of BMO.
func newUnthrottledClient() client.Client {
	cfg := rest.CopyConfig(clusterProxy.GetRESTConfig())
	// A negative QPS disables the client-side rate limiter.
	cfg.QPS = -1
	cl, err := client.New(cfg, client.Options{Scheme: clusterProxy.GetScheme()})
	Expect(err).NotTo(HaveOccurred(), "Failed to create unthrottled client")
	return cl
}

// createScalabilityHosts creates numHosts BMHs, each with its own BMC
// credentials secret (appended to toCleanup), and returns the created BMHs.
// The BMC address points to the sushy-tools fake system with the UUID of the
// same index (see systemUUID) and the boot MAC address matches the first NIC
// of that system.
func createScalabilityHosts(cl client.Client, namespace string, numHosts int, toCleanup *[]client.Object) []metal3api.BareMetalHost {
	bmhs := make([]metal3api.BareMetalHost, numHosts)
	for i := range numHosts {
		secretName := fmt.Sprintf("bmc-creds-%04d", i)
		*toCleanup = append(*toCleanup, CreateSecret(ctx, cl, namespace, secretName, map[string]string{
			"username": "admin",
			"password": "password",
		}))

		bmh := metal3api.BareMetalHost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("scale-host-%04d", i),
				Namespace: namespace,
			},
			Spec: metal3api.BareMetalHostSpec{
				Online: true,
				BMC: metal3api.BMCDetails{
					Address:                        fmt.Sprintf("redfish+http://%s/redfish/v1/Systems/%s", scalabilityBMCEndpoint, systemUUID(i)),
					CredentialsName:                secretName,
					DisableCertificateVerification: true,
				},
				BootMACAddress:        macFromIndex(i),
				BootMode:              metal3api.UEFI,
				AutomatedCleaningMode: metal3api.CleaningModeDisabled,
				InspectionMode:        metal3api.InspectionModeDisabled,
			},
		}
		Expect(cl.Create(ctx, &bmh)).To(Succeed(), "Failed to create BMH %s", bmh.Name)
		bmhs[i] = bmh
	}
	return bmhs
}

// waitForBmhsInState waits until exactly expected BMHs in the namespace are in
// the given provisioning state. It uses a single List per poll instead of a
// Get per host.
func waitForBmhsInState(cl client.Client, namespace string, expected int, state metal3api.ProvisioningState, intervals ...interface{}) {
	Eventually(func(g Gomega) {
		list := metal3api.BareMetalHostList{}
		g.Expect(cl.List(ctx, &list, client.InNamespace(namespace))).To(Succeed())

		inState, withErrors := 0, 0
		for i := range list.Items {
			if list.Items[i].Status.Provisioning.State == state {
				inState++
			}
			if list.Items[i].Status.ErrorType != "" {
				withErrors++
			}
		}
		g.Expect(inState).To(Equal(expected),
			fmt.Sprintf("Expected %d hosts in state %q, got %d (%d with errors)", expected, state, inState, withErrors))
	}, intervals...).Should(Succeed())
}

// Scalability tests are Serial to avoid namespace conflicts when Ginkgo
// runs multiple parallel nodes. They create many resources and should
// not compete for controller capacity with other tests.
//
// The container is Ordered: the hosts are created once, the first spec waits
// for them to be enrolled and the second one provisions the same hosts.
var _ = Describe("Scalability", Serial, Ordered, Label("scalability"), func() {
	var (
		specName        = "scalability"
		namespace       *corev1.Namespace
		cancelWatches   context.CancelFunc
		cl              client.Client
		numHosts        int
		apiThreshold    time.Duration
		apiMeasurements int
		ironicClient    *gophercloud.ServiceClient
		toCleanup       []client.Object
		bmhs            []metal3api.BareMetalHost
		createStart     time.Time
	)

	BeforeAll(func() {
		numHosts = getEnvInt(envScalabilityHosts, defaultScalabilityHosts)
		apiThreshold = time.Duration(getEnvInt(envAPILatencyThresholdMs, defaultAPILatencyThresholdMs)) * time.Millisecond
		apiMeasurements = getEnvInt(envAPILatencyMeasurements, defaultAPILatencyMeasurements)
		if e2eConfig.GetBoolVariable("DEPLOY_IRONIC") {
			ironicClient = CreateIronicClient(e2eConfig)
		}
		cl = newUnthrottledClient()

		namespaceInput := framework.CreateNamespaceAndWatchEventsInput{
			Creator:   clusterProxy.GetClient(),
			ClientSet: clusterProxy.GetClientSet(),
			LogFolder: artifactFolder,
		}
		if e2eConfig.GetBoolVariable("NAMESPACE_SCOPED") {
			// Namespace-scoped BMO only watches the namespaces listed in the
			// BMO kustomization, so use the one prepared for this test.
			namespaceInput.Name = specName
			namespaceInput.IgnoreAlreadyExists = true
		} else {
			namespaceInput.Name = fmt.Sprintf("%s-%s", specName, util.RandomString(6))
		}
		namespace, cancelWatches = framework.CreateNamespaceAndWatchEvents(ctx, namespaceInput)

		By(fmt.Sprintf("Creating %d BMH resources with credentials", numHosts))
		createStart = time.Now()
		bmhs = createScalabilityHosts(cl, namespace.Name, numHosts, &toCleanup)
		Logf("Created %d BMH resources in %s", numHosts, time.Since(createStart))
	})

	It("should enroll multiple BMHs within the time window", func() {
		var snapshots []latencySnapshot

		By(fmt.Sprintf("Waiting for all %d BMHs to reach 'available' state", numHosts))
		waitForBmhsInState(cl, namespace.Name, numHosts, metal3api.StateAvailable,
			e2eConfig.GetIntervals(specName, "wait-enrollment")...)

		// Measured from the start of the creation.
		enrollDuration := time.Since(createStart)
		logResults(scalabilityResults{
			Phase:       "Enrollment",
			NumHosts:    numHosts,
			DurationSec: enrollDuration.Seconds(),
			Throughput:  (float64(numHosts) / enrollDuration.Seconds()) * 60,
		})
		snapshots = measureIronicLatency(ctx, ironicClient, snapshots, "Enrollment", bmhs, apiMeasurements)
		checkIronicLatency(snapshots, apiThreshold)
	})

	It("should provision multiple BMHs within the time window", func() {
		var snapshots []latencySnapshot

		// The hosts were left available by the previous spec. This returns
		// immediately in that case and only matters if this spec is run alone.
		By(fmt.Sprintf("Waiting for all %d BMHs to be 'available'", numHosts))
		waitForBmhsInState(cl, namespace.Name, numHosts, metal3api.StateAvailable,
			e2eConfig.GetIntervals(specName, "wait-enrollment")...)

		By(fmt.Sprintf("Patching all %d BMHs with an image to trigger provisioning", numHosts))
		list := metal3api.BareMetalHostList{}
		Expect(cl.List(ctx, &list, client.InNamespace(namespace.Name))).To(Succeed())
		Expect(list.Items).To(HaveLen(numHosts))

		image := &metal3api.Image{
			URL:          e2eConfig.GetVariable("IMAGE_URL"),
			Checksum:     e2eConfig.GetVariable("IMAGE_CHECKSUM"),
			ChecksumType: metal3api.MD5,
		}
		provisionStart := time.Now()

		// Submit the patches concurrently, with a bounded number in flight.
		// Merge patches do not conflict with BMO updating the same hosts.
		var wg sync.WaitGroup
		sem := make(chan struct{}, patchConcurrency)
		for i := range list.Items {
			wg.Add(1)
			sem <- struct{}{}
			go func(bmh *metal3api.BareMetalHost) {
				defer wg.Done()
				defer GinkgoRecover()
				defer func() { <-sem }()

				base := bmh.DeepCopy()
				bmh.Spec.Image = image.DeepCopy()
				bmh.Spec.RootDeviceHints = &metal3api.RootDeviceHints{DeviceName: "/dev/vda"}
				Expect(cl.Patch(ctx, bmh, client.MergeFrom(base))).To(Succeed(), "Failed to patch BMH %s", bmh.Name)
			}(&list.Items[i])
		}
		wg.Wait()

		// Measure while BMO is driving all hosts through provisioning.
		snapshots = measureIronicLatency(ctx, ironicClient, snapshots, "Provisioning", bmhs, apiMeasurements)

		By(fmt.Sprintf("Waiting for all %d BMHs to reach 'provisioned' state", numHosts))
		waitForBmhsInState(cl, namespace.Name, numHosts, metal3api.StateProvisioned,
			e2eConfig.GetIntervals(specName, "wait-provisioning")...)

		provisionDuration := time.Since(provisionStart)
		logResults(scalabilityResults{
			Phase:       "Provisioning",
			NumHosts:    numHosts,
			DurationSec: provisionDuration.Seconds(),
			Throughput:  (float64(numHosts) / provisionDuration.Seconds()) * 60,
		})
		snapshots = measureIronicLatency(ctx, ironicClient, snapshots, "Provisioned", bmhs, apiMeasurements)
		checkIronicLatency(snapshots, apiThreshold)
	})

	AfterAll(func() {
		// Dump while the hosts still exist, so a timeout leaves something to
		// look at.
		DumpResources(ctx, e2eConfig, clusterProxy, path.Join(artifactFolder, specName))

		if skipCleanup {
			return
		}

		By("Deleting all BMHs in the test namespace")
		DeleteBmhsInNamespace(ctx, cl, namespace.Name)

		if e2eConfig.GetBoolVariable("NAMESPACE_SCOPED") {
			// The namespace and its RBAC come from the BMO kustomization and
			// are reused, so only remove what this test created. The shared
			// Cleanup helper deletes BMHs one at a time, which is too slow for
			// many hosts, so wait for all of them together instead.
			By("Waiting for all BMHs to be deleted")
			Eventually(func(g Gomega) {
				remaining := metal3api.BareMetalHostList{}
				g.Expect(cl.List(ctx, &remaining, client.InNamespace(namespace.Name))).To(Succeed())
				g.Expect(remaining.Items).To(BeEmpty())
			}, e2eConfig.GetIntervals(specName, "wait-namespace-deleted")...).Should(Succeed())

			By("Deleting BMC credential secrets")
			for _, obj := range toCleanup {
				Expect(client.IgnoreNotFound(cl.Delete(ctx, obj))).To(Succeed(),
					"Unable to delete %T %s/%s", obj, obj.GetNamespace(), obj.GetName())
			}
		} else {
			By("Deleting test namespace (cascading delete of all resources)")
			framework.DeleteNamespace(ctx, framework.DeleteNamespaceInput{
				Deleter: cl,
				Name:    namespace.Name,
			})
			WaitForNamespaceDeleted(ctx, WaitForNamespaceDeletedInput{
				Getter:    cl,
				Namespace: *namespace,
			}, e2eConfig.GetIntervals(specName, "wait-namespace-deleted")...)
		}

		if cancelWatches != nil {
			cancelWatches()
		}
	})
})
