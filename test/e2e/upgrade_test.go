package e2e

import (
	"fmt"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/bootstrap"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/patch"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
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

var _ = Describe("BMO Upgrade", Label("optional", "upgrade"), func() {
	var (
		specName               = "upgrade"
		secretName             = "bmc-credentials"
		namespace              *corev1.Namespace
		bmoIronicNamespace     = "baremetal-operator-system"
		upgradeClusterProvider bootstrap.ClusterProvider
		upgradeClusterProxy    framework.ClusterProxy
		bmh                    metal3api.BareMetalHost
	)
	BeforeEach(func() {
		var kubeconfigPath string

		if useExistingCluster {
			kubeconfigPath = os.Getenv("KUBECONFIG")
			if kubeconfigPath == "" {
				kubeconfigPath = os.Getenv("HOME") + "/.kube/config"
			}
		} else {
			By("Creating a separate cluster for upgrade tests")
			upgradeClusterProvider = bootstrap.CreateKindBootstrapClusterAndLoadImages(ctx, bootstrap.CreateKindBootstrapClusterAndLoadImagesInput{
				Name:   "bmo-e2e-upgrade",
				Images: e2eConfig.Images,
			})
			Expect(upgradeClusterProvider).ToNot(BeNil(), "Failed to create a cluster")
			kubeconfigPath = upgradeClusterProvider.GetKubeconfigPath()
		}
		Expect(kubeconfigPath).To(BeAnExistingFile(), "Failed to get the kubeconfig file for the cluster")
		scheme := runtime.NewScheme()
		framework.TryAddDefaultSchemes(scheme)
		metal3api.AddToScheme(scheme)
		upgradeClusterProxy = framework.NewClusterProxy("bmo-e2e-upgrade", kubeconfigPath, scheme)

		if e2eConfig.GetVariable("UPGRADE_DEPLOY_CERT_MANAGER") != "false" {
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

	It("Should upgrade BMO to latest version", func() {
		if e2eConfig.GetVariable("UPGRADE_DEPLOY_IRONIC") != "false" {
			// Install Ironic
			ironicKustomization := e2eConfig.GetVariable("IRONIC_KUSTOMIZATION")
			By("Installing Ironic on the upgrade cluster")
			err := BuildAndApplyKustomization(ctx, &BuildAndApplyKustomizationInput{
				Kustomization:       ironicKustomization,
				ClusterProxy:        upgradeClusterProxy,
				WaitForDeployment:   true,
				WatchDeploymentLogs: true,
				DeploymentName:      "ironic",
				DeploymentNamespace: bmoIronicNamespace,
				LogPath:             filepath.Join(artifactFolder, "logs", fmt.Sprintf("%s-%s", bmoIronicNamespace, specName)),
				WaitIntervals:       e2eConfig.GetIntervals("default", "wait-deployment"),
			})
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() {
				By("Removing Ironic on the upgrade cluster")
				BuildAndRemoveKustomization(ctx, ironicKustomization, upgradeClusterProxy)
			})
		}

		if e2eConfig.GetVariable("UPGRADE_DEPLOY_BMO") != "false" {
			bmoFromKustomization := e2eConfig.GetVariable("UPGRADE_BMO_KUSTOMIZATION_FROM")
			bmoFromKustomizationName := filepath.Base(bmoFromKustomization)
			By(fmt.Sprintf("Installing BMO from %s on the upgrade cluster", bmoFromKustomization))
			err := BuildAndApplyKustomization(ctx, &BuildAndApplyKustomizationInput{
				Kustomization:       bmoFromKustomization,
				ClusterProxy:        upgradeClusterProxy,
				WaitForDeployment:   true,
				WatchDeploymentLogs: true,
				DeploymentName:      "baremetal-operator-controller-manager",
				DeploymentNamespace: bmoIronicNamespace,
				LogPath:             filepath.Join(artifactFolder, "logs", fmt.Sprintf("%s-%s", bmoIronicNamespace, specName), fmt.Sprintf("bmo-%s", bmoFromKustomizationName)),
				WaitIntervals:       e2eConfig.GetIntervals("default", "wait-deployment"),
			})
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() {
				By(fmt.Sprintf("Removing BMO from %s on the upgrade cluster", bmoFromKustomization))
				BuildAndRemoveKustomization(ctx, bmoFromKustomization, upgradeClusterProxy)
			})
		}

		namespace, cancelWatches = framework.CreateNamespaceAndWatchEvents(ctx, framework.CreateNamespaceAndWatchEventsInput{
			Creator:   upgradeClusterProxy.GetClient(),
			ClientSet: upgradeClusterProxy.GetClientSet(),
			Name:      fmt.Sprintf("%s-%s", specName, util.RandomString(6)),
			LogFolder: artifactFolder,
		})
		By("Creating a secret with BMH credentials")
		bmcCredentialsData := map[string]string{
			"username": bmc.User,
			"password": bmc.Password,
		}
		CreateSecret(ctx, upgradeClusterProxy.GetClient(), namespace.Name, secretName, bmcCredentialsData)

		By("Creating a BMH with inspection disabled and hardware details added")
		bmh = metal3api.BareMetalHost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      specName,
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

		By("Upgrading BMO deployment")
		clientSet := upgradeClusterProxy.GetClientSet()
		bmoDeployName := "baremetal-operator-controller-manager"
		deploy, err := clientSet.AppsV1().Deployments(bmoIronicNamespace).Get(ctx, bmoDeployName, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		bmoKustomization := e2eConfig.GetVariable("BMO_KUSTOMIZATION")
		bmoKustomizationName := filepath.Base(bmoKustomization)
		err = BuildAndApplyKustomization(ctx, &BuildAndApplyKustomizationInput{
			Kustomization:       bmoKustomization,
			ClusterProxy:        upgradeClusterProxy,
			WaitForDeployment:   false,
			WatchDeploymentLogs: true,
			DeploymentName:      "baremetal-operator-controller-manager",
			DeploymentNamespace: bmoIronicNamespace,
			LogPath:             filepath.Join(artifactFolder, "logs", fmt.Sprintf("%s-%s", bmoIronicNamespace, specName), fmt.Sprintf("bmo-%s", bmoKustomizationName)),
			WaitIntervals:       e2eConfig.GetIntervals("default", "wait-deployment"),
		})
		DeferCleanup(func() {
			By("Removing BMO main e2e deployment")
			BuildAndRemoveKustomization(ctx, bmoKustomization, upgradeClusterProxy)
		})
		Expect(err).NotTo(HaveOccurred())
		By("Waiting for BMO update to rollout")
		Eventually(func() bool {
			return DeploymentRolledOut(ctx, upgradeClusterProxy, bmoDeployName, bmoIronicNamespace, deploy.Status.ObservedGeneration+1)
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
	})

	AfterEach(func() {
		cleanup(ctx, upgradeClusterProxy, namespace, cancelWatches, e2eConfig.GetIntervals("default", "wait-namespace-deleted")...)
		if !skipCleanup {
			if upgradeClusterProxy != nil {
				upgradeClusterProxy.Dispose(ctx)
			}
			if upgradeClusterProvider != nil {
				upgradeClusterProvider.Dispose(ctx)
			}
		}
	})

})
