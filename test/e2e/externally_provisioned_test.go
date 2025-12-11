//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"path"
	"time"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Create as externally provisioned, deprovision", Label("required", "provision", "deprovision"),
	func() {
		var (
			specName      = "externally-provisioned"
			secretName    = "bmc-credentials"
			namespace     *corev1.Namespace
			cancelWatches context.CancelFunc
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

		It("provisions a BMH as externally provisioned, validates state immutability, then deprovisions", func() {
			testSecretName := secretName + "-external"
			By("Creating a secret with BMH credentials")
			bmcCredentialsData := map[string]string{
				"username": bmc.User,
				"password": bmc.Password,
			}
			CreateSecret(ctx, clusterProxy.GetClient(), namespace.Name, testSecretName, bmcCredentialsData)

			By("Creating a BMH as externally provisioned")
			bmh := metal3api.BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      specName + "-external",
					Namespace: namespace.Name,
				},
				Spec: metal3api.BareMetalHostSpec{
					Online: true,
					BMC: metal3api.BMCDetails{
						Address:                        bmc.Address,
						CredentialsName:                testSecretName,
						DisableCertificateVerification: bmc.DisableCertificateVerification,
					},
					BootMACAddress:        bmc.BootMacAddress,
					ExternallyProvisioned: true,
				},
			}
			err := clusterProxy.GetClient().Create(ctx, &bmh)
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for the BMH to become externally provisioned")
			WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
				Client: clusterProxy.GetClient(),
				Bmh:    bmh,
				State:  metal3api.StateExternallyProvisioned,
			}, e2eConfig.GetIntervals(specName, "wait-externally-provisioned")...)

			By("Retrieving the latest BMH object")
			err = clusterProxy.GetClient().Get(ctx, types.NamespacedName{
				Name:      bmh.Name,
				Namespace: bmh.Namespace,
			}, &bmh)
			Expect(err).NotTo(HaveOccurred())

			By("checking that the BMH was not inspected or deployed")
			Expect(bmh.Status.OperationHistory.Inspect.Start.IsZero()).To(BeTrue())
			Expect(bmh.Status.OperationHistory.Provision.Start.IsZero()).To(BeTrue())

			By("Attempting to set ExternallyProvisioned to false (should be blocked by webhook)")
			err = clusterProxy.GetClient().Get(ctx, types.NamespacedName{
				Name:      bmh.Name,
				Namespace: bmh.Namespace,
			}, &bmh)
			Expect(err).NotTo(HaveOccurred())

			patch := client.MergeFrom(bmh.DeepCopy())
			bmh.Spec.ExternallyProvisioned = false
			err = clusterProxy.GetClient().Patch(ctx, &bmh, patch)
			Expect(err).To(HaveOccurred(), "Webhook should reject transition from true to false")
			Expect(err.Error()).To(ContainSubstring("externallyProvisioned can not be changed from true to false"))

			By("Verifying BMH remains in ExternallyProvisioned state")
			err = clusterProxy.GetClient().Get(ctx, types.NamespacedName{
				Name:      bmh.Name,
				Namespace: bmh.Namespace,
			}, &bmh)
			Expect(err).NotTo(HaveOccurred())
			Expect(bmh.Spec.ExternallyProvisioned).To(BeTrue(), "ExternallyProvisioned should still be true")
			Expect(bmh.Status.Provisioning.State).To(Equal(metal3api.StateExternallyProvisioned))

			By("Deleting the BMH")
			// Wait for 2 seconds to allow time to confirm annotation is set
			// TODO: fix this so we do not need the sleep
			time.Sleep(2 * time.Second)

			err = clusterProxy.GetClient().Delete(ctx, &bmh)
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for the BMH to be deleted")
			WaitForBmhDeleted(ctx, WaitForBmhDeletedInput{
				Client:    clusterProxy.GetClient(),
				BmhName:   bmh.Name,
				Namespace: bmh.Namespace,
				UndesiredStates: []metal3api.ProvisioningState{
					metal3api.StateProvisioning,
					metal3api.StateRegistering,
					metal3api.StateDeprovisioning,
					metal3api.StateInspecting,
				},
			}, e2eConfig.GetIntervals(specName, "wait-bmh-deleted")...)

			By("Waiting for the secret to be deleted")
			Eventually(func() bool {
				err := clusterProxy.GetClient().Get(ctx, types.NamespacedName{
					Name:      testSecretName,
					Namespace: namespace.Name,
				}, &corev1.Secret{})
				return k8serrors.IsNotFound(err)
			}, e2eConfig.GetIntervals(specName, "wait-secret-deletion")...).Should(BeTrue())
		})

		It("transitions from Available to ExternallyProvisioned", func() {
			testSecretName := secretName + "-available-to-ext"
			By("Creating a secret with BMH credentials")
			bmcCredentialsData := map[string]string{
				"username": bmc.User,
				"password": bmc.Password,
			}
			CreateSecret(ctx, clusterProxy.GetClient(), namespace.Name, testSecretName, bmcCredentialsData)

			By("Creating a BMH without ExternallyProvisioned to allow inspection")
			bmh := metal3api.BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      specName + "-available-to-ext",
					Namespace: namespace.Name,
				},
				Spec: metal3api.BareMetalHostSpec{
					BMC: metal3api.BMCDetails{
						Address:                        bmc.Address,
						CredentialsName:                testSecretName,
						DisableCertificateVerification: bmc.DisableCertificateVerification,
					},
					BootMode:              metal3api.Legacy,
					BootMACAddress:        bmc.BootMacAddress,
					ExternallyProvisioned: false, // Start with false to allow inspection
				},
			}
			err := clusterProxy.GetClient().Create(ctx, &bmh)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for the BMH to be in inspecting state")
			WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
				Client: clusterProxy.GetClient(),
				Bmh:    bmh,
				State:  metal3api.StateInspecting,
			}, e2eConfig.GetIntervals(specName, "wait-inspecting")...)

			By("Waiting for the BMH to complete inspection and reach Available state")
			WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
				Client: clusterProxy.GetClient(),
				Bmh:    bmh,
				State:  metal3api.StateAvailable,
			}, e2eConfig.GetIntervals(specName, "wait-available")...)

			By("Retrieving the BMH to verify inspection completed")
			err = clusterProxy.GetClient().Get(ctx, types.NamespacedName{
				Name:      bmh.Name,
				Namespace: bmh.Namespace,
			}, &bmh)
			Expect(err).NotTo(HaveOccurred())

			By("Verifying that inspection was performed")
			Expect(bmh.Status.OperationHistory.Inspect.Start.IsZero()).To(BeFalse(), "Inspection should have been performed")
			Expect(bmh.Status.HardwareDetails).NotTo(BeNil(), "Hardware details should be available after inspection")

			By("Setting ExternallyProvisioned to true from Available state (allowed by webhook)")
			patch := client.MergeFrom(bmh.DeepCopy())
			bmh.Spec.ExternallyProvisioned = true
			err = clusterProxy.GetClient().Patch(ctx, &bmh, patch)
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for the BMH to transition to ExternallyProvisioned state")
			WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
				Client: clusterProxy.GetClient(),
				Bmh:    bmh,
				State:  metal3api.StateExternallyProvisioned,
			}, e2eConfig.GetIntervals(specName, "wait-externally-provisioned")...)

			By("Verifying that no BMO provisioning occurred after transition")
			err = clusterProxy.GetClient().Get(ctx, types.NamespacedName{
				Name:      bmh.Name,
				Namespace: bmh.Namespace,
			}, &bmh)
			Expect(err).NotTo(HaveOccurred())
			Expect(bmh.Status.OperationHistory.Provision.Start.IsZero()).To(BeTrue(), "BMO provisioning should not have occurred")

			By("Cleaning up the BMH")
			err = clusterProxy.GetClient().Delete(ctx, &bmh)
			Expect(err).NotTo(HaveOccurred())

			WaitForBmhDeleted(ctx, WaitForBmhDeletedInput{
				Client:    clusterProxy.GetClient(),
				BmhName:   bmh.Name,
				Namespace: bmh.Namespace,
			}, e2eConfig.GetIntervals(specName, "wait-bmh-deleted")...)

			By("Waiting for the secret to be deleted")
			Eventually(func() bool {
				err := clusterProxy.GetClient().Get(ctx, types.NamespacedName{
					Name:      testSecretName,
					Namespace: namespace.Name,
				}, &corev1.Secret{})
				return k8serrors.IsNotFound(err)
			}, e2eConfig.GetIntervals(specName, "wait-secret-deletion")...).Should(BeTrue())
		})

		AfterEach(func() {
			DumpResources(ctx, e2eConfig, clusterProxy, path.Join(artifactFolder, specName))
			if !skipCleanup {
				isNamespaced := e2eConfig.GetBoolVariable("NAMESPACE_SCOPED")
				Cleanup(ctx, clusterProxy, namespace, cancelWatches, isNamespaced, e2eConfig.GetIntervals("default", "wait-namespace-deleted")...)
			}
		})
	})
