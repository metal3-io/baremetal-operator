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
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/util/patch"
)

// These example keys and values are hardcoded in sushy-tools.
// Warning: be careful when updating or reusing any keys: updated values will be persisted in the emulator between tests!
const (
	hfsTestKey1       = "ProcTurboMode"
	hfsTestOrigValue1 = "Enabled"
	hfsTestNewValue1  = "Disabled"
	hfsTestKey2       = "QuietBoot"
)

var _ = Describe("Host Firmware Settings", Label("required", "firmware"), func() {
	var (
		specName      = "firmware-settings"
		namespace     *corev1.Namespace
		cancelWatches context.CancelFunc
	)

	BeforeEach(func() {
		// FIXME(dtantsur): find a more elegant way to check for this feature
		if !e2eConfig.GetBoolVariable("DEPLOY_IRONIC") || !strings.Contains(bmc.Address, "redfish") {
			Skip("HFS tests require a real Ironic and a host with Redfish")
		}

		namespace, cancelWatches = framework.CreateNamespaceAndWatchEvents(ctx, framework.CreateNamespaceAndWatchEventsInput{
			Creator:             clusterProxy.GetClient(),
			ClientSet:           clusterProxy.GetClientSet(),
			Name:                specName,
			LogFolder:           artifactFolder,
			IgnoreAlreadyExists: true,
		})
	})

	It("should apply firmware settings created before the host", func() {
		bmhName := specName + "-before-bmh"
		secretName := bmhName + "-bmc"

		By("Creating a secret with BMH credentials")
		bmcCredentialsData := map[string]string{
			"username": bmc.User,
			"password": bmc.Password,
		}
		CreateSecret(ctx, clusterProxy.GetClient(), namespace.Name, secretName, bmcCredentialsData)

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

		By("Updating HostFirmwareSettings with a new value")
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

		By("Making sure HFS was deleted too")
		Eventually(func() bool {
			return k8serrors.IsNotFound(clusterProxy.GetClient().Get(ctx, types.NamespacedName{
				Name:      bmhName,
				Namespace: namespace.Name,
			}, hfs))
		}, e2eConfig.GetIntervals(specName, "wait-bmh-deleted")...).Should(BeTrue())
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
