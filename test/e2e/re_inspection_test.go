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
	"k8s.io/utils/pointer"
	"sigs.k8s.io/cluster-api/test/framework"
)

var _ = Describe("Re-Inspection", Label("required", "re-inspection"), func() {
	var (
		specName      = "re-inspection"
		secretName    = "bmc-credentials"
		namespace     *corev1.Namespace
		cancelWatches context.CancelFunc
	)
	const (
		wrongHostName = "wrongHostName"
	)
	BeforeEach(func() {
		namespace, cancelWatches = framework.CreateNamespaceAndWatchEvents(ctx, framework.CreateNamespaceAndWatchEventsInput{
			Creator:             clusterProxy.GetClient(),
			ClientSet:           clusterProxy.GetClientSet(),
			Name:                specName,
			LogFolder:           artifactFolder,
			IgnoreAlreadyExists: true,
		})
	})

	It("should re-inspect the annotated BMH", func() {
		By("Creating a secret with BMH credentials")
		bmcCredentialsData := map[string]string{
			"username": bmc.User,
			"password": bmc.Password,
		}
		CreateSecret(ctx, clusterProxy.GetClient(), namespace.Name, secretName, bmcCredentialsData)

		By("creating a BMH with inspection disabled and hardware details added with wrong HostName")
		newHardwareDetails := strings.Replace(hardwareDetails, "localhost.localdomain", wrongHostName, 1)
		bmh := metal3api.BareMetalHost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      specName + "-reinspect",
				Namespace: namespace.Name,
				Annotations: map[string]string{
					metal3api.InspectAnnotationPrefix:   "disabled",
					metal3api.HardwareDetailsAnnotation: newHardwareDetails,
				},
			},
			Spec: metal3api.BareMetalHostSpec{
				BMC: metal3api.BMCDetails{
					Address:                        bmc.Address,
					CredentialsName:                "bmc-credentials",
					DisableCertificateVerification: bmc.DisableCertificateVerification,
				},
				BootMode:       metal3api.BootMode(e2eConfig.GetVariable("BOOT_MODE")),
				BootMACAddress: bmc.BootMacAddress,
			},
		}
		err := clusterProxy.GetClient().Create(ctx, &bmh)
		Expect(err).NotTo(HaveOccurred())

		By("waiting for the BMH to become available")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: clusterProxy.GetClient(),
			Bmh:    bmh,
			State:  metal3api.StateAvailable,
		}, e2eConfig.GetIntervals(specName, "wait-available")...)

		By("checking that the BMH has wrong HostName")
		key := types.NamespacedName{Namespace: bmh.Namespace, Name: bmh.Name}
		Expect(clusterProxy.GetClient().Get(ctx, key, &bmh)).To(Succeed())
		Expect(bmh.Status.HardwareDetails.Hostname).To(Equal(wrongHostName))

		By("removing HardwareDetailsAnnotation")
		AnnotateBmh(ctx, clusterProxy.GetClient(), bmh, metal3api.HardwareDetailsAnnotation, nil)

		By("adding InspectAnnotation to re-inspect")
		AnnotateBmh(ctx, clusterProxy.GetClient(), bmh, metal3api.InspectAnnotationPrefix, pointer.String(""))

		By("waiting for the BMH to be in inspecting state after inspection annotaion")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: clusterProxy.GetClient(),
			Bmh:    bmh,
			State:  metal3api.StateInspecting,
		}, e2eConfig.GetIntervals(specName, "wait-inspecting")...)

		By("waiting for the BMH to become available after re-inspection")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: clusterProxy.GetClient(),
			Bmh:    bmh,
			State:  metal3api.StateAvailable,
		}, e2eConfig.GetIntervals(specName, "wait-available")...)

		By("checking that the hardware details are corrected after re-inspection")
		key = types.NamespacedName{Namespace: bmh.Namespace, Name: bmh.Name}
		Expect(clusterProxy.GetClient().Get(ctx, key, &bmh)).To(Succeed())
		// We are just checking that it changed from wrongHostName to something else.
		Expect(bmh.Status.HardwareDetails.Hostname).To(Not(Equal(wrongHostName)))

		By("Delete BMH")
		err = clusterProxy.GetClient().Delete(ctx, &bmh)
		Expect(err).NotTo(HaveOccurred())

		By("Waiting for the BMH to be deleted")
		WaitForBmhDeleted(ctx, WaitForBmhDeletedInput{
			Client:    clusterProxy.GetClient(),
			BmhName:   bmh.Name,
			Namespace: bmh.Namespace,
		}, e2eConfig.GetIntervals(specName, "wait-bmh-deleted")...)
	})

	AfterEach(func() {
		DumpResources(ctx, e2eConfig, clusterProxy, path.Join(artifactFolder, specName))
		if !skipCleanup {
			isNamespaced := e2eConfig.GetBoolVariable("NAMESPACE_SCOPED")
			Cleanup(ctx, clusterProxy, namespace, cancelWatches, isNamespaced, e2eConfig.GetIntervals("default", "wait-namespace-deleted")...)
		}
	})
})
