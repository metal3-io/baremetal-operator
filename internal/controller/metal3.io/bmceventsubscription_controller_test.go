package controllers

import (
	"testing"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/fixture"
	"github.com/metal3-io/baremetal-operator/pkg/utils"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func newBMCTestReconcilerWithFixture(t *testing.T, fix *fixture.Fixture, initObjs ...runtime.Object) *BMCEventSubscriptionReconciler {
	t.Helper()
	clientBuilder := fakeclient.NewClientBuilder().WithRuntimeObjects(initObjs...)
	for _, v := range initObjs {
		object, ok := v.(client.Object)
		require.True(t, ok, "failed to cast object to client.Object")
		clientBuilder = clientBuilder.WithStatusSubresource(object)
	}
	c := clientBuilder.Build()
	// Add a default secret that can be used by most subscriptions.
	bmcSecret := newBMCCredsSecret(defaultSecretName, "User", "Pass")
	err := c.Create(t.Context(), bmcSecret)
	if err != nil {
		return nil
	}
	return &BMCEventSubscriptionReconciler{
		Client:             c,
		ProvisionerFactory: fix,
		Log:                ctrl.Log.WithName("controllers").WithName("BMCEventSubscription"),
		APIReader:          c,
	}
}

type BMCDoneFunc func(subscription *metal3api.BMCEventSubscription, result reconcile.Result) bool

func newBMCTestReconciler(t *testing.T, initObjs ...runtime.Object) *BMCEventSubscriptionReconciler {
	t.Helper()
	fix := fixture.Fixture{}
	return newBMCTestReconcilerWithFixture(t, &fix, initObjs...)
}

func newBMCRequest(subscription *metal3api.BMCEventSubscription) ctrl.Request {
	namespacedName := types.NamespacedName{
		Namespace: subscription.ObjectMeta.Namespace,
		Name:      subscription.ObjectMeta.Name,
	}
	return ctrl.Request{NamespacedName: namespacedName}
}

func newSubscription(name string, spec *metal3api.BMCEventSubscriptionSpec) *metal3api.BMCEventSubscription {
	return &metal3api.BMCEventSubscription{
		TypeMeta: metav1.TypeMeta{
			Kind:       "BareMetalHost",
			APIVersion: "metal3.io/v1alpha1",
		}, ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: *spec,
	}
}

func newDefaultNamedSubscription(t *testing.T, name string) *metal3api.BMCEventSubscription {
	t.Helper()
	spec := &metal3api.BMCEventSubscriptionSpec{
		HostName:    t.Name(),
		Destination: "user destination",
		Context:     "user context",
		HTTPHeadersRef: &corev1.SecretReference{
			Name:      defaultSecretName,
			Namespace: namespace,
		},
	}
	t.Logf("newNamedSubscription(%s)", name)
	subscription := newSubscription(name, spec)
	return subscription
}

func newDefaultSubscription(t *testing.T) *metal3api.BMCEventSubscription {
	t.Helper()
	return newDefaultNamedSubscription(t, t.Name())
}

func HostWithProvisioningID(t *testing.T, host *metal3api.BareMetalHost) *metal3api.BareMetalHost {
	t.Helper()
	host.Status.Provisioning.ID = "made-up-id"
	return host
}

func TestBMCAddFinalizers(t *testing.T) {
	host := newDefaultHost(t)
	subscription := newDefaultSubscription(t)
	r := newBMCTestReconciler(t, subscription, host)
	err := r.addFinalizer(t.Context(), subscription)
	if err != nil {
		t.Error(err)
	}
	t.Logf("subscription finalizers: %v", subscription.Finalizers)
	if !utils.StringInList(subscription.Finalizers, metal3api.BMCEventSubscriptionFinalizer) {
		t.Error("Expected finalizers to be added")
	}
}

func TestBMCGetProvisioner(t *testing.T) {
	host := newDefaultHost(t)
	subscription := newDefaultSubscription(t)
	request := newBMCRequest(subscription)
	r := newBMCTestReconciler(t, subscription, host)
	for _, tc := range []struct {
		Scenario string
		Host     *metal3api.BareMetalHost
		Expected bool
	}{
		{
			Scenario: "No provisioning id is provided",
			Host:     host,
			Expected: true,
		},
		{
			Scenario: "Provisioning id is provided",
			Host:     HostWithProvisioningID(t, host),
			Expected: true,
		},
	} {
		t.Run(tc.Scenario, func(t *testing.T) {
			prov, actual, err := r.getProvisioner(t.Context(), request, tc.Host)
			if err != nil {
				t.Error(err)
			}
			t.Log("Provisioner Details:", prov)
			if tc.Expected && !actual {
				t.Error("Expected a ready provisioner")
			}
		})
	}
}

func TestGetHTTPHeaders(t *testing.T) {
	// NOTE: This subscription references the defaultSecretName for http headers.
	// The secret is automatically created by newBMCTestReconciler.
	host := newDefaultHost(t)
	subscription := newDefaultSubscription(t)
	r := newBMCTestReconciler(t, subscription, host)

	for _, tc := range []struct {
		Scenario      string
		Subscription  *metal3api.BMCEventSubscription
		Secret        *corev1.Secret
		ExpectedError bool
	}{
		{
			Scenario:     "Secret exists and has some content",
			Subscription: subscription,
			// Already created by newBMCTestReconciler.
			Secret:        nil,
			ExpectedError: false,
		},
		{
			Scenario: "Secret does not exist",
			Subscription: &metal3api.BMCEventSubscription{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-subscription",
					Namespace: namespace,
				},
				Spec: metal3api.BMCEventSubscriptionSpec{
					HostName: host.Name,
					HTTPHeadersRef: &corev1.SecretReference{
						Name:      "non-existent-secret",
						Namespace: namespace,
					},
				},
			},
			Secret:        nil,
			ExpectedError: true,
		},
		{
			Scenario: "Secret in wrong namespace",
			Subscription: &metal3api.BMCEventSubscription{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-subscription",
					Namespace: namespace,
				},
				Spec: metal3api.BMCEventSubscriptionSpec{
					HostName: host.Name,
					HTTPHeadersRef: &corev1.SecretReference{
						Name:      "test",
						Namespace: "separate-namespace",
					},
				},
			},
			Secret:        nil,
			ExpectedError: true,
		},
	} {
		t.Run(tc.Scenario, func(t *testing.T) {
			if tc.Secret != nil {
				err := r.Create(t.Context(), tc.Secret)
				if err != nil {
					t.Fatal(err)
				}
			}

			headers, err := r.getHTTPHeaders(t.Context(), *tc.Subscription)
			if tc.ExpectedError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tc.ExpectedError {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				}
				if len(headers) == 0 {
					t.Error("Expected headers but got none")
				}
			}
		})
	}
}
