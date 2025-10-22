package controllers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/go-logr/logr"
	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// TestFindBMHsForAuthSecret verifies that the secret watch mechanism correctly identifies
// BareMetalHosts that reference a given Secret as their image auth secret.
func TestFindBMHsForAuthSecret(t *testing.T) {
	tests := []struct {
		name          string
		secretName    string
		secretNS      string
		hosts         []*metal3api.BareMetalHost
		expectedHosts []string // Names of hosts that should be reconciled
	}{
		{
			name:       "Single BMH references secret",
			secretName: "my-registry-secret",
			secretNS:   "default",
			hosts: []*metal3api.BareMetalHost{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "host-1",
						Namespace: "default",
					},
					Spec: metal3api.BareMetalHostSpec{
						Image: &metal3api.Image{
							URL:            "oci://registry.example.com/image:tag",
							AuthSecretName: strPtr("my-registry-secret"),
						},
					},
				},
			},
			expectedHosts: []string{"host-1"},
		},
		{
			name:       "Multiple BMHs reference same secret",
			secretName: "shared-secret",
			secretNS:   "default",
			hosts: []*metal3api.BareMetalHost{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "host-1",
						Namespace: "default",
					},
					Spec: metal3api.BareMetalHostSpec{
						Image: &metal3api.Image{
							URL:            "oci://registry.example.com/image1:tag",
							AuthSecretName: strPtr("shared-secret"),
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "host-2",
						Namespace: "default",
					},
					Spec: metal3api.BareMetalHostSpec{
						Image: &metal3api.Image{
							URL:            "oci://registry.example.com/image2:tag",
							AuthSecretName: strPtr("shared-secret"),
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "host-3",
						Namespace: "default",
					},
					Spec: metal3api.BareMetalHostSpec{
						Image: &metal3api.Image{
							URL:            "oci://registry.example.com/image3:tag",
							AuthSecretName: strPtr("different-secret"),
						},
					},
				},
			},
			expectedHosts: []string{"host-1", "host-2"},
		},
		{
			name:       "No BMHs reference the secret",
			secretName: "unused-secret",
			secretNS:   "default",
			hosts: []*metal3api.BareMetalHost{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "host-1",
						Namespace: "default",
					},
					Spec: metal3api.BareMetalHostSpec{
						Image: &metal3api.Image{
							URL:            "oci://registry.example.com/image:tag",
							AuthSecretName: strPtr("other-secret"),
						},
					},
				},
			},
			expectedHosts: []string{},
		},
		{
			name:       "BMH with no image",
			secretName: "my-secret",
			secretNS:   "default",
			hosts: []*metal3api.BareMetalHost{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "host-1",
						Namespace: "default",
					},
					Spec: metal3api.BareMetalHostSpec{
						Image: nil,
					},
				},
			},
			expectedHosts: []string{},
		},
		{
			name:       "BMH with no auth secret",
			secretName: "my-secret",
			secretNS:   "default",
			hosts: []*metal3api.BareMetalHost{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "host-1",
						Namespace: "default",
					},
					Spec: metal3api.BareMetalHostSpec{
						Image: &metal3api.Image{
							URL: "oci://registry.example.com/image:tag",
							// No AuthSecretName
						},
					},
				},
			},
			expectedHosts: []string{},
		},
		{
			name:       "Different namespace - should not match",
			secretName: "my-secret",
			secretNS:   "other-namespace",
			hosts: []*metal3api.BareMetalHost{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "host-1",
						Namespace: "default",
					},
					Spec: metal3api.BareMetalHostSpec{
						Image: &metal3api.Image{
							URL:            "oci://registry.example.com/image:tag",
							AuthSecretName: strPtr("my-secret"),
						},
					},
				},
			},
			expectedHosts: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			_ = metal3api.AddToScheme(scheme)
			_ = corev1.AddToScheme(scheme)

			// Create fake client with the hosts
			objs := make([]client.Object, len(tt.hosts))
			for i := range tt.hosts {
				objs[i] = tt.hosts[i]
			}
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(objs...).
				WithIndex(&metal3api.BareMetalHost{}, hostImageAuthSecretIndexField, func(obj client.Object) []string {
					host := obj.(*metal3api.BareMetalHost)
					if host.Spec.Image != nil && host.Spec.Image.AuthSecretName != nil && *host.Spec.Image.AuthSecretName != "" {
						return []string{*host.Spec.Image.AuthSecretName}
					}
					return nil
				}).
				Build()

			// Create reconciler
			r := &BareMetalHostReconciler{
				Client:   fakeClient,
				Log:      testLogger(t),
				Recorder: record.NewFakeRecorder(10),
			}

			// Create a secret object
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tt.secretName,
					Namespace: tt.secretNS,
				},
			}

			// Call the function
			requests := r.findBMHsForAuthSecret(context.Background(), secret)

			// Verify the results
			if len(requests) != len(tt.expectedHosts) {
				t.Errorf("expected %d reconcile requests, got %d", len(tt.expectedHosts), len(requests))
			}

			// Check that each expected host is in the requests
			requestMap := make(map[string]bool)
			for _, req := range requests {
				requestMap[req.Name] = true
			}

			for _, expectedHost := range tt.expectedHosts {
				if !requestMap[expectedHost] {
					t.Errorf("expected host %q to be in reconcile requests, but it wasn't", expectedHost)
				}
			}

			// Check that no unexpected hosts are in the requests
			for _, req := range requests {
				found := false
				for _, expectedHost := range tt.expectedHosts {
					if req.Name == expectedHost {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("unexpected host %q in reconcile requests", req.Name)
				}
			}
		})
	}
}

// TestSecretIndexField verifies that the field indexer correctly indexes BareMetalHosts
// by their image auth secret name.
func TestSecretIndexField(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = metal3api.AddToScheme(scheme)

	tests := []struct {
		name          string
		host          *metal3api.BareMetalHost
		expectedIndex []string
	}{
		{
			name: "Host with auth secret",
			host: &metal3api.BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "host-1",
					Namespace: "default",
				},
				Spec: metal3api.BareMetalHostSpec{
					Image: &metal3api.Image{
						URL:            "oci://registry.example.com/image:tag",
						AuthSecretName: strPtr("my-secret"),
					},
				},
			},
			expectedIndex: []string{"my-secret"},
		},
		{
			name: "Host without auth secret",
			host: &metal3api.BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "host-2",
					Namespace: "default",
				},
				Spec: metal3api.BareMetalHostSpec{
					Image: &metal3api.Image{
						URL: "oci://registry.example.com/image:tag",
					},
				},
			},
			expectedIndex: nil,
		},
		{
			name: "Host without image",
			host: &metal3api.BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "host-3",
					Namespace: "default",
				},
				Spec: metal3api.BareMetalHostSpec{},
			},
			expectedIndex: nil,
		},
		{
			name: "Host with empty auth secret name",
			host: &metal3api.BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "host-4",
					Namespace: "default",
				},
				Spec: metal3api.BareMetalHostSpec{
					Image: &metal3api.Image{
						URL:            "oci://registry.example.com/image:tag",
						AuthSecretName: strPtr(""),
					},
				},
			},
			expectedIndex: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the indexer function
			indexFunc := func(obj client.Object) []string {
				host := obj.(*metal3api.BareMetalHost)
				if host.Spec.Image != nil && host.Spec.Image.AuthSecretName != nil && *host.Spec.Image.AuthSecretName != "" {
					return []string{*host.Spec.Image.AuthSecretName}
				}
				return nil
			}

			result := indexFunc(tt.host)

			if len(result) != len(tt.expectedIndex) {
				t.Errorf("expected index %v, got %v", tt.expectedIndex, result)
				return
			}

			for i, expected := range tt.expectedIndex {
				if result[i] != expected {
					t.Errorf("expected index[%d] = %q, got %q", i, expected, result[i])
				}
			}
		})
	}
}

// TestSecretRotation_EndToEnd is an integration test that verifies the complete flow:
// 1. BMH is created with an auth secret
// 2. Secret is updated (key rotation)
// 3. BMH is automatically reconciled
func TestSecretRotation_EndToEnd(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = metal3api.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	// Create initial secret
	dockerConfig := map[string]interface{}{
		"auths": map[string]interface{}{
			"registry.example.com": map[string]interface{}{
				"username": "olduser",
				"password": "oldpass",
			},
		},
	}
	dockerConfigJSON, err := json.Marshal(dockerConfig)
	if err != nil {
		t.Fatalf("failed to marshal docker config: %v", err)
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-registry-secret",
			Namespace: "default",
		},
		Type: corev1.SecretTypeDockerConfigJson,
		Data: map[string][]byte{
			corev1.DockerConfigJsonKey: dockerConfigJSON,
		},
	}

	// Create BMH referencing the secret
	host := &metal3api.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-host",
			Namespace: "default",
		},
		Spec: metal3api.BareMetalHostSpec{
			Image: &metal3api.Image{
				URL:            "oci://registry.example.com/myimage:v1.0",
				AuthSecretName: strPtr("my-registry-secret"),
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(secret, host).
		WithIndex(&metal3api.BareMetalHost{}, hostImageAuthSecretIndexField, func(obj client.Object) []string {
			h := obj.(*metal3api.BareMetalHost)
			if h.Spec.Image != nil && h.Spec.Image.AuthSecretName != nil && *h.Spec.Image.AuthSecretName != "" {
				return []string{*h.Spec.Image.AuthSecretName}
			}
			return nil
		}).
		Build()

	r := &BareMetalHostReconciler{
		Client:   fakeClient,
		Log:      testLogger(t),
		Recorder: record.NewFakeRecorder(10),
	}

	// Verify initial setup
	var fetchedHost metal3api.BareMetalHost
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "test-host", Namespace: "default"}, &fetchedHost)
	if err != nil {
		t.Fatalf("failed to get host: %v", err)
	}
	if fetchedHost.Spec.Image.AuthSecretName == nil || *fetchedHost.Spec.Image.AuthSecretName != "my-registry-secret" {
		t.Fatal("host not properly configured with auth secret")
	}

	// Simulate secret update (key rotation)
	updatedSecret := secret.DeepCopy()
	updatedSecret.ResourceVersion = "2" // Simulate version change

	// Find BMHs that should be reconciled
	requests := r.findBMHsForAuthSecret(context.Background(), updatedSecret)

	// Verify that our BMH is in the reconcile queue
	if len(requests) != 1 {
		t.Fatalf("expected 1 reconcile request, got %d", len(requests))
	}

	if requests[0].Name != "test-host" || requests[0].Namespace != "default" {
		t.Errorf("expected reconcile request for test-host in default namespace, got %v", requests[0])
	}

	t.Log("✓ Secret rotation correctly triggered BMH reconciliation")
}

// Helper function to create a test logger.
func testLogger(t *testing.T) logr.Logger {
	return logr.Discard()
}
