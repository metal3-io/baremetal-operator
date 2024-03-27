package e2e

import (
	"context"
	"fmt"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/patch"

	"sigs.k8s.io/cluster-api/test/framework"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
)

const (
	bmhRootDeviceHint       = "/dev/vda"
	bmoDeploymentName       = "baremetal-operator-controller-manager"
	bmoIronicDeploymentName = "ironic"
)

type CleanupCallback func()

type UpgradePlanner interface {
	Prepare()
	Apply(*ClusterUpgradeStrategy)
	Dispose()
}

type BMOUpgradePlanner struct {
	bmc      *BMC
	bmhImage *metal3api.Image
	*CertManagerDeployStrategy
	*WaitInterval
	specName            string
	artifactFolder      string
	secretName          string
	hardwareDetail      string
	ctx                 context.Context
	upgradeClusterProxy framework.ClusterProxy
}

type WaitInterval struct {
	waitDeployment       []interface{}
	waitAvailable        []interface{}
	waitProvisioned      []interface{}
	waitNamespaceDeleted []interface{}
}

type CertManagerDeployStrategy struct {
	isDeployCertManager string
	certManagerVersion  string
}

type ClusterUpgradeStrategy struct {
	*BMOUpgradeStrategy
	*IronicDeployStrategy
}

type BMOUpgradeStrategy struct {
	isBMOUpgrade            string
	sourceKustomizationPath string
	targetKustomizationPath string
}

type IronicDeployStrategy struct {
	isDeployIronic          string
	ironicKustomizationPath string
	ironicNamespace         string
}

type rollingUpdateSpec struct {
	WaitForDeployment      bool
	WatchForDeploymentLogs bool
	KustomizationPath      string
	DeploymentName         string
	DeploymentNamespace    string
	LogPath                string
	CleanupCallback        CleanupCallback
}

func NewBMOUpgradePlanner(ctx context.Context, upgradeClusterProxy framework.ClusterProxy, bmc *BMC, bmhImage *metal3api.Image, waitIntervals *WaitInterval, certStrategy *CertManagerDeployStrategy, specName, artifactFolder, secretName, hardwareDetail string) UpgradePlanner {
	return &BMOUpgradePlanner{
		ctx:                       ctx,
		upgradeClusterProxy:       upgradeClusterProxy,
		artifactFolder:            artifactFolder,
		secretName:                secretName,
		bmc:                       bmc,
		bmhImage:                  bmhImage,
		WaitInterval:              waitIntervals,
		CertManagerDeployStrategy: certStrategy,
		hardwareDetail:            hardwareDetail,
		specName:                  specName,
	}
}

func (planner *BMOUpgradePlanner) Dispose() {
	planner.upgradeClusterProxy.Dispose(planner.ctx)
}

func (planner *BMOUpgradePlanner) Prepare() {
	if planner.isDeployCertManager != "false" {
		By("Installing cert-manager on the upgrade cluster")
		Expect(installCertManager(planner.ctx, planner.upgradeClusterProxy, planner.certManagerVersion)).To(Succeed())
		By("Waiting for cert-manager webhook")
		Eventually(func() error {
			return checkCertManagerWebhook(planner.ctx, planner.upgradeClusterProxy)
		}, planner.waitDeployment...).Should(Succeed())
		Expect(checkCertManagerAPI(planner.upgradeClusterProxy)).To(Succeed())
	}
}

func (planner *BMOUpgradePlanner) Apply(upgradeStrategy *ClusterUpgradeStrategy) {
	if upgradeStrategy.isDeployIronic != "false" {
		By("Installing Ironic on the upgrade cluster")
		planner.applyRollingUpdate(&rollingUpdateSpec{
			WaitForDeployment:      true,
			WatchForDeploymentLogs: true,
			KustomizationPath:      upgradeStrategy.ironicKustomizationPath,
			DeploymentName:         bmoIronicDeploymentName,
			DeploymentNamespace:    upgradeStrategy.ironicNamespace,
			LogPath:                fmt.Sprintf("%s-%s", upgradeStrategy.ironicNamespace, planner.specName),
			CleanupCallback: func() {
				By("Removing Ironic on the upgrade cluster")
				_ = planner.removeDeployment(upgradeStrategy.ironicKustomizationPath)
			},
		})
	}

	if upgradeStrategy.isBMOUpgrade != "false" {
		By(fmt.Sprintf("Installing BMO from %s on the upgrade cluster", upgradeStrategy.sourceKustomizationPath))
		planner.applyRollingUpdate(&rollingUpdateSpec{
			WaitForDeployment:      true,
			WatchForDeploymentLogs: true,
			KustomizationPath:      upgradeStrategy.sourceKustomizationPath,
			DeploymentName:         bmoDeploymentName,
			DeploymentNamespace:    upgradeStrategy.ironicNamespace,
			LogPath:                filepath.Join("logs", fmt.Sprintf("%s-%s", upgradeStrategy.ironicNamespace, planner.specName), fmt.Sprintf("bmo-%s", filepath.Base(upgradeStrategy.sourceKustomizationPath))),
			CleanupCallback: func() {
				By(fmt.Sprintf("Removing BMO from %s on the upgrade cluster", upgradeStrategy.sourceKustomizationPath))
				_ = planner.removeDeployment(upgradeStrategy.sourceKustomizationPath)
			},
		})
	}

	namespace, cancelWatches := framework.CreateNamespaceAndWatchEvents(planner.ctx, framework.CreateNamespaceAndWatchEventsInput{
		Creator:   planner.upgradeClusterProxy.GetClient(),
		ClientSet: planner.upgradeClusterProxy.GetClientSet(),
		Name:      fmt.Sprintf("%s-%s", planner.specName, util.RandomString(6)),
		LogFolder: planner.artifactFolder,
	})
	By("Creating a secret with BMH credentials")
	bmcCredentialsData := map[string]string{
		"username": planner.bmc.User,
		"password": planner.bmc.Password,
	}
	CreateSecret(planner.ctx, planner.upgradeClusterProxy.GetClient(), namespace.Name, planner.secretName, bmcCredentialsData)

	By("Creating a BMH with inspection disabled and hardware details added")
	bmh := metal3api.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      planner.specName,
			Namespace: namespace.Name,
			Annotations: map[string]string{
				metal3api.InspectAnnotationPrefix:   "disabled",
				metal3api.HardwareDetailsAnnotation: planner.hardwareDetail,
			},
		},
		Spec: metal3api.BareMetalHostSpec{
			Online: true,
			BMC: metal3api.BMCDetails{
				Address:         planner.bmc.Address,
				CredentialsName: planner.secretName,
			},
			BootMode:       metal3api.Legacy,
			BootMACAddress: planner.bmc.BootMacAddress,
		},
	}
	Expect(planner.upgradeClusterProxy.GetClient().Create(planner.ctx, &bmh)).To(Succeed())

	By("Waiting for the BMH to become available")
	WaitForBmhInProvisioningState(planner.ctx, WaitForBmhInProvisioningStateInput{
		Client: planner.upgradeClusterProxy.GetClient(),
		Bmh:    bmh,
		State:  metal3api.StateAvailable,
	}, planner.waitAvailable...)

	By("Upgrading BMO deployment")
	clientSet := planner.upgradeClusterProxy.GetClientSet()
	deploy, err := clientSet.AppsV1().Deployments(upgradeStrategy.ironicNamespace).Get(planner.ctx, bmoDeploymentName, metav1.GetOptions{})
	Expect(err).NotTo(HaveOccurred())
	planner.applyRollingUpdate(&rollingUpdateSpec{
		WaitForDeployment:      false,
		WatchForDeploymentLogs: true,
		KustomizationPath:      upgradeStrategy.targetKustomizationPath,
		DeploymentName:         bmoDeploymentName,
		DeploymentNamespace:    upgradeStrategy.ironicNamespace,
		LogPath:                filepath.Join("logs", fmt.Sprintf("%s-%s", upgradeStrategy.ironicNamespace, planner.specName), fmt.Sprintf("bmo-%s", filepath.Base(upgradeStrategy.targetKustomizationPath))),
		CleanupCallback: func() {
			By("Removing BMO main e2e deployment")
			_ = planner.removeDeployment(upgradeStrategy.targetKustomizationPath)
		},
	})

	By("Waiting for BMO update to rollout")
	Eventually(func() bool {
		return DeploymentRolledOut(planner.ctx, planner.upgradeClusterProxy, bmoDeploymentName, upgradeStrategy.ironicNamespace, deploy.Status.ObservedGeneration+1)
	}, planner.waitDeployment...).Should(BeTrue())

	By("Patching the BMH to test provisioning")
	helper, err := patch.NewHelper(&bmh, planner.upgradeClusterProxy.GetClient())
	Expect(err).NotTo(HaveOccurred())
	bmh.Spec.Image = planner.bmhImage
	bmh.Spec.RootDeviceHints = &metal3api.RootDeviceHints{
		DeviceName: bmhRootDeviceHint,
	}
	Expect(helper.Patch(planner.ctx, &bmh)).To(Succeed())

	By("Waiting for the BMH to become provisioned")
	WaitForBmhInProvisioningState(planner.ctx, WaitForBmhInProvisioningStateInput{
		Client: planner.upgradeClusterProxy.GetClient(),
		Bmh:    bmh,
		State:  metal3api.StateProvisioned,
	}, planner.waitProvisioned...)

	DeferCleanup(func() {
		cleanup(planner.ctx, planner.upgradeClusterProxy, namespace, cancelWatches, planner.waitNamespaceDeleted...)
	})
}

func (planner *BMOUpgradePlanner) applyRollingUpdate(updateSpec *rollingUpdateSpec) {
	Expect(BuildAndApplyKustomization(planner.ctx, &BuildAndApplyKustomizationInput{
		Kustomization:       updateSpec.KustomizationPath,
		ClusterProxy:        planner.upgradeClusterProxy,
		WaitForDeployment:   updateSpec.WaitForDeployment,
		WatchDeploymentLogs: updateSpec.WatchForDeploymentLogs,
		DeploymentName:      updateSpec.DeploymentName,
		DeploymentNamespace: updateSpec.DeploymentNamespace,
		LogPath:             filepath.Join(planner.artifactFolder, updateSpec.LogPath),
		WaitIntervals:       planner.waitDeployment,
	})).To(Succeed())
	DeferCleanup(updateSpec.CleanupCallback)
}

func (planner *BMOUpgradePlanner) removeDeployment(kustomizationPath string) error {
	return BuildAndRemoveKustomization(planner.ctx, kustomizationPath, planner.upgradeClusterProxy)
}
