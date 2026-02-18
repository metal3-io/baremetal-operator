//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/bootstrap"
)

// RunUpgradeTest tests upgrade from an older version of BMO or Ironic --> main branch version with the following steps:
//   - Initiate the cluster with an the older version of either BMO or Ironic, and the latest Ironic/BMO version that is suitable with it
//   - Create a new namespace, and in it a BMH object with "disabled" annotation.
//   - Wait until the BMH gets to "available" state. Because of the "disabled" annotation, it won't get further provisioned.
//   - Upgrade BMO/Ironic to latest version.
//   - Patch the BMH object with proper specs, so that it could be provisioned.
//   - If the BMH is successfully provisioned, it means the upgraded BMO/Ironic recognized that BMH, hence the upgrade succeeded.
//
// The function returns the namespace object, with its cancelFunc. These can be used to clean up the created resources.
// The testCaseArtifactFolder parameter specifies where to store test artifacts.
func RunUpgradeTest(ctx context.Context, input *BMOIronicUpgradeInput, upgradeClusterProxy framework.ClusterProxy, testCaseArtifactFolder string) (*corev1.Namespace, context.CancelFunc) {
	bmoIronicNamespace := "baremetal-operator-system"
	initBMOKustomization := input.InitBMOKustomization
	initIronicKustomization := input.InitIronicKustomization
	upgradeEntityName := input.UpgradeEntityName
	specName := "upgrade"
	testCaseName := getUpgradeTestCaseName(input)
	var upgradeDeploymentName string
	switch upgradeEntityName {
	case bmoString:
		upgradeDeploymentName = "baremetal-operator-controller-manager"
	case ironicString:
		upgradeDeploymentName = "ironic-service"
	}
	if input.DeployIronic {
		// Install Ironic
		By(fmt.Sprintf("Installing Ironic from kustomization %s on the upgrade cluster", initIronicKustomization))
		err := BuildAndApplyKustomization(ctx, &BuildAndApplyKustomizationInput{
			Kustomization:       initIronicKustomization,
			ClusterProxy:        upgradeClusterProxy,
			WaitForDeployment:   false,
			WatchDeploymentLogs: true,
			DeploymentName:      "ironic-service",
			DeploymentNamespace: bmoIronicNamespace,
			LogPath:             filepath.Join(testCaseArtifactFolder, "logs", "init-ironic"),
		})
		WaitForIronicReady(ctx, WaitForIronicInput{
			Client:    clusterProxy.GetClient(),
			Name:      "ironic",
			Namespace: bmoIronicNamespace,
			Intervals: e2eConfig.GetIntervals("ironic", "wait-deployment"),
		})
		Expect(err).NotTo(HaveOccurred())
	}
	if input.DeployBMO {
		// Install BMO
		By(fmt.Sprintf("Installing BMO from %s on the upgrade cluster", initBMOKustomization))
		err := FlakeAttempt(2, func() error {
			return BuildAndApplyKustomization(ctx, &BuildAndApplyKustomizationInput{
				Kustomization:       initBMOKustomization,
				ClusterProxy:        upgradeClusterProxy,
				WaitForDeployment:   true,
				WatchDeploymentLogs: true,
				DeploymentName:      "baremetal-operator-controller-manager",
				DeploymentNamespace: bmoIronicNamespace,
				LogPath:             filepath.Join(testCaseArtifactFolder, "logs", "init-bmo"),
				WaitIntervals:       e2eConfig.GetIntervals("default", "wait-deployment"),
			})
		})
		Expect(err).NotTo(HaveOccurred())
	}

	namespace, cancelWatches := framework.CreateNamespaceAndWatchEvents(ctx, framework.CreateNamespaceAndWatchEventsInput{
		Creator:             upgradeClusterProxy.GetClient(),
		ClientSet:           upgradeClusterProxy.GetClientSet(),
		Name:                "upgrade-" + input.UpgradeEntityName,
		LogFolder:           testCaseArtifactFolder,
		IgnoreAlreadyExists: true,
	})

	By("Creating a secret with BMH credentials")
	bmcCredentialsData := map[string]string{
		"username": bmc.User,
		"password": bmc.Password,
	}
	secretName := "bmc-credentials"
	CreateSecret(ctx, upgradeClusterProxy.GetClient(), namespace.Name, secretName, bmcCredentialsData)

	By("Creating a BMH with inspection disabled and hardware details added")
	bmh := metal3api.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testCaseName,
			Namespace: namespace.Name,
			Annotations: map[string]string{
				metal3api.InspectAnnotationPrefix:   "disabled",
				metal3api.HardwareDetailsAnnotation: hardwareDetails,
			},
		},
		Spec: metal3api.BareMetalHostSpec{
			Online: true,
			BMC: metal3api.BMCDetails{
				Address:                        bmc.Address,
				CredentialsName:                secretName,
				DisableCertificateVerification: bmc.DisableCertificateVerification,
			},
			BootMode:       metal3api.BootMode(e2eConfig.GetVariable("BOOT_MODE")),
			BootMACAddress: bmc.BootMacAddress,
		},
	}
	err := upgradeClusterProxy.GetClient().Create(ctx, &bmh)
	Expect(err).NotTo(HaveOccurred())

	By("Waiting for the BMH to become available")
	WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
		Client: upgradeClusterProxy.GetClient(),
		Bmh:    bmh,
		State:  metal3api.StateAvailable,
	}, e2eConfig.GetIntervals(specName, "wait-available")...)

	// TODO(lentzi90): Since the introduction of IrSO, we should not be dealing with Deployments for Ironic.
	// We should split this test into Ironic upgrade and BMO upgrade and handle them separately.
	By(fmt.Sprintf("Upgrading %s deployment", input.UpgradeEntityName))
	clientSet := upgradeClusterProxy.GetClientSet()
	deploy, err := clientSet.AppsV1().Deployments(bmoIronicNamespace).Get(ctx, upgradeDeploymentName, metav1.GetOptions{})
	Expect(err).NotTo(HaveOccurred())
	upgradeKustomization := input.UpgradeEntityKustomization
	err = FlakeAttempt(2, func() error {
		return BuildAndApplyKustomization(ctx, &BuildAndApplyKustomizationInput{
			Kustomization:       upgradeKustomization,
			ClusterProxy:        upgradeClusterProxy,
			WaitForDeployment:   false,
			WatchDeploymentLogs: true,
			DeploymentName:      upgradeDeploymentName,
			DeploymentNamespace: bmoIronicNamespace,
			LogPath:             filepath.Join(testCaseArtifactFolder, "logs", "bmo-upgrade-main"),
		})
	})
	Expect(err).NotTo(HaveOccurred())

	By(fmt.Sprintf("Waiting for %s update to rollout", input.UpgradeEntityName))
	Eventually(func() bool {
		return DeploymentRolledOut(ctx, upgradeClusterProxy, upgradeDeploymentName, bmoIronicNamespace, deploy.Status.ObservedGeneration+1)
	},
		e2eConfig.GetIntervals("ironic", "wait-deployment")...,
	).Should(BeTrue())
	if input.UpgradeEntityName == ironicString {
		WaitForIronicReady(ctx, WaitForIronicInput{
			Client:    clusterProxy.GetClient(),
			Name:      "ironic",
			Namespace: bmoIronicNamespace,
			Intervals: e2eConfig.GetIntervals("ironic", "wait-deployment"),
		})
	}

	// After deployment rollout, wait for the BMO controller to actually be processing events.
	// There's a race condition where the deployment is "available" but the new pod hasn't
	// acquired the leader lease yet or hasn't started its watches. If we patch the BMH
	// during this window, the controller may miss the update event.
	By("Waiting for BMO controller to be ready to process events")
	WaitForBmhReconciled(ctx, upgradeClusterProxy.GetClient(), bmh,
		e2eConfig.GetIntervals("default", "wait-deployment")...)

	By("Patching the BMH to test provisioning")
	// Using Eventually here since the webhook can take some time after the deployment is ready
	Eventually(func() error {
		return PatchBMHForProvisioning(ctx, PatchBMHForProvisioningInput{
			client:    upgradeClusterProxy.GetClient(),
			bmh:       &bmh,
			bmc:       bmc,
			e2eConfig: e2eConfig,
		})
	}, e2eConfig.GetIntervals("default", "wait-deployment")...).Should(Succeed())

	By("Waiting for the BMH to become provisioned")
	WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
		Client: upgradeClusterProxy.GetClient(),
		Bmh:    bmh,
		State:  metal3api.StateProvisioned,
	}, e2eConfig.GetIntervals(specName, "wait-provisioned")...)
	return namespace, cancelWatches
}

// getUpgradeTestCaseName returns the test case name for an upgrade test.
// The name takes the form [upgrade-entity]-upgrade-from-[kustomization-path].
func getUpgradeTestCaseName(input *BMOIronicUpgradeInput) string {
	var upgradeFromKustomization string
	switch input.UpgradeEntityName {
	case bmoString:
		upgradeFromKustomization = input.InitBMOKustomization
	case ironicString:
		upgradeFromKustomization = input.InitIronicKustomization
	}
	upgradeFromKustomizationName := strings.ReplaceAll(filepath.Base(upgradeFromKustomization), ".", "-")
	return fmt.Sprintf("%s-upgrade-from-%s", input.UpgradeEntityName, upgradeFromKustomizationName)
}

var _ = Describe("Upgrade", Label("optional", "upgrade"), func() {

	var (
		upgradeClusterProxy    framework.ClusterProxy
		upgradeClusterProvider bootstrap.ClusterProvider
		entries                = make([]TableEntry, 0, len(e2eConfig.BMOIronicUpgradeSpecs))
		namespace              *corev1.Namespace
		cancelWatches          context.CancelFunc
		testArtifactFolder     string
	)

	for i := range e2eConfig.BMOIronicUpgradeSpecs {
		entries = append(entries, Entry(nil, ctx, &e2eConfig.BMOIronicUpgradeSpecs[i]))
	}

	BeforeEach(func() {
		// Before each test, we need to	initiate the cluster and/or prepare it to be ready for the test
		var kubeconfigPath string

		if e2eConfig.GetBoolVariable("UPGRADE_USE_EXISTING_CLUSTER") {
			kubeconfigPath = GetKubeconfigPath()
		} else {
			By("Creating a separate cluster for upgrade tests")
			upgradeClusterName := fmt.Sprintf("bmo-e2e-upgrade-%d", GinkgoParallelProcess())
			upgradeClusterProvider = bootstrap.CreateKindBootstrapClusterAndLoadImages(ctx, bootstrap.CreateKindBootstrapClusterAndLoadImagesInput{
				Name:              upgradeClusterName,
				Images:            e2eConfig.GetClusterctlImages(),
				ExtraPortMappings: e2eConfig.KindExtraPortMappings,
			})
			Expect(upgradeClusterProvider).ToNot(BeNil(), "Failed to create a cluster")
			kubeconfigPath = upgradeClusterProvider.GetKubeconfigPath()

			// Configure provisioning network for dnsmasq to work properly.
			// TODO(lentzi90): This is a workaround. Fix it properly and get rid of it.
			ConfigureProvisioningNetwork(ctx, upgradeClusterName, e2eConfig.GetVariable("IRONIC_PROVISIONING_IP"))
		}
		Expect(kubeconfigPath).To(BeAnExistingFile(), "Failed to get the kubeconfig file for the cluster")
		scheme := runtime.NewScheme()
		framework.TryAddDefaultSchemes(scheme)
		err := metal3api.AddToScheme(scheme)
		Expect(err).NotTo(HaveOccurred())
		upgradeClusterProxy = framework.NewClusterProxy("bmo-e2e-upgrade", kubeconfigPath, scheme)

		if e2eConfig.GetBoolVariable("UPGRADE_DEPLOY_CERT_MANAGER") {
			By("Installing cert-manager on the upgrade cluster")
			cmVersion := e2eConfig.GetVariable("CERT_MANAGER_VERSION")
			err := installCertManager(ctx, upgradeClusterProxy, cmVersion)
			Expect(err).NotTo(HaveOccurred())
			By("Waiting for cert-manager webhook")
			Eventually(func() error {
				return checkCertManagerWebhook(ctx, upgradeClusterProxy)
			}, e2eConfig.GetIntervals("default", "wait-available")...).Should(Succeed())
			err = checkCertManagerAPI(upgradeClusterProxy)
			Expect(err).NotTo(HaveOccurred())
		}
		if e2eConfig.GetBoolVariable("UPGRADE_DEPLOY_IRSO") {
			BuildAndApplyKustomization(ctx, &BuildAndApplyKustomizationInput{
				Kustomization:       e2eConfig.GetVariable("IRSO_KUSTOMIZATION"),
				ClusterProxy:        clusterProxy,
				WaitForDeployment:   true,
				WatchDeploymentLogs: true,
				DeploymentName:      "ironic-standalone-operator-controller-manager",
				DeploymentNamespace: "ironic-standalone-operator-system",
				LogPath:             filepath.Join(artifactFolder, "logs", "ironic-standalone-operator-system"),
				WaitIntervals:       e2eConfig.GetIntervals("default", "wait-deployment"),
			})
		}
	})
	DescribeTable("",
		// Test function that runs for each table entry
		func(ctx context.Context, input *BMOIronicUpgradeInput) {
			testCaseName := getUpgradeTestCaseName(input)
			// Set testArtifactFolder before RunUpgradeTest so it's available in AfterEach even if the test fails
			testArtifactFolder = filepath.Join(artifactFolder, testCaseName)
			namespace, cancelWatches = RunUpgradeTest(ctx, input, upgradeClusterProxy, testArtifactFolder)
		},
		// Description function that generates test descriptions
		func(ctx context.Context, input *BMOIronicUpgradeInput) string {
			var upgradeFromKustomization string
			upgradeEntityName := input.UpgradeEntityName
			switch upgradeEntityName {
			case bmoString:
				upgradeFromKustomization = input.InitBMOKustomization
			case ironicString:
				upgradeFromKustomization = input.InitIronicKustomization
			}
			return fmt.Sprintf("Should upgrade %s from %s to %s", input.UpgradeEntityName, upgradeFromKustomization, input.UpgradeEntityKustomization)
		},
		entries,
	)

	AfterEach(func() {
		DumpResources(ctx, e2eConfig, upgradeClusterProxy, testArtifactFolder)
		if !skipCleanup {
			cleanup(ctx, upgradeClusterProxy, namespace, cancelWatches, e2eConfig.GetIntervals("default", "wait-namespace-deleted")...)
			if e2eConfig.GetBoolVariable("UPGRADE_USE_EXISTING_CLUSTER") {
				// Try to clean up as best as we can.
				// Note that we only delete the "normal" BMO kustomization. There could be small
				// differences between this and the initial or upgrade kustomization, but this also
				// cleans up the namespace, which should take care of everything except CRDs
				// and cluster-scoped RBAC, including Ironic if it was deployed.
				// There is a theoretical risk that we leak cluster-scoped resources for the
				// next test here, if there are differences between the kustomizations.
				cleanupBaremetalOperatorSystem(ctx, upgradeClusterProxy, e2eConfig.GetVariable("BMO_KUSTOMIZATION"))
			} else {
				// We are using a kind cluster for the upgrade tests, so we just delete the cluster.
				upgradeClusterProvider.Dispose(ctx)
			}
			upgradeClusterProxy.Dispose(ctx)
		}
	})
})

// cleanupBaremetalOperatorSystem removes the kustomization from the cluster and waits for the
// baremetal-operator-system namespace to be deleted.
func cleanupBaremetalOperatorSystem(ctx context.Context, clusterProxy framework.ClusterProxy, kustomization string) {
	BuildAndRemoveKustomization(ctx, kustomization, clusterProxy)
	// We need to ensure that the namespace actually gets deleted.
	WaitForNamespaceDeleted(ctx, WaitForNamespaceDeletedInput{
		Getter:    clusterProxy.GetClient(),
		Namespace: corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "baremetal-operator-system"}},
	}, e2eConfig.GetIntervals("default", "wait-namespace-deleted")...)
}
