package e2e

import (
	"fmt"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/bootstrap"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
)

var _ = Describe("BMO Upgrade", Label("optional", "upgrade"), func() {
	var (
		specName                 = "upgrade"
		secretName               = "bmc-credentials"
		clusterName              = "bmo-e2e-upgrade"
		bmoIronicNamespace       = "baremetal-operator-system"
		upgradeClusterProvider   bootstrap.ClusterProvider
		upgradePlanner           UpgradePlanner
		kubeconfigPath           string
		clusterUpgradeStrategies []*ClusterUpgradeStrategy
	)

	BeforeEach(func() {
		if useExistingCluster {
			kubeconfigPath = os.Getenv("KUBECONFIG")
			if kubeconfigPath == "" {
				kubeconfigPath = os.Getenv("HOME") + "/.kube/config"
			}
		} else {
			By("Creating a separate cluster for upgrade tests")
			upgradeClusterProvider = bootstrap.CreateKindBootstrapClusterAndLoadImages(ctx, bootstrap.CreateKindBootstrapClusterAndLoadImagesInput{
				Name:   clusterName,
				Images: e2eConfig.Images,
			})
			Expect(upgradeClusterProvider).ToNot(BeNil(), "Failed to create a cluster")
			kubeconfigPath = upgradeClusterProvider.GetKubeconfigPath()
		}
		Expect(kubeconfigPath).To(BeAnExistingFile(), "Failed to get the kubeconfig file for the cluster")

		// Initialize upgrade strategies
		clusterUpgradeStrategies = []*ClusterUpgradeStrategy{
			{
				BMOUpgradeStrategy: &BMOUpgradeStrategy{
					isBMOUpgrade:            e2eConfig.GetVariable("UPGRADE_DEPLOY_BMO"),
					sourceKustomizationPath: e2eConfig.GetVariable("UPGRADE_BMO_KUSTOMIZATION_FROM"),
					targetKustomizationPath: e2eConfig.GetVariable("BMO_KUSTOMIZATION"),
				},
				IronicDeployStrategy: &IronicDeployStrategy{
					isIronicUpgrade:         e2eConfig.GetVariable("UPGRADE_DEPLOY_IRONIC"),
					ironicKustomizationPath: e2eConfig.GetVariable("IRONIC_KUSTOMIZATION"),
					ironicNamespace:         bmoIronicNamespace,
				},
			},
		}
	})

	JustBeforeEach(func() {
		scheme := runtime.NewScheme()
		framework.TryAddDefaultSchemes(scheme)
		Expect(metal3api.AddToScheme(scheme)).To(Succeed())
		upgradeClusterProxy := framework.NewClusterProxy(clusterName, kubeconfigPath, scheme)
		upgradePlanner = NewBMOUpgradePlanner(
			ctx,
			upgradeClusterProxy,
			&bmc,
			&metal3api.Image{
				URL:      e2eConfig.GetVariable("IMAGE_URL"),
				Checksum: e2eConfig.GetVariable("IMAGE_CHECKSUM"),
			},
			&WaitInterval{
				waitDeployment:       e2eConfig.GetIntervals("default", "wait-deployment"),
				waitUpgradeAvailable: e2eConfig.GetIntervals(specName, "wait-available"),
				waitDefaultAvailable: e2eConfig.GetIntervals("default", "wait-available"),
				waitProvisioned:      e2eConfig.GetIntervals(specName, "wait-provisioned"),
				waitNamespaceDeleted: e2eConfig.GetIntervals("default", "wait-namespace-deleted"),
			},
			&CertManagerDeployStrategy{
				isDeployCertManager: e2eConfig.GetVariable("UPGRADE_DEPLOY_CERT_MANAGER"),
				certManagerVersion:  e2eConfig.GetVariable("CERT_MANAGER_VERSION"),
			},
			specName,
			artifactFolder,
			secretName,
			hardwareDetailsRelease04,
		)

		upgradePlanner.Prepare()
	})

	DescribeTable("Should able to upgrade BMO",
		func(upgradeStrategies []*ClusterUpgradeStrategy, index int) {
			By(fmt.Sprintf("Start upgrading BMO cluster from release %s to release %s", ExtractReleaseFromKustomization(upgradeStrategies[index].sourceKustomizationPath), ExtractReleaseFromKustomization(upgradeStrategies[index].targetKustomizationPath)))
			upgradePlanner.Apply(upgradeStrategies[index])
		},
		func(upgradeStrategies []*ClusterUpgradeStrategy, index int) string {
			return fmt.Sprintf("Upgrade BMO test: %d\n", index+1)
		},
		Entry(nil, clusterUpgradeStrategies, 0),
	)

	AfterEach(func() {
		if !skipCleanup {
			upgradePlanner.Dispose()
			if upgradeClusterProvider != nil {
				upgradeClusterProvider.Dispose(ctx)
			}
		}
	})

})
