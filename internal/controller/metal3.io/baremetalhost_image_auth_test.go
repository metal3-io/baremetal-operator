package controllers

import (
	"encoding/json"
	"testing"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/imageauthvalidator"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// TestGetImageAuthSecret_ConditionsSet verifies that status conditions are properly
// set by the controller for various image auth scenarios.
func TestGetImageAuthSecret_ConditionsSet(t *testing.T) {
	tests := []struct {
		name                         string
		image                        *metal3api.Image
		secret                       *corev1.Secret
		expectedImageAuthValid       metav1.ConditionStatus
		expectedImageAuthValidReason string
		expectedImageAuthInUse       metav1.ConditionStatus
		expectedImageAuthInUseReason string
	}{
		{
			name: "No OCI image - conditions show not required",
			image: &metal3api.Image{
				URL: "http://example.com/image.iso",
			},
			expectedImageAuthValid:       metav1.ConditionFalse,
			expectedImageAuthValidReason: imageauthvalidator.ReasonNotRequired,
			expectedImageAuthInUse:       metav1.ConditionFalse,
			expectedImageAuthInUseReason: imageauthvalidator.ReasonNoOCIImage,
		},
		{
			name: "OCI image without auth secret - not required",
			image: &metal3api.Image{
				URL: "oci://registry.example.com/repo/image:tag",
			},
			expectedImageAuthValid:       metav1.ConditionFalse,
			expectedImageAuthValidReason: imageauthvalidator.ReasonNotRequired,
			expectedImageAuthInUse:       metav1.ConditionFalse,
			expectedImageAuthInUseReason: imageauthvalidator.ReasonNoOCIImage,
		},
		{
			name: "OCI image with valid auth secret - credentials injected",
			image: &metal3api.Image{
				URL:            "oci://registry.example.com/repo/image:tag",
				AuthSecretName: strPtr("test-secret"),
			},
			secret:                       createValidDockerConfigSecret("test-secret", "registry.example.com"),
			expectedImageAuthValid:       metav1.ConditionTrue,
			expectedImageAuthValidReason: imageauthvalidator.ReasonValid,
			expectedImageAuthInUse:       metav1.ConditionTrue,
			expectedImageAuthInUseReason: imageauthvalidator.ReasonCredentialsInjected,
		},
		{
			name: "OCI image with missing secret - secret not found",
			image: &metal3api.Image{
				URL:            "oci://registry.example.com/repo/image:tag",
				AuthSecretName: strPtr("missing-secret"),
			},
			expectedImageAuthValid:       metav1.ConditionFalse,
			expectedImageAuthValidReason: imageauthvalidator.ReasonSecretNotFound,
			expectedImageAuthInUse:       metav1.ConditionFalse,
			expectedImageAuthInUseReason: imageauthvalidator.ReasonNoOCIImage,
		},
		{
			name: "OCI image with wrong secret type - wrong type error",
			image: &metal3api.Image{
				URL:            "oci://registry.example.com/repo/image:tag",
				AuthSecretName: strPtr("wrong-type-secret"),
			},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "wrong-type-secret",
					Namespace: "default",
				},
				Type: corev1.SecretTypeOpaque,
				Data: map[string][]byte{
					"username": []byte("user"),
					"password": []byte("pass"),
				},
			},
			expectedImageAuthValid:       metav1.ConditionFalse,
			expectedImageAuthValidReason: imageauthvalidator.ReasonWrongType,
			expectedImageAuthInUse:       metav1.ConditionFalse,
			expectedImageAuthInUseReason: imageauthvalidator.ReasonNoOCIImage,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			_ = metal3api.AddToScheme(scheme)
			_ = corev1.AddToScheme(scheme)

			// Create host
			host := &metal3api.BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-host",
					Namespace: "default",
				},
				Spec: metal3api.BareMetalHostSpec{
					Image: tt.image,
				},
			}

			// Build client with objects
			objs := []client.Object{host}
			if tt.secret != nil {
				objs = append(objs, tt.secret)
			}
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()

			// Create reconciler
			recorder := record.NewFakeRecorder(10)
			r := &BareMetalHostReconciler{
				Client:   c,
				Recorder: recorder,
			}

			// Call getImageAuthSecret
			ctx := t.Context()
			request := ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      host.Name,
					Namespace: host.Namespace,
				},
			}
			_, _ = r.getImageAuthSecret(ctx, request, host, tt.image)

			// Check conditions were set
			imageAuthValidCond := findCondition(host.Status.Conditions, imageauthvalidator.ConditionImageAuthValid)
			if imageAuthValidCond == nil {
				t.Fatalf("ImageAuthValid condition not set")
			}
			if imageAuthValidCond.Status != tt.expectedImageAuthValid {
				t.Errorf("ImageAuthValid status: expected %s, got %s",
					tt.expectedImageAuthValid, imageAuthValidCond.Status)
			}
			if imageAuthValidCond.Reason != tt.expectedImageAuthValidReason {
				t.Errorf("ImageAuthValid reason: expected %s, got %s",
					tt.expectedImageAuthValidReason, imageAuthValidCond.Reason)
			}

			imageAuthInUseCond := findCondition(host.Status.Conditions, imageauthvalidator.ConditionImageAuthInUse)
			if imageAuthInUseCond == nil {
				t.Fatalf("ImageAuthInUse condition not set")
			}
			if imageAuthInUseCond.Status != tt.expectedImageAuthInUse {
				t.Errorf("ImageAuthInUse status: expected %s, got %s",
					tt.expectedImageAuthInUse, imageAuthInUseCond.Status)
			}
			if imageAuthInUseCond.Reason != tt.expectedImageAuthInUseReason {
				t.Errorf("ImageAuthInUse reason: expected %s, got %s",
					tt.expectedImageAuthInUseReason, imageAuthInUseCond.Reason)
			}
		})
	}
}

// Helper: create a valid docker config secret.
func createValidDockerConfigSecret(name, registry string) *corev1.Secret {
	dockerConfig := map[string]interface{}{
		"auths": map[string]interface{}{
			registry: map[string]interface{}{
				"username": "testuser",
				"password": "testpass",
			},
		},
	}
	dockerConfigJSON, err := json.Marshal(dockerConfig)
	if err != nil {
		return nil
	}

	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Type: corev1.SecretTypeDockerConfigJson,
		Data: map[string][]byte{
			corev1.DockerConfigJsonKey: dockerConfigJSON,
		},
	}
}

// Helper: find a condition by type.
func findCondition(conditions []metav1.Condition, condType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == condType {
			return &conditions[i]
		}
	}
	return nil
}

// Helper: string pointer.
func strPtr(s string) *string {
	return &s
}
