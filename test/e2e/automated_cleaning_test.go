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

var _ = Describe("Automated cleaning", Label("required", "automated-cleaning"), func() {
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

	It("should perform automated cleaning when enabled", func() {
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
				BootMode:              metal3api.Legacy,
				BootMACAddress:        bmc.BootMacAddress,
				AutomatedCleaningMode: metal3api.CleaningModeMetadata,
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
		userDataSecretName := "user-data-disk-test"
		sshPubKeyPath := e2eConfig.GetVariable("SSH_PUB_KEY")
		createDiskTestUserdata(ctx, clusterProxy.GetClient(), namespace.Name, userDataSecretName, sshPubKeyPath)
		userDataSecret := &corev1.SecretReference{
			Name:      userDataSecretName,
			Namespace: namespace.Name,
		}
		err = PatchBMHForProvisioning(ctx, PatchBMHForProvisioningInput{
			client:         clusterProxy.GetClient(),
			bmh:            &bmh,
			bmc:            bmc,
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

		By("Connecting via SSH to check disk state")
		client := EstablishSSHConnection(e2eConfig, bmc.IPAddress)

		By("Check that the mount point exists")
		output, err := executeSSHCommand(client, "lsblk -o NAME,MOUNTPOINT | grep vdb")
		Expect(err).NotTo(HaveOccurred())
		Expect(output).To(ContainSubstring("/mnt/data"), "Mount point /mnt/data should exist")

		By("Checking that the disks have the test file")
		_, err = executeSSHCommand(client, "ls -la /test_file_vda.txt")
		Expect(err).NotTo(HaveOccurred())

		_, err = executeSSHCommand(client, "ls -la /mnt/data/test_file_vdb.txt")
		Expect(err).NotTo(HaveOccurred())
		client.Close()

		By("Deprovisioning the BMH to trigger automated cleaning")
		helper, err := patch.NewHelper(&bmh, clusterProxy.GetClient())
		Expect(err).NotTo(HaveOccurred())
		bmh.Spec.Image = nil
		bmh.Spec.UserData = nil
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
		userDataSecretName = "user-data-ssh-setup"
		// Create new userdata secret for only SSH setup
		createSSHSetupUserdata(ctx, clusterProxy.GetClient(), namespace.Name, userDataSecretName, sshPubKeyPath, bmc.IPAddress)
		userDataSecret = &corev1.SecretReference{
			Name:      userDataSecretName,
			Namespace: namespace.Name,
		}
		err = PatchBMHForProvisioning(ctx, PatchBMHForProvisioningInput{
			client:         clusterProxy.GetClient(),
			bmh:            &bmh,
			bmc:            bmc,
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

		By("Checking that the first disk has been cleaned")
		output, err = executeSSHCommand(client, "ls -la /test_file_vda.txt 2>/dev/null || echo 'file not found'")
		Expect(err).NotTo(HaveOccurred())
		Expect(output).To(ContainSubstring("file not found"), "Test file /test_file_vda.txt should have been cleaned")

		By("Verifying second disk filesystem is cleaned")
		output, err = executeSSHCommand(client, "lsblk -o NAME,MOUNTPOINT,FSTYPE | grep vdb")
		Expect(err).NotTo(HaveOccurred())
		Expect(output).NotTo(ContainSubstring("ext4"), "Second disk should not have ext4 filesystem after cleaning")
		Expect(output).NotTo(ContainSubstring("/mnt"), "Second disk should not be mounted after cleaning")

		By("Creating new filesystem and mounting for verification")
		_, err = executeSSHCommand(client, "sudo mkfs.ext4 /dev/vdb && sudo mkdir -p /mnt/data && sudo mount /dev/vdb /mnt/data")
		Expect(err).NotTo(HaveOccurred())

		By("Checking that the test file on the second disk has been cleaned")
		output, err = executeSSHCommand(client, "ls -la /mnt/data/test_file_vdb.txt 2>/dev/null || echo 'file not found'")
		Expect(err).NotTo(HaveOccurred())
		Expect(output).To(ContainSubstring("file not found"), "Test file /mnt/data/test_file_vdb.txt should have been cleaned")
	})
	AfterEach(func() {
		DumpResources(ctx, e2eConfig, clusterProxy, path.Join(artifactFolder, specName))
		if !skipCleanup {
			cleanup(ctx, clusterProxy, namespace, cancelWatches, e2eConfig.GetIntervals("default", "wait-namespace-deleted")...)
		}
	})
})
