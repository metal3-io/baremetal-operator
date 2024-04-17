package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"golang.org/x/crypto/ssh"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/patch"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
)

var _ = Describe("Provision, detach, recreate from status and deprovision", Label("required", "provision", "detach", "status", "deprovision"),
	func() {
		var (
			specName      = "provisioning-ops"
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

		It("provisions a BMH, applies detached and status annotations, then deprovisions", func() {
			By("Creating a secret with BMH credentials")
			bmcCredentialsData := map[string]string{
				"username": bmc.User,
				"password": bmc.Password,
			}
			CreateSecret(ctx, clusterProxy.GetClient(), namespace.Name, secretName, bmcCredentialsData)

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
						Address:         bmc.Address,
						CredentialsName: "bmc-credentials",
					},
					BootMode:              metal3api.Legacy,
					BootMACAddress:        bmc.BootMacAddress,
					AutomatedCleaningMode: "disabled",
				},
			}
			err := clusterProxy.GetClient().Create(ctx, &bmh)
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
				URL:          e2eConfig.GetVariable("IMAGE_URL"),
				Checksum:     e2eConfig.GetVariable("IMAGE_CHECKSUM"),
				ChecksumType: metal3api.AutoChecksum,
			}
			bmh.Spec.RootDeviceHints = &metal3api.RootDeviceHints{
				DeviceName: "/dev/vda",
			}
			// The ssh check is not possible in all situations (e.g. fixture) so it can be skipped
			if e2eConfig.GetVariable("SSH_CHECK_PROVISIONED") == "true" {
				userDataSecretName := "user-data"
				sshPubKeyPath := e2eConfig.GetVariable("SSH_PUB_KEY")
				createCirrosInstanceAndHostnameUserdata(ctx, clusterProxy.GetClient(), namespace.Name, userDataSecretName, sshPubKeyPath)
				bmh.Spec.UserData = &corev1.SecretReference{
					Name:      userDataSecretName,
					Namespace: namespace.Name,
				}
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

			// The ssh check is not possible in all situations (e.g. fixture) so it can be skipped
			if e2eConfig.GetVariable("SSH_CHECK_PROVISIONED") == "true" {
				By("Verifying the node booting from disk")
				keyPath := e2eConfig.GetVariable("SSH_PRIV_KEY")
				key, err := os.ReadFile(keyPath)
				Expect(err).NotTo(HaveOccurred(), "unable to read private key")

				signer, err := ssh.ParsePrivateKey(key)
				Expect(err).NotTo(HaveOccurred(), "unable to parse private key")

				auth := ssh.PublicKeys(signer)
				PerformSSHBootCheck(e2eConfig, "disk", auth, fmt.Sprintf("%s:%s", bmc.IPAddress, bmc.SSHPort))
			} else {
				Logf("WARNING: Skipping SSH check since SSH_CHECK_PROVISIONED != true")
			}

			By("Retrieving the latest BMH object")
			err = clusterProxy.GetClient().Get(ctx, types.NamespacedName{
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
			WaitForBmhDeleted(ctx, WaitForBmhDeletedInput{
				Client:    clusterProxy.GetClient(),
				BmhName:   bmh.Name,
				Namespace: bmh.Namespace,
				UndesiredStates: []metal3api.ProvisioningState{
					metal3api.StateProvisioning,
					metal3api.StateRegistering,
					metal3api.StateDeprovisioning,
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

			By("Creating a secret with BMH credentials")
			bmcCredentialsData = map[string]string{
				"username": bmc.User,
				"password": bmc.Password,
			}
			CreateSecret(ctx, clusterProxy.GetClient(), namespace.Name, secretName, bmcCredentialsData)

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
						Address:         bmc.Address,
						CredentialsName: "bmc-credentials",
					},
					BootMode:              metal3api.Legacy,
					BootMACAddress:        bmc.BootMacAddress,
					AutomatedCleaningMode: "disabled",
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
			WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
				Client: clusterProxy.GetClient(),
				Bmh:    bmh,
				State:  metal3api.StateProvisioned,
				UndesiredStates: []metal3api.ProvisioningState{
					metal3api.StateProvisioning,
					metal3api.StateRegistering,
					metal3api.StateDeprovisioning,
					metal3api.StateDeleting,
				},
			}, e2eConfig.GetIntervals(specName, "wait-provisioned")...)

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
			if !skipCleanup {
				cleanup(ctx, clusterProxy, namespace, cancelWatches, e2eConfig.GetIntervals("default", "wait-namespace-deleted")...)
			}
		})
	})
