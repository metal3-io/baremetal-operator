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
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	selectorLabel = "test.metal3.io/class"
	selectorValue = "selected"
)

func createBmh(name, namespace string, labels map[string]string, hardwareDetails string) *metal3api.BareMetalHost {
	bmh := &metal3api.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
			Annotations: map[string]string{
				metal3api.HardwareDetailsAnnotation: hardwareDetails,
			},
		},
		Spec: metal3api.BareMetalHostSpec{
			Online: false,
			BMC: metal3api.BMCDetails{
				Address:                        bmc.Address,
				CredentialsName:                "bmc-credentials",
				DisableCertificateVerification: bmc.DisableCertificateVerification,
			},
			BootMode:              metal3api.BootMode(e2eConfig.GetVariable("BOOT_MODE")),
			BootMACAddress:        bmc.BootMacAddress,
			AutomatedCleaningMode: metal3api.CleaningModeDisabled,
			InspectionMode:        metal3api.InspectionModeDisabled,
			RootDeviceHints:       &bmc.RootDeviceHints,
		},
	}
	return bmh
}

var _ = Describe("Associate a hostclaim to a BMH and delete the claim.", Label("required", "hostclaim"),
	func() {
		var (
			specName       = "hostclaims"
			secretName     = "bmc-credentials"
			namespaceBMH   *corev1.Namespace
			namespaceClaim *corev1.Namespace
			cancelWatches  context.CancelFunc
			toCleanupBMH   []client.Object
			toCleanupClaim []client.Object
		)

		BeforeEach(func() {
			toCleanupBMH = nil
			toCleanupClaim = nil
			namespaceBMH, cancelWatches = framework.CreateNamespaceAndWatchEvents(ctx, framework.CreateNamespaceAndWatchEventsInput{
				Creator:             clusterProxy.GetClient(),
				ClientSet:           clusterProxy.GetClientSet(),
				Name:                specName + "-infra",
				LogFolder:           artifactFolder,
				IgnoreAlreadyExists: true,
			})
			namespaceClaim = framework.CreateNamespace(ctx, framework.CreateNamespaceInput{
				Creator:             clusterProxy.GetClient(),
				Name:                specName + "-tenant",
				IgnoreAlreadyExists: true,
			})
		})

		It("Create a claim, associate it to a BMH", func() {
			By("Creating a secret with BMH credentials")
			bmcCredentialsData := map[string]string{
				"username": bmc.User,
				"password": bmc.Password,
			}
			CreateSecret(ctx, clusterProxy.GetClient(), namespaceBMH.Name, secretName, bmcCredentialsData)

			By("Creating a BMH with inspection disabled and hardware details added")
			hardwareDetails := hardwareDetailsFor(&bmc)
			bmh := createBmh("bmh", namespaceBMH.Name, map[string]string{selectorLabel: selectorValue}, hardwareDetails)
			err := clusterProxy.GetClient().Create(ctx, bmh)
			Expect(err).NotTo(HaveOccurred())
			toCleanupBMH = append(toCleanupBMH, bmh)

			By("Waiting for the BMH to become available")
			WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
				Client: clusterProxy.GetClient(),
				Bmh:    *bmh,
				State:  metal3api.StateAvailable,
			}, e2eConfig.GetIntervals(specName, "wait-available")...)

			By("Creating a suitable hostDeployPolicy")
			hostDeployPolicy := &metal3api.HostDeployPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      specName,
					Namespace: namespaceBMH.Name,
				},
				Spec: metal3api.HostDeployPolicySpec{
					HostClaimNamespaces: &metal3api.HostClaimNamespaces{
						Names: []string{namespaceClaim.Name},
					},
				},
			}
			err = clusterProxy.GetClient().Create(ctx, hostDeployPolicy)
			Expect(err).NotTo(HaveOccurred())
			toCleanupBMH = append(toCleanupBMH, hostDeployPolicy)

			By("Creating a hostClaim")
			hostClaim := &metal3api.HostClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      specName,
					Namespace: namespaceClaim.Name,
				},
				Spec: metal3api.HostClaimSpec{
					Image: &metal3api.Image{
						URL:      e2eConfig.GetVariable("IMAGE_URL"),
						Checksum: e2eConfig.GetVariable("IMAGE_CHECKSUM"),
					},
					HostSelector: metal3api.HostSelector{
						MatchLabels: map[string]string{selectorLabel: selectorValue},
					},
					PoweredOn: true,
				},
			}
			err = clusterProxy.GetClient().Create(ctx, hostClaim)
			Expect(err).NotTo(HaveOccurred())
			toCleanupClaim = append(toCleanupClaim, hostClaim)

			By("Waiting for the HostClaim to become associated")
			WaitForHostClaimCondition(ctx, WaitForHostClaimConditionInput{
				Client:        clusterProxy.GetClient(),
				HostClaim:     hostClaim,
				ConditionType: metal3api.AssociatedCondition,
				Status:        metav1.ConditionTrue,
			}, e2eConfig.GetIntervals(specName, "wait-associated")...)

			By("Deleting the hostclaim")
			err = clusterProxy.GetClient().Delete(ctx, hostClaim)
			Expect(err).NotTo(HaveOccurred())
			Eventually(func(g Gomega) {
				currentBmh := metal3api.BareMetalHost{}
				key := types.NamespacedName{Namespace: bmh.Namespace, Name: bmh.Name}
				g.Expect(clusterProxy.GetClient().Get(ctx, key, &currentBmh)).To(Succeed())
				g.Expect(currentBmh.Spec.ConsumerRef).To(BeNil())
			}, e2eConfig.GetIntervals(specName, "wait-deleted")...).Should(Succeed())
		})

		AfterEach(func() {
			DumpResources(ctx, e2eConfig, clusterProxy, path.Join(artifactFolder, specName))
			if !skipCleanup {
				Cleanup(ctx, clusterProxy, namespaceBMH, cancelWatches, e2eConfig, toCleanupBMH)
				Cleanup(ctx, clusterProxy, namespaceClaim, nil, e2eConfig, toCleanupClaim)
			}
		})
	},
)
