//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"fmt"
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
	"sigs.k8s.io/cluster-api/util"
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
				Creator:   clusterProxy.GetClient(),
				ClientSet: clusterProxy.GetClientSet(),
				Name:      fmt.Sprintf("%s-%s", specName, util.RandomString(6)),
				LogFolder: artifactFolder,
			})
		})

		It("provisions a BMH as externally provisioned, then deprovisions", func() {
			By("Creating a secret with BMH credentials")
			bmcCredentialsData := map[string]string{
				"username": bmc.User,
				"password": bmc.Password,
			}
			CreateSecret(ctx, clusterProxy.GetClient(), namespace.Name, secretName, bmcCredentialsData)

			By("Creating a BMH as externally provisioned")
			bmh := metal3api.BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      specName,
					Namespace: namespace.Name,
				},
				Spec: metal3api.BareMetalHostSpec{
					Online: true,
					BMC: metal3api.BMCDetails{
						Address:         bmc.Address,
						CredentialsName: "bmc-credentials",
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
			}, e2eConfig.GetIntervals(specName, "wait-deleted")...)

			By("Waiting for the secret to be deleted")
			Eventually(func() bool {
				err := clusterProxy.GetClient().Get(ctx, types.NamespacedName{
					Name:      "bmc-credentials",
					Namespace: namespace.Name,
				}, &corev1.Secret{})
				return k8serrors.IsNotFound(err)
			}, e2eConfig.GetIntervals(specName, "wait-secret-deletion")...).Should(BeTrue())
		})

		AfterEach(func() {
			DumpResources(ctx, e2eConfig, clusterProxy, path.Join(artifactFolder, specName))
			if !skipCleanup {
				cleanup(ctx, clusterProxy, namespace, cancelWatches, e2eConfig.GetIntervals("default", "wait-namespace-deleted")...)
			}
		})
	})
