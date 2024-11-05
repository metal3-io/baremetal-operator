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
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/patch"
)

const hardwareDetailsRelease04 = `
{
  "cpu": {
    "arch": "x86_64",
    "count": 2,
    "flags": [
      "3dnowprefetch",
      "abm",
      "adx",
      "aes",
      "apic",
      "arat",
      "arch_capabilities",
      "avx",
      "avx2",
      "avx_vnni",
      "bmi1",
      "bmi2",
      "clflush",
      "clflushopt",
      "clwb",
      "cmov",
      "constant_tsc",
      "cpuid",
      "cpuid_fault",
      "cx16",
      "cx8",
      "de",
      "ept",
      "ept_ad",
      "erms",
      "f16c",
      "flexpriority",
      "fma",
      "fpu",
      "fsgsbase",
      "fsrm",
      "fxsr",
      "gfni",
      "hypervisor",
      "ibpb",
      "ibrs",
      "ibrs_enhanced",
      "invpcid",
      "lahf_lm",
      "lm",
      "mca",
      "mce",
      "md_clear",
      "mmx",
      "movbe",
      "movdir64b",
      "movdiri",
      "msr",
      "mtrr",
      "nopl",
      "nx",
      "ospke",
      "pae",
      "pat",
      "pclmulqdq",
      "pdpe1gb",
      "pge",
      "pku",
      "pni",
      "popcnt",
      "pse",
      "pse36",
      "rdpid",
      "rdrand",
      "rdseed",
      "rdtscp",
      "rep_good",
      "sep",
      "serialize",
      "sha_ni",
      "smap",
      "smep",
      "ss",
      "ssbd",
      "sse",
      "sse2",
      "sse4_1",
      "sse4_2",
      "ssse3",
      "stibp",
      "syscall",
      "tpr_shadow",
      "tsc",
      "tsc_adjust",
      "tsc_deadline_timer",
      "tsc_known_freq",
      "umip",
      "vaes",
      "vme",
      "vmx",
      "vnmi",
      "vpclmulqdq",
      "vpid",
      "waitpkg",
      "x2apic",
      "xgetbv1",
      "xsave",
      "xsavec",
      "xsaveopt",
      "xsaves",
      "xtopology"
    ],
    "model": "12th Gen Intel(R) Core(TM) i9-12900H"
  },
  "firmware": {
    "bios": {
      "date": "04/01/2014",
      "vendor": "SeaBIOS",
      "version": "1.15.0-1"
    }
  },
  "hostname": "bmo-e2e-1",
  "nics": [
    {
      "ip": "192.168.223.122",
      "mac": "00:60:2f:31:81:02",
      "model": "0x1af4 0x0001",
      "name": "enp1s0",
      "pxe": true
    },
    {
      "ip": "fe80::570a:edf2:a3a7:4eb8%enp1s0",
      "mac": "00:60:2f:31:81:02",
      "model": "0x1af4 0x0001",
      "name": "enp1s0",
      "pxe": true
    }
  ],
  "ramMebibytes": 4096,
  "storage": [
    {
      "name": "/dev/disk/by-path/pci-0000:04:00.0",
      "rotational": true,
      "sizeBytes": 21474836480,
      "type": "HDD",
      "vendor": "0x1af4"
    }
  ],
  "systemVendor": {
    "manufacturer": "QEMU",
    "productName": "Standard PC (Q35 + ICH9, 2009)"
  }
}
`

// RunUpgradeTest tests upgrade from an older version of BMO or Ironic --> main branch version with the following steps:
//	- Initiate the cluster with an the older version of either BMO or Ironic, and the latest Ironic/BMO version that is suitable with it
//	- Create a new namespace, and in it a BMH object with "disabled" annotation.
//	- Wait until the BMH gets to "available" state. Because of the "disabled" annotation, it won't get further provisioned.
//	- Upgrade BMO/Ironic to latest version.
//	- Patch the BMH object with proper specs, so that it could be provisioned.
//	- If the BMH is successfully provisioned, it means the upgraded BMO/Ironic recognized that BMH, hence the upgrade succeeded.
// The function returns the namespace object, with its cancelFunc. These can be used to clean up the created resources.

func RunUpgradeTest(ctx context.Context, input *BMOIronicUpgradeInput, upgradeClusterProxy framework.ClusterProxy) (*corev1.Namespace, context.CancelFunc) {
	bmoIronicNamespace := "baremetal-operator-system"
	initBMOKustomization := input.InitBMOKustomization
	initIronicKustomization := input.InitIronicKustomization
	upgradeEntityName := input.UpgradeEntityName
	specName := "upgrade"
	var upgradeDeploymentName, upgradeFromKustomization string
	switch upgradeEntityName {
	case "bmo":
		upgradeFromKustomization = initBMOKustomization
		upgradeDeploymentName = "baremetal-operator-controller-manager"
	case "ironic":
		upgradeFromKustomization = initIronicKustomization
		upgradeDeploymentName = "ironic"
	}
	upgradeFromKustomizationName := strings.ReplaceAll(filepath.Base(upgradeFromKustomization), ".", "-")
	testCaseName := fmt.Sprintf("%s-upgrade-from-%s", upgradeEntityName, upgradeFromKustomizationName)
	testCaseArtifactFolder := filepath.Join(artifactFolder, testCaseName)
	if input.DeployIronic {
		// Install Ironic
		By(fmt.Sprintf("Installing Ironic from kustomization %s on the upgrade cluster", initIronicKustomization))
		err := FlakeAttempt(2, func() error {
			return BuildAndApplyKustomization(ctx, &BuildAndApplyKustomizationInput{
				Kustomization:       initIronicKustomization,
				ClusterProxy:        upgradeClusterProxy,
				WaitForDeployment:   true,
				WatchDeploymentLogs: true,
				DeploymentName:      "ironic",
				DeploymentNamespace: bmoIronicNamespace,
				LogPath:             filepath.Join(testCaseArtifactFolder, "logs", "init-ironic"),
				WaitIntervals:       e2eConfig.GetIntervals("default", "wait-deployment"),
			})
		})
		Expect(err).NotTo(HaveOccurred())

		DeferCleanup(func() {
			By(fmt.Sprintf("Removing Ironic kustomization %s from the upgrade cluster", initIronicKustomization))
			cleanupBaremetalOperatorSystem(ctx, upgradeClusterProxy, initIronicKustomization)
		})
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
		DeferCleanup(func() {
			By(fmt.Sprintf("Removing BMO kustomization %s from the upgrade cluster", initBMOKustomization))
			cleanupBaremetalOperatorSystem(ctx, upgradeClusterProxy, initBMOKustomization)
		})
	}

	namespace, cancelWatches := framework.CreateNamespaceAndWatchEvents(ctx, framework.CreateNamespaceAndWatchEventsInput{
		Creator:   upgradeClusterProxy.GetClient(),
		ClientSet: upgradeClusterProxy.GetClientSet(),
		Name:      fmt.Sprintf("upgrade-%s-%s", input.UpgradeEntityName, util.RandomString(6)),
		LogFolder: testCaseArtifactFolder,
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
			Name:      "upgrade",
			Namespace: namespace.Name,
			Annotations: map[string]string{
				metal3api.InspectAnnotationPrefix: "disabled",
				// hardwareDetails of release0.4 is compatible to release0.3 and release0.5 as well
				// This can be changed to the new hardwareDetails once we no longer test release0.4
				metal3api.HardwareDetailsAnnotation: hardwareDetailsRelease04,
			},
		},
		Spec: metal3api.BareMetalHostSpec{
			Online: true,
			BMC: metal3api.BMCDetails{
				Address:         bmc.Address,
				CredentialsName: secretName,
			},
			BootMode:       metal3api.Legacy,
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
			WaitIntervals:       e2eConfig.GetIntervals("default", "wait-deployment"),
		})
	})
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(func() {
		By(fmt.Sprintf("Removing %s kustomization %s from the upgrade cluster", input.UpgradeEntityName, upgradeKustomization))
		cleanupBaremetalOperatorSystem(ctx, upgradeClusterProxy, upgradeKustomization)
	})

	By(fmt.Sprintf("Waiting for %s update to rollout", input.UpgradeEntityName))
	Eventually(func() bool {
		return DeploymentRolledOut(ctx, upgradeClusterProxy, upgradeDeploymentName, bmoIronicNamespace, deploy.Status.ObservedGeneration+1)
	},
		e2eConfig.GetIntervals("default", "wait-deployment")...,
	).Should(BeTrue())

	By("Patching the BMH to test provisioning")
	helper, err := patch.NewHelper(&bmh, upgradeClusterProxy.GetClient())
	Expect(err).NotTo(HaveOccurred())
	bmh.Spec.Image = &metal3api.Image{
		URL:      e2eConfig.GetVariable("IMAGE_URL"),
		Checksum: e2eConfig.GetVariable("IMAGE_CHECKSUM"),
	}
	bmh.Spec.RootDeviceHints = &metal3api.RootDeviceHints{
		DeviceName: "/dev/vda",
	}
	Expect(helper.Patch(ctx, &bmh)).To(Succeed())

	By("Waiting for the BMH to become provisioned")
	WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
		Client: upgradeClusterProxy.GetClient(),
		Bmh:    bmh,
		State:  metal3api.StateProvisioned,
	}, e2eConfig.GetIntervals(specName, "wait-provisioned")...)
	return namespace, cancelWatches
}

var _ = Describe("Upgrade", Label("optional", "upgrade"), func() {

	var (
		upgradeClusterProxy framework.ClusterProxy
		entries             []TableEntry
		namespace           *corev1.Namespace
		cancelWatches       context.CancelFunc
	)

	for i := range e2eConfig.BMOIronicUpgradeSpecs {
		entries = append(entries, Entry(nil, ctx, &e2eConfig.BMOIronicUpgradeSpecs[i]))
	}

	BeforeEach(func() {
		// Before each test, we need to	initiate the cluster and/or prepare it to be ready for the test
		var kubeconfigPath string
		upgradeClusterName := "bmo-e2e-upgrade"

		if useExistingCluster {
			kubeconfigPath = GetKubeconfigPath()
		} else {
			By("Creating a separate cluster for upgrade tests")
			upgradeClusterName = fmt.Sprintf("bmo-e2e-upgrade-%d", GinkgoParallelProcess())
			upgradeClusterProvider := bootstrap.CreateKindBootstrapClusterAndLoadImages(ctx, bootstrap.CreateKindBootstrapClusterAndLoadImagesInput{
				Name:   upgradeClusterName,
				Images: e2eConfig.Images,
			})
			Expect(upgradeClusterProvider).ToNot(BeNil(), "Failed to create a cluster")
			kubeconfigPath = upgradeClusterProvider.GetKubeconfigPath()
			DeferCleanup(func() {
				By(fmt.Sprintf("Disposing the kind cluster %s", upgradeClusterName))
				upgradeClusterProvider.Dispose(ctx)
			})
		}
		Expect(kubeconfigPath).To(BeAnExistingFile(), "Failed to get the kubeconfig file for the cluster")
		scheme := runtime.NewScheme()
		framework.TryAddDefaultSchemes(scheme)
		err := metal3api.AddToScheme(scheme)
		Expect(err).NotTo(HaveOccurred())
		upgradeClusterProxy = framework.NewClusterProxy("bmo-e2e-upgrade", kubeconfigPath, scheme)
		DeferCleanup(func() {
			upgradeClusterProxy.Dispose(ctx)
		})

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
	})
	DescribeTable("",
		func(ctx context.Context, input *BMOIronicUpgradeInput) {
			namespace, cancelWatches = RunUpgradeTest(ctx, input, upgradeClusterProxy)
		},
		func(ctx context.Context, input *BMOIronicUpgradeInput) string {
			var upgradeFromKustomization string
			upgradeEntityName := input.UpgradeEntityName
			switch upgradeEntityName {
			case "bmo":
				upgradeFromKustomization = input.InitBMOKustomization
			case "ironic":
				upgradeFromKustomization = input.InitIronicKustomization
			}
			return fmt.Sprintf("Should upgrade %s from %s to latest version", input.UpgradeEntityName, upgradeFromKustomization)
		},
		entries,
	)

	AfterEach(func() {
		cleanup(ctx, upgradeClusterProxy, namespace, cancelWatches, e2eConfig.GetIntervals("default", "wait-namespace-deleted")...)
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
