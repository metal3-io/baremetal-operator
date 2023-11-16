package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/controller-runtime/pkg/client"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
)

var _ = Describe("BMH Provisioning and Annotation Management", func() {
	var (
		specName       = "provisioning-ops"
		namespace      *corev1.Namespace
		cancelWatches  context.CancelFunc
		bmcUser        string
		bmcPassword    string
		bmcAddress     string
		bootMacAddress string
	)

	BeforeEach(func() {
		bmcUser = e2eConfig.GetVariable("BMC_USER")
		bmcPassword = e2eConfig.GetVariable("BMC_PASSWORD")
		bmcAddress = e2eConfig.GetVariable("BMC_ADDRESS")
		bootMacAddress = e2eConfig.GetVariable("BOOT_MAC_ADDRESS")

		namespace, cancelWatches = framework.CreateNamespaceAndWatchEvents(ctx, framework.CreateNamespaceAndWatchEventsInput{
			Creator:   clusterProxy.GetClient(),
			ClientSet: clusterProxy.GetClientSet(),
			Name:      fmt.Sprintf("%s-%s", specName, util.RandomString(6)),
			LogFolder: artifactFolder,
		})
	})

	It("provisions a BMH, applies detached and status annotations, then deprovisions", func() {
		By("Creating a secret with BMH credentials")
		bmcCredentials := corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "bmc-credentials",
				Namespace: namespace.Name,
			},
			StringData: map[string]string{
				"username": bmcUser,
				"password": bmcPassword,
			},
		}
		err := clusterProxy.GetClient().Create(ctx, &bmcCredentials)
		Expect(err).NotTo(HaveOccurred())

		By("Creating a BMH with inspection disabled and hardware details added")
		bmh := metal3api.BareMetalHost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      specName,
				Namespace: namespace.Name,
				Annotations: map[string]string{
					metal3api.InspectAnnotationPrefix:   "disabled",
					metal3api.HardwareDetailsAnnotation: hardwareDetails,
				},
			},
			Spec: metal3api.BareMetalHostSpec{
				Online: true,
				BMC: metal3api.BMCDetails{
					Address:         bmcAddress,
					CredentialsName: "bmc-credentials",
				},
				BootMode:       metal3api.Legacy,
				BootMACAddress: bootMacAddress,
			},
		}
		err = clusterProxy.GetClient().Create(ctx, &bmh)
		Expect(err).NotTo(HaveOccurred())

		By("Waiting for the BMH to become available")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: clusterProxy.GetClient(),
			Bmh:    bmh,
			State:  metal3api.StateAvailable,
		}, e2eConfig.GetIntervals(specName, "wait-available")...)

		By("Patching the BMH to test provisioning")
		helper, err := patch.NewHelper(&bmh, clusterProxy.GetClient())
		Expect(err).NotTo(HaveOccurred())
		bmh.Spec.Image = &metal3api.Image{
			URL:      e2eConfig.GetVariable("IMAGE_URL"),
			Checksum: e2eConfig.GetVariable("IMAGE_CHECKSUM"),
		}
		bmh.Spec.RootDeviceHints = &metal3api.RootDeviceHints{
			DeviceName: "/dev/vda",
		}
		Expect(helper.Patch(ctx, &bmh)).To(Succeed())

		By("Waiting for the BMH to be in provisioning state")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: clusterProxy.GetClient(),
			Bmh:    bmh,
			State:  metal3api.StateProvisioning,
		}, e2eConfig.GetIntervals(specName, "wait-provisioning")...)

		By("Waiting for the BMH to become provisioned")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: clusterProxy.GetClient(),
			Bmh:    bmh,
			State:  metal3api.StateProvisioned,
		}, e2eConfig.GetIntervals(specName, "wait-provisioned")...)

		By("Retrieving the latest BMH object")
		err = clusterProxy.GetClient().Get(ctx, client.ObjectKey{
			Name:      bmh.Name,
			Namespace: bmh.Namespace,
		}, &bmh)
		Expect(err).NotTo(HaveOccurred())

		By("Adding the detached annotation")
		helper, err = patch.NewHelper(&bmh, clusterProxy.GetClient())
		Expect(err).NotTo(HaveOccurred())

		// Add the detached annotation; "true" is used explicitly to clarify intent.
		bmh.ObjectMeta.Annotations["baremetalhost.metal3.io/detached"] = "true"

		Expect(helper.Patch(ctx, &bmh)).To(Succeed())

		By("Saving the status to a JSON string")
		savedStatus := bmh.Status
		statusJSON, err := json.Marshal(savedStatus)
		Expect(err).NotTo(HaveOccurred())

		By("Deleting the BMH")
		// Wait for 2 seconds to allow time to confirm annotation is set
		// TODO: fix this so we do not need the sleep
		time.Sleep(2 * time.Second)

		err = clusterProxy.GetClient().Delete(ctx, &bmh)
		Expect(err).NotTo(HaveOccurred())

		By("Waiting for the BMH to be deleted")
		Eventually(func() (string, error) {
			var currentBmh metal3api.BareMetalHost
			err := clusterProxy.GetClient().Get(ctx, types.NamespacedName{Name: bmh.Name, Namespace: bmh.Namespace}, &currentBmh)
			if err != nil {
				if apierrors.IsNotFound(err) {
					// If the BMH is not found, we assume it has been successfully deleted.
					return "deleted", nil
				}
				// Any other error should be returned.
				return "", err
			}

			currentStatus := currentBmh.Status.Provisioning.State

			// If the state is 'deleting' or 'provisioned', we continue polling.
			if currentStatus == "deleting" || currentStatus == "provisioned" {
				return string(currentStatus), nil
			}

			// Any other state is unexpected, and we stop the polling.
			return "", StopTrying(fmt.Sprintf("BMH is in an unexpected state: %s", currentStatus))
		}, e2eConfig.GetIntervals(specName, "wait-deleted")...).Should(Equal("deleted"))

		By("Waiting for the secret to be deleted")
		Eventually(func() bool {
			err := clusterProxy.GetClient().Get(ctx, client.ObjectKey{
				Name:      "bmc-credentials",
				Namespace: namespace.Name,
			}, &corev1.Secret{})
			return apierrors.IsNotFound(err)
		}, e2eConfig.GetIntervals(specName, "wait-secret-deletion")...).Should(BeTrue())

		By("Creating a secret with BMH credentials")
		secretName := "bmc-credentials"
		bmcCredentials = corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: namespace.Name,
			},
			StringData: map[string]string{
				"username": bmcUser,
				"password": bmcPassword,
			},
		}

		err = clusterProxy.GetClient().Create(ctx, &bmcCredentials)
		Expect(err).NotTo(HaveOccurred())

		By("Recreating the BMH with the previously saved status in the status annotation")
		bmh = metal3api.BareMetalHost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      specName,
				Namespace: namespace.Name,
				Annotations: map[string]string{
					metal3api.StatusAnnotation: string(statusJSON),
				},
			},
			Spec: metal3api.BareMetalHostSpec{
				Online: true,
				BMC: metal3api.BMCDetails{
					Address:         bmcAddress,
					CredentialsName: "bmc-credentials",
				},
				BootMode:       metal3api.Legacy,
				BootMACAddress: bootMacAddress,
				Image: &metal3api.Image{
					URL:      e2eConfig.GetVariable("IMAGE_URL"),
					Checksum: e2eConfig.GetVariable("IMAGE_CHECKSUM"),
				},
				RootDeviceHints: &metal3api.RootDeviceHints{
					DeviceName: "/dev/vda",
				},
			},
		}

		err = clusterProxy.GetClient().Create(ctx, &bmh)
		Expect(err).NotTo(HaveOccurred())

		By("Checking that the BMH goes directly to 'provisioned' state")
		Eventually(func() (string, error) {
			var currentBmh metal3api.BareMetalHost
			err := clusterProxy.GetClient().Get(ctx, types.NamespacedName{Name: bmh.Name, Namespace: bmh.Namespace}, &currentBmh)
			if err != nil {
				// Handle errors that may occur while fetching the BMH.
				return "", err
			}

			currentStatus := currentBmh.Status.Provisioning.State

			if currentStatus == "provisioned" || currentStatus == "" {
				return string(currentStatus), nil
			}

			// Any other state is unexpected, and we stop the polling.
			return "", StopTrying(fmt.Sprintf("BMH should not be in '%s' state", currentStatus))
		}, e2eConfig.GetIntervals(specName, "wait-provisioned")...).Should(Equal("provisioned"))

		By("Triggering the deprovisioning of the BMH")
		helper, err = patch.NewHelper(&bmh, clusterProxy.GetClient())
		Expect(err).NotTo(HaveOccurred())
		bmh.Spec.Image = nil
		Expect(helper.Patch(ctx, &bmh)).To(Succeed())

		By("Waiting for the BMH to be in deprovisioning state")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: clusterProxy.GetClient(),
			Bmh:    bmh,
			State:  metal3api.StateDeprovisioning,
		}, e2eConfig.GetIntervals(specName, "wait-deprovisioning")...)

		By("Waiting for the BMH to become available again")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: clusterProxy.GetClient(),
			Bmh:    bmh,
			State:  metal3api.StateAvailable,
		}, e2eConfig.GetIntervals(specName, "wait-available")...)
	})

	AfterEach(func() {
		cleanup(ctx, clusterProxy, namespace, cancelWatches, e2eConfig.GetIntervals("default", "wait-namespace-deleted")...)
	})
})
