package bmc

import (
	goctx "context"
	"encoding/base64"
	"testing"

	metalkubeapis "github.com/metalkube/baremetal-operator/pkg/apis"
	metalkubev1alpha1 "github.com/metalkube/baremetal-operator/pkg/apis/metalkube/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

const (
	namespace         string = "test-namespace"
	defaultSecretName string = "bmc-creds-valid"
)

func init() {
	logf.SetLogger(logf.ZapLogger(true))
	// Register our package types with the global scheme
	metalkubeapis.AddToScheme(scheme.Scheme)
}

func newSecret(name, username, password string) *corev1.Secret {
	data := make(map[string][]byte)
	data["username"] = []byte(base64.StdEncoding.EncodeToString([]byte(username)))
	data["password"] = []byte(base64.StdEncoding.EncodeToString([]byte(password)))

	secret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			ResourceVersion: "1",
		},
		Data: data,
	}

	return secret
}

func newHost(name string, spec *metalkubev1alpha1.BareMetalHostSpec) *metalkubev1alpha1.BareMetalHost {
	return &metalkubev1alpha1.BareMetalHost{
		TypeMeta: metav1.TypeMeta{
			Kind:       "BareMetalHost",
			APIVersion: "metalkube.org/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: *spec,
	}
}

func newDefaultHost(t *testing.T) *metalkubev1alpha1.BareMetalHost {
	spec := &metalkubev1alpha1.BareMetalHostSpec{
		BMC: metalkubev1alpha1.BMCDetails{
			Address:         "ipmi://192.168.122.1:6233",
			CredentialsName: defaultSecretName,
		},
	}
	t.Logf("newDefaultHost(%s)", t.Name())
	return newHost(t.Name(), spec)
}
func TestValidCredentials(t *testing.T) {

	host := newDefaultHost(t)

	c := fakeclient.NewFakeClient()

	// Create a BMC secret with proper username and password
	c.Create(goctx.TODO(), newSecret(defaultSecretName, "User", "Pass"))

	// Create and return a BMC Credentials object
	_, err := NewCredentials(c, host)

	if err != nil {
		t.Fatalf("unexpected failure when building valid credentials object: %q", err)
	}

}

func TestMissingUser(t *testing.T) {

	host := newDefaultHost(t)

	c := fakeclient.NewFakeClient()

	// Create a BMC secret with an empty user value
	c.Create(goctx.TODO(), newSecret(defaultSecretName, "", "Pass"))

	// Create and return a BMC Credentials object
	_, err := NewCredentials(c, host)

	if err == nil {
		t.Fatalf("unexpected success when building invalid credentials object: %q", err)
	}

	if err, ok := err.(*Error); ok {
		if err.Err != ErrMissingUsername {
			t.Fatalf("got unexpected reason for invalid creds: %q", err)
		}
	}
}

func TestMissingPassword(t *testing.T) {

	host := newDefaultHost(t)

	c := fakeclient.NewFakeClient()

	// Create a BMC secret with an empty password value
	c.Create(goctx.TODO(), newSecret(defaultSecretName, "User", ""))

	// Create and return a BMC Credentials object
	_, err := NewCredentials(c, host)

	if err == nil {
		t.Fatalf("unexpected success when building invalid credentials object: %q", err)
	}

	if err, ok := err.(*Error); ok {
		if err.Err != ErrMissingPassword {
			t.Fatalf("got unexpected reason for invalid creds: %q", err)
		}
	}

}
