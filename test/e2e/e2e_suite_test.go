//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/bootstrap"
	ctrl "sigs.k8s.io/controller-runtime"
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
	Expect(configPath).To(BeAnExistingFile(), "Invalid test suite argument. e2e.config should be an existing file.")
	e2eConfig = LoadE2EConfig(configPath)
	RunSpecs(t, "E2e Suite")
}

const namespace = "baremetal-operator-system"
const serviceAccountName = "baremetal-operator-controller-manager"
const metricsServiceName = "baremetal-operator-controller-manager-metrics-service"
const metricsRoleBindingName = "baremetal-operator-metrics-binding"

// serviceAccountToken returns a token for the specified service account in the given namespace.
// It uses the Kubernetes TokenRequest API to generate a token by directly sending a request
// and parsing the resulting token from the API response.
func serviceAccountToken() (string, error) {
	const tokenRequestRawString = `{
		"apiVersion": "authentication.k8s.io/v1",
		"kind": "TokenRequest"
	}`

	// Temporary file to store the token request
	secretName := fmt.Sprintf("%s-token-request", serviceAccountName)
	tokenRequestFile := filepath.Join("/tmp", secretName) //nolint: gocritic
	err := os.WriteFile(tokenRequestFile, []byte(tokenRequestRawString), os.FileMode(0o644))
	if err != nil {
		return "", err
	}

	var out string
	verifyTokenCreation := func(g Gomega) {
		// Execute kubectl command to create the token
		cmd := exec.Command("kubectl", "create", "--raw", fmt.Sprintf(
			"/api/v1/namespaces/%s/serviceaccounts/%s/token",
			namespace,
			serviceAccountName,
		), "-f", tokenRequestFile)

		output, err := cmd.CombinedOutput()
		g.Expect(err).NotTo(HaveOccurred())

		// Parse the JSON output to extract the token
		var token tokenRequest
		err = json.Unmarshal(output, &token)
		g.Expect(err).NotTo(HaveOccurred())

		out = token.Status.Token
	}
	Eventually(verifyTokenCreation).Should(Succeed())

	return out, err
}

// tokenRequest is a simplified representation of the Kubernetes TokenRequest API response,
// containing only the token field that we need to extract.
type tokenRequest struct {
	Status struct {
		Token string `json:"token"`
	} `json:"status"`
}

// getMetricsOutput retrieves and returns the logs from the curl pod used to access the metrics endpoint.
func getMetricsOutput() string {
	By("getting the curl-metrics logs")
	cmd := exec.Command("kubectl", "logs", "curl-metrics", "-n", namespace)
	metricsOutput, err := cmd.CombinedOutput()
	Expect(err).NotTo(HaveOccurred(), "Failed to retrieve logs from curl pod")
	return string(metricsOutput)
}

var _ = SynchronizedBeforeSuite(func() []byte {
	var kubeconfigPath string

	if useExistingCluster {
		kubeconfigPath = GetKubeconfigPath()
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

	if e2eConfig.GetBoolVariable("DEPLOY_CERT_MANAGER") {
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

	if e2eConfig.GetBoolVariable("DEPLOY_IRONIC") {
		// Install Ironic
		By("Installing Ironic")
		err := FlakeAttempt(2, func() error {
			return BuildAndApplyKustomization(ctx, &BuildAndApplyKustomizationInput{
				Kustomization:       e2eConfig.GetVariable("IRONIC_KUSTOMIZATION"),
				ClusterProxy:        clusterProxy,
				WaitForDeployment:   true,
				WatchDeploymentLogs: true,
				DeploymentName:      "ironic",
				DeploymentNamespace: bmoIronicNamespace,
				LogPath:             filepath.Join(artifactFolder, "logs", bmoIronicNamespace),
				WaitIntervals:       e2eConfig.GetIntervals("default", "wait-deployment"),
			})
		})
		Expect(err).NotTo(HaveOccurred())
	}

	if e2eConfig.GetBoolVariable("DEPLOY_BMO") {
		// Install BMO
		By("Installing BMO")
		err := FlakeAttempt(2, func() error {
			return BuildAndApplyKustomization(ctx, &BuildAndApplyKustomizationInput{
				Kustomization:       e2eConfig.GetVariable("BMO_KUSTOMIZATION"),
				ClusterProxy:        clusterProxy,
				WaitForDeployment:   true,
				WatchDeploymentLogs: true,
				DeploymentName:      "baremetal-operator-controller-manager",
				DeploymentNamespace: bmoIronicNamespace,
				LogPath:             filepath.Join(artifactFolder, "logs", bmoIronicNamespace),
				WaitIntervals:       e2eConfig.GetIntervals("default", "wait-deployment"),
			})
		})
		Expect(err).NotTo(HaveOccurred())

		// Metrics test start
		By("creating a ClusterRoleBinding for the service account to allow access to metrics")
		cmd := exec.Command("kubectl", "create", "clusterrolebinding", metricsRoleBindingName,
			"--clusterrole=baremetal-operator-metrics-reader",
			fmt.Sprintf("--serviceaccount=%s:%s", namespace, serviceAccountName),
		)
		_, err = cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "Failed to create ClusterRoleBinding")

		By("validating that the metrics service is available")
		Eventually(func() error {
			cmd := exec.Command("kubectl", "get", "service", metricsServiceName, "-n", namespace)
			output, err := cmd.CombinedOutput()
			if err != nil {
				fmt.Printf("Service check output: %s\n", string(output))
				return err
			}
			return nil
		}, "30s", "5s").Should(Succeed(), "Metrics service is not available")

		By("getting the service account token")
		token, err := serviceAccountToken()
		Expect(err).NotTo(HaveOccurred())
		Expect(token).NotTo(BeEmpty())

		By("waiting for the metrics endpoint to be ready")
		verifyMetricsEndpointReady := func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "endpoints", metricsServiceName, "-n", namespace)
			output, err := cmd.CombinedOutput()
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(output).To(ContainSubstring("8443"), "Metrics endpoint is not ready")
		}
		Eventually(verifyMetricsEndpointReady).Should(Succeed())

		By("creating the curl-metrics pod to access the metrics endpoint")
		cmd = exec.Command("kubectl", "run", "curl-metrics", "--restart=Never",
			"--namespace", namespace,
			"--image=curlimages/curl:7.87.0",
			"--command",
			"--", "curl", "-v", "--tlsv1.3", "-k", "-H", fmt.Sprintf("Authorization:Bearer %s", token),
			fmt.Sprintf("https://%s.%s.svc.cluster.local:8443/metrics", metricsServiceName, namespace))
		_, err = cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "Failed to create curl-metrics pod")

		By("waiting for the curl-metrics pod to complete.")
		verifyCurlUp := func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "pods", "curl-metrics",
				"-o", "jsonpath={.status.phase}",
				"-n", namespace)
			output, err := cmd.CombinedOutput()
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(string(output)).To(Equal("Succeeded"), "curl pod in wrong status")
		}
		Eventually(verifyCurlUp, 5*time.Minute).Should(Succeed())

		By("getting the metrics by checking curl-metrics logs")
		metricsOutput := getMetricsOutput()
		Expect(metricsOutput).To(ContainSubstring(
			"controller_runtime_reconcile_total",
		))
		// Metrics test end

	}

	return []byte(strings.Join([]string{clusterProxy.GetKubeconfigPath()}, ","))
}, func(data []byte) {
	// Before each parallel node
	parts := strings.Split(string(data), ",")
	Expect(parts).To(HaveLen(1))

	kubeconfigPath := parts[0]
	scheme := runtime.NewScheme()
	framework.TryAddDefaultSchemes(scheme)
	err := metal3api.AddToScheme(scheme)
	Expect(err).NotTo(HaveOccurred())
	e2eConfig = LoadE2EConfig(configPath)
	bmcs, err := LoadBMCConfig(bmcConfigPath)
	Expect(err).ToNot(HaveOccurred(), "Failed to read the bmcs config file")
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
