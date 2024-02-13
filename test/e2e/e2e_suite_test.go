package e2e

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/bootstrap"
	ctrl "sigs.k8s.io/controller-runtime"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
)

var (
	ctx = ctrl.SetupSignalHandler()
	// watchesCtx is used in log streaming to be able to get canceld via cancelWatches after ending the test suite.
	watchesCtx, cancelWatches = context.WithCancel(ctx)

	// configPath is the path to the e2e config file.
	configPath string

	// bmcConfigPath is the path to the file whose content is the list of bmcs used in the test.
	bmcConfigPath string

	// useExistingCluster instructs the test to use the current cluster instead of creating a new one (default discovery rules apply).
	useExistingCluster bool

	// e2eConfig to be used for this test, read from configPath.
	e2eConfig *Config

	// bmcs to be used for this test, read from bmcConfigPath.
	bmcs *[]BMC

	// artifactFolder is the folder to store e2e test artifacts.
	artifactFolder string

	// skipCleanup prevents cleanup of test resources e.g. for debug purposes.
	skipCleanup bool

	// clusterProxy allows to interact with the cluster to be used for the e2e tests.
	clusterProxy framework.ClusterProxy

	// clusterProvider manages provisioning of the cluster to be used for the e2e tests.
	// Please note that provisioning will be skipped if e2e.use-existing-cluster is provided.
	clusterProvider bootstrap.ClusterProvider

	// the BMC instance to use in a parallel test.
	bmc BMC
)

func init() {
	flag.StringVar(&configPath, "e2e.config", "", "path to the e2e config file")
	flag.StringVar(&bmcConfigPath, "e2e.bmcsConfig", "", "path to the bmcs config file")
	flag.StringVar(&artifactFolder, "e2e.artifacts-folder", "_artifacts", "folder where e2e test artifact should be stored")
	flag.BoolVar(&useExistingCluster, "e2e.use-existing-cluster", false, "if true, the test uses the current cluster instead of creating a new one (default discovery rules apply)")
	flag.BoolVar(&skipCleanup, "e2e.skip-resource-cleanup", false, "if true, the resource cleanup after tests will be skipped")
}

func TestE2e(t *testing.T) {
	g := NewWithT(t)
	ctrl.SetLogger(klog.Background())

	// ensure the artifacts folder exists
	g.Expect(os.MkdirAll(artifactFolder, 0755)).To(Succeed(), "Invalid test suite argument. Can't create e2e.artifacts-folder %q", artifactFolder)

	RegisterFailHandler(Fail)
	RunSpecs(t, "E2e Suite")
}

var _ = SynchronizedBeforeSuite(func() []byte {
	var kubeconfigPath string
	Expect(configPath).To(BeAnExistingFile(), "Invalid test suite argument. e2e.config should be an existing file.")
	e2eConfig = LoadE2EConfig(configPath)

	if useExistingCluster {
		kubeconfigPath = os.Getenv("KUBECONFIG")
		if kubeconfigPath == "" {
			kubeconfigPath = os.Getenv("HOME") + "/.kube/config"
		}
	} else {
		clusterProvider = bootstrap.CreateKindBootstrapClusterAndLoadImages(ctx, bootstrap.CreateKindBootstrapClusterAndLoadImagesInput{
			Name:   "bmo-e2e",
			Images: e2eConfig.Images,
		})
		Expect(clusterProvider).ToNot(BeNil(), "Failed to create a cluster")
		kubeconfigPath = clusterProvider.GetKubeconfigPath()
	}
	Expect(kubeconfigPath).To(BeAnExistingFile(), "Failed to get the kubeconfig file for the cluster")

	scheme := runtime.NewScheme()
	framework.TryAddDefaultSchemes(scheme)
	clusterProxy := framework.NewClusterProxy("bmo-e2e", kubeconfigPath, scheme)
	Expect(clusterProxy).ToNot(BeNil(), "Failed to get a cluster proxy")

	DeferCleanup(func() {
		clusterProxy.Dispose(ctx)
	})

	os.Setenv("KUBECONFIG", clusterProxy.GetKubeconfigPath())

	if e2eConfig.GetVariable("DEPLOY_CERT_MANAGER") != "false" {
		// Install cert-manager
		By("Installing cert-manager")
		err := checkCertManagerAPI(clusterProxy)
		if err != nil {
			cmVersion := e2eConfig.GetVariable("CERT_MANAGER_VERSION")
			err = installCertManager(ctx, clusterProxy, cmVersion)
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for cert-manager webhook")
			Eventually(func() error {
				return checkCertManagerWebhook(ctx, clusterProxy)
			}, e2eConfig.GetIntervals("default", "wait-available")...).Should(Succeed())
			err = checkCertManagerAPI(clusterProxy)
			Expect(err).NotTo(HaveOccurred())
		}
	}

	bmoIronicNamespace := "baremetal-operator-system"

	if e2eConfig.GetVariable("DEPLOY_IRONIC") != "false" {
		// Install Ironic
		By("Installing Ironic")
		err := BuildAndApplyKustomization(ctx, &BuildAndApplyKustomizationInput{
			Kustomization:       e2eConfig.GetVariable("IRONIC_KUSTOMIZATION"),
			ClusterProxy:        clusterProxy,
			WaitForDeployment:   true,
			WatchDeploymentLogs: true,
			DeploymentName:      "ironic",
			DeploymentNamespace: bmoIronicNamespace,
			LogPath:             filepath.Join(artifactFolder, "logs", bmoIronicNamespace),
			WaitIntervals:       e2eConfig.GetIntervals("default", "wait-deployment"),
		})
		Expect(err).NotTo(HaveOccurred())

	}

	if e2eConfig.GetVariable("DEPLOY_BMO") != "false" {
		// Install BMO
		By("Installing BMO")
		err := BuildAndApplyKustomization(ctx, &BuildAndApplyKustomizationInput{
			Kustomization:       e2eConfig.GetVariable("BMO_KUSTOMIZATION"),
			ClusterProxy:        clusterProxy,
			WaitForDeployment:   true,
			WatchDeploymentLogs: true,
			DeploymentName:      "baremetal-operator-controller-manager",
			DeploymentNamespace: bmoIronicNamespace,
			LogPath:             filepath.Join(artifactFolder, "logs", bmoIronicNamespace),
			WaitIntervals:       e2eConfig.GetIntervals("default", "wait-deployment"),
		})
		Expect(err).NotTo(HaveOccurred())
	}

	return []byte(strings.Join([]string{clusterProxy.GetKubeconfigPath()}, ","))
}, func(data []byte) {
	// Before each parallel node
	parts := strings.Split(string(data), ",")
	Expect(parts).To(HaveLen(1))

	kubeconfigPath := parts[0]
	scheme := runtime.NewScheme()
	framework.TryAddDefaultSchemes(scheme)
	metal3api.AddToScheme(scheme)

	e2eConfig = LoadE2EConfig(configPath)
	bmcs = LoadBMCConfig(bmcConfigPath)
	bmc = (*bmcs)[GinkgoParallelProcess()-1]
	clusterProxy = framework.NewClusterProxy("bmo-e2e", kubeconfigPath, scheme)
})

// Using a SynchronizedAfterSuite for controlling how to delete resources shared across ParallelNodes (~ginkgo threads).
// The kubernetes cluster is shared across all the tests, so it should be deleted only after all ParallelNodes completes.
// The artifact folder is preserved.
var _ = SynchronizedAfterSuite(func() {
	// After each ParallelNode.
}, func() {
	// After all ParallelNodes.

	cancelWatches()

	By("Tearing down the management cluster")
	if !skipCleanup {
		if clusterProxy != nil {
			clusterProxy.Dispose(ctx)
		}
		if clusterProvider != nil {
			clusterProvider.Dispose(ctx)
		}
	}
})
