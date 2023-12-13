package e2e

import (
	"context"
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/util"

	capm3_e2e "github.com/metal3-io/cluster-api-provider-metal3/test/e2e"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
)

var _ = Describe("Re-Inspection", func() {
	var (
		specName         = "re-inspection"
		secretName       = "bmc-credentials"
		namespace        *corev1.Namespace
		cancelWatches    context.CancelFunc
		bmcUser          string
		bmcPassword      string
		bmcAddress       string
		bootMacAddress   string
		expectedHostName string
	)
	const (
		wrongHostName = "wrongHostName"
	)
	BeforeEach(func() {
		bmcUser = e2eConfig.GetVariable("BMC_USER")
		bmcPassword = e2eConfig.GetVariable("BMC_PASSWORD")
		bmcAddress = e2eConfig.GetVariable("BMC_ADDRESS")
		bootMacAddress = e2eConfig.GetVariable("BOOT_MAC_ADDRESS")
		expectedHostName = e2eConfig.GetVariable("EXPECTED_HOST_NAME")

		namespace, cancelWatches = framework.CreateNamespaceAndWatchEvents(ctx, framework.CreateNamespaceAndWatchEventsInput{
			Creator:   clusterProxy.GetClient(),
			ClientSet: clusterProxy.GetClientSet(),
			Name:      fmt.Sprintf("%s-%s", specName, util.RandomString(6)),
			LogFolder: artifactFolder,
		})
	})

	It("should re-inspect the annotated BMH", func() {
		By("Creating a secret with BMH credentials")
		bmcCredentialsData := map[string]string{
			"username": bmcUser,
			"password": bmcPassword,
		}
		CreateSecret(ctx, clusterProxy.GetClient(), namespace.Name, secretName, bmcCredentialsData)

		By("creating a BMH with inspection disabled and hardware details added with wrong HostName")
		newHardwareDetails := strings.Replace(hardwareDetails, "bmo-e2e-0", wrongHostName, 1)
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
					Address:         bmcAddress,
					CredentialsName: "bmc-credentials",
				},
				BootMode:       metal3api.Legacy,
				BootMACAddress: bootMacAddress,
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
		capm3_e2e.AnnotateBmh(ctx, clusterProxy.GetClient(), bmh, metal3api.HardwareDetailsAnnotation, nil)

		By("adding InspectAnnotation to re-inspect")
		capm3_e2e.AnnotateBmh(ctx, clusterProxy.GetClient(), bmh, metal3api.InspectAnnotationPrefix, pointer.String(""))

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
		Expect(bmh.Status.HardwareDetails.Hostname).To(Equal(expectedHostName))
	})

	AfterEach(func() {
		cleanup(ctx, clusterProxy, namespace, cancelWatches, e2eConfig.GetIntervals("default", "wait-namespace-deleted")...)
	})
})
