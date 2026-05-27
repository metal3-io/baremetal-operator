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
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// The firmware update URL does not need to point to a real binary since
	// sushy-tools emulates the upgrade by incrementing the BIOS version
	// without downloading anything.
	biosFirmwareUpdateURL = "http://192.168.222.1/bios-update.bin"
)

var _ = Describe("Host Firmware Components", Label("required", "firmware-components"), func() {
	var (
		specName      = "firmware-components"
		namespace     *corev1.Namespace
		cancelWatches context.CancelFunc
		toCleanup     []client.Object
	)

	BeforeEach(func() {
		if !e2eConfig.GetBoolVariable("DEPLOY_IRONIC") || bmc.AccessDetails.Driver() != "redfish" {
			Skip("Firmware component tests require a real Ironic and a host with Redfish")
		}

		toCleanup = nil
		namespace, cancelWatches = framework.CreateNamespaceAndWatchEvents(ctx, framework.CreateNamespaceAndWatchEventsInput{
			Creator:             clusterProxy.GetClient(),
			ClientSet:           clusterProxy.GetClientSet(),
			Name:                specName,
			LogFolder:           artifactFolder,
			IgnoreAlreadyExists: true,
		})
	})

	It("should apply BIOS firmware update during cleaning on a BMH", func() {
		bmhName := specName + "-before-bmh"
		secretName := bmhName + "-bmc"

		By("Creating a secret with BMH credentials")
		bmcCredentialsData := map[string]string{
			"username": bmc.User,
			"password": bmc.Password,
		}
		secret := CreateSecret(ctx, clusterProxy.GetClient(), namespace.Name, secretName, bmcCredentialsData)
		toCleanup = append(toCleanup, secret)

		By("Creating a HostFirmwareComponents with a BIOS update before creating the BMH")
		hfc := &metal3api.HostFirmwareComponents{
			ObjectMeta: metav1.ObjectMeta{
				Name:      bmhName,
				Namespace: namespace.Name,
			},
			Spec: metal3api.HostFirmwareComponentsSpec{
				Updates: []metal3api.FirmwareUpdate{
					{
						Component: "bios",
						URL:       biosFirmwareUpdateURL,
					},
				},
			},
		}
		Expect(clusterProxy.GetClient().Create(ctx, hfc)).To(Succeed())

		By("Creating a BMH with inspection and cleaning disabled")
		bmh := metal3api.BareMetalHost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      bmhName,
				Namespace: namespace.Name,
			},
			Spec: metal3api.BareMetalHostSpec{
				Online: true,
				BMC: metal3api.BMCDetails{
					Address:                        bmc.Address,
					CredentialsName:                secretName,
					DisableCertificateVerification: bmc.DisableCertificateVerification,
				},
				BootMode:              metal3api.BootMode(e2eConfig.GetVariable("BOOT_MODE")),
				BootMACAddress:        bmc.BootMacAddress,
				AutomatedCleaningMode: metal3api.CleaningModeDisabled,
				InspectionMode:        metal3api.InspectionModeDisabled,
			},
		}
		Expect(clusterProxy.GetClient().Create(ctx, &bmh)).To(Succeed())
		toCleanup = append(toCleanup, &bmh)

		By("Waiting for the BMH to start preparing")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: clusterProxy.GetClient(),
			Bmh:    bmh,
			State:  metal3api.StatePreparing,
		}, e2eConfig.GetIntervals(specName, "wait-available")...)

		By("Waiting for the BMH to become available")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: clusterProxy.GetClient(),
			Bmh:    bmh,
			State:  metal3api.StateAvailable,
		}, e2eConfig.GetIntervals(specName, "wait-firmware-components")...)

		By("Verifying the HFC Status")
		Expect(clusterProxy.GetClient().Get(ctx, types.NamespacedName{
			Name:      bmhName,
			Namespace: namespace.Name,
		}, hfc)).To(Succeed())
		Expect(hfc.Status.Updates).To(HaveLen(1))
		Expect(hfc.Status.Updates[0].Component).To(Equal("bios"))
		Expect(hfc.Status.Updates[0].URL).To(Equal(biosFirmwareUpdateURL))
		Expect(hfc.Status.Conditions).To(ContainCondition(string(metal3api.HostFirmwareComponentsChangeDetected), metav1.ConditionFalse))
		Expect(hfc.Status.Conditions).To(ContainCondition(string(metal3api.HostFirmwareComponentsValid), metav1.ConditionTrue))

		By("Waiting for the HFC controller to update BIOS component status from Ironic")
		Eventually(func(g Gomega) {
			g.Expect(clusterProxy.GetClient().Get(ctx, types.NamespacedName{
				Name:      bmhName,
				Namespace: namespace.Name,
			}, hfc)).To(Succeed())
			var biosComponent *metal3api.FirmwareComponentStatus
			for i := range hfc.Status.Components {
				if hfc.Status.Components[i].Component == "bios" {
					biosComponent = &hfc.Status.Components[i]
					break
				}
			}
			g.Expect(biosComponent).NotTo(BeNil(), "bios component should exist in status.components")
			g.Expect(biosComponent.InitialVersion).NotTo(BeEmpty())
			g.Expect(biosComponent.CurrentVersion).NotTo(BeEmpty())
			g.Expect(biosComponent.CurrentVersion).NotTo(Equal(biosComponent.InitialVersion),
				"CurrentVersion should differ from InitialVersion after firmware update")
		}, e2eConfig.GetIntervals(specName, "wait-firmware-components")...).Should(Succeed())

		By("Deleting the BMH to get rid of cached values")
		Expect(clusterProxy.GetClient().Delete(ctx, &bmh)).To(Succeed())

		By("Waiting for the BMH to be deleted")
		WaitForBmhDeleted(ctx, WaitForBmhDeletedInput{
			Client:    clusterProxy.GetClient(),
			BmhName:   bmhName,
			Namespace: namespace.Name,
		}, e2eConfig.GetIntervals(specName, "wait-bmh-deleted")...)

		By("Making sure HFC was deleted too")
		Eventually(func() bool {
			return k8serrors.IsNotFound(clusterProxy.GetClient().Get(ctx, types.NamespacedName{
				Name:      bmhName,
				Namespace: namespace.Name,
			}, hfc))
		}, e2eConfig.GetIntervals(specName, "wait-bmh-deleted")...).Should(BeTrue())

		By("Creating a secret with BMH credentials again")
		secret = CreateSecret(ctx, clusterProxy.GetClient(), namespace.Name, secretName, bmcCredentialsData)
		toCleanup = append(toCleanup, secret)

		By("Re-creating the BMH to execute a new BIOS firmware update")
		bmh = metal3api.BareMetalHost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      bmhName,
				Namespace: namespace.Name,
			},
			Spec: metal3api.BareMetalHostSpec{
				Online: true,
				BMC: metal3api.BMCDetails{
					Address:                        bmc.Address,
					CredentialsName:                secretName,
					DisableCertificateVerification: bmc.DisableCertificateVerification,
				},
				BootMode:              metal3api.BootMode(e2eConfig.GetVariable("BOOT_MODE")),
				BootMACAddress:        bmc.BootMacAddress,
				AutomatedCleaningMode: metal3api.CleaningModeDisabled,
				InspectionMode:        metal3api.InspectionModeDisabled,
			},
		}
		Expect(clusterProxy.GetClient().Create(ctx, &bmh)).To(Succeed())

		By("Waiting for the BMH to become available")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: clusterProxy.GetClient(),
			Bmh:    bmh,
			State:  metal3api.StateAvailable,
		}, e2eConfig.GetIntervals(specName, "wait-available")...)

		By("Verifying that HostFirmwareComponents was created automatically")
		hfc2 := &metal3api.HostFirmwareComponents{}
		Expect(clusterProxy.GetClient().Get(ctx, types.NamespacedName{
			Name:      bmhName,
			Namespace: namespace.Name,
		}, hfc2)).To(Succeed())

		By("Updating HostFirmwareComponents with a BIOS update")
		helper, err := patch.NewHelper(hfc2, clusterProxy.GetClient())
		Expect(err).NotTo(HaveOccurred())
		hfc2.Spec.Updates = []metal3api.FirmwareUpdate{
			{
				Component: "bios",
				URL:       biosFirmwareUpdateURL + "v2",
			},
		}
		Expect(helper.Patch(ctx, hfc2)).To(Succeed())

		By("Verifying the conditions on HFC")
		Eventually(func(g Gomega) {
			g.Expect(clusterProxy.GetClient().Get(ctx, types.NamespacedName{
				Name:      bmhName,
				Namespace: namespace.Name,
			}, hfc2)).To(Succeed())
			g.Expect(hfc2.Status.Conditions).To(ContainCondition(string(metal3api.HostFirmwareComponentsValid), metav1.ConditionTrue))
			g.Expect(hfc2.Status.Conditions).To(ContainCondition(string(metal3api.HostFirmwareComponentsChangeDetected), metav1.ConditionTrue))
		}, e2eConfig.GetIntervals(specName, "wait-reconcile")...).Should(Succeed())

		By("Waiting for the BMH to start preparing")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: clusterProxy.GetClient(),
			Bmh:    bmh,
			State:  metal3api.StatePreparing,
		}, e2eConfig.GetIntervals(specName, "wait-available")...)

		By("Waiting for the BMH to become available")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: clusterProxy.GetClient(),
			Bmh:    bmh,
			State:  metal3api.StateAvailable,
		}, e2eConfig.GetIntervals(specName, "wait-firmware-components")...)

		By("Verifying the HFC Status")
		Expect(clusterProxy.GetClient().Get(ctx, types.NamespacedName{
			Name:      bmhName,
			Namespace: namespace.Name,
		}, hfc2)).To(Succeed())
		Expect(hfc2.Status.Updates).To(HaveLen(1))
		Expect(hfc2.Status.Updates[0].Component).To(Equal("bios"))
		Expect(hfc2.Status.Updates[0].URL).To(Equal(biosFirmwareUpdateURL + "v2"))
		Expect(hfc2.Status.Conditions).To(ContainCondition(string(metal3api.HostFirmwareComponentsChangeDetected), metav1.ConditionFalse))
		Expect(hfc2.Status.Conditions).To(ContainCondition(string(metal3api.HostFirmwareComponentsValid), metav1.ConditionTrue))

		By("Waiting for the HFC controller to update BIOS component status from Ironic")
		Eventually(func(g Gomega) {
			g.Expect(clusterProxy.GetClient().Get(ctx, types.NamespacedName{
				Name:      bmhName,
				Namespace: namespace.Name,
			}, hfc2)).To(Succeed())
			var biosComponent2 *metal3api.FirmwareComponentStatus
			for i := range hfc2.Status.Components {
				if hfc2.Status.Components[i].Component == "bios" {
					biosComponent2 = &hfc2.Status.Components[i]
					break
				}
			}
			g.Expect(biosComponent2).NotTo(BeNil(), "bios component should exist in status.components")
			g.Expect(biosComponent2.InitialVersion).NotTo(BeEmpty())
			g.Expect(biosComponent2.CurrentVersion).NotTo(BeEmpty())
			g.Expect(biosComponent2.CurrentVersion).NotTo(Equal(biosComponent2.InitialVersion),
				"CurrentVersion should differ from InitialVersion after second firmware update")
		}, e2eConfig.GetIntervals(specName, "wait-firmware-components")...).Should(Succeed())

		By("Deleting the BMH")
		Expect(clusterProxy.GetClient().Delete(ctx, &bmh)).To(Succeed())

		By("Waiting for the BMH to be deleted")
		WaitForBmhDeleted(ctx, WaitForBmhDeletedInput{
			Client:    clusterProxy.GetClient(),
			BmhName:   bmhName,
			Namespace: namespace.Name,
		}, e2eConfig.GetIntervals(specName, "wait-bmh-deleted")...)
	})

	It("should update BIOS firmware on a provisioned host via servicing", func() {
		bmhName := specName + "-servicing"
		secretName := bmhName + "-bmc"

		By("Creating a secret with BMH credentials")
		bmcCredentialsData := map[string]string{
			"username": bmc.User,
			"password": bmc.Password,
		}
		secret := CreateSecret(ctx, clusterProxy.GetClient(), namespace.Name, secretName, bmcCredentialsData)
		toCleanup = append(toCleanup, secret)

		By("Creating a BMH with inspection and cleaning disabled")
		bmh := metal3api.BareMetalHost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      bmhName,
				Namespace: namespace.Name,
			},
			Spec: metal3api.BareMetalHostSpec{
				Online: true,
				BMC: metal3api.BMCDetails{
					Address:                        bmc.Address,
					CredentialsName:                secretName,
					DisableCertificateVerification: bmc.DisableCertificateVerification,
				},
				BootMode:              metal3api.BootMode(e2eConfig.GetVariable("BOOT_MODE")),
				BootMACAddress:        bmc.BootMacAddress,
				AutomatedCleaningMode: metal3api.CleaningModeDisabled,
				InspectionMode:        metal3api.InspectionModeDisabled,
			},
		}
		Expect(clusterProxy.GetClient().Create(ctx, &bmh)).To(Succeed())
		toCleanup = append(toCleanup, &bmh)

		By("Waiting for the BMH to become available")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: clusterProxy.GetClient(),
			Bmh:    bmh,
			State:  metal3api.StateAvailable,
		}, e2eConfig.GetIntervals(specName, "wait-available")...)

		By("Verifying that HostFirmwareComponents was created automatically")
		hfc := &metal3api.HostFirmwareComponents{}
		Expect(clusterProxy.GetClient().Get(ctx, types.NamespacedName{
			Name:      bmhName,
			Namespace: namespace.Name,
		}, hfc)).To(Succeed())

		By("Provisioning the BMH")
		var userDataSecret *corev1.SecretReference
		if e2eConfig.GetVariable("SSH_CHECK_PROVISIONED") == "true" {
			userDataSecretName := "user-data"
			sshPubKeyPath := e2eConfig.GetVariable("SSH_PUB_KEY")
			createSSHSetupUserdata(ctx, clusterProxy.GetClient(), namespace.Name, userDataSecretName, sshPubKeyPath, bmc.IPAddress)
			userDataSecret = &corev1.SecretReference{
				Name:      userDataSecretName,
				Namespace: namespace.Name,
			}
		}
		Expect(PatchBMHForProvisioning(ctx, PatchBMHForProvisioningInput{
			client:         clusterProxy.GetClient(),
			bmh:            &bmh,
			bmc:            bmc,
			e2eConfig:      e2eConfig,
			namespace:      namespace.Name,
			userDataSecret: userDataSecret,
		})).To(Succeed())

		By("Waiting for the BMH to be provisioned")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: clusterProxy.GetClient(),
			Bmh:    bmh,
			State:  metal3api.StateProvisioned,
		}, e2eConfig.GetIntervals(specName, "wait-provisioned")...)

		By("Creating a HostUpdatePolicy to allow firmware updates on reboot")
		hup := &metal3api.HostUpdatePolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      bmhName,
				Namespace: namespace.Name,
			},
			Spec: metal3api.HostUpdatePolicySpec{
				FirmwareUpdates: metal3api.HostUpdatePolicyOnReboot,
			},
		}
		Expect(clusterProxy.GetClient().Create(ctx, hup)).To(Succeed())

		By("Updating HostFirmwareComponents with a BIOS update")
		helper, err := patch.NewHelper(hfc, clusterProxy.GetClient())
		Expect(err).NotTo(HaveOccurred())
		hfc.Spec.Updates = []metal3api.FirmwareUpdate{
			{
				Component: "bios",
				URL:       biosFirmwareUpdateURL + "servicingv1",
			},
		}
		Expect(helper.Patch(ctx, hfc)).To(Succeed())

		By("Verifying the conditions on HFC")
		Eventually(func(g Gomega) {
			g.Expect(clusterProxy.GetClient().Get(ctx, types.NamespacedName{
				Name:      bmhName,
				Namespace: namespace.Name,
			}, hfc)).To(Succeed())
			g.Expect(hfc.Status.Conditions).To(ContainCondition(string(metal3api.HostFirmwareComponentsValid), metav1.ConditionTrue))
			g.Expect(hfc.Status.Conditions).To(ContainCondition(string(metal3api.HostFirmwareComponentsChangeDetected), metav1.ConditionTrue))
		}, e2eConfig.GetIntervals(specName, "wait-reconcile")...).Should(Succeed())

		By("Triggering a reboot via annotation")
		Expect(clusterProxy.GetClient().Get(ctx, types.NamespacedName{
			Name:      bmhName,
			Namespace: namespace.Name,
		}, &bmh)).To(Succeed())
		// Use hard reboot to avoid wasting time on soft power off.
		// CirrOS may not reliably handle ACPI shutdown events, causing Ironic to
		// wait up to 180s before falling back to hard power off anyway.
		AnnotateBmh(ctx, clusterProxy.GetClient(), bmh, metal3api.RebootAnnotationPrefix, ptr.To(`{"mode": "hard"}`))

		By("Waiting for the BMH to enter servicing")
		WaitForBmhInOperationalStatus(ctx, WaitForBmhInOperationalStatusInput{
			Client: clusterProxy.GetClient(),
			Bmh:    bmh,
			State:  metal3api.OperationalStatusServicing,
			UndesiredStates: []metal3api.OperationalStatus{
				metal3api.OperationalStatusError,
			},
		}, e2eConfig.GetIntervals(specName, "wait-servicing")...)

		By("Waiting for servicing to complete")
		WaitForBmhInOperationalStatus(ctx, WaitForBmhInOperationalStatusInput{
			Client: clusterProxy.GetClient(),
			Bmh:    bmh,
			State:  metal3api.OperationalStatusOK,
			UndesiredStates: []metal3api.OperationalStatus{
				metal3api.OperationalStatusError,
			},
		}, e2eConfig.GetIntervals(specName, "wait-firmware-components")...)

		By("Verifying the HFC Status")
		Expect(clusterProxy.GetClient().Get(ctx, types.NamespacedName{
			Name:      bmhName,
			Namespace: namespace.Name,
		}, hfc)).To(Succeed())
		Expect(hfc.Status.Updates).To(HaveLen(1))
		Expect(hfc.Status.Updates[0].Component).To(Equal("bios"))
		Expect(hfc.Status.Updates[0].URL).To(Equal(biosFirmwareUpdateURL + "servicingv1"))
		Expect(hfc.Status.Conditions).To(ContainCondition(string(metal3api.HostFirmwareComponentsChangeDetected), metav1.ConditionFalse))
		Expect(hfc.Status.Conditions).To(ContainCondition(string(metal3api.HostFirmwareComponentsValid), metav1.ConditionTrue))

		By("Waiting for the HFC controller to update BIOS component status from Ironic")
		Eventually(func(g Gomega) {
			g.Expect(clusterProxy.GetClient().Get(ctx, types.NamespacedName{
				Name:      bmhName,
				Namespace: namespace.Name,
			}, hfc)).To(Succeed())
			var biosComponent *metal3api.FirmwareComponentStatus
			for i := range hfc.Status.Components {
				if hfc.Status.Components[i].Component == "bios" {
					biosComponent = &hfc.Status.Components[i]
					break
				}
			}
			g.Expect(biosComponent).NotTo(BeNil(), "bios component should exist in status.components")
			g.Expect(biosComponent.InitialVersion).NotTo(BeEmpty())
			g.Expect(biosComponent.CurrentVersion).NotTo(BeEmpty())
			g.Expect(biosComponent.CurrentVersion).NotTo(Equal(biosComponent.InitialVersion),
				"CurrentVersion should differ from InitialVersion after firmware update")
		}, e2eConfig.GetIntervals(specName, "wait-firmware-components")...).Should(Succeed())

		if e2eConfig.GetVariable("SSH_CHECK_PROVISIONED") == "true" {
			By("Verifying the instance is still accessible via SSH")
			PerformSSHBootCheck(e2eConfig, "disk", bmc.IPAddress)
		} else {
			Logf("WARNING: Skipping SSH check since SSH_CHECK_PROVISIONED != true")
		}

		By("Deleting the BMH")
		Expect(clusterProxy.GetClient().Delete(ctx, &bmh)).To(Succeed())

		By("Waiting for the BMH to be deleted")
		WaitForBmhDeleted(ctx, WaitForBmhDeletedInput{
			Client:    clusterProxy.GetClient(),
			BmhName:   bmhName,
			Namespace: namespace.Name,
		}, e2eConfig.GetIntervals(specName, "wait-bmh-deleted")...)

		By("Making sure HFC was deleted too")
		Eventually(func() bool {
			return k8serrors.IsNotFound(clusterProxy.GetClient().Get(ctx, types.NamespacedName{
				Name:      bmhName,
				Namespace: namespace.Name,
			}, hfc))
		}, e2eConfig.GetIntervals(specName, "wait-bmh-deleted")...).Should(BeTrue())

		By("Making sure HUP was deleted too")
		Eventually(func() bool {
			return k8serrors.IsNotFound(clusterProxy.GetClient().Get(ctx, types.NamespacedName{
				Name:      bmhName,
				Namespace: namespace.Name,
			}, hup))
		}, e2eConfig.GetIntervals(specName, "wait-bmh-deleted")...).Should(BeTrue())
	})

	It("should update BIOS firmware on an externally provisioned host via servicing", func() {
		bmhName := specName + "-ext-servicing"
		secretName := bmhName + "-bmc"

		By("Creating a secret with BMH credentials")
		bmcCredentialsData := map[string]string{
			"username": bmc.User,
			"password": bmc.Password,
		}
		secret := CreateSecret(ctx, clusterProxy.GetClient(), namespace.Name, secretName, bmcCredentialsData)
		toCleanup = append(toCleanup, secret)

		By("Creating a BMH with externally provisioned flag")
		bmh := metal3api.BareMetalHost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      bmhName,
				Namespace: namespace.Name,
			},
			Spec: metal3api.BareMetalHostSpec{
				Online: true,
				BMC: metal3api.BMCDetails{
					Address:                        bmc.Address,
					CredentialsName:                secretName,
					DisableCertificateVerification: bmc.DisableCertificateVerification,
				},
				BootMode:              metal3api.BootMode(e2eConfig.GetVariable("BOOT_MODE")),
				BootMACAddress:        bmc.BootMacAddress,
				ExternallyProvisioned: true,
			},
		}
		Expect(clusterProxy.GetClient().Create(ctx, &bmh)).To(Succeed())
		toCleanup = append(toCleanup, &bmh)

		By("Waiting for the BMH to become externally provisioned")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: clusterProxy.GetClient(),
			Bmh:    bmh,
			State:  metal3api.StateExternallyProvisioned,
		}, e2eConfig.GetIntervals(specName, "wait-externally-provisioned")...)

		By("Verifying that HostFirmwareComponents was created automatically")
		hfc2 := &metal3api.HostFirmwareComponents{}
		Expect(clusterProxy.GetClient().Get(ctx, types.NamespacedName{
			Name:      bmhName,
			Namespace: namespace.Name,
		}, hfc2)).To(Succeed())

		By("Creating a HostUpdatePolicy to allow firmware updates on reboot")
		hup2 := &metal3api.HostUpdatePolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      bmhName,
				Namespace: namespace.Name,
			},
			Spec: metal3api.HostUpdatePolicySpec{
				FirmwareUpdates: metal3api.HostUpdatePolicyOnReboot,
			},
		}
		Expect(clusterProxy.GetClient().Create(ctx, hup2)).To(Succeed())

		By("Updating HostFirmwareComponents with a BIOS update")
		helper, err := patch.NewHelper(hfc2, clusterProxy.GetClient())
		Expect(err).NotTo(HaveOccurred())
		hfc2.Spec.Updates = []metal3api.FirmwareUpdate{
			{
				Component: "bios",
				URL:       biosFirmwareUpdateURL + "servicingv2",
			},
		}
		Expect(helper.Patch(ctx, hfc2)).To(Succeed())

		By("Verifying the conditions on HFC")
		Eventually(func(g Gomega) {
			g.Expect(clusterProxy.GetClient().Get(ctx, types.NamespacedName{
				Name:      bmhName,
				Namespace: namespace.Name,
			}, hfc2)).To(Succeed())
			g.Expect(hfc2.Status.Conditions).To(ContainCondition(string(metal3api.HostFirmwareComponentsValid), metav1.ConditionTrue))
			g.Expect(hfc2.Status.Conditions).To(ContainCondition(string(metal3api.HostFirmwareComponentsChangeDetected), metav1.ConditionTrue))
		}, e2eConfig.GetIntervals(specName, "wait-reconcile")...).Should(Succeed())

		By("Triggering a reboot via annotation")
		Expect(clusterProxy.GetClient().Get(ctx, types.NamespacedName{
			Name:      bmhName,
			Namespace: namespace.Name,
		}, &bmh)).To(Succeed())
		// Use hard reboot to avoid wasting time on soft power off.
		// CirrOS may not reliably handle ACPI shutdown events, causing Ironic to
		// wait up to 180s before falling back to hard power off anyway.
		AnnotateBmh(ctx, clusterProxy.GetClient(), bmh, metal3api.RebootAnnotationPrefix, ptr.To(`{"mode": "hard"}`))

		By("Waiting for the BMH to enter servicing")
		WaitForBmhInOperationalStatus(ctx, WaitForBmhInOperationalStatusInput{
			Client: clusterProxy.GetClient(),
			Bmh:    bmh,
			State:  metal3api.OperationalStatusServicing,
			UndesiredStates: []metal3api.OperationalStatus{
				metal3api.OperationalStatusError,
			},
		}, e2eConfig.GetIntervals(specName, "wait-servicing")...)

		By("Waiting for servicing to complete")
		WaitForBmhInOperationalStatus(ctx, WaitForBmhInOperationalStatusInput{
			Client: clusterProxy.GetClient(),
			Bmh:    bmh,
			State:  metal3api.OperationalStatusOK,
			UndesiredStates: []metal3api.OperationalStatus{
				metal3api.OperationalStatusError,
			},
		}, e2eConfig.GetIntervals(specName, "wait-firmware-components")...)

		By("Verifying the HFC Status")
		Expect(clusterProxy.GetClient().Get(ctx, types.NamespacedName{
			Name:      bmhName,
			Namespace: namespace.Name,
		}, hfc2)).To(Succeed())
		Expect(hfc2.Status.Updates).To(HaveLen(1))
		Expect(hfc2.Status.Updates[0].Component).To(Equal("bios"))
		Expect(hfc2.Status.Updates[0].URL).To(Equal(biosFirmwareUpdateURL + "servicingv2"))
		Expect(hfc2.Status.Conditions).To(ContainCondition(string(metal3api.HostFirmwareComponentsChangeDetected), metav1.ConditionFalse))
		Expect(hfc2.Status.Conditions).To(ContainCondition(string(metal3api.HostFirmwareComponentsValid), metav1.ConditionTrue))

		By("Waiting for the HFC controller to update BIOS component status from Ironic")
		Eventually(func(g Gomega) {
			g.Expect(clusterProxy.GetClient().Get(ctx, types.NamespacedName{
				Name:      bmhName,
				Namespace: namespace.Name,
			}, hfc2)).To(Succeed())
			var biosComponent2 *metal3api.FirmwareComponentStatus
			for i := range hfc2.Status.Components {
				if hfc2.Status.Components[i].Component == "bios" {
					biosComponent2 = &hfc2.Status.Components[i]
					break
				}
			}
			g.Expect(biosComponent2).NotTo(BeNil(), "bios component should exist in status.components")
			g.Expect(biosComponent2.InitialVersion).NotTo(BeEmpty())
			g.Expect(biosComponent2.CurrentVersion).NotTo(BeEmpty())
			g.Expect(biosComponent2.CurrentVersion).NotTo(Equal(biosComponent2.InitialVersion),
				"CurrentVersion should differ from InitialVersion after firmware update")
		}, e2eConfig.GetIntervals(specName, "wait-firmware-components")...).Should(Succeed())

		By("Deleting the BMH")
		Expect(clusterProxy.GetClient().Delete(ctx, &bmh)).To(Succeed())

		By("Waiting for the BMH to be deleted")
		WaitForBmhDeleted(ctx, WaitForBmhDeletedInput{
			Client:    clusterProxy.GetClient(),
			BmhName:   bmhName,
			Namespace: namespace.Name,
		}, e2eConfig.GetIntervals(specName, "wait-bmh-deleted")...)

		By("Making sure HFC was deleted too")
		Eventually(func() bool {
			return k8serrors.IsNotFound(clusterProxy.GetClient().Get(ctx, types.NamespacedName{
				Name:      bmhName,
				Namespace: namespace.Name,
			}, hfc2))
		}, e2eConfig.GetIntervals(specName, "wait-bmh-deleted")...).Should(BeTrue())

		By("Making sure HUP was deleted too")
		Eventually(func() bool {
			return k8serrors.IsNotFound(clusterProxy.GetClient().Get(ctx, types.NamespacedName{
				Name:      bmhName,
				Namespace: namespace.Name,
			}, hup2))
		}, e2eConfig.GetIntervals(specName, "wait-bmh-deleted")...).Should(BeTrue())

	})

	AfterEach(func() {
		CollectSerialLogs(bmc.Name, path.Join(artifactFolder, specName))
		DumpResources(ctx, e2eConfig, clusterProxy, path.Join(artifactFolder, specName))
		if !skipCleanup {
			Cleanup(ctx, clusterProxy, namespace, cancelWatches, e2eConfig, toCleanup)
		}
	})
})
