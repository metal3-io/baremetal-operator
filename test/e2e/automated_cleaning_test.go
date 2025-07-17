//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"fmt"
	"path"

	"github.com/dypflying/go-qcow2lib/qcow2"
	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"libvirt.org/go/libvirt"
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

		// Only create second disk for environments with Ironic (e.g. not fixture)
		if e2eConfig.GetVariable("DEPLOY_IRONIC") == "true" {
			By("Creating second disk for the VM")
			if err = CreateAndAttachSecondDisk(ctx, bmc.Name); err != nil {
				Fail(fmt.Sprintf("Failed to create and attach second disk: %v", err))
			}
		} else {
			Logf("Skipping second disk creation because Ironic is not deployed")
		}

		By("Patching the BMH to trigger provisioning")
		var userDataSecret *corev1.SecretReference
		userDataSecretName := "user-data-disk-test"
		createDiskTestUserdata(ctx, clusterProxy.GetClient(), namespace.Name, userDataSecretName)
		userDataSecret = &corev1.SecretReference{
			Name:      userDataSecretName,
			Namespace: namespace.Name,
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
		// The ssh check is not possible in all situations (e.g. fixture) so it can be skipped
		if e2eConfig.GetVariable("PERFORM_SSH_CHECK") == "true" {
			userDataSecretName := "user-data-ssh-setup"
			sshPubKeyPath := e2eConfig.GetVariable("SSH_PUB_KEY")
			// Create new userdata secret for only SSH setup
			createSSHSetupUserdata(ctx, clusterProxy.GetClient(), namespace.Name, userDataSecretName, sshPubKeyPath)
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
			By("Connecting via SSH to check disk state after cleaning")
			client := EstablishSSHConnection(e2eConfig, bmc.IPAddress)
			defer client.Close()

			By("Checking that the first disk has been cleaned")
			output, err := executeSSHCommand(client, "ls -la /test_file_vda.txt 2>/dev/null || echo 'file not found'")
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring("file not found"), "Test file /test_file_vda.txt should have been cleaned")

			By("Verifying second disk exists but filesystem is cleaned")
			output, err = executeSSHCommand(client, "lsblk | grep vdb")
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring("vdb"), "Second disk /dev/vdb should exist")

			By("Attempting to mount the second disk (should fail if cleaned)")
			output, err = executeSSHCommand(client, "sudo mount /dev/vdb /mnt/data 2>&1 || echo 'mount failed as expected'")
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring("mount failed"), "Mounting /dev/vdb should fail if the disk was cleaned")

			_, err = executeSSHCommand(client, "sudo mkfs.ext4 /dev/vdb && sudo mkdir -p /mnt/data && sudo mount /dev/vdb /mnt/data")
			Expect(err).NotTo(HaveOccurred())

			By("Checking that the second disk has been cleaned")
			output, err = executeSSHCommand(client, "ls -la /test_file_vdb.txt 2>/dev/null || echo 'file not found'")
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring("file not found"), "Test file /test_file_vdb.txt should have been cleaned")
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

func CreateAndAttachSecondDisk(ctx context.Context, vmName string) error {
	conn, err := libvirt.NewConnect("qemu:///system")
	if err != nil {
		return err
	}
	defer conn.Close()

	domain, err := conn.LookupDomainByName(vmName)
	if err != nil {
		return err
	}

	// Create second disk file
	opts := make(map[string]any)
	opts[qcow2.OPT_SIZE] = 1 * (1 << 30) // qcow2 file's size is 1 GiB
	opts[qcow2.OPT_FMT] = "qcow2"

	diskPath := fmt.Sprintf("/tmp/%s_test_disk2.qcow2", vmName)
	if err := qcow2.Blk_Create(diskPath, opts); err != nil {
		return err
	}

	// Create XML and attach disk to domain
	diskXML := fmt.Sprintf(`<disk type='file' device='disk'>
        <driver name='qemu' type='qcow2'/>
        <source file='%s'/>
        <target dev='vdb' bus='virtio'/>
    </disk>`, diskPath)

	// Allocate the disk to the domain configuration. Domain is not started yet.
	// From the docs, CONFIG is used when "the device shall be allocated to
	// the persisted domain configuration only"
	return domain.AttachDeviceFlags(diskXML, libvirt.DOMAIN_DEVICE_MODIFY_CONFIG)
}
