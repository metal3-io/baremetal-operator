//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"fmt"
	"path"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/deprecated/v1beta1/patch"
)

var _ = Describe("automated-cleaning", Label("required", "automated-cleaning"), func() {
	var (
		specName      = "automated-cleaning"
		secretName    = "bmc-credentials"
		namespace     *corev1.Namespace
		cancelWatches context.CancelFunc
	)

	BeforeEach(func() {
		namespace, cancelWatches = framework.CreateNamespaceAndWatchEvents(ctx, framework.CreateNamespaceAndWatchEventsInput{
			Creator:   clusterProxy.GetClient(),
			ClientSet: clusterProxy.GetClientSet(),
			Name:      fmt.Sprintf("%s-%s", specName, util.RandomString(6)),
			LogFolder: artifactFolder,
		})
	})

	It("should start automated cleaning when enabled", func() {
		By("Creating a secret with BMH credentials")
		bmcCredentialsData := map[string]string{
			"username": bmc.User,
			"password": bmc.Password,
		}
		CreateSecret(ctx, clusterProxy.GetClient(), namespace.Name, secretName, bmcCredentialsData)

		By("Creating a BMH")
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
					CredentialsName: "bmc-credentials",
				},
				BootMode:       metal3api.Legacy,
				BootMACAddress: bmc.BootMacAddress,
			},
		}
		err := clusterProxy.GetClient().Create(ctx, &bmh)
		Expect(err).NotTo(HaveOccurred())

		By("Waiting for the BMH to become available")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: clusterProxy.GetClient(),
			Bmh:    bmh,
			State:  metal3api.StateAvailable,
		}, e2eConfig.GetIntervals(specName, "wait-available")...)

		By("Patching the BMH to trigger provisioning")
		var userDataSecret *corev1.SecretReference
		if e2eConfig.GetVariable("PERFORM_SSH_CHECK") == "true" {
			userDataSecretName := "user-data"
			sshPubKeyPath := e2eConfig.GetVariable("SSH_PUB_KEY")
			createCirrosInstanceAndHostnameUserdata(ctx, clusterProxy.GetClient(), namespace.Name, userDataSecretName, sshPubKeyPath)
			userDataSecret = &corev1.SecretReference{
				Name:      userDataSecretName,
				Namespace: namespace.Name,
			}
		}
		err = PatchBMHForProvisioning(ctx, PatchBMHForProvisioningInput{
			client:         clusterProxy.GetClient(),
			bmh:            &bmh,
			e2eConfig:      e2eConfig,
			namespace:      namespace.Name,
			userDataSecret: userDataSecret,
		})
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

		if e2eConfig.GetVariable("PERFORM_SSH_CHECK") == "true" {
			By("Connecting via SSH and writing test marker to the disk")
			client := EstablishSSHConnection(e2eConfig, bmc.IPAddress)
			defer client.Close()

			_, err := executeSSHCommand(client, "echo 'TEST_MARKER' | sudo dd of=/dev/vda bs=1M seek=500 count=1 status=none")
			Expect(err).NotTo(HaveOccurred())

			By("Deprovisioning the BMH to trigger automated cleaning")
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

			By("Patching the BMH again to trigger re-provisioning")
			err = PatchBMHForProvisioning(ctx, PatchBMHForProvisioningInput{
				client:         clusterProxy.GetClient(),
				bmh:            &bmh,
				e2eConfig:      e2eConfig,
				namespace:      namespace.Name,
				userDataSecret: userDataSecret,
			})
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

			By("Connecting via SSH to check disk state after cleaning")
			client = EstablishSSHConnection(e2eConfig, bmc.IPAddress)
			defer client.Close()

			output, err := executeSSHCommand(client, "sudo dd if=/dev/vda bs=1M skip=500 count=1 status=none | grep TEST_MARKER || true")
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(BeEmpty(), "Disk marker should have been cleaned, but was found")
		} else {
			Logf("WARNING: Skipping SSH check since PERFORM_SSH_CHECK != true")
		}
	})
	AfterEach(func() {
		DumpResources(ctx, e2eConfig, clusterProxy, path.Join(artifactFolder, specName))
		if !skipCleanup {
			cleanup(ctx, clusterProxy, namespace, cancelWatches, e2eConfig.GetIntervals("default", "wait-namespace-deleted")...)
		}
	})
})
