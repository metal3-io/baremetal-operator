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
		specName               = "upgrade"
		secretName             = "bmc-credentials"
		clusterName            = "bmo-e2e-upgrade"
		bmoIronicNamespace     = "baremetal-operator-system"
		upgradeClusterProvider bootstrap.ClusterProvider
		upgradePlanner         UpgradePlanner
		kubeconfigPath         string
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
				waitAvailable:        e2eConfig.GetIntervals(specName, "wait-available"),
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
		func(upgradeStrategy *ClusterUpgradeStrategy) {
			upgradePlanner.Apply(upgradeStrategy)
		},
		func(upgradeStrategy *ClusterUpgradeStrategy) string {
			return fmt.Sprintf("Upgrade BMO from release %s to release %s", ExtractReleaseFromKustomization(upgradeStrategy.sourceKustomizationPath), ExtractReleaseFromKustomization(upgradeStrategy.targetKustomizationPath))
		},
		Entry(nil, &ClusterUpgradeStrategy{
			BMOUpgradeStrategy: &BMOUpgradeStrategy{
				isBMOUpgrade:            e2eConfig.GetVariable("UPGRADE_DEPLOY_BMO"),
				sourceKustomizationPath: e2eConfig.GetVariable("UPGRADE_BMO_KUSTOMIZATION_FROM"),
				targetKustomizationPath: e2eConfig.GetVariable("BMO_KUSTOMIZATION"),
			},
			IronicDeployStrategy: &IronicDeployStrategy{
				isDeployIronic:          e2eConfig.GetVariable("UPGRADE_DEPLOY_IRONIC"),
				ironicKustomizationPath: e2eConfig.GetVariable("IRONIC_KUSTOMIZATION"),
				ironicNamespace:         bmoIronicNamespace,
			},
		}),
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
