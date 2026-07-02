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
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/bootstrap"
)

// RunIronicUpgradeTest tests upgrading Ironic.
// Steps:
//   - Deploy BMO and an older version of Ironic.
//   - Create a BMH with "disabled" inspection annotation and wait for it to become available.
//   - Upgrade Ironic to the latest version.
//   - Patch the BMH for provisioning and verify it becomes provisioned.
func RunIronicUpgradeTest(ctx context.Context, input *IronicUpgradeSpec, upgradeClusterProxy framework.ClusterProxy, testCaseArtifactFolder string) (*corev1.Namespace, context.CancelFunc) {
	bmoIronicNamespace := "baremetal-operator-system"
	testCaseName := getIronicUpgradeTestCaseName(input)

	if e2eConfig.GetBoolVariable("UPGRADE_DEPLOY_IRSO") {
		if input.IrsoKustomization == "" {
			Fail("deployIRSO is enabled but IrsoKustomization is empty; please specify a valid kustomization path in the test config")
		}
		err := BuildAndApplyKustomization(ctx, &BuildAndApplyKustomizationInput{
			Kustomization:       input.IrsoKustomization,
			ClusterProxy:        upgradeClusterProxy,
			WaitForDeployment:   true,
			WatchDeploymentLogs: true,
			DeploymentName:      "ironic-standalone-operator-controller-manager",
			DeploymentNamespace: "ironic-standalone-operator-system",
			LogPath:             filepath.Join(testCaseArtifactFolder, "logs"),
			WaitIntervals:       e2eConfig.GetIntervals("default", "wait-deployment"),
		})
		Expect(err).NotTo(HaveOccurred())
	}

	if input.DeployIronic {
		By(fmt.Sprintf("Installing Ironic from kustomization %s on the upgrade cluster", input.InitIronicKustomization))
		err := BuildAndApplyKustomization(ctx, &BuildAndApplyKustomizationInput{
			Kustomization:       input.InitIronicKustomization,
			ClusterProxy:        upgradeClusterProxy,
			WaitForDeployment:   false,
			WatchDeploymentLogs: true,
			DeploymentName:      "ironic-service",
			DeploymentNamespace: bmoIronicNamespace,
			LogPath:             filepath.Join(testCaseArtifactFolder, "logs"),
		})
		Expect(err).NotTo(HaveOccurred())
		WaitForIronicReady(ctx, WaitForIronicInput{
			Client:    upgradeClusterProxy.GetClient(),
			Name:      "ironic",
			Namespace: bmoIronicNamespace,
			Intervals: e2eConfig.GetIntervals("ironic", "wait-deployment"),
		})
	}

	if input.DeployBMO {
		By(fmt.Sprintf("Installing BMO from %s on the upgrade cluster", input.InitBMOKustomization))
		err := FlakeAttempt(2, func() error {
			return BuildAndApplyKustomization(ctx, &BuildAndApplyKustomizationInput{
				Kustomization:       input.InitBMOKustomization,
				ClusterProxy:        upgradeClusterProxy,
				WaitForDeployment:   true,
				WatchDeploymentLogs: true,
				DeploymentName:      "baremetal-operator-controller-manager",
				DeploymentNamespace: bmoIronicNamespace,
				LogPath:             filepath.Join(testCaseArtifactFolder, "logs"),
				WaitIntervals:       e2eConfig.GetIntervals("default", "wait-deployment"),
			})
		})
		Expect(err).NotTo(HaveOccurred())
	}

	namespace, cancelWatches := framework.CreateNamespaceAndWatchEvents(ctx, framework.CreateNamespaceAndWatchEventsInput{
		Creator:             upgradeClusterProxy.GetClient(),
		ClientSet:           upgradeClusterProxy.GetClientSet(),
		Name:                "upgrade-ironic",
		LogFolder:           testCaseArtifactFolder,
		IgnoreAlreadyExists: true,
	})

	By("Creating a secret with BMH credentials")
	bmcCredentialsData := map[string]string{
		"username": bmc.User,
		"password": bmc.Password,
	}
	secretName := "bmc-credentials"
	// Delete any leftover secret from a previous run (namespace may be reused with IgnoreAlreadyExists).
	DeleteSecretIfExists(ctx, upgradeClusterProxy.GetClient(), namespace.Name, secretName)
	CreateSecret(ctx, upgradeClusterProxy.GetClient(), namespace.Name, secretName, bmcCredentialsData)

	By("Creating a BMH with inspection disabled and hardware details added")
	hardwareDetails := hardwareDetailsFor(&bmc)
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
	}, e2eConfig.GetIntervals("upgrade", "wait-available")...)

	By("Upgrading Ironic deployment")
	clientSet := upgradeClusterProxy.GetClientSet()
	deploy, err := clientSet.AppsV1().Deployments(bmoIronicNamespace).Get(ctx, "ironic-service", metav1.GetOptions{})
	Expect(err).NotTo(HaveOccurred())
	err = FlakeAttempt(2, func() error {
		return BuildAndApplyKustomization(ctx, &BuildAndApplyKustomizationInput{
			Kustomization:       input.UpgradeIronicKustomization,
			ClusterProxy:        upgradeClusterProxy,
			WaitForDeployment:   false,
			WatchDeploymentLogs: true,
			DeploymentName:      "ironic-service",
			DeploymentNamespace: bmoIronicNamespace,
			LogPath:             filepath.Join(testCaseArtifactFolder, "logs"),
		})
	})
	Expect(err).NotTo(HaveOccurred())

	By("Waiting for Ironic update to rollout")
	Eventually(func() bool {
		return DeploymentRolledOut(ctx, upgradeClusterProxy, "ironic-service", bmoIronicNamespace, deploy.Status.ObservedGeneration+1)
	},
		e2eConfig.GetIntervals("ironic", "wait-deployment")...,
	).Should(BeTrue())
	WaitForIronicReady(ctx, WaitForIronicInput{
		Client:    upgradeClusterProxy.GetClient(),
		Name:      "ironic",
		Namespace: bmoIronicNamespace,
		Intervals: e2eConfig.GetIntervals("ironic", "wait-deployment"),
	})

	By("Waiting for BMO controller to be ready to process events")
	WaitForBmhReconciled(ctx, upgradeClusterProxy.GetClient(), bmh,
		e2eConfig.GetIntervals("default", "wait-deployment")...)

	By("Patching the BMH to test provisioning")
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
	}, e2eConfig.GetIntervals("upgrade", "wait-provisioned")...)
	return namespace, cancelWatches
}

// getIronicUpgradeTestCaseName returns the test case name for an Ironic upgrade test.
func getIronicUpgradeTestCaseName(input *IronicUpgradeSpec) string {
	upgradeFromKustomizationName := strings.ReplaceAll(filepath.Base(input.InitIronicKustomization), ".", "-")
	return "ironic-upgrade-from-" + upgradeFromKustomizationName
}

// The Ordered decorator ensures upgrade tests run one at a time.
// This is important since the same provisioning IP is used in each test.
// They can still run in parallel with other tests (since they use another IP).
var _ = Describe("Ironic upgrade", Serial, Ordered, Label("optional", "upgrade", "ironic-upgrade"), func() {
	var (
		upgradeClusterProxy    framework.ClusterProxy
		upgradeClusterProvider bootstrap.ClusterProvider
		entries                = make([]TableEntry, 0, len(e2eConfig.IronicUpgradeSpecs))
		namespace              *corev1.Namespace
		cancelWatches          context.CancelFunc
		testArtifactFolder     string
	)

	for i := range e2eConfig.IronicUpgradeSpecs {
		entries = append(entries, Entry(nil, ctx, &e2eConfig.IronicUpgradeSpecs[i]))
	}

	BeforeEach(func() {
		upgradeClusterProxy, upgradeClusterProvider = setupUpgradeCluster(ctx)
	})

	DescribeTable("",
		func(ctx context.Context, input *IronicUpgradeSpec) {
			testCaseName := getIronicUpgradeTestCaseName(input)
			testArtifactFolder = filepath.Join(artifactFolder, testCaseName)
			namespace, cancelWatches = RunIronicUpgradeTest(ctx, input, upgradeClusterProxy, testArtifactFolder)
		},
		func(ctx context.Context, input *IronicUpgradeSpec) string {
			return fmt.Sprintf("Should upgrade Ironic from %s to %s", input.InitIronicKustomization, input.UpgradeIronicKustomization)
		},
		entries,
	)

	AfterEach(func() {
		cleanupUpgradeTest(ctx, upgradeClusterProxy, upgradeClusterProvider, namespace, cancelWatches, testArtifactFolder)
	})
})
