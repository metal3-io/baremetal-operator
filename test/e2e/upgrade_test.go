package e2e

import (
	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("When testing BMO cluster Upgrade (v0.4=>current)", Label("optional", "upgrade"), func() {
	specName := "upgrade"
	BMOUpgradeSpec(ctx, func() BMOUpgradeSpecInput {
		return BMOUpgradeSpecInput{
			SpecName:           specName,
			SkipCleanup:        skipCleanup,
			ArtifactFolder:     artifactFolder,
			UseExistingCluster: useExistingCluster,
			Bmc:                &bmc,
			BmhImage: &metal3api.Image{
				URL:      e2eConfig.GetVariable("IMAGE_URL"),
				Checksum: e2eConfig.GetVariable("IMAGE_CHECKSUM"),
			},
			WaitInterval: &WaitInterval{
				waitDeployment:       e2eConfig.GetIntervals("default", "wait-deployment"),
				waitUpgradeAvailable: e2eConfig.GetIntervals(specName, "wait-available"),
				waitDefaultAvailable: e2eConfig.GetIntervals("default", "wait-available"),
				waitProvisioned:      e2eConfig.GetIntervals(specName, "wait-provisioned"),
				waitNamespaceDeleted: e2eConfig.GetIntervals("default", "wait-namespace-deleted"),
			},
			CertManagerDeployConfiguration: &CertManagerDeployConfiguration{
				isDeployCertManager: e2eConfig.GetVariable("UPGRADE_DEPLOY_CERT_MANAGER"),
				certManagerVersion:  e2eConfig.GetVariable("CERT_MANAGER_VERSION"),
			},
			HardwareDetail: hardwareDetailsRelease04,
			KindImages:     e2eConfig.Images,
			ClusterUpgradeInputUpgrades: []*ClusterUpgradeInputUpgrade{
				{
					BMOUpgradeStrategy: &BMOUpgradeStrategy{
						isBMOUpgrade:            e2eConfig.GetVariable("UPGRADE_DEPLOY_BMO"),
						sourceKustomizationPath: e2eConfig.GetVariable("UPGRADE_BMO_KUSTOMIZATION_FROM"),
						targetKustomizationPath: e2eConfig.GetVariable("BMO_KUSTOMIZATION"),
					},
					IronicDeployStrategy: &IronicDeployStrategy{
						isIronicUpgrade:         e2eConfig.GetVariable("UPGRADE_DEPLOY_IRONIC"),
						ironicKustomizationPath: e2eConfig.GetVariable("IRONIC_KUSTOMIZATION"),
					},
				},
			},
		}
	})
})
