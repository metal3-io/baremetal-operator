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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/util/patch"
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
	)

	BeforeEach(func() {
		if !e2eConfig.GetBoolVariable("DEPLOY_IRONIC") || !strings.Contains(bmc.Address, "redfish") {
			Skip("Firmware component tests require a real Ironic and a host with Redfish")
		}

		namespace, cancelWatches = framework.CreateNamespaceAndWatchEvents(ctx, framework.CreateNamespaceAndWatchEventsInput{
			Creator:             clusterProxy.GetClient(),
			ClientSet:           clusterProxy.GetClientSet(),
			Name:                specName,
			LogFolder:           artifactFolder,
			IgnoreAlreadyExists: true,
		})
	})

	It("should upgrade BIOS firmware via cleaning when HostFirmwareComponents is updated on an available host", func() {
		bmhName := specName + "-bios-cleaning"
		secretName := bmhName + "-bmc"

		By("Creating a secret with BMH credentials")
		bmcCredentialsData := map[string]string{
			"username": bmc.User,
			"password": bmc.Password,
		}
		CreateSecret(ctx, clusterProxy.GetClient(), namespace.Name, secretName, bmcCredentialsData)

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
		Expect(hfc.Status.Conditions).To(ContainCondition(string(metal3api.HostFirmwareComponentsValid), metav1.ConditionTrue))
		Expect(hfc.Status.Conditions).To(ContainCondition(string(metal3api.HostFirmwareComponentsChangeDetected), metav1.ConditionFalse))

		By("Recording initial BIOS component version from status")
		var initialBiosVersion string
		for _, comp := range hfc.Status.Components {
			if comp.Component == "bios" {
				initialBiosVersion = comp.CurrentVersion
				break
			}
		}
		Logf("Initial BIOS version: %s", initialBiosVersion)

		By("Updating HostFirmwareComponents with a BIOS firmware update")
		helper, err := patch.NewHelper(hfc, clusterProxy.GetClient())
		Expect(err).NotTo(HaveOccurred())
		hfc.Spec.Updates = []metal3api.FirmwareUpdate{
			{
				Component: "bios",
				URL:       biosFirmwareUpdateURL,
			},
		}
		Expect(helper.Patch(ctx, hfc)).To(Succeed())

		By("Verifying the ChangeDetected condition becomes True")
		Eventually(func(g Gomega) {
			g.Expect(clusterProxy.GetClient().Get(ctx, types.NamespacedName{
				Name:      bmhName,
				Namespace: namespace.Name,
			}, hfc)).To(Succeed())
			g.Expect(hfc.Status.Conditions).To(ContainCondition(string(metal3api.HostFirmwareComponentsValid), metav1.ConditionTrue))
			g.Expect(hfc.Status.Conditions).To(ContainCondition(string(metal3api.HostFirmwareComponentsChangeDetected), metav1.ConditionTrue))
		}, e2eConfig.GetIntervals(specName, "wait-reconcile")...).Should(Succeed())

		By("Waiting for the BMH to start preparing (cleaning)")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: clusterProxy.GetClient(),
			Bmh:    bmh,
			State:  metal3api.StatePreparing,
		}, e2eConfig.GetIntervals(specName, "wait-available")...)

		By("Waiting for the BMH to become available again after firmware update")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: clusterProxy.GetClient(),
			Bmh:    bmh,
			State:  metal3api.StateAvailable,
		}, e2eConfig.GetIntervals(specName, "wait-firmware-components")...)

		By("Verifying the HFC status shows updates were applied")
		Expect(clusterProxy.GetClient().Get(ctx, types.NamespacedName{
			Name:      bmhName,
			Namespace: namespace.Name,
		}, hfc)).To(Succeed())
		Expect(hfc.Status.Updates).To(HaveLen(1))
		Expect(hfc.Status.Updates[0].Component).To(Equal("bios"))
		Expect(hfc.Status.Updates[0].URL).To(Equal(biosFirmwareUpdateURL))
		Expect(hfc.Status.Conditions).To(ContainCondition(string(metal3api.HostFirmwareComponentsChangeDetected), metav1.ConditionFalse))
		Expect(hfc.Status.Conditions).To(ContainCondition(string(metal3api.HostFirmwareComponentsValid), metav1.ConditionTrue))

		By("Verifying the BIOS component version was updated")
		var newBiosVersion string
		for _, comp := range hfc.Status.Components {
			if comp.Component == "bios" {
				newBiosVersion = comp.CurrentVersion
				break
			}
		}
		Logf("New BIOS version: %s", newBiosVersion)
		if initialBiosVersion != "" {
			Expect(newBiosVersion).NotTo(Equal(initialBiosVersion),
				"BIOS version should have changed after firmware update")
		}

		By("Deleting the BMH")
		Expect(clusterProxy.GetClient().Delete(ctx, &bmh)).To(Succeed())

		By("Waiting for the BMH to be deleted")
		WaitForBmhDeleted(ctx, WaitForBmhDeletedInput{
			Client:    clusterProxy.GetClient(),
			BmhName:   bmhName,
			Namespace: namespace.Name,
		}, e2eConfig.GetIntervals(specName, "wait-bmh-deleted")...)
	})

	It("should upgrade BIOS firmware via cleaning when HostFirmwareComponents is created before the host", func() {
		bmhName := specName + "-bios-before-bmh"
		secretName := bmhName + "-bmc"

		By("Creating a secret with BMH credentials")
		bmcCredentialsData := map[string]string{
			"username": bmc.User,
			"password": bmc.Password,
		}
		CreateSecret(ctx, clusterProxy.GetClient(), namespace.Name, secretName, bmcCredentialsData)

		By("Creating HostFirmwareComponents with a BIOS update before creating the BMH")
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

		By("Waiting for the BMH to start preparing (cleaning with firmware update)")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: clusterProxy.GetClient(),
			Bmh:    bmh,
			State:  metal3api.StatePreparing,
		}, e2eConfig.GetIntervals(specName, "wait-available")...)

		By("Waiting for the BMH to become available after firmware update")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: clusterProxy.GetClient(),
			Bmh:    bmh,
			State:  metal3api.StateAvailable,
		}, e2eConfig.GetIntervals(specName, "wait-firmware-components")...)

		By("Verifying the HFC status shows updates were applied")
		Expect(clusterProxy.GetClient().Get(ctx, types.NamespacedName{
			Name:      bmhName,
			Namespace: namespace.Name,
		}, hfc)).To(Succeed())
		Expect(hfc.Status.Updates).To(HaveLen(1))
		Expect(hfc.Status.Updates[0].Component).To(Equal("bios"))
		Expect(hfc.Status.Updates[0].URL).To(Equal(biosFirmwareUpdateURL))
		Expect(hfc.Status.Conditions).To(ContainCondition(string(metal3api.HostFirmwareComponentsChangeDetected), metav1.ConditionFalse))
		Expect(hfc.Status.Conditions).To(ContainCondition(string(metal3api.HostFirmwareComponentsValid), metav1.ConditionTrue))

		By("Verifying the BIOS component version was updated in status")
		var newBiosVersion string
		for _, comp := range hfc.Status.Components {
			if comp.Component == "bios" {
				newBiosVersion = comp.CurrentVersion
				break
			}
		}
		Logf("BIOS version after update: %s", newBiosVersion)
		Expect(newBiosVersion).NotTo(BeEmpty(), "BIOS component should have a version after firmware update")

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
			isNamespaced := e2eConfig.GetBoolVariable("NAMESPACE_SCOPED")
			Cleanup(ctx, clusterProxy, namespace, cancelWatches, isNamespaced, e2eConfig.GetIntervals("default", "wait-namespace-deleted")...)
		}
	})
})
