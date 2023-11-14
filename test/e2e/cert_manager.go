package e2e

import (
	"context"
	"fmt"
	"io"
	"net/http"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cmapi "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	"github.com/pkg/errors"
)

func checkCertManagerAPI(clusterProxy framework.ClusterProxy) error {
	certManagerAPIVersion := "cert-manager.io/v1"
	clientset := clusterProxy.GetClientSet()
	_, err := clientset.Discovery().ServerResourcesForGroupVersion(certManagerAPIVersion)
	return err
}

func installCertManager(ctx context.Context, clusterProxy framework.ClusterProxy, cmVersion string) error {
	response, err := http.Get(fmt.Sprintf("https://github.com/cert-manager/cert-manager/releases/download/%s/cert-manager.yaml", cmVersion))
	if err != nil {
		return errors.Wrapf(err, "Error downloading cert-manager manifest")
	}
	defer response.Body.Close()
	manifests, err := io.ReadAll(response.Body)
	if err != nil {
		return errors.Wrapf(err, "Error reading downloaded cert-manager manifest")
	}
	err = clusterProxy.Apply(ctx, manifests)
	if err != nil {
		return errors.Wrapf(err, "Error installing cert-manager from downloaded manifest")
	}
	return nil
}

// checkCertManagerWebhook attempts to perform a dry-run create of a cert-manager
// Issuer and Certificate resources in order to verify that CRDs are installed and all the
// required webhooks are reachable by the K8S API server.
func checkCertManagerWebhook(ctx context.Context, clusterProxy framework.ClusterProxy) error {
	scheme := clusterProxy.GetScheme()
	const ns = "cert-manager"
	cmapi.AddToScheme(scheme)
	cl, err := client.New(clusterProxy.GetRESTConfig(), client.Options{
		Scheme: scheme,
	})
	if err != nil {
		return err
	}
	c := client.NewNamespacedClient(client.NewDryRunClient(cl), ns)
	issuer := &cmapi.Issuer{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cmapichecker",
		},
		Spec: cmapi.IssuerSpec{
			IssuerConfig: cmapi.IssuerConfig{
				SelfSigned: &cmapi.SelfSignedIssuer{},
			},
		},
	}
	if err = c.Create(ctx, issuer); err != nil {
		return errors.Wrapf(err, "cert-manager webhook not ready")
	}
	cert := &cmapi.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cmapichecker",
		},
		Spec: cmapi.CertificateSpec{
			DNSNames:   []string{"cmapichecker.example"},
			SecretName: "cmapichecker",
			IssuerRef: cmmeta.ObjectReference{
				Name: "cmapichecker",
			},
		},
	}

	if err = c.Create(ctx, cert); err != nil {
		return errors.Wrapf(err, "cert-manager webhook not ready")
	}
	return nil
}
