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
	v1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	// useExistingCluster instructs the test to use the current cluster instead of creating a new one (default discovery rules apply).
	useExistingCluster bool

	// e2eConfig to be used for this test, read from configPath.
	e2eConfig *Config

	// artifactFolder is the folder to store e2e test artifacts.
	artifactFolder string

	// skipCleanup prevents cleanup of test resources e.g. for debug purposes.
	skipCleanup bool

	// clusterProxy allows to interact with the cluster to be used for the e2e tests.
	clusterProxy framework.ClusterProxy

	// clusterProvider manages provisioning of the cluster to be used for the e2e tests.
	// Please note that provisioning will be skipped if e2e.use-existing-cluster is provided.
	clusterProvider bootstrap.ClusterProvider
)

func init() {
	flag.StringVar(&configPath, "e2e.config", "", "path to the e2e config file")
	flag.StringVar(&artifactFolder, "e2e.artifacts-folder", "_artifacts", "folder where e2e test artifact should be stored")
	flag.BoolVar(&useExistingCluster, "e2e.use-existing-cluster", false, "if true, the test uses the current cluster instead of creating a new one (default discovery rules apply)")
	flag.BoolVar(&skipCleanup, "e2e.skip-resource-cleanup", false, "if true, the resource cleanup after tests will be skipped")
}

func TestE2e(t *testing.T) {
	g := NewWithT(t)
	ctrl.SetLogger(klog.Background())

	// ensure the artifacts folder exists
	g.Expect(os.MkdirAll(artifactFolder, 0755)).To(Succeed(), "Invalid test suite argument. Can't create e2e.artifacts-folder %q", artifactFolder) //nolint:gosec

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

	if e2eConfig.GetVariable("DEPLOY_IRONIC") != "false" {
		// Install Ironic
		By("Installing Ironic")
		kustomization := e2eConfig.GetVariable("IRONIC_KUSTOMIZATION")
		manifest, err := buildKustomizeManifest(kustomization)
		Expect(err).NotTo(HaveOccurred())
		err = clusterProxy.Apply(ctx, manifest)
		Expect(err).NotTo(HaveOccurred())

		ironicDeployment := &v1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ironic",
				Namespace: "baremetal-operator-system",
			},
		}
		// Wait for it to become available
		framework.WaitForDeploymentsAvailable(ctx, framework.WaitForDeploymentsAvailableInput{
			Getter:     clusterProxy.GetClient(),
			Deployment: ironicDeployment,
		}, e2eConfig.GetIntervals("ironic", "wait-deployment")...)
		// Set up log watcher
		framework.WatchDeploymentLogsByName(ctx, framework.WatchDeploymentLogsByNameInput{
			GetLister:  clusterProxy.GetClient(),
			Cache:      clusterProxy.GetCache(ctx),
			ClientSet:  clusterProxy.GetClientSet(),
			Deployment: ironicDeployment,
			LogPath:    filepath.Join(artifactFolder, "logs", ironicDeployment.GetNamespace()),
		})
	}

	if e2eConfig.GetVariable("DEPLOY_BMO") != "false" {
		// Install BMO
		By("Installing BMO")
		kustomization := e2eConfig.GetVariable("BMO_KUSTOMIZATION")
		manifest, err := buildKustomizeManifest(kustomization)
		Expect(err).NotTo(HaveOccurred())
		err = clusterProxy.Apply(ctx, manifest)
		Expect(err).NotTo(HaveOccurred())

		bmoDeployment := &v1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "baremetal-operator-controller-manager",
				Namespace: "baremetal-operator-system",
			},
		}
		// Wait for it to become available
		framework.WaitForDeploymentsAvailable(ctx, framework.WaitForDeploymentsAvailableInput{
			Getter:     clusterProxy.GetClient(),
			Deployment: bmoDeployment,
		}, e2eConfig.GetIntervals("default", "wait-deployment")...)
		// Set up log watcher
		framework.WatchDeploymentLogsByName(ctx, framework.WatchDeploymentLogsByNameInput{
			GetLister:  clusterProxy.GetClient(),
			Cache:      clusterProxy.GetCache(ctx),
			ClientSet:  clusterProxy.GetClientSet(),
			Deployment: bmoDeployment,
			LogPath:    filepath.Join(artifactFolder, "logs", bmoDeployment.GetNamespace()),
		})
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
