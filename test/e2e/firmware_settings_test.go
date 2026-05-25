//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"path"
	"strings"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// These example keys and values are hardcoded in sushy-tools.
const (
	hfsTestKey1       = "ProcTurboMode"
	hfsTestOrigValue1 = "Enabled"
	hfsTestNewValue1  = "Disabled"
	hfsTestKey2       = "EmbeddedSata"
	hfsTestOrigValue2 = "Raid"
	hfsTestNewValue2  = "Ata"
	hfsTestKey3       = "SerialNumber"
	hfsTestOrigValue3 = "QPX12345"
	hfsTestNewValue3  = "ABCDEF"
)

var _ = Describe("Host Firmware Settings", Label("required", "firmware"), func() {
	var (
		specName      = "firmware-settings"
		namespace     *corev1.Namespace
		cancelWatches context.CancelFunc
		toCleanup     []client.Object
	)

	BeforeEach(func() {
		// FIXME(dtantsur): find a more elegant way to check for this feature
		if !e2eConfig.GetBoolVariable("DEPLOY_IRONIC") || !strings.Contains(bmc.Address, "redfish") {
			Skip("HFS tests require a real Ironic and a host with Redfish")
		}

		// Ensure that tests don't conflict with each other
		RedfishResetBios(ctx, bmc)

		toCleanup = nil
		namespace, cancelWatches = framework.CreateNamespaceAndWatchEvents(ctx, framework.CreateNamespaceAndWatchEventsInput{
			Creator:             clusterProxy.GetClient(),
			ClientSet:           clusterProxy.GetClientSet(),
			Name:                specName,
			LogFolder:           artifactFolder,
			IgnoreAlreadyExists: true,
		})
	})

	It("should apply firmware settings created before the host, then apply a new value for available host", func() {
		bmhName := specName + "-before-bmh"
		secretName := bmhName + "-bmc"

		By("Creating a secret with BMH credentials")
		bmcCredentialsData := map[string]string{
			"username": bmc.User,
			"password": bmc.Password,
		}
		secret := CreateSecret(ctx, clusterProxy.GetClient(), namespace.Name, secretName, bmcCredentialsData)
		toCleanup = append(toCleanup, secret)

		By("Creating a HostFirmwareSettings with modified value before BMH")
		hfs := &metal3api.HostFirmwareSettings{
			ObjectMeta: metav1.ObjectMeta{
				Name:      bmhName,
				Namespace: namespace.Name,
			},
			Spec: metal3api.HostFirmwareSettingsSpec{
				Settings: metal3api.DesiredSettingsMap{
					hfsTestKey1: intstr.FromString(hfsTestNewValue1),
				},
			},
		}
		Expect(clusterProxy.GetClient().Create(ctx, hfs)).To(Succeed())

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
		}, e2eConfig.GetIntervals(specName, "wait-firmware-settings")...)

		By("Verifying the firmware setting was applied in HFS status")
		Expect(clusterProxy.GetClient().Get(ctx, types.NamespacedName{
			Name:      bmhName,
			Namespace: namespace.Name,
		}, hfs)).To(Succeed())
		Expect(hfs.Status.Settings).To(HaveKeyWithValue(hfsTestKey1, hfsTestNewValue1))
		Expect(hfs.Status.Conditions).To(ContainCondition(string(metal3api.FirmwareSettingsValid), metav1.ConditionTrue))
		Expect(hfs.Status.Conditions).To(ContainCondition(string(metal3api.FirmwareSettingsChangeDetected), metav1.ConditionFalse))

		By("Checking firmware schema")
		fSchema := &metal3api.FirmwareSchema{}
		Expect(clusterProxy.GetClient().Get(ctx, types.NamespacedName{
			Name:      hfs.Status.FirmwareSchema.Name,
			Namespace: hfs.Status.FirmwareSchema.Namespace,
		}, fSchema)).To(Succeed())
		Expect(fSchema.Spec.Schema).To(HaveKey(hfsTestKey1))
		Expect(fSchema.Spec.Schema).To(HaveKey(hfsTestKey2))

		By("Deleting the BMH to get rid of cached values")
		Expect(clusterProxy.GetClient().Delete(ctx, &bmh)).To(Succeed())

		By("Waiting for the BMH to be deleted")
		WaitForBmhDeleted(ctx, WaitForBmhDeletedInput{
			Client:    clusterProxy.GetClient(),
			BmhName:   bmhName,
			Namespace: namespace.Name,
		}, e2eConfig.GetIntervals(specName, "wait-bmh-deleted")...)

		By("Making sure HFS was deleted too")
		Eventually(func() bool {
			return k8serrors.IsNotFound(clusterProxy.GetClient().Get(ctx, types.NamespacedName{
				Name:      bmhName,
				Namespace: namespace.Name,
			}, hfs))
		}, e2eConfig.GetIntervals(specName, "wait-bmh-deleted")...).Should(BeTrue())

		By("Creating a secret with BMH credentials again")
		CreateSecret(ctx, clusterProxy.GetClient(), namespace.Name, secretName, bmcCredentialsData)

		By("Re-creating the BMH to check that the settings were saved in the backend")
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

		By("Verifying the updated firmware setting in HFS status")
		Expect(clusterProxy.GetClient().Get(ctx, types.NamespacedName{
			Name:      bmhName,
			Namespace: namespace.Name,
		}, hfs)).To(Succeed())
		Expect(hfs.Status.Settings).To(HaveKeyWithValue(hfsTestKey1, hfsTestNewValue1))
		Expect(hfs.Status.Conditions).To(ContainCondition(string(metal3api.FirmwareSettingsValid), metav1.ConditionTrue))
		Expect(hfs.Status.Conditions).To(ContainCondition(string(metal3api.FirmwareSettingsChangeDetected), metav1.ConditionFalse))

		By("Updating HostFirmwareSettings with the original value")
		helper, err := patch.NewHelper(hfs, clusterProxy.GetClient())
		Expect(err).NotTo(HaveOccurred())
		hfs.Spec.Settings = metal3api.DesiredSettingsMap{
			hfsTestKey1: intstr.FromString(hfsTestOrigValue1),
		}
		Expect(helper.Patch(ctx, hfs)).To(Succeed())

		By("Verifying the conditions on firmware setting")
		Eventually(func(g Gomega) {
			g.Expect(clusterProxy.GetClient().Get(ctx, types.NamespacedName{
				Name:      bmhName,
				Namespace: namespace.Name,
			}, hfs)).To(Succeed())
			g.Expect(hfs.Status.Conditions).To(ContainCondition(string(metal3api.FirmwareSettingsValid), metav1.ConditionTrue))
			g.Expect(hfs.Status.Conditions).To(ContainCondition(string(metal3api.FirmwareSettingsChangeDetected), metav1.ConditionTrue))
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
		}, e2eConfig.GetIntervals(specName, "wait-firmware-settings")...)

		By("Verifying the firmware setting was applied in HFS status")
		Expect(clusterProxy.GetClient().Get(ctx, types.NamespacedName{
			Name:      bmhName,
			Namespace: namespace.Name,
		}, hfs)).To(Succeed())
		Expect(hfs.Status.Settings).To(HaveKeyWithValue(hfsTestKey1, hfsTestOrigValue1))
		Expect(hfs.Status.Conditions).To(ContainCondition(string(metal3api.FirmwareSettingsValid), metav1.ConditionTrue))
		Expect(hfs.Status.Conditions).To(ContainCondition(string(metal3api.FirmwareSettingsChangeDetected), metav1.ConditionFalse))

		By("Deleting the BMH")
		Expect(clusterProxy.GetClient().Delete(ctx, &bmh)).To(Succeed())

		By("Waiting for the BMH to be deleted")
		WaitForBmhDeleted(ctx, WaitForBmhDeletedInput{
			Client:    clusterProxy.GetClient(),
			BmhName:   bmhName,
			Namespace: namespace.Name,
		}, e2eConfig.GetIntervals(specName, "wait-bmh-deleted")...)
	})

	It("should update firmware settings on a provisioned host via servicing", func() {
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

		By("Checking the original value of the parameter")
		origHFS := &metal3api.HostFirmwareSettings{}
		Expect(clusterProxy.GetClient().Get(ctx, types.NamespacedName{
			Name:      bmhName,
			Namespace: namespace.Name,
		}, origHFS)).To(Succeed())
		Expect(origHFS.Status.Settings).To(HaveKeyWithValue(hfsTestKey2, hfsTestOrigValue2))

		By("Checking firmware schema")
		fSchema := &metal3api.FirmwareSchema{}
		Expect(clusterProxy.GetClient().Get(ctx, types.NamespacedName{
			Name:      origHFS.Status.FirmwareSchema.Name,
			Namespace: origHFS.Status.FirmwareSchema.Namespace,
		}, fSchema)).To(Succeed())
		Expect(fSchema.Spec.Schema).To(HaveKey(hfsTestKey1))
		Expect(fSchema.Spec.Schema).To(HaveKey(hfsTestKey2))

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

		By("Creating a HostUpdatePolicy to allow firmware changes on reboot")
		hup := &metal3api.HostUpdatePolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      bmhName,
				Namespace: namespace.Name,
			},
			Spec: metal3api.HostUpdatePolicySpec{
				FirmwareSettings: metal3api.HostUpdatePolicyOnReboot,
			},
		}
		Expect(clusterProxy.GetClient().Create(ctx, hup)).To(Succeed())

		By("Updating HostFirmwareSettings with a new value")
		hfs := &metal3api.HostFirmwareSettings{}
		Expect(clusterProxy.GetClient().Get(ctx, types.NamespacedName{
			Name:      bmhName,
			Namespace: namespace.Name,
		}, hfs)).To(Succeed())

		helper, err := patch.NewHelper(hfs, clusterProxy.GetClient())
		Expect(err).NotTo(HaveOccurred())
		hfs.Spec.Settings = metal3api.DesiredSettingsMap{
			hfsTestKey2: intstr.FromString(hfsTestNewValue2),
		}
		Expect(helper.Patch(ctx, hfs)).To(Succeed())

		By("Verifying the conditions on firmware setting")
		Eventually(func(g Gomega) {
			g.Expect(clusterProxy.GetClient().Get(ctx, types.NamespacedName{
				Name:      bmhName,
				Namespace: namespace.Name,
			}, hfs)).To(Succeed())
			g.Expect(hfs.Status.Conditions).To(ContainCondition(string(metal3api.FirmwareSettingsValid), metav1.ConditionTrue))
			g.Expect(hfs.Status.Conditions).To(ContainCondition(string(metal3api.FirmwareSettingsChangeDetected), metav1.ConditionTrue))
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
		}, e2eConfig.GetIntervals(specName, "wait-firmware-settings")...)

		By("Verifying the firmware setting was applied in HFS status")
		Expect(clusterProxy.GetClient().Get(ctx, types.NamespacedName{
			Name:      bmhName,
			Namespace: namespace.Name,
		}, hfs)).To(Succeed())
		Expect(hfs.Status.Settings).To(HaveKeyWithValue(hfsTestKey2, hfsTestNewValue2))
		Expect(hfs.Status.Conditions).To(ContainCondition(string(metal3api.FirmwareSettingsValid), metav1.ConditionTrue))
		Expect(hfs.Status.Conditions).To(ContainCondition(string(metal3api.FirmwareSettingsChangeDetected), metav1.ConditionFalse))

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

		By("Making sure HFS was deleted too")
		Eventually(func() bool {
			return k8serrors.IsNotFound(clusterProxy.GetClient().Get(ctx, types.NamespacedName{
				Name:      bmhName,
				Namespace: namespace.Name,
			}, hfs))
		}, e2eConfig.GetIntervals(specName, "wait-bmh-deleted")...).Should(BeTrue())
	})

	It("should update firmware settings on an externally provisioned host via servicing", func() {
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

		By("Checking the original value of the parameter")
		origHFS := &metal3api.HostFirmwareSettings{}
		Eventually(func(g Gomega) {
			g.Expect(clusterProxy.GetClient().Get(ctx, types.NamespacedName{
				Name:      bmhName,
				Namespace: namespace.Name,
			}, origHFS)).To(Succeed())
			g.Expect(origHFS.Status.Settings).To(HaveKeyWithValue(hfsTestKey3, hfsTestOrigValue3))
		}, e2eConfig.GetIntervals(specName, "wait-reconcile")...).Should(Succeed())

		By("Checking firmware schema")
		fSchema := &metal3api.FirmwareSchema{}
		Expect(clusterProxy.GetClient().Get(ctx, types.NamespacedName{
			Name:      origHFS.Status.FirmwareSchema.Name,
			Namespace: origHFS.Status.FirmwareSchema.Namespace,
		}, fSchema)).To(Succeed())
		Expect(fSchema.Spec.Schema).To(HaveKey(hfsTestKey1))
		Expect(fSchema.Spec.Schema).To(HaveKey(hfsTestKey2))
		Expect(fSchema.Spec.Schema).To(HaveKey(hfsTestKey3))

		By("Creating a HostUpdatePolicy to allow firmware changes on reboot")
		hup := &metal3api.HostUpdatePolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      bmhName,
				Namespace: namespace.Name,
			},
			Spec: metal3api.HostUpdatePolicySpec{
				FirmwareSettings: metal3api.HostUpdatePolicyOnReboot,
			},
		}
		Expect(clusterProxy.GetClient().Create(ctx, hup)).To(Succeed())

		By("Updating HostFirmwareSettings with a new value")
		hfs := &metal3api.HostFirmwareSettings{}
		Expect(clusterProxy.GetClient().Get(ctx, types.NamespacedName{
			Name:      bmhName,
			Namespace: namespace.Name,
		}, hfs)).To(Succeed())

		helper, err := patch.NewHelper(hfs, clusterProxy.GetClient())
		Expect(err).NotTo(HaveOccurred())
		hfs.Spec.Settings = metal3api.DesiredSettingsMap{
			hfsTestKey3: intstr.FromString(hfsTestNewValue3),
		}
		Expect(helper.Patch(ctx, hfs)).To(Succeed())

		By("Verifying the conditions on firmware setting")
		Eventually(func(g Gomega) {
			g.Expect(clusterProxy.GetClient().Get(ctx, types.NamespacedName{
				Name:      bmhName,
				Namespace: namespace.Name,
			}, hfs)).To(Succeed())
			g.Expect(hfs.Status.Conditions).To(ContainCondition(string(metal3api.FirmwareSettingsValid), metav1.ConditionTrue))
			g.Expect(hfs.Status.Conditions).To(ContainCondition(string(metal3api.FirmwareSettingsChangeDetected), metav1.ConditionTrue))
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
		}, e2eConfig.GetIntervals(specName, "wait-firmware-settings")...)

		By("Verifying the firmware setting was applied in HFS status")
		Expect(clusterProxy.GetClient().Get(ctx, types.NamespacedName{
			Name:      bmhName,
			Namespace: namespace.Name,
		}, hfs)).To(Succeed())
		Expect(hfs.Status.Settings).To(HaveKeyWithValue(hfsTestKey3, hfsTestNewValue3))
		Expect(hfs.Status.Conditions).To(ContainCondition(string(metal3api.FirmwareSettingsValid), metav1.ConditionTrue))
		Expect(hfs.Status.Conditions).To(ContainCondition(string(metal3api.FirmwareSettingsChangeDetected), metav1.ConditionFalse))
	})

	It("should abort servicing when HostFirmwareSettings spec is cleared during servicing", func() {
		bmhName := specName + "-servicing-abort"
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

		By("Provisioning the BMH")
		Expect(PatchBMHForProvisioning(ctx, PatchBMHForProvisioningInput{
			client:    clusterProxy.GetClient(),
			bmh:       &bmh,
			bmc:       bmc,
			e2eConfig: e2eConfig,
			namespace: namespace.Name,
		})).To(Succeed())

		By("Waiting for the BMH to be provisioned")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: clusterProxy.GetClient(),
			Bmh:    bmh,
			State:  metal3api.StateProvisioned,
		}, e2eConfig.GetIntervals(specName, "wait-provisioned")...)

		By("Creating a HostUpdatePolicy to allow firmware changes on reboot")
		hup := &metal3api.HostUpdatePolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      bmhName,
				Namespace: namespace.Name,
			},
			Spec: metal3api.HostUpdatePolicySpec{
				FirmwareSettings: metal3api.HostUpdatePolicyOnReboot,
			},
		}
		Expect(clusterProxy.GetClient().Create(ctx, hup)).To(Succeed())

		By("Updating HostFirmwareSettings with a new value")
		hfs := &metal3api.HostFirmwareSettings{}
		Expect(clusterProxy.GetClient().Get(ctx, types.NamespacedName{
			Name:      bmhName,
			Namespace: namespace.Name,
		}, hfs)).To(Succeed())

		helper, err := patch.NewHelper(hfs, clusterProxy.GetClient())
		Expect(err).NotTo(HaveOccurred())
		hfs.Spec.Settings = metal3api.DesiredSettingsMap{
			hfsTestKey2: intstr.FromString(hfsTestNewValue2),
		}
		Expect(helper.Patch(ctx, hfs)).To(Succeed())

		By("Verifying the conditions on firmware setting")
		Eventually(func(g Gomega) {
			g.Expect(clusterProxy.GetClient().Get(ctx, types.NamespacedName{
				Name:      bmhName,
				Namespace: namespace.Name,
			}, hfs)).To(Succeed())
			g.Expect(hfs.Status.Conditions).To(ContainCondition(string(metal3api.FirmwareSettingsValid), metav1.ConditionTrue))
			g.Expect(hfs.Status.Conditions).To(ContainCondition(string(metal3api.FirmwareSettingsChangeDetected), metav1.ConditionTrue))
		}, e2eConfig.GetIntervals(specName, "wait-reconcile")...).Should(Succeed())

		By("Triggering a reboot via annotation")
		Expect(clusterProxy.GetClient().Get(ctx, types.NamespacedName{
			Name:      bmhName,
			Namespace: namespace.Name,
		}, &bmh)).To(Succeed())
		// Use hard reboot to avoid wasting time on soft power off.
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

		By("Clearing HostFirmwareSettings spec to abort servicing")
		Expect(clusterProxy.GetClient().Get(ctx, types.NamespacedName{
			Name:      bmhName,
			Namespace: namespace.Name,
		}, hfs)).To(Succeed())
		helper, err = patch.NewHelper(hfs, clusterProxy.GetClient())
		Expect(err).NotTo(HaveOccurred())
		// spec.settings is required by CRD validation; use empty map, not nil.
		hfs.Spec.Settings = metal3api.DesiredSettingsMap{}
		Expect(helper.Patch(ctx, hfs)).To(Succeed())

		By("Waiting for servicing to abort and the BMH to return to OK")
		WaitForBmhInOperationalStatus(ctx, WaitForBmhInOperationalStatusInput{
			Client: clusterProxy.GetClient(),
			Bmh:    bmh,
			State:  metal3api.OperationalStatusOK,
			UndesiredStates: []metal3api.OperationalStatus{
				metal3api.OperationalStatusError,
				metal3api.OperationalStatusServicing,
			},
		}, e2eConfig.GetIntervals(specName, "wait-firmware-settings")...)

		By("Verifying the host is not stuck in ServicingError")
		Expect(clusterProxy.GetClient().Get(ctx, types.NamespacedName{
			Name:      bmhName,
			Namespace: namespace.Name,
		}, &bmh)).To(Succeed())
		Expect(bmh.Status.ErrorType).To(BeEmpty())

		By("Deleting the BMH")
		Expect(clusterProxy.GetClient().Delete(ctx, &bmh)).To(Succeed())

		By("Waiting for the BMH to be deleted")
		WaitForBmhDeleted(ctx, WaitForBmhDeletedInput{
			Client:    clusterProxy.GetClient(),
			BmhName:   bmhName,
			Namespace: namespace.Name,
		}, e2eConfig.GetIntervals(specName, "wait-bmh-deleted")...)
	})

	AfterEach(func() {
		CollectSerialLogs(bmc.Name, path.Join(artifactFolder, specName))
		DumpResources(ctx, e2eConfig, clusterProxy, path.Join(artifactFolder, specName))
		if !skipCleanup {
			Cleanup(ctx, clusterProxy, namespace, cancelWatches, e2eConfig, toCleanup)
		}
	})
})
