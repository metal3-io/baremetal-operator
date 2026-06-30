//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"fmt"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	irsov1alpha1 "github.com/metal3-io/ironic-standalone-operator/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/bootstrap"
)

// setupUpgradeCluster creates or connects to a cluster for upgrade testing.
// It installs cert-manager if configured. Returns the cluster proxy and provider
// (provider may be nil if using an existing cluster).
func setupUpgradeCluster(ctx context.Context) (framework.ClusterProxy, bootstrap.ClusterProvider) {
	var kubeconfigPath string
	var upgradeClusterProvider bootstrap.ClusterProvider

	if e2eConfig.GetBoolVariable("UPGRADE_USE_EXISTING_CLUSTER") {
		kubeconfigPath = GetKubeconfigPath()
	} else {
		By("Creating a separate cluster for upgrade tests")
		upgradeClusterName := fmt.Sprintf("bmo-e2e-upgrade-%d", GinkgoParallelProcess())
		upgradeClusterProvider = bootstrap.CreateKindBootstrapClusterAndLoadImages(ctx, bootstrap.CreateKindBootstrapClusterAndLoadImagesInput{
			Name:   upgradeClusterName,
			Images: e2eConfig.GetClusterctlImages(),
		})
		Expect(upgradeClusterProvider).ToNot(BeNil(), "Failed to create a cluster")
		kubeconfigPath = upgradeClusterProvider.GetKubeconfigPath()

		// Configure provisioning network for dnsmasq to work properly.
		// This is only needed when Ironic is deployed (not for fixture tests).
		// TODO(lentzi90): This is a workaround. Fix it properly and get rid of it.
		if e2eConfig.HasVariable("UPGRADE_IRONIC_PROVISIONING_IP") {
			ConfigureProvisioningNetwork(ctx, upgradeClusterName, e2eConfig.GetVariable("UPGRADE_IRONIC_PROVISIONING_IP"))
		}
	}
	Expect(kubeconfigPath).To(BeAnExistingFile(), "Failed to get the kubeconfig file for the cluster")
	scheme := runtime.NewScheme()
	framework.TryAddDefaultSchemes(scheme)
	Expect(irsov1alpha1.AddToScheme(scheme)).To(Succeed())
	Expect(metal3api.AddToScheme(scheme)).To(Succeed())
	upgradeClusterProxy := framework.NewClusterProxy("bmo-e2e-upgrade", kubeconfigPath, scheme)

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

	return upgradeClusterProxy, upgradeClusterProvider
}

// cleanupUpgradeTest performs cleanup after an upgrade test.
func cleanupUpgradeTest(ctx context.Context, upgradeClusterProxy framework.ClusterProxy, upgradeClusterProvider bootstrap.ClusterProvider, namespace *corev1.Namespace, cancelWatches context.CancelFunc, testArtifactFolder string) {
	CollectSerialLogs(bmc.Name, testArtifactFolder)
	upgradeIronicIP := ""
	if e2eConfig.HasVariable("UPGRADE_IRONIC_PROVISIONING_IP") {
		upgradeIronicIP = e2eConfig.GetVariable("UPGRADE_IRONIC_PROVISIONING_IP")
	}
	DumpResources(ctx, e2eConfig, upgradeClusterProxy, testArtifactFolder, upgradeIronicIP)
	if !skipCleanup {
		if e2eConfig.GetBoolVariable("UPGRADE_USE_EXISTING_CLUSTER") {
			if namespace == nil {
				// Namespace is only set after CreateNamespaceAndWatchEvents succeeds.
				// If the test failed before that, there's nothing to clean up here.
				if cancelWatches != nil {
					cancelWatches()
				}
				if upgradeClusterProxy != nil {
					upgradeClusterProxy.Dispose(ctx)
				}
				return
			}
			// Trigger deletion of BMHs before deleting the namespace.
			// This way there should be no risk of BMO getting stuck trying to progress
			// and create HardwareDetails or similar, while the namespace is terminating.
			DeleteBmhsInNamespace(ctx, upgradeClusterProxy.GetClient(), namespace.Name)
			framework.DeleteNamespace(ctx, framework.DeleteNamespaceInput{
				Deleter: upgradeClusterProxy.GetClient(),
				Name:    namespace.Name,
			})
			WaitForNamespaceDeleted(ctx, WaitForNamespaceDeletedInput{
				Getter:    upgradeClusterProxy.GetClient(),
				Namespace: *namespace,
			}, e2eConfig.GetIntervals("default", "wait-namespace-deleted")...)
			// Try to clean up as best as we can.
			// Note that we only delete the "normal" BMO kustomization. There could be small
			// differences between this and the initial or upgrade kustomization, but this also
			// cleans up the namespace, which should take care of everything except CRDs
			// and cluster-scoped RBAC, including Ironic if it was deployed.
			// There is a theoretical risk that we leak cluster-scoped resources for the
			// next test here, if there are differences between the kustomizations.
			cleanupBaremetalOperatorSystem(ctx, upgradeClusterProxy, e2eConfig.GetVariable("BMO_KUSTOMIZATION"))
		} else {
			// We are using a kind cluster for the upgrade tests, so we just delete the cluster.
			upgradeClusterProvider.Dispose(ctx)
		}
		if cancelWatches != nil {
			cancelWatches()
		}
		if upgradeClusterProxy != nil {
			upgradeClusterProxy.Dispose(ctx)
		}
	}
}

// cleanupBaremetalOperatorSystem removes the kustomization from the cluster and waits for the
// baremetal-operator-system namespace to be deleted.
func cleanupBaremetalOperatorSystem(ctx context.Context, clusterProxy framework.ClusterProxy, kustomization string) {
	_ = BuildAndRemoveKustomization(ctx, kustomization, clusterProxy)
	// We need to ensure that the namespace actually gets deleted.
	WaitForNamespaceDeleted(ctx, WaitForNamespaceDeletedInput{
		Getter:    clusterProxy.GetClient(),
		Namespace: corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "baremetal-operator-system"}},
	}, e2eConfig.GetIntervals("default", "wait-namespace-deleted")...)
}
