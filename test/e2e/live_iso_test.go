package e2e

import (
	"context"
	"fmt"
	"os/exec"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/patch"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
)

var _ = Describe("Live ISO", func() {
	var (
		specName        = "live-iso-test"
		namespace       *corev1.Namespace
		cancelWatches   context.CancelFunc
		liveISOImageURL string
		bmcUser         string
		bmcPassword     string
		liveISOAddress  string
		bootMacAddress  string
	)

	BeforeEach(func() {
		bmcUser = e2eConfig.GetVariable("BMC_USER")
		bmcPassword = e2eConfig.GetVariable("BMC_PASSWORD")
		bootMacAddress = e2eConfig.GetVariable("BOOT_MAC_ADDRESS")
		liveISOImageURL = e2eConfig.GetVariable("LIVE_ISO_IMAGE")
		liveISOAddress = e2eConfig.GetVariable("LIVE_ISO_ADDRESS")
		namespace, cancelWatches = framework.CreateNamespaceAndWatchEvents(ctx, framework.CreateNamespaceAndWatchEventsInput{
			Creator:   clusterProxy.GetClient(),
			ClientSet: clusterProxy.GetClientSet(),
			Name:      fmt.Sprintf("%s-%s", specName, util.RandomString(6)),
			LogFolder: artifactFolder,
		})
	})

	It("should provision a BMH with live ISO and then deprovision it", func() {
		By("Creating a secret with BMH credentials")
		bmcCredentials := corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "bmc-credentials",
				Namespace: namespace.Name,
			},
			StringData: map[string]string{
				"username": bmcUser,
				"password": bmcPassword,
			},
		}
		err := clusterProxy.GetClient().Create(ctx, &bmcCredentials)
		Expect(err).NotTo(HaveOccurred())

		By("Creating a BMH with inspection disabled and configured for live ISO boot")
		bmh := metal3api.BareMetalHost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      specName,
				Namespace: namespace.Name,
				Annotations: map[string]string{
					metal3api.InspectAnnotationPrefix:   "disabled",
					metal3api.HardwareDetailsAnnotation: hardwareDetails,
				},
			},
			Spec: metal3api.BareMetalHostSpec{
				Online: true,
				BMC: metal3api.BMCDetails{
					Address:         liveISOAddress,
					CredentialsName: "bmc-credentials",
				},
				BootMode:       metal3api.Legacy,
				BootMACAddress: bootMacAddress,
				Image: &metal3api.Image{
					URL:          liveISOImageURL,
					Checksum:     "",
					ChecksumType: "",
					DiskFormat:   StringPtr("live-iso"),
				},
			},
		}

		err = clusterProxy.GetClient().Create(ctx, &bmh)
		Expect(err).NotTo(HaveOccurred())

		By("Waiting for BMH to transition to Provisioning state")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: clusterProxy.GetClient(),
			Bmh:    bmh,
			State:  metal3api.StateProvisioning,
		}, e2eConfig.GetIntervals(specName, "wait-provisioning")...)

		By("Waiting for BMH to transition to Provisioned state")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: clusterProxy.GetClient(),
			Bmh:    bmh,
			State:  metal3api.StateProvisioned,
		}, e2eConfig.GetIntervals(specName, "wait-provisioned")...)

		By("Verifying the node booted from live ISO by reading serial logs to verify the node was booted from live ISO image")
		var currentBmh metal3api.BareMetalHost
		err = clusterProxy.GetClient().Get(ctx, types.NamespacedName{Name: bmh.Name, Namespace: namespace.Name}, &currentBmh)
		Expect(err).NotTo(HaveOccurred(), "Error in getting BMH")

		serialLogFile := "/var/log/libvirt/qemu/bmo-e2e-0-serial0.log"

		Eventually(func(g Gomega) {
			cmd := fmt.Sprintf("sudo cat %s | grep 'Welcome'", serialLogFile)
			output, err := exec.Command("/bin/sh", "-c", cmd).Output()
			g.Expect(err).To(BeNil(), "Error executing command to read serial logs")
			g.Expect(output).ToNot(BeNil(), fmt.Sprintf("Failed to read serial logs from %s", serialLogFile))
		}, e2eConfig.GetIntervals(specName, "wait-read")...).Should(Succeed())

		By("Triggering the deprovisioning of the BMH")
		helper, err := patch.NewHelper(&bmh, clusterProxy.GetClient())
		Expect(err).NotTo(HaveOccurred())
		bmh.Spec.Image = nil
		Expect(helper.Patch(ctx, &bmh)).To(Succeed())

		By("Waiting for the BMH to be in deprovisioning state")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: clusterProxy.GetClient(),
			Bmh:    bmh,
			State:  metal3api.StateDeprovisioning,
		}, e2eConfig.GetIntervals(specName, "wait-deprovisioning")...)

		By("Waiting for BMH to become available again")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: clusterProxy.GetClient(),
			Bmh:    bmh,
			State:  metal3api.StateAvailable,
		}, e2eConfig.GetIntervals(specName, "wait-available")...)
	})

	AfterEach(func() {
		cleanup(ctx, clusterProxy, namespace, cancelWatches, e2eConfig.GetIntervals("default", "wait-namespace-deleted")...)
	})
})
