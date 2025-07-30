//go:build e2e
// +build e2e

package e2e

import (
	"fmt"
	"io"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/cluster-api/test/framework"
)

var _ = Describe("Metrics Service", Label("required", "metrics-service"), func() {
	var (
		existingNamespace      = "baremetal-operator-system"
		serviceAccountName     = "baremetal-operator-controller-manager"
		metricsServiceName     = "baremetal-operator-controller-manager-metrics-service"
		metricsRoleBindingName = "baremetal-operator-metrics-binding"
	)

	BeforeEach(func() {
		go func() {
			defer GinkgoRecover()
			framework.WatchNamespaceEvents(ctx, framework.WatchNamespaceEventsInput{
				ClientSet: clusterProxy.GetClientSet(),
				Name:      "baremetal-operator-system",
				LogFolder: artifactFolder,
			})
		}()
	})
	It("should verify metrics are accessible and validate functionality", func() {
		client := clusterProxy.GetClient()
		clientSet := clusterProxy.GetClientSet()

		By("Creating a ClusterRoleBinding for the service account to allow access to metrics")
		metricsRoleBinding := &rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name: metricsRoleBindingName,
			},
			Subjects: []rbacv1.Subject{
				{
					Kind:      "ServiceAccount",
					Name:      serviceAccountName,
					Namespace: existingNamespace,
				},
			},
			RoleRef: rbacv1.RoleRef{
				Kind:     "ClusterRole",
				Name:     "baremetal-operator-metrics-reader",
				APIGroup: "rbac.authorization.k8s.io",
			},
		}
		err := client.Create(ctx, metricsRoleBinding)
		Expect(err).NotTo(HaveOccurred(), "Failed to create ClusterRoleBinding")

		By("Waiting for the metrics service to be available")
		Eventually(func() error {
			key := types.NamespacedName{Name: metricsServiceName, Namespace: existingNamespace}
			metricsService := &corev1.Service{}
			return client.Get(ctx, key, metricsService)
		}, "30s", "5s").Should(Succeed(), "Metrics service is not available")

		By("Creating a service account token to access the metrics endpoint")
		var token string
		var result *authenticationv1.TokenRequest
		Eventually(func(g Gomega) {
			result, err = clientSet.CoreV1().ServiceAccounts(existingNamespace).CreateToken(
				ctx,
				serviceAccountName,
				&authenticationv1.TokenRequest{},
				metav1.CreateOptions{},
			)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(result.Status.Token).NotTo(BeEmpty())
			token = result.Status.Token
		}).Should(Succeed())

		By("Waiting for the metrics endpoint to be ready")
		Eventually(func(g Gomega) {
			key := types.NamespacedName{
				Name:      metricsServiceName,
				Namespace: existingNamespace,
			}
			metricsService := &corev1.Service{}
			err = client.Get(ctx, key, metricsService)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(metricsService.Spec.Ports[0].Port).To(Equal(int32(8443)), "Metrics endpoint is not ready")
		}).Should(Succeed())

		By("Creating the curl-metrics pod to access the metrics endpoint")
		curlPod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "curl-metrics",
				Namespace: existingNamespace,
			},
			Spec: corev1.PodSpec{
				RestartPolicy: corev1.RestartPolicyNever,
				Containers: []corev1.Container{
					{
						Name:  "curl",
						Image: "curlimages/curl:7.87.0",
						Command: []string{
							"curl",
							"-v",
							"--tlsv1.3",
							"-k",
							"-H", "Authorization:Bearer " + token,
							fmt.Sprintf("https://%s.%s.svc.cluster.local:8443/metrics", metricsServiceName, existingNamespace),
						},
					},
				},
			},
		}
		err = client.Create(ctx, curlPod)
		Expect(err).NotTo(HaveOccurred(), "Failed to create curl-metrics pod")

		By("Waiting for the curl-metrics pod to complete")
		Eventually(func(g Gomega) {
			key := types.NamespacedName{Name: "curl-metrics", Namespace: existingNamespace}
			pod := &corev1.Pod{}
			err = client.Get(ctx, key, pod)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(pod.Status.Phase).To(Equal(corev1.PodSucceeded), "curl-metrics pod in wrong status")
		}, 5*time.Minute).Should(Succeed())

		By("Getting the metrics by checking curl-metrics logs")
		req := clientSet.CoreV1().Pods(existingNamespace).GetLogs("curl-metrics", &corev1.PodLogOptions{})
		logs, err := req.Stream(ctx)
		Expect(err).NotTo(HaveOccurred(), "Failed to get log stream")
		defer logs.Close()

		buf, err := io.ReadAll(logs)
		Expect(err).NotTo(HaveOccurred(), "Failed to read logs")
		metricsOutput := string(buf)

		Expect(metricsOutput).To(ContainSubstring(
			"controller_runtime_reconcile_total",
		), "Expected metrics not found in output")
	})
})
