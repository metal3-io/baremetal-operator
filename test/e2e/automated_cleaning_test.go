//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"path"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Automated cleaning", Label("required", "automated-cleaning", "ironic"), func() {
	var (
		specName      = "automated-cleaning"
		namespace     *corev1.Namespace
		cancelWatches context.CancelFunc
		toCleanup     []client.Object
	)

	BeforeEach(func() {
		toCleanup = nil
		namespace, cancelWatches = framework.CreateNamespaceAndWatchEvents(ctx, framework.CreateNamespaceAndWatchEventsInput{
			Creator:             clusterProxy.GetClient(),
			ClientSet:           clusterProxy.GetClientSet(),
			Name:                specName,
			LogFolder:           artifactFolder,
			IgnoreAlreadyExists: true,
		})
	})

	It("should clean the disks on deprovisioning when enabled", func() {
		By("Creating a secret with BMH credentials")
		bmcCredentialsData := map[string]string{
			"username": bmc.User,
			"password": bmc.Password,
		}
		secret := CreateSecret(ctx, clusterProxy.GetClient(), namespace.Name, "bmc-credentials", bmcCredentialsData)
		toCleanup = append(toCleanup, secret)

		By("Creating a BMH")
		bmh := metal3api.BareMetalHost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      specName,
				Namespace: namespace.Name,
			},
			Spec: metal3api.BareMetalHostSpec{
				Online: true,
				BMC: metal3api.BMCDetails{
					Address:                        bmc.Address,
					CredentialsName:                "bmc-credentials",
					DisableCertificateVerification: bmc.DisableCertificateVerification,
				},
				BootMode:       metal3api.BootMode(e2eConfig.GetVariable("BOOT_MODE")),
				BootMACAddress: bmc.BootMacAddress,
				// NOTE(dtantsur): not enabling cleaning on enrollment to save some time.
				// It's enabled right before deprovisioning.
				AutomatedCleaningMode: metal3api.CleaningModeDisabled,
				InspectionMode:        metal3api.InspectionModeDisabled,
			},
		}
		err := clusterProxy.GetClient().Create(ctx, &bmh)
		Expect(err).NotTo(HaveOccurred())
		toCleanup = append(toCleanup, &bmh)

		By("Waiting for the BMH to become available")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: clusterProxy.GetClient(),
			Bmh:    bmh,
			State:  metal3api.StateAvailable,
		}, e2eConfig.GetIntervals(specName, "wait-available")...)

		By("Patching the BMH to trigger provisioning")
		userDataSecretName := "user-data-disk-test"
		sshPubKeyPath := e2eConfig.GetVariable("SSH_PUB_KEY")
		obj := createDiskTestUserdata(ctx, clusterProxy.GetClient(), namespace.Name, userDataSecretName, sshPubKeyPath, bmc.IPAddress)
		toCleanup = append(toCleanup, obj)
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
		Eventually(func(g Gomega) {
			output, sshErr := executeSSHCommand(client, "lsblk -o NAME,MOUNTPOINT | grep vdb")
			g.Expect(sshErr).NotTo(HaveOccurred())
			g.Expect(output).To(ContainSubstring("/mnt/data"), "Mount point /mnt/data should exist")
		}, e2eConfig.GetIntervals(specName, "wait-user-data")...).Should(Succeed())

		By("Checking that the disks have the test file")
		_, err = executeSSHCommand(client, "ls -la /test_file_vda.txt")
		Expect(err).NotTo(HaveOccurred())

		_, err = executeSSHCommand(client, "ls -la /mnt/data/test_file_vdb.txt")
		Expect(err).NotTo(HaveOccurred())
		client.Close()

		By("Enabling cleaning and deprovisioning the BMH to trigger it")
		helper, err := patch.NewHelper(&bmh, clusterProxy.GetClient())
		Expect(err).NotTo(HaveOccurred())
		bmh.Spec.AutomatedCleaningMode = metal3api.CleaningModeMetadata
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
		obj = createSSHSetupUserdata(ctx, clusterProxy.GetClient(), namespace.Name, userDataSecretName, sshPubKeyPath, bmc.IPAddress)
		toCleanup = append(toCleanup, obj)
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
		output, err := executeSSHCommand(client, "ls -la /test_file_vda.txt 2>/dev/null || echo 'file not found'")
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

		By("Disabling cleaning to allow quick deletion")
		helper, err = patch.NewHelper(&bmh, clusterProxy.GetClient())
		Expect(err).NotTo(HaveOccurred())
		bmh.Spec.AutomatedCleaningMode = metal3api.CleaningModeDisabled
		Expect(helper.Patch(ctx, &bmh)).To(Succeed())

	})

	It("should clean the disks before provisioning when enabled", func() {
		bmhName := specName + "-enroll"

		By("Creating a secret with BMH credentials")
		bmcCredentialsData := map[string]string{
			"username": bmc.User,
			"password": bmc.Password,
		}
		secret := CreateSecret(ctx, clusterProxy.GetClient(), namespace.Name, bmhName+"-bmc", bmcCredentialsData)
		toCleanup = append(toCleanup, secret)

		By("Creating a BMH the first time")
		bmh := metal3api.BareMetalHost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      bmhName,
				Namespace: namespace.Name,
			},
			Spec: metal3api.BareMetalHostSpec{
				Online: true,
				BMC: metal3api.BMCDetails{
					Address:                        bmc.Address,
					CredentialsName:                bmhName + "-bmc",
					DisableCertificateVerification: bmc.DisableCertificateVerification,
				},
				BootMode:       metal3api.BootMode(e2eConfig.GetVariable("BOOT_MODE")),
				BootMACAddress: bmc.BootMacAddress,
				// NOTE(dtantsur): we're not testing cleaning in this phase yet.
				// It will be enabled when the host is created the 2nd time.
				AutomatedCleaningMode: metal3api.CleaningModeDisabled,
				InspectionMode:        metal3api.InspectionModeDisabled,
			},
		}
		err := clusterProxy.GetClient().Create(ctx, &bmh)
		Expect(err).NotTo(HaveOccurred())
		toCleanup = append(toCleanup, &bmh)

		By("Waiting for the BMH to become available")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: clusterProxy.GetClient(),
			Bmh:    bmh,
			State:  metal3api.StateAvailable,
		}, e2eConfig.GetIntervals(specName, "wait-available")...)

		By("Patching the BMH to trigger provisioning")
		userDataSecretName := bmhName + "-user-data-disk"
		sshPubKeyPath := e2eConfig.GetVariable("SSH_PUB_KEY")
		obj := createDiskTestUserdata(ctx, clusterProxy.GetClient(), namespace.Name, userDataSecretName, sshPubKeyPath, bmc.IPAddress)
		toCleanup = append(toCleanup, obj)
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
		sshClient := EstablishSSHConnection(e2eConfig, bmc.IPAddress)

		By("Check that the mount point exists")
		Eventually(func(g Gomega) {
			output, sshErr := executeSSHCommand(sshClient, "lsblk -o NAME,MOUNTPOINT | grep vdb")
			g.Expect(sshErr).NotTo(HaveOccurred())
			g.Expect(output).To(ContainSubstring("/mnt/data"), "Mount point /mnt/data should exist")
		}, e2eConfig.GetIntervals(specName, "wait-user-data")...).Should(Succeed())

		By("Checking that the disks have the test file")
		_, err = executeSSHCommand(sshClient, "ls -la /test_file_vda.txt")
		Expect(err).NotTo(HaveOccurred())

		_, err = executeSSHCommand(sshClient, "ls -la /mnt/data/test_file_vdb.txt")
		Expect(err).NotTo(HaveOccurred())
		sshClient.Close()

		By("Adding the detached annotation")
		helper, err := patch.NewHelper(&bmh, clusterProxy.GetClient())
		Expect(err).NotTo(HaveOccurred())

		AnnotateBmh(ctx, clusterProxy.GetClient(), bmh, metal3api.DetachedAnnotation, ptr.To("true"))
		Expect(helper.Patch(ctx, &bmh)).To(Succeed())

		By("Waiting for the BMH to be detached")
		WaitForBmhInOperationalStatus(ctx, WaitForBmhInOperationalStatusInput{
			Client: clusterProxy.GetClient(),
			Bmh:    bmh,
			State:  metal3api.OperationalStatusDetached,
			UndesiredStates: []metal3api.OperationalStatus{
				metal3api.OperationalStatusError,
			},
		}, e2eConfig.GetIntervals(specName, "wait-detached")...)

		By("Deleting the BMH")
		err = clusterProxy.GetClient().Delete(ctx, &bmh)
		Expect(err).NotTo(HaveOccurred())

		By("Waiting for the BMH to be deleted")
		WaitForBmhDeleted(ctx, WaitForBmhDeletedInput{
			Client:    clusterProxy.GetClient(),
			BmhName:   bmh.Name,
			Namespace: bmh.Namespace,
			UndesiredStates: []metal3api.ProvisioningState{
				metal3api.StateDeprovisioning,
				metal3api.StatePoweringOffBeforeDelete,
			},
		}, e2eConfig.GetIntervals(specName, "wait-bmh-deleted")...)

		By("Waiting for the original secret to be deleted")
		Eventually(func() bool {
			getErr := clusterProxy.GetClient().Get(ctx, client.ObjectKey{
				Name:      bmhName + "-bmc",
				Namespace: namespace.Name,
			}, &corev1.Secret{})
			return getErr != nil && client.IgnoreNotFound(getErr) == nil
		}, e2eConfig.GetIntervals(specName, "wait-secret-deletion")...).Should(BeTrue())

		By("Creating the same secret with BMH credentials")
		secret = CreateSecret(ctx, clusterProxy.GetClient(), namespace.Name, bmhName+"-bmc", bmcCredentialsData)
		toCleanup = append(toCleanup, secret)

		By("Creating the same BMH again")
		bmh = metal3api.BareMetalHost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      bmhName,
				Namespace: namespace.Name,
			},
			Spec: metal3api.BareMetalHostSpec{
				Online: true,
				BMC: metal3api.BMCDetails{
					Address:                        bmc.Address,
					CredentialsName:                bmhName + "-bmc",
					DisableCertificateVerification: bmc.DisableCertificateVerification,
				},
				BootMode:              metal3api.BootMode(e2eConfig.GetVariable("BOOT_MODE")),
				BootMACAddress:        bmc.BootMacAddress,
				AutomatedCleaningMode: metal3api.CleaningModeMetadata,
				InspectionMode:        metal3api.InspectionModeDisabled,
			},
		}
		err = clusterProxy.GetClient().Create(ctx, &bmh)
		Expect(err).NotTo(HaveOccurred())
		toCleanup = append(toCleanup, &bmh)

		By("Waiting for the BMH to become available again")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: clusterProxy.GetClient(),
			Bmh:    bmh,
			State:  metal3api.StateAvailable,
		}, e2eConfig.GetIntervals(specName, "wait-available")...)

		By("Patching the BMH again to trigger re-provisioning")
		userDataSecretName = bmhName + "-user-data-ssh"
		obj = createSSHSetupUserdata(ctx, clusterProxy.GetClient(), namespace.Name, userDataSecretName, sshPubKeyPath, bmc.IPAddress)
		toCleanup = append(toCleanup, obj)
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
		sshClient = EstablishSSHConnection(e2eConfig, bmc.IPAddress)
		defer sshClient.Close()

		By("Checking that the first disk has been cleaned")
		output, err := executeSSHCommand(sshClient, "ls -la /test_file_vda.txt 2>/dev/null || echo 'file not found'")
		Expect(err).NotTo(HaveOccurred())
		Expect(output).To(ContainSubstring("file not found"), "Test file /test_file_vda.txt should have been cleaned")

		By("Verifying second disk filesystem is cleaned")
		output, err = executeSSHCommand(sshClient, "lsblk -o NAME,MOUNTPOINT,FSTYPE | grep vdb")
		Expect(err).NotTo(HaveOccurred())
		Expect(output).NotTo(ContainSubstring("ext4"), "Second disk should not have ext4 filesystem after cleaning")
		Expect(output).NotTo(ContainSubstring("/mnt"), "Second disk should not be mounted after cleaning")

		By("Creating new filesystem and mounting for verification")
		_, err = executeSSHCommand(sshClient, "sudo mkfs.ext4 /dev/vdb && sudo mkdir -p /mnt/data && sudo mount /dev/vdb /mnt/data")
		Expect(err).NotTo(HaveOccurred())

		By("Checking that the test file on the second disk has been cleaned")
		output, err = executeSSHCommand(sshClient, "ls -la /mnt/data/test_file_vdb.txt 2>/dev/null || echo 'file not found'")
		Expect(err).NotTo(HaveOccurred())
		Expect(output).To(ContainSubstring("file not found"), "Test file /mnt/data/test_file_vdb.txt should have been cleaned")

		By("Disabling cleaning to allow quick deletion")
		helper, err = patch.NewHelper(&bmh, clusterProxy.GetClient())
		Expect(err).NotTo(HaveOccurred())
		bmh.Spec.AutomatedCleaningMode = metal3api.CleaningModeDisabled
		Expect(helper.Patch(ctx, &bmh)).To(Succeed())

	})
	AfterEach(func() {
		CollectSerialLogs(bmc.Name, path.Join(artifactFolder, specName))
		DumpResources(ctx, e2eConfig, clusterProxy, path.Join(artifactFolder, specName))
		if !skipCleanup {
			Cleanup(ctx, clusterProxy, namespace, cancelWatches, e2eConfig, toCleanup)
		}
	})
})
