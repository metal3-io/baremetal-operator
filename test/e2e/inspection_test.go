//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"fmt"
	"path"

	ironicPort "github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/ports"
	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Inspection", Label("required", "inspection"), func() {
	var (
		specName      = "inspection"
		namespace     *corev1.Namespace
		cancelWatches context.CancelFunc
		toCleanup     []client.Object
	)
	BeforeEach(func() {
		toCleanup = nil
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
		toCleanup = append(toCleanup, &bmh)

		By("waiting for the BMH to be in unmanaged state")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: clusterProxy.GetClient(),
			Bmh:    bmh,
			State:  metal3api.StateUnmanaged,
		}, e2eConfig.GetIntervals(specName, "wait-unmanaged")...)

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
		toCleanup = append(toCleanup, &bmh)

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
	})

	It("should inspect a newly created BMH", func() {
		By("Creating a secret with BMH credentials")

		bmcCredentialsData := map[string]string{
			"username": bmc.User,
			"password": bmc.Password,
		}
		secret := CreateSecret(ctx, clusterProxy.GetClient(), namespace.Name, "bmc-credentials", bmcCredentialsData)
		toCleanup = append(toCleanup, secret)

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
				BootMode:       metal3api.BootMode(e2eConfig.GetVariable("BOOT_MODE")),
				BootMACAddress: bmc.BootMacAddress,
			},
		}
		err := clusterProxy.GetClient().Create(ctx, &bmh)
		Expect(err).NotTo(HaveOccurred())
		toCleanup = append(toCleanup, &bmh)

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

		if e2eConfig.GetVariable("SSH_CHECK_PROVISIONED") == "true" {
			By("verifying IPA booted from the expected Ironic-managed boot source")
			VerifyIronicManagedBoot(e2eConfig, bmc.Address, bmc.IPAddress)
		} else {
			Logf("WARNING: Skipping boot source verification since SSH_CHECK_PROVISIONED != true")
		}

		if e2eConfig.GetBoolVariable("DEPLOY_IRONIC") {
			getMacList := func(ports []ironicPort.Port) []string {
				macs := make([]string, 0, len(ports))
				for _, port := range ports {
					macs = append(macs, port.Address)
				}
				return macs
			}

			By("Get ports in Ironic before dropping the database")
			portsBefore, errPortsBefore := getIronicPorts(ctx, e2eConfig)
			Expect(errPortsBefore).NotTo(HaveOccurred())
			Expect(portsBefore).To(Not(BeEmpty()))

			By("Redeploy Ironic deployment to drop its database")
			WaitForIronicRedeploy(ctx, WaitForIronicInput{
				Client:    clusterProxy.GetClient(),
				Name:      "ironic-service",
				Namespace: "baremetal-operator-system",
				Intervals: e2eConfig.GetIntervals("default", "wait-deployment"),
			})

			By("Waiting for BMH to be reconciled")
			WaitForBmhReconciled(ctx, clusterProxy.GetClient(), bmh,
				e2eConfig.GetIntervals("default", "wait-deployment")...)

			By("Get ports in Ironic after dropping the database and reconciling and check if they are the same")
			Eventually(func(g Gomega) {
				portsAfter, errPotrsAfter := getIronicPorts(ctx, e2eConfig)
				g.Expect(errPotrsAfter).NotTo(HaveOccurred())
				g.Expect(getMacList(portsAfter)).To(ConsistOf(getMacList(portsBefore)))
			}, e2eConfig.GetIntervals("default", "wait-deployment")...).Should(Succeed())
		}
	})

	AfterEach(func() {
		CollectSerialLogs(bmc.Name, path.Join(artifactFolder, specName))
		DumpResources(ctx, e2eConfig, clusterProxy, path.Join(artifactFolder, specName))
		if !skipCleanup {
			Cleanup(ctx, clusterProxy, namespace, cancelWatches, e2eConfig, toCleanup)
		}
	})
})
