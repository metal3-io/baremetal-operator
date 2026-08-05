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
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("DataImage Finalizer", Label("required", "dataimage"), func() {
	const dataImageURL = "http://example.com/ubuntu.iso"

	var (
		specName      = "dataimage"
		namespace     *corev1.Namespace
		cancelWatches context.CancelFunc
		toCleanup     []client.Object
	)

	BeforeEach(func() {
		toCleanup = nil

		namespace, cancelWatches = framework.CreateNamespaceAndWatchEvents(ctx, framework.CreateNamespaceAndWatchEventsInput{
			Creator:             clusterProxy.GetClient(),
			ClientSet:           clusterProxy.GetClientSet(),
			Name:                specName,
			LogFolder:           artifactFolder,
			IgnoreAlreadyExists: true,
		})
	})

	It("should add a finalizer to a new DataImage when a BMH exists", func() {
		bmhName := specName + "-add-finalizer"
		secretName := bmhName + "-bmc-creds"

		By("Creating a secret with BMH credentials")
		bmcCredentialsData := map[string]string{
			"username": bmc.User,
			"password": bmc.Password,
		}
		secret := CreateSecret(ctx, clusterProxy.GetClient(), namespace.Name, secretName, bmcCredentialsData)
		toCleanup = append(toCleanup, secret)

		By("Creating a BMH with inspection disabled")
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
		err := clusterProxy.GetClient().Create(ctx, &bmh)
		Expect(err).NotTo(HaveOccurred())
		toCleanup = append(toCleanup, &bmh)

		By("Creating a DataImage with the same name as the BMH")
		di := &metal3api.DataImage{
			ObjectMeta: metav1.ObjectMeta{
				Name:      bmhName,
				Namespace: namespace.Name,
			},
			Spec: metal3api.DataImageSpec{
				URL: dataImageURL,
			},
		}
		err = clusterProxy.GetClient().Create(ctx, di)
		Expect(err).NotTo(HaveOccurred())
		toCleanup = append(toCleanup, di)

		By("Waiting for the finalizer to be added to the DataImage")
		Eventually(func(g Gomega) {
			updatedDI := &metal3api.DataImage{}
			g.Expect(clusterProxy.GetClient().Get(ctx, types.NamespacedName{
				Name:      bmhName,
				Namespace: namespace.Name,
			}, updatedDI)).To(Succeed())
			g.Expect(updatedDI.Finalizers).To(ContainElement(metal3api.DataImageFinalizer))
		}, e2eConfig.GetIntervals(specName, "wait-available")...).Should(Succeed())
	})

	It("should remove the finalizer when DataImage is deleted and no BMH exists", func() {
		diName := specName + "-no-bmh"

		By("Creating a DataImage without a corresponding BMH")
		di := &metal3api.DataImage{
			ObjectMeta: metav1.ObjectMeta{
				Name:       diName,
				Namespace:  namespace.Name,
				Finalizers: []string{metal3api.DataImageFinalizer},
			},
			Spec: metal3api.DataImageSpec{
				URL: dataImageURL,
			},
		}
		err := clusterProxy.GetClient().Create(ctx, di)
		Expect(err).NotTo(HaveOccurred())

		By("Waiting for the controller to remove the finalizer (no BMH exists)")
		Eventually(func(g Gomega) {
			updatedDI := &metal3api.DataImage{}
			g.Expect(clusterProxy.GetClient().Get(ctx, types.NamespacedName{
				Name:      diName,
				Namespace: namespace.Name,
			}, updatedDI)).To(Succeed())
			g.Expect(updatedDI.Finalizers).NotTo(ContainElement(metal3api.DataImageFinalizer))
		}, e2eConfig.GetIntervals(specName, "wait-available")...).Should(Succeed())

		By("Deleting the DataImage")
		err = clusterProxy.GetClient().Delete(ctx, di)
		Expect(err).NotTo(HaveOccurred())

		By("Waiting for the DataImage to be fully removed")
		Eventually(func() bool {
			err := clusterProxy.GetClient().Get(ctx, types.NamespacedName{
				Name:      diName,
				Namespace: namespace.Name,
			}, &metal3api.DataImage{})
			return k8serrors.IsNotFound(err)
		}, e2eConfig.GetIntervals(specName, "wait-available")...).Should(BeTrue())
	})

	It("should remove the finalizer when BMH is detached and DataImage is deleted", func() {
		bmhName := specName + "-detached"
		secretName := bmhName + "-bmc-creds"

		By("Creating a secret with BMH credentials")
		bmcCredentialsData := map[string]string{
			"username": bmc.User,
			"password": bmc.Password,
		}
		secret := CreateSecret(ctx, clusterProxy.GetClient(), namespace.Name, secretName, bmcCredentialsData)
		toCleanup = append(toCleanup, secret)

		By("Creating a BMH with inspection disabled")
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
		err := clusterProxy.GetClient().Create(ctx, &bmh)
		Expect(err).NotTo(HaveOccurred())
		toCleanup = append(toCleanup, &bmh)

		By("Creating a DataImage with the same name")
		di := &metal3api.DataImage{
			ObjectMeta: metav1.ObjectMeta{
				Name:      bmhName,
				Namespace: namespace.Name,
			},
			Spec: metal3api.DataImageSpec{
				URL: dataImageURL,
			},
		}
		err = clusterProxy.GetClient().Create(ctx, di)
		Expect(err).NotTo(HaveOccurred())

		By("Waiting for the finalizer to be added")
		Eventually(func(g Gomega) {
			updatedDI := &metal3api.DataImage{}
			g.Expect(clusterProxy.GetClient().Get(ctx, types.NamespacedName{
				Name:      bmhName,
				Namespace: namespace.Name,
			}, updatedDI)).To(Succeed())
			g.Expect(updatedDI.Finalizers).To(ContainElement(metal3api.DataImageFinalizer))
		}, e2eConfig.GetIntervals(specName, "wait-available")...).Should(Succeed())

		By("Adding the detached annotation to the BMH")
		detachedValue := "{}"
		AnnotateBmh(ctx, clusterProxy.GetClient(), bmh, metal3api.DetachedAnnotation, &detachedValue)

		By("Waiting for the BMH to be in detached operational status")
		WaitForBmhInOperationalStatus(ctx, WaitForBmhInOperationalStatusInput{
			Client: clusterProxy.GetClient(),
			Bmh:    bmh,
			State:  metal3api.OperationalStatusDetached,
		}, e2eConfig.GetIntervals(specName, "wait-available")...)

		By("Deleting the DataImage")
		err = clusterProxy.GetClient().Delete(ctx, di)
		Expect(err).NotTo(HaveOccurred())

		By("Waiting for the DataImage to be fully removed (finalizer removed in detached state)")
		Eventually(func() bool {
			err := clusterProxy.GetClient().Get(ctx, types.NamespacedName{
				Name:      bmhName,
				Namespace: namespace.Name,
			}, &metal3api.DataImage{})
			return k8serrors.IsNotFound(err)
		}, e2eConfig.GetIntervals(specName, "wait-available")...).Should(BeTrue())
	})

	It("should remove the finalizer when image is not attached and DataImage is deleted", func() {
		if e2eConfig.GetBoolVariable("DEPLOY_IRONIC") {
			Skip("With Ironic deployed, the image gets attached and detach is not triggered by deletion alone")
		}

		bmhName := specName + "-image-detached"
		secretName := bmhName + "-bmc-creds"

		By("Creating a secret with BMH credentials")
		bmcCredentialsData := map[string]string{
			"username": bmc.User,
			"password": bmc.Password,
		}
		secret := CreateSecret(ctx, clusterProxy.GetClient(), namespace.Name, secretName, bmcCredentialsData)
		toCleanup = append(toCleanup, secret)

		By("Creating a BMH with inspection disabled")
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
		err := clusterProxy.GetClient().Create(ctx, &bmh)
		Expect(err).NotTo(HaveOccurred())
		toCleanup = append(toCleanup, &bmh)

		By("Waiting for the BMH to become available")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: clusterProxy.GetClient(),
			Bmh:    bmh,
			State:  metal3api.StateAvailable,
		}, e2eConfig.GetIntervals(specName, "wait-available")...)

		By("Creating a DataImage with the same name")
		di := &metal3api.DataImage{
			ObjectMeta: metav1.ObjectMeta{
				Name:      bmhName,
				Namespace: namespace.Name,
			},
			Spec: metal3api.DataImageSpec{
				URL: dataImageURL,
			},
		}
		err = clusterProxy.GetClient().Create(ctx, di)
		Expect(err).NotTo(HaveOccurred())

		By("Waiting for the finalizer to be added")
		Eventually(func(g Gomega) {
			updatedDI := &metal3api.DataImage{}
			g.Expect(clusterProxy.GetClient().Get(ctx, types.NamespacedName{
				Name:      bmhName,
				Namespace: namespace.Name,
			}, updatedDI)).To(Succeed())
			g.Expect(updatedDI.Finalizers).To(ContainElement(metal3api.DataImageFinalizer))
		}, e2eConfig.GetIntervals(specName, "wait-available")...).Should(Succeed())

		By("Deleting the DataImage")
		err = clusterProxy.GetClient().Delete(ctx, di)
		Expect(err).NotTo(HaveOccurred())

		By("Waiting for the DataImage to be fully removed (image not attached, finalizer removed)")
		Eventually(func() bool {
			err := clusterProxy.GetClient().Get(ctx, types.NamespacedName{
				Name:      bmhName,
				Namespace: namespace.Name,
			}, &metal3api.DataImage{})
			return k8serrors.IsNotFound(err)
		}, "20m", "10s").Should(BeTrue())
	})

	AfterEach(func() {
		CollectSerialLogs(bmc.Name, path.Join(artifactFolder, specName))
		DumpResources(ctx, e2eConfig, clusterProxy, path.Join(artifactFolder, specName))
		if !skipCleanup {
			Cleanup(ctx, clusterProxy, namespace, cancelWatches, e2eConfig, toCleanup)
		}
	})
})
