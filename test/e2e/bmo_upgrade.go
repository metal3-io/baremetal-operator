package e2e

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/cluster-api/test/framework/bootstrap"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/patch"

	"sigs.k8s.io/cluster-api/test/framework"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
)

// BMOUpgradeSpecInput holds the inputs for BMOUpgradeSpec.
type BMOUpgradeSpecInput struct {
	// Spec name of this upgrade.
	SpecName string
	// Whether to skip cleanup after each test.
	// Can be overridden with e2e.skip-resource-cleanup cli flag.
	SkipCleanup bool
	// Indicate if the test should use an existing cluster.
	UseExistingCluster bool
	// BMH details to be used in test.
	// Value is passed through e2e.bmcsConfig
	Bmc *BMC
	// BMH image to be used in test.
	// Can be overridden with IMAGE_URL and IMAGE_CHECKSUM environment variables.
	BmhImage *metal3api.Image
	// Indicate configuration for cert manager to be deployed as prerequisite of the test.
	// UPGRADE_DEPLOY_CERT_MANAGER can be overridden with false to skip cert manager deployment.
	// CERT_MANAGER_VERSION can be overridden with the desired version of cert manager if not skipped.
	*CertManagerDeployConfiguration
	// Multiple types of waiting intervals to be used in the test.
	*WaitInterval
	// Based folder to store the test artifacts.
	ArtifactFolder string
	// Annotations to be used in BMH creation.
	HardwareDetail string
	// List of Kind cluster images to be used in the test.
	// Field is not relevant when UseExistingCluster is false.
	KindImages []clusterctl.ContainerImage
	// Upgrades allow to define upgrade sequences for the test.
	ClusterUpgradeInputUpgrades []*ClusterUpgradeInputUpgrade
}

// WaitInterval holds the waiting intervals for the test.
type WaitInterval struct {
	// waitDeployment is the interval to wait for deployment to be ready.
	waitDeployment       []interface{}
	waitUpgradeAvailable []interface{}
	waitDefaultAvailable []interface{}
	// waitProvisioned is the interval to wait for BMH to be provisioned.
	waitProvisioned []interface{}
	// waitNamespaceDeleted is the interval to wait for namespace to be deleted.
	waitNamespaceDeleted []interface{}
}

// CertManagerDeployConfiguration holds the cert manager deployment configurations.
type CertManagerDeployConfiguration struct {
	// Indicate if cert manager should be deployed.
	isDeployCertManager string
	// Version of cert manager to be deployed.
	certManagerVersion string
}

// ClusterUpgradeInputUpgrade holds the detailed upgrade input for the test.
type ClusterUpgradeInputUpgrade struct {
	// BMOUpgradeStrategy defines the BMO upgrade direction.
	*BMOUpgradeStrategy
	// IronicDeployStrategy defines the Ironic upgrade direction.
	*IronicDeployStrategy
}

// BMOUpgradeStrategy defines the BMO upgrade direction.
type BMOUpgradeStrategy struct {
	// Indicate if BMO upgrade is required.
	isBMOUpgrade string
	// Source BMO kustomization path to upgrade from.
	sourceKustomizationPath string
	// Target BMO kustomization path to upgrade to.
	targetKustomizationPath string
}

// IronicDeployStrategy defines the Ironic upgrade direction.
type IronicDeployStrategy struct {
	// Indicate if Ironic upgrade is required.
	isIronicUpgrade string
	// Ironic kustomization path to upgrade to.
	ironicKustomizationPath string
}

// cleanupCallback is the type for cleanup functions.
type cleanupCallback func()

// upgradeRunner is the interface for upgrade flow.
type upgradeRunner interface {
	Prepare()
	Apply(*ClusterUpgradeInputUpgrade)
	Dispose()
}

// bmoUpgradeRunner implements upgrade flow for BMO cluster.
type bmoUpgradeRunner struct {
	bmc      *BMC
	bmhImage *metal3api.Image
	*CertManagerDeployConfiguration
	*WaitInterval
	bmoDeploymentName       string
	bmoIronicDeploymentName string
	bmoIronicNamespace      string
	bmhRootDeviceHint       string
	specName                string
	artifactFolder          string
	secretName              string
	hardwareDetail          string
	ctx                     context.Context
	upgradeClusterProxy     framework.ClusterProxy
}

// rollingUpdateSpec is the input to applyRollingUpdate.
type rollingUpdateSpec struct {
	WaitForDeployment      bool
	WatchForDeploymentLogs bool
	KustomizationPath      string
	DeploymentName         string
	DeploymentNamespace    string
	LogPath                string
	cleanup                cleanupCallback
}

// BMOUpgradeSpec implements a test that verifies the upgrade procedure of a BMO cluster.
func BMOUpgradeSpec(ctx context.Context, inputGetter func() BMOUpgradeSpecInput) {
	var (
		secretName              = "bmc-credentials"
		clusterName             = "bmo-e2e-upgrade"
		bmoIronicNamespace      = "baremetal-operator-system"
		bmhRootDeviceHint       = "/dev/vda"
		bmoDeploymentName       = "baremetal-operator-controller-manager"
		bmoIronicDeploymentName = "ironic"
		kubeconfigPath          string
		upgradeClusterProvider  bootstrap.ClusterProvider
		runner                  upgradeRunner
		input                   BMOUpgradeSpecInput
	)

	BeforeEach(func() {
		input = inputGetter()
		Expect(ctx).NotTo(BeNil(), "ctx is required for %s spec", input.SpecName)
		if input.UseExistingCluster {
			kubeconfigPath = os.Getenv("KUBECONFIG")
			if kubeconfigPath == "" {
				kubeconfigPath = os.Getenv("HOME") + "/.kube/config"
			}
		} else {
			By("Creating a separate cluster for upgrade tests")
			upgradeClusterProvider = bootstrap.CreateKindBootstrapClusterAndLoadImages(ctx, bootstrap.CreateKindBootstrapClusterAndLoadImagesInput{
				Name:   clusterName,
				Images: input.KindImages,
			})
			Expect(upgradeClusterProvider).ToNot(BeNil(), "Failed to create a cluster")
			kubeconfigPath = upgradeClusterProvider.GetKubeconfigPath()
		}
		Expect(kubeconfigPath).To(BeAnExistingFile(), "Failed to get the kubeconfig file for the cluster")
	})

	JustBeforeEach(func() {
		scheme := runtime.NewScheme()
		framework.TryAddDefaultSchemes(scheme)
		Expect(metal3api.AddToScheme(scheme)).To(Succeed())
		upgradeClusterProxy := framework.NewClusterProxy(clusterName, kubeconfigPath, scheme)
		runner = &bmoUpgradeRunner{
			bmc:                            input.Bmc,
			bmhImage:                       input.BmhImage,
			CertManagerDeployConfiguration: input.CertManagerDeployConfiguration,
			WaitInterval:                   input.WaitInterval,
			bmoDeploymentName:              bmoDeploymentName,
			bmoIronicDeploymentName:        bmoIronicDeploymentName,
			bmoIronicNamespace:             bmoIronicNamespace,
			bmhRootDeviceHint:              bmhRootDeviceHint,
			specName:                       input.SpecName,
			artifactFolder:                 input.ArtifactFolder,
			secretName:                     secretName,
			hardwareDetail:                 input.HardwareDetail,
			ctx:                            ctx,
			upgradeClusterProxy:            upgradeClusterProxy,
		}

		runner.Prepare()
	})

	DescribeTable("Should able to upgrade BMO",
		func(upgradeStrategies []*ClusterUpgradeInputUpgrade, index int) {
			By(fmt.Sprintf("Start upgrading BMO cluster from release %s to release %s", ExtractReleaseFromKustomization(upgradeStrategies[index].sourceKustomizationPath), ExtractReleaseFromKustomization(upgradeStrategies[index].targetKustomizationPath)))
			runner.Apply(upgradeStrategies[index])
		},
		func(upgradeStrategies []*ClusterUpgradeInputUpgrade, index int) string {
			return fmt.Sprintf("Upgrade BMO test: %d\n", index+1)
		},
		Entry(nil, input.ClusterUpgradeInputUpgrades, 0),
	)

	AfterEach(func() {
		if !input.SkipCleanup {
			runner.Dispose()
			if upgradeClusterProvider != nil {
				upgradeClusterProvider.Dispose(ctx)
			}
		}
	})
}

func (runner *bmoUpgradeRunner) Dispose() {
	runner.upgradeClusterProxy.Dispose(runner.ctx)
}

func (runner *bmoUpgradeRunner) Prepare() {
	if runner.isDeployCertManager != "false" {
		By("Installing cert-manager on the upgrade cluster")
		Expect(installCertManager(runner.ctx, runner.upgradeClusterProxy, runner.certManagerVersion)).To(Succeed())
		By("Waiting for cert-manager webhook")
		Eventually(func() error {
			return checkCertManagerWebhook(runner.ctx, runner.upgradeClusterProxy)
		}, runner.waitDefaultAvailable...).Should(Succeed())
		Expect(checkCertManagerAPI(runner.upgradeClusterProxy)).To(Succeed())
	}
}

func (runner *bmoUpgradeRunner) Apply(upgradeStrategy *ClusterUpgradeInputUpgrade) {
	if upgradeStrategy.isIronicUpgrade != "false" {
		By("Installing Ironic on the upgrade cluster")
		runner.applyRollingUpdate(&rollingUpdateSpec{
			WaitForDeployment:      true,
			WatchForDeploymentLogs: true,
			KustomizationPath:      upgradeStrategy.ironicKustomizationPath,
			DeploymentName:         runner.bmoIronicDeploymentName,
			DeploymentNamespace:    runner.bmoIronicNamespace,
			LogPath:                filepath.Join("logs", fmt.Sprintf("%s-%s", runner.bmoIronicNamespace, runner.specName)),
			cleanup: func() {
				By("Removing Ironic on the upgrade cluster")
				_ = runner.removeDeployment(upgradeStrategy.ironicKustomizationPath)
			},
		})
	}

	if upgradeStrategy.isBMOUpgrade != "false" {
		By(fmt.Sprintf("Installing BMO from %s on the upgrade cluster", upgradeStrategy.sourceKustomizationPath))
		runner.applyRollingUpdate(&rollingUpdateSpec{
			WaitForDeployment:      true,
			WatchForDeploymentLogs: true,
			KustomizationPath:      upgradeStrategy.sourceKustomizationPath,
			DeploymentName:         runner.bmoDeploymentName,
			DeploymentNamespace:    runner.bmoIronicNamespace,
			LogPath:                filepath.Join("logs", fmt.Sprintf("%s-%s", runner.bmoIronicNamespace, runner.specName), fmt.Sprintf("bmo-%s", filepath.Base(upgradeStrategy.sourceKustomizationPath))),
			cleanup: func() {
				By(fmt.Sprintf("Removing BMO from %s on the upgrade cluster", upgradeStrategy.sourceKustomizationPath))
				_ = runner.removeDeployment(upgradeStrategy.sourceKustomizationPath)
			},
		})
	}

	namespace, cancelWatches := framework.CreateNamespaceAndWatchEvents(runner.ctx, framework.CreateNamespaceAndWatchEventsInput{
		Creator:   runner.upgradeClusterProxy.GetClient(),
		ClientSet: runner.upgradeClusterProxy.GetClientSet(),
		Name:      fmt.Sprintf("%s-%s", runner.specName, util.RandomString(6)),
		LogFolder: runner.artifactFolder,
	})
	By("Creating a secret with BMH credentials")
	bmcCredentialsData := map[string]string{
		"username": runner.bmc.User,
		"password": runner.bmc.Password,
	}
	CreateSecret(runner.ctx, runner.upgradeClusterProxy.GetClient(), namespace.Name, runner.secretName, bmcCredentialsData)

	By("Creating a BMH with inspection disabled and hardware details added")
	bmh := metal3api.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      runner.specName,
			Namespace: namespace.Name,
			Annotations: map[string]string{
				metal3api.InspectAnnotationPrefix:   "disabled",
				metal3api.HardwareDetailsAnnotation: runner.hardwareDetail,
			},
		},
		Spec: metal3api.BareMetalHostSpec{
			Online: true,
			BMC: metal3api.BMCDetails{
				Address:         runner.bmc.Address,
				CredentialsName: runner.secretName,
			},
			BootMode:       metal3api.Legacy,
			BootMACAddress: runner.bmc.BootMacAddress,
		},
	}
	Expect(runner.upgradeClusterProxy.GetClient().Create(runner.ctx, &bmh)).To(Succeed())

	By("Waiting for the BMH to become available")
	WaitForBmhInProvisioningState(runner.ctx, WaitForBmhInProvisioningStateInput{
		Client: runner.upgradeClusterProxy.GetClient(),
		Bmh:    bmh,
		State:  metal3api.StateAvailable,
	}, runner.waitUpgradeAvailable...)

	By("Upgrading BMO deployment")
	clientSet := runner.upgradeClusterProxy.GetClientSet()
	deploy, err := clientSet.AppsV1().Deployments(runner.bmoIronicNamespace).Get(runner.ctx, runner.bmoDeploymentName, metav1.GetOptions{})
	Expect(err).NotTo(HaveOccurred())
	runner.applyRollingUpdate(&rollingUpdateSpec{
		WaitForDeployment:      false,
		WatchForDeploymentLogs: true,
		KustomizationPath:      upgradeStrategy.targetKustomizationPath,
		DeploymentName:         runner.bmoDeploymentName,
		DeploymentNamespace:    runner.bmoIronicNamespace,
		LogPath:                filepath.Join("logs", fmt.Sprintf("%s-%s", runner.bmoIronicNamespace, runner.specName), fmt.Sprintf("bmo-%s", filepath.Base(upgradeStrategy.targetKustomizationPath))),
		cleanup: func() {
			By("Removing BMO main e2e deployment")
			_ = runner.removeDeployment(upgradeStrategy.targetKustomizationPath)
		},
	})

	By("Waiting for BMO update to rollout")
	Eventually(func() bool {
		return DeploymentRolledOut(runner.ctx, runner.upgradeClusterProxy, runner.bmoDeploymentName, runner.bmoIronicNamespace, deploy.Status.ObservedGeneration+1)
	}, runner.waitDeployment...).Should(BeTrue())

	By("Patching the BMH to test provisioning")
	helper, err := patch.NewHelper(&bmh, runner.upgradeClusterProxy.GetClient())
	Expect(err).NotTo(HaveOccurred())
	bmh.Spec.Image = runner.bmhImage
	bmh.Spec.RootDeviceHints = &metal3api.RootDeviceHints{
		DeviceName: runner.bmhRootDeviceHint,
	}
	Expect(helper.Patch(runner.ctx, &bmh)).To(Succeed())

	By("Waiting for the BMH to become provisioned")
	WaitForBmhInProvisioningState(runner.ctx, WaitForBmhInProvisioningStateInput{
		Client: runner.upgradeClusterProxy.GetClient(),
		Bmh:    bmh,
		State:  metal3api.StateProvisioned,
	}, runner.waitProvisioned...)

	DeferCleanup(func() {
		cleanup(runner.ctx, runner.upgradeClusterProxy, namespace, cancelWatches, runner.waitNamespaceDeleted...)
	})
}

func (runner *bmoUpgradeRunner) applyRollingUpdate(updateSpec *rollingUpdateSpec) {
	Expect(BuildAndApplyKustomization(runner.ctx, &BuildAndApplyKustomizationInput{
		Kustomization:       updateSpec.KustomizationPath,
		ClusterProxy:        runner.upgradeClusterProxy,
		WaitForDeployment:   updateSpec.WaitForDeployment,
		WatchDeploymentLogs: updateSpec.WatchForDeploymentLogs,
		DeploymentName:      updateSpec.DeploymentName,
		DeploymentNamespace: updateSpec.DeploymentNamespace,
		LogPath:             filepath.Join(runner.artifactFolder, updateSpec.LogPath),
		WaitIntervals:       runner.waitDeployment,
	})).To(Succeed())
	DeferCleanup(updateSpec.cleanup)
}

func (runner *bmoUpgradeRunner) removeDeployment(kustomizationPath string) error {
	return BuildAndRemoveKustomization(runner.ctx, kustomizationPath, runner.upgradeClusterProxy)
}
