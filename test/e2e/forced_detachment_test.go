//go:build e2e

package e2e

import (
	"context"
	"path"

	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/nodes"
	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/util/deprecated/v1beta1/patch"
)

var _ = Describe("Start provisioning, force detachment, delete and recreate, provision, detach again and delete", Label("required", "provision", "detach", "force-detach"),
	func() {
		var (
			specName      = "force-detach"
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

		It("starts provisioning, forces detachment, removes the host", func() {
			bmhName := specName + "-remove"
			secretName := bmhName + "-bmc-creds"

			By("Creating a secret with BMH credentials")
			bmcCredentialsData := map[string]string{
				"username": bmc.User,
				"password": bmc.Password,
			}
			CreateSecret(ctx, clusterProxy.GetClient(), namespace.Name, secretName, bmcCredentialsData)

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
			err := clusterProxy.GetClient().Create(ctx, &bmh)
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for the BMH to become available")
			WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
				Client: clusterProxy.GetClient(),
				Bmh:    bmh,
				State:  metal3api.StateAvailable,
			}, e2eConfig.GetIntervals(specName, "wait-available")...)

			By("Patching the BMH to test provisioning")
			err = PatchBMHForProvisioning(ctx, PatchBMHForProvisioningInput{
				client:    clusterProxy.GetClient(),
				bmh:       &bmh,
				bmc:       bmc,
				e2eConfig: e2eConfig,
				namespace: namespace.Name,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for the BMH to be in provisioning state")
			WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
				Client: clusterProxy.GetClient(),
				Bmh:    bmh,
				State:  metal3api.StateProvisioning,
			}, e2eConfig.GetIntervals(specName, "wait-provisioning")...)

			if e2eConfig.GetBoolVariable("DEPLOY_IRONIC") {
				By("Waiting for the Ironic node to start deploying")
				ironicClient := CreateIronicClient(e2eConfig)
				ironicNodeName := IronicNodeName(namespace.Name, bmhName)
				WaitForIronicNodeProvisionState(ctx, WaitForIronicNodeProvisionStateInput{
					Client:   ironicClient,
					NodeName: ironicNodeName,
					States:   []nodes.ProvisionState{nodes.DeployWait},
				}, e2eConfig.GetIntervals(specName, "wait-ironic-state")...)
			}

			By("Retrieving the latest BMH object")
			err = clusterProxy.GetClient().Get(ctx, types.NamespacedName{
				Name:      bmh.Name,
				Namespace: bmh.Namespace,
			}, &bmh)
			Expect(err).NotTo(HaveOccurred())

			By("Adding the detached annotation")
			helper, err := patch.NewHelper(&bmh, clusterProxy.GetClient())
			Expect(err).NotTo(HaveOccurred())

			if bmh.Annotations == nil {
				bmh.Annotations = make(map[string]string, 1)
			}
			bmh.Annotations["baremetalhost.metal3.io/detached"] = "{\"force\": true}"

			Expect(helper.Patch(ctx, &bmh)).To(Succeed())

			By("Waiting for the BMH to be detached")
			WaitForBmhInOperationalStatus(ctx, WaitForBmhInOperationalStatusInput{
				Client: clusterProxy.GetClient(),
				Bmh:    bmh,
				State:  metal3api.OperationalStatusDetached,
				UndesiredStates: []metal3api.OperationalStatus{
					metal3api.OperationalStatusError,
				},
			}, e2eConfig.GetIntervals(specName, "wait-detached")...)

			By("Retrieving and checking the latest BMH object")
			err = clusterProxy.GetClient().Get(ctx, types.NamespacedName{
				Name:      bmh.Name,
				Namespace: bmh.Namespace,
			}, &bmh)
			Expect(err).NotTo(HaveOccurred())
			Expect(bmh.Status.Provisioning.State).To(Equal(metal3api.StateProvisioning))

			By("Deleting the BMH")
			err = clusterProxy.GetClient().Delete(ctx, &bmh)
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for the BMH to be deleted")
			WaitForBmhDeleted(ctx, WaitForBmhDeletedInput{
				Client:    clusterProxy.GetClient(),
				BmhName:   bmh.Name,
				Namespace: bmh.Namespace,
				UndesiredStates: []metal3api.ProvisioningState{
					metal3api.StateDeprovisioning,
					metal3api.StatePoweringOffBeforeDelete,
				},
			}, e2eConfig.GetIntervals(specName, "wait-bmh-deleted")...)
		})

		It("starts provisioning, forces detachment, re-attaches and finishes provisioning", func() {
			if !e2eConfig.GetBoolVariable("DEPLOY_IRONIC") {
				Skip("Test on provisioning after detachment relies on provisioning taking non-zero time")
			}

			bmhName := specName + "-finish"
			secretName := specName + "-bmc-creds"

			By("Creating a secret with BMH credentials")
			bmcCredentialsData := map[string]string{
				"username": bmc.User,
				"password": bmc.Password,
			}
			CreateSecret(ctx, clusterProxy.GetClient(), namespace.Name, secretName, bmcCredentialsData)

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
			err := clusterProxy.GetClient().Create(ctx, &bmh)
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for the BMH to become available")
			WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
				Client: clusterProxy.GetClient(),
				Bmh:    bmh,
				State:  metal3api.StateAvailable,
			}, e2eConfig.GetIntervals(specName, "wait-available")...)

			By("Patching the BMH to test provisioning")
			var userDataSecret *corev1.SecretReference
			if e2eConfig.GetVariable("SSH_CHECK_PROVISIONED") == "true" {
				userDataSecretName := "user-data"
				sshPubKeyPath := e2eConfig.GetVariable("SSH_PUB_KEY")
				createSSHSetupUserdata(ctx, clusterProxy.GetClient(), namespace.Name, userDataSecretName, sshPubKeyPath, bmc.IPAddress)
				userDataSecret = &corev1.SecretReference{
					Name:      userDataSecretName,
					Namespace: namespace.Name,
				}
			}
			err = PatchBMHForProvisioning(ctx, PatchBMHForProvisioningInput{
				client:         clusterProxy.GetClient(),
				bmh:            &bmh,
				bmc:            bmc,
				e2eConfig:      e2eConfig,
				namespace:      namespace.Name,
				userDataSecret: userDataSecret,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for the BMH to be in provisioning state")
			WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
				Client: clusterProxy.GetClient(),
				Bmh:    bmh,
				State:  metal3api.StateProvisioning,
			}, e2eConfig.GetIntervals(specName, "wait-provisioning")...)

			By("Waiting for the Ironic node to start deploying")
			ironicClient := CreateIronicClient(e2eConfig)
			ironicNodeName := IronicNodeName(namespace.Name, bmhName)
			WaitForIronicNodeProvisionState(ctx, WaitForIronicNodeProvisionStateInput{
				Client:   ironicClient,
				NodeName: ironicNodeName,
				States:   []nodes.ProvisionState{nodes.DeployWait},
			}, e2eConfig.GetIntervals(specName, "wait-ironic-state")...)

			By("Retrieving the latest BMH object")
			err = clusterProxy.GetClient().Get(ctx, types.NamespacedName{
				Name:      bmh.Name,
				Namespace: bmh.Namespace,
			}, &bmh)
			Expect(err).NotTo(HaveOccurred())

			By("Adding the detached annotation")
			helper, err := patch.NewHelper(&bmh, clusterProxy.GetClient())
			Expect(err).NotTo(HaveOccurred())

			if bmh.Annotations == nil {
				bmh.Annotations = make(map[string]string, 1)
			}
			bmh.Annotations["baremetalhost.metal3.io/detached"] = "{\"force\": true}"

			Expect(helper.Patch(ctx, &bmh)).To(Succeed())

			By("Waiting for the BMH to be detached")
			WaitForBmhInOperationalStatus(ctx, WaitForBmhInOperationalStatusInput{
				Client: clusterProxy.GetClient(),
				Bmh:    bmh,
				State:  metal3api.OperationalStatusDetached,
				UndesiredStates: []metal3api.OperationalStatus{
					metal3api.OperationalStatusError,
				},
			}, e2eConfig.GetIntervals(specName, "wait-detached")...)

			By("Retrieving and checking the latest BMH object")
			err = clusterProxy.GetClient().Get(ctx, types.NamespacedName{
				Name:      bmh.Name,
				Namespace: bmh.Namespace,
			}, &bmh)
			Expect(err).NotTo(HaveOccurred())
			Expect(bmh.Status.Provisioning.State).To(Equal(metal3api.StateProvisioning))

			By("Removing the detached annotation")
			helper, err = patch.NewHelper(&bmh, clusterProxy.GetClient())
			Expect(err).NotTo(HaveOccurred())

			delete(bmh.Annotations, "baremetalhost.metal3.io/detached")
			Expect(helper.Patch(ctx, &bmh)).To(Succeed())

			By("Waiting for the BMH to become provisioned")
			WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
				Client: clusterProxy.GetClient(),
				Bmh:    bmh,
				State:  metal3api.StateProvisioned,
			}, e2eConfig.GetIntervals(specName, "wait-provisioned")...)

			// The ssh check is not possible in all situations (e.g. fixture) so it can be skipped
			if e2eConfig.GetVariable("SSH_CHECK_PROVISIONED") == "true" {
				By("Verifying the node booting from disk")
				PerformSSHBootCheck(e2eConfig, "disk", bmc.IPAddress)
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

			if bmh.Annotations == nil {
				bmh.Annotations = make(map[string]string, 1)
			}
			// Making sure that forced detachment works in the normal case too
			bmh.Annotations["baremetalhost.metal3.io/detached"] = "{\"force\": true}"

			Expect(helper.Patch(ctx, &bmh)).To(Succeed())

			By("Waiting for the BMH to be detached")
			WaitForBmhInOperationalStatus(ctx, WaitForBmhInOperationalStatusInput{
				Client: clusterProxy.GetClient(),
				Bmh:    bmh,
				State:  metal3api.OperationalStatusDetached,
				UndesiredStates: []metal3api.OperationalStatus{
					metal3api.OperationalStatusError,
				},
			}, e2eConfig.GetIntervals(specName, "wait-detached")...)

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
			CollectSerialLogs(bmc.Name, path.Join(artifactFolder, specName))
			DumpResources(ctx, e2eConfig, clusterProxy, path.Join(artifactFolder, specName))
			if !skipCleanup {
				isNamespaced := e2eConfig.GetBoolVariable("NAMESPACE_SCOPED")
				Cleanup(ctx, clusterProxy, namespace, cancelWatches, isNamespaced, e2eConfig.GetIntervals("default", "wait-namespace-deleted")...)
			}
		})
	})
