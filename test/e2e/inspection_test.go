//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"fmt"
	"path"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/util"
)

var _ = Describe("Inspection", Label("required", "inspection"), func() {
	var (
		specName      = "inspection"
		secretName    = "bmc-credentials"
		namespace     *corev1.Namespace
		cancelWatches context.CancelFunc
	)
	BeforeEach(func() {

		isNamespaced := e2eConfig.GetBoolVariable("NAMESPACE_SCOPED")

		namespaceInput := framework.CreateNamespaceAndWatchEventsInput{
			Creator:   clusterProxy.GetClient(),
			ClientSet: clusterProxy.GetClientSet(),
			LogFolder: artifactFolder,
		}

		if isNamespaced {
			namespaceInput.Name = specName
			namespaceInput.IgnoreAlreadyExists = true
		} else {
			namespaceInput.Name = fmt.Sprintf("%s-%s", specName, util.RandomString(6))
		}

		namespace, cancelWatches = framework.CreateNamespaceAndWatchEvents(ctx, namespaceInput)
	})

	It("should put BMH without BMC credentials in unmanaged state", func() {
		By("creating a BMH")
		bmh := metal3api.BareMetalHost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      specName + "-unmanaged",
				Namespace: namespace.Name,
			},
		}
		err := clusterProxy.GetClient().Create(ctx, &bmh)
		Expect(err).NotTo(HaveOccurred())

		By("waiting for the BMH to be in unmanaged state")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: clusterProxy.GetClient(),
			Bmh:    bmh,
			State:  metal3api.StateUnmanaged,
		}, e2eConfig.GetIntervals(specName, "wait-unmanaged")...)

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

	It("should fail to register the BMH if the secret is missing", func() {
		By("creating a BMH")
		bmh := metal3api.BareMetalHost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      specName + "-registration-error",
				Namespace: namespace.Name,
			},
			Spec: metal3api.BareMetalHostSpec{
				BMC: metal3api.BMCDetails{
					Address:                        "ipmi://127.0.0.1:5678",
					CredentialsName:                "bmc-credentials",
					DisableCertificateVerification: bmc.DisableCertificateVerification,
				},
			},
		}
		err := clusterProxy.GetClient().Create(ctx, &bmh)
		Expect(err).NotTo(HaveOccurred())

		By("trying to register the BMH")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: clusterProxy.GetClient(),
			Bmh:    bmh,
			State:  metal3api.StateRegistering,
		}, e2eConfig.GetIntervals(specName, "wait-registering")...)

		By("waiting for registration error on the BMH")
		Eventually(func(g Gomega) {
			key := types.NamespacedName{Namespace: bmh.Namespace, Name: bmh.Name}
			g.Expect(clusterProxy.GetClient().Get(ctx, key, &bmh)).To(Succeed())
			g.Expect(bmh.Status.ErrorType).To(Equal(metal3api.RegistrationError))
		}, e2eConfig.GetIntervals(specName, "wait-registration-error")...).Should(Succeed())

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

	It("should inspect a newly created BMH", func() {
		By("Creating a secret with BMH credentials")

		bmcCredentialsData := map[string]string{
			"username": bmc.User,
			"password": bmc.Password,
		}
		CreateSecret(ctx, clusterProxy.GetClient(), namespace.Name, secretName, bmcCredentialsData)

		By("creating a BMH")
		bmh := metal3api.BareMetalHost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      specName + "-inspect",
				Namespace: namespace.Name,
			},
			Spec: metal3api.BareMetalHostSpec{
				BMC: metal3api.BMCDetails{
					Address:                        bmc.Address,
					CredentialsName:                "bmc-credentials",
					DisableCertificateVerification: bmc.DisableCertificateVerification,
				},
				BootMode:       metal3api.Legacy,
				BootMACAddress: bmc.BootMacAddress,
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

		By("waiting for the BMH to become available")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: clusterProxy.GetClient(),
			Bmh:    bmh,
			State:  metal3api.StateAvailable,
		}, e2eConfig.GetIntervals(specName, "wait-available")...)

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
