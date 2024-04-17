package e2e

import (
	"context"
	"fmt"
	"os"

	"golang.org/x/crypto/ssh"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/patch"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	metal3bmc "github.com/metal3-io/baremetal-operator/pkg/hardwareutils/bmc"
)

var _ = Describe("Live-ISO", Label("required", "live-iso"), func() {
	var (
		specName      = "live-iso-ops"
		secretName    = "bmc-credentials"
		namespace     *corev1.Namespace
		cancelWatches context.CancelFunc
		imageURL      string
	)

	BeforeEach(func() {
		// Check what kind of BMC we are dealing with
		// It may be *possible* to boot a live-ISO over (i)PXE, but there are severe limitations.
		// Therefore we skip the test if it doesn't support ISO preprovisioning images.
		// See https://docs.openstack.org/ironic/latest/admin/ramdisk-boot.html
		accessDetails, err := metal3bmc.NewAccessDetails(bmc.Address, false)
		Expect(err).NotTo(HaveOccurred())
		if !accessDetails.SupportsISOPreprovisioningImage() {
			Skip(fmt.Sprintf("BMC does not support ISO images. It does not make sense to test live-ISO here. BMC address: %s", bmc.Address))
		}

		imageURL = e2eConfig.GetVariable("ISO_IMAGE_URL")

		namespace, cancelWatches = framework.CreateNamespaceAndWatchEvents(ctx, framework.CreateNamespaceAndWatchEventsInput{
			Creator:   clusterProxy.GetClient(),
			ClientSet: clusterProxy.GetClientSet(),
			Name:      fmt.Sprintf("%s-%s", specName, util.RandomString(6)),
			LogFolder: artifactFolder,
		})
	})

	It("should provision a BMH with live ISO and then deprovision it", func() {
		By("Creating a secret with BMH credentials")
		bmcCredentialsData := map[string]string{
			"username": bmc.User,
			"password": bmc.Password,
		}
		CreateSecret(ctx, clusterProxy.GetClient(), namespace.Name, secretName, bmcCredentialsData)

		By("Creating a BMH with inspection disabled and hardware details added")
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
					Address:         bmc.Address,
					CredentialsName: secretName,
				},
				Image: &metal3api.Image{
					URL:        imageURL,
					DiskFormat: pointer.String("live-iso"),
				},
				BootMode:              metal3api.Legacy,
				BootMACAddress:        bmc.BootMacAddress,
				AutomatedCleaningMode: metal3api.CleaningModeDisabled,
			},
		}
		err := clusterProxy.GetClient().Create(ctx, &bmh)
		Expect(err).NotTo(HaveOccurred())

		By("Waiting for the BMH to be in provisioning state")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: clusterProxy.GetClient(),
			Bmh:    bmh,
			State:  metal3api.StateProvisioning,
		}, e2eConfig.GetIntervals(specName, "wait-provisioning")...)

		By("Waiting for the BMH to become provisioned")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: clusterProxy.GetClient(),
			Bmh:    bmh,
			State:  metal3api.StateProvisioned,
		}, e2eConfig.GetIntervals(specName, "wait-provisioned")...)

		// The ssh check is not possible in all situations (e.g. fixture) so it can be skipped
		if e2eConfig.GetVariable("SSH_CHECK_PROVISIONED") == "true" {
			By("Verifying the node booted from live ISO image")
			keyPath := e2eConfig.GetVariable("SSH_PRIV_KEY")
			key, err := os.ReadFile(keyPath)
			Expect(err).NotTo(HaveOccurred(), "unable to read private key")
			signer, err := ssh.ParsePrivateKey(key)
			Expect(err).NotTo(HaveOccurred(), "unable to parse private key")
			auth := ssh.PublicKeys(signer)
			PerformSSHBootCheck(e2eConfig, "memory", auth, fmt.Sprintf("%s:%s", bmc.IPAddress, bmc.SSHPort))
		} else {
			Logf("WARNING: Skipping SSH check since SSH_CHECK_PROVISIONED != true")
		}

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

		By("Waiting for the BMH to become available again")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: clusterProxy.GetClient(),
			Bmh:    bmh,
			State:  metal3api.StateAvailable,
		}, e2eConfig.GetIntervals(specName, "wait-available")...)
	})

	AfterEach(func() {
		if !skipCleanup {
			cleanup(ctx, clusterProxy, namespace, cancelWatches, e2eConfig.GetIntervals("default", "wait-namespace-deleted")...)
		}
	})
})
