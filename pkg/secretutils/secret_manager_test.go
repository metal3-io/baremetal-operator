package secretutils

import (
	"testing"

	"github.com/go-logr/logr"
	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newTestScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = metal3api.AddToScheme(scheme)
	return scheme
}

func TestSecretManager_ObtainSecret_NotFound(t *testing.T) {
	scheme := newTestScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	sm := NewSecretManager(t.Context(), logr.Discard(), fakeClient, fakeClient)

	_, err := sm.ObtainSecret(types.NamespacedName{Name: "nonexistent", Namespace: "test"})
	require.Error(t, err)
}

func TestSecretManager_ObtainSecret_FoundInCache(t *testing.T) {
	scheme := newTestScheme()
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "test",
			Labels: map[string]string{
				LabelEnvironmentName: LabelEnvironmentValue,
			},
		},
		Data: map[string][]byte{
			"username": []byte("admin"),
			"password": []byte("secret"),
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()

	sm := NewSecretManager(t.Context(), logr.Discard(), fakeClient, fakeClient)

	result, err := sm.ObtainSecret(types.NamespacedName{Name: "test-secret", Namespace: "test"})
	require.NoError(t, err)
	assert.Equal(t, "test-secret", result.Name)
	assert.Equal(t, LabelEnvironmentValue, result.Labels[LabelEnvironmentName])
}

func TestSecretManager_ObtainSecret_AddsLabel(t *testing.T) {
	scheme := newTestScheme()
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "test",
		},
		Data: map[string][]byte{
			"username": []byte("admin"),
			"password": []byte("secret"),
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()

	sm := NewSecretManager(t.Context(), logr.Discard(), fakeClient, fakeClient)

	result, err := sm.ObtainSecret(types.NamespacedName{Name: "test-secret", Namespace: "test"})
	require.NoError(t, err)
	assert.Equal(t, LabelEnvironmentValue, result.Labels[LabelEnvironmentName])

	// Verify the label was persisted
	var updated corev1.Secret
	err = fakeClient.Get(t.Context(), types.NamespacedName{Name: "test-secret", Namespace: "test"}, &updated)
	require.NoError(t, err)
	assert.Equal(t, LabelEnvironmentValue, updated.Labels[LabelEnvironmentName])
}

func TestSecretManager_AcquireSecret_WithOwner(t *testing.T) {
	scheme := newTestScheme()
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "test",
		},
		Data: map[string][]byte{
			"username": []byte("admin"),
			"password": []byte("secret"),
		},
	}

	owner := &metal3api.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-host",
			Namespace: "test",
			UID:       "test-uid-12345",
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret, owner).Build()

	sm := NewSecretManager(t.Context(), logr.Discard(), fakeClient, fakeClient)

	result, err := sm.AcquireSecret(types.NamespacedName{Name: "test-secret", Namespace: "test"}, owner, false)
	require.NoError(t, err)
	assert.Equal(t, LabelEnvironmentValue, result.Labels[LabelEnvironmentName])

	// Verify owner reference was added
	var updated corev1.Secret
	err = fakeClient.Get(t.Context(), types.NamespacedName{Name: "test-secret", Namespace: "test"}, &updated)
	require.NoError(t, err)
	assert.Len(t, updated.OwnerReferences, 1)
	assert.Equal(t, owner.UID, updated.OwnerReferences[0].UID)
}

func TestSecretManager_AcquireSecret_WithFinalizer(t *testing.T) {
	scheme := newTestScheme()
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "test",
		},
		Data: map[string][]byte{
			"username": []byte("admin"),
		},
	}

	owner := &metal3api.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-host",
			Namespace: "test",
			UID:       "test-uid-12345",
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret, owner).Build()

	sm := NewSecretManager(t.Context(), logr.Discard(), fakeClient, fakeClient)

	result, err := sm.AcquireSecret(types.NamespacedName{Name: "test-secret", Namespace: "test"}, owner, true)
	require.NoError(t, err)
	assert.Equal(t, LabelEnvironmentValue, result.Labels[LabelEnvironmentName])

	// Verify finalizer was added
	var updated corev1.Secret
	err = fakeClient.Get(t.Context(), types.NamespacedName{Name: "test-secret", Namespace: "test"}, &updated)
	require.NoError(t, err)
	assert.Contains(t, updated.Finalizers, SecretsFinalizer)
}

func TestSecretManager_AcquireSecret_AlreadyLabeled(t *testing.T) {
	scheme := newTestScheme()
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "test",
			Labels: map[string]string{
				LabelEnvironmentName: LabelEnvironmentValue,
			},
		},
		Data: map[string][]byte{
			"username": []byte("admin"),
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()

	sm := NewSecretManager(t.Context(), logr.Discard(), fakeClient, fakeClient)

	result, err := sm.ObtainSecret(types.NamespacedName{Name: "test-secret", Namespace: "test"})
	require.NoError(t, err)
	assert.Equal(t, LabelEnvironmentValue, result.Labels[LabelEnvironmentName])
}

func TestSecretManager_AcquireSecret_AlreadyOwned(t *testing.T) {
	scheme := newTestScheme()

	owner := &metal3api.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-host",
			Namespace: "test",
			UID:       "test-uid-12345",
		},
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "test",
			Labels: map[string]string{
				LabelEnvironmentName: LabelEnvironmentValue,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "metal3.io/v1alpha1",
					Kind:       "BareMetalHost",
					Name:       owner.Name,
					UID:        owner.UID,
					// Note: Controller should be nil for BMO secrets
				},
			},
		},
		Data: map[string][]byte{
			"username": []byte("admin"),
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret, owner).Build()

	sm := NewSecretManager(t.Context(), logr.Discard(), fakeClient, fakeClient)

	result, err := sm.AcquireSecret(types.NamespacedName{Name: "test-secret", Namespace: "test"}, owner, false)
	require.NoError(t, err)
	assert.Equal(t, "test-secret", result.Name)

	// Should still have exactly one owner reference
	var updated corev1.Secret
	err = fakeClient.Get(t.Context(), types.NamespacedName{Name: "test-secret", Namespace: "test"}, &updated)
	require.NoError(t, err)
	assert.Len(t, updated.OwnerReferences, 1)
}

func TestSecretManager_AcquireSecret_PanicsWithNilOwner(t *testing.T) {
	scheme := newTestScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	sm := NewSecretManager(t.Context(), logr.Discard(), fakeClient, fakeClient)

	assert.Panics(t, func() {
		_, _ = sm.AcquireSecret(types.NamespacedName{Name: "test", Namespace: "test"}, nil, false)
	})
}

func TestSecretManager_FallbackToAPIReader(t *testing.T) {
	scheme := newTestScheme()

	// Secret exists in both clients (simulates real scenario where cache would filter it out)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "uncached-secret",
			Namespace: "test",
		},
		Data: map[string][]byte{
			"data": []byte("value"),
		},
	}

	cacheClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()
	apiReader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()

	sm := NewSecretManager(t.Context(), logr.Discard(), cacheClient, apiReader)

	result, err := sm.ObtainSecret(types.NamespacedName{Name: "uncached-secret", Namespace: "test"})
	require.NoError(t, err)
	assert.Equal(t, "uncached-secret", result.Name)
	assert.Equal(t, LabelEnvironmentValue, result.Labels[LabelEnvironmentName])
}

func TestSecretManager_NotInCacheButInAPI(t *testing.T) {
	scheme := newTestScheme()

	// Secret exists only in the API reader, not in the cache client
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "api-only-secret",
			Namespace: "test",
		},
		Data: map[string][]byte{
			"data": []byte("value"),
		},
	}

	// Empty cache client, secret only in API reader
	cacheClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	apiReader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()

	sm := NewSecretManager(t.Context(), logr.Discard(), cacheClient, apiReader)

	// findSecret should find it via API fallback, but claimSecret will fail
	// because the secret doesn't exist in the cache client for update
	_, err := sm.ObtainSecret(types.NamespacedName{Name: "api-only-secret", Namespace: "test"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestSecretManager_ReleaseSecret(t *testing.T) {
	scheme := newTestScheme()
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-secret",
			Namespace:  "test",
			Finalizers: []string{SecretsFinalizer, "other-finalizer"},
		},
		Data: map[string][]byte{
			"data": []byte("value"),
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()

	sm := NewSecretManager(t.Context(), logr.Discard(), fakeClient, fakeClient)

	err := sm.ReleaseSecret(secret)
	require.NoError(t, err)

	// Verify finalizer was removed
	var updated corev1.Secret
	err = fakeClient.Get(t.Context(), types.NamespacedName{Name: "test-secret", Namespace: "test"}, &updated)
	require.NoError(t, err)
	assert.NotContains(t, updated.Finalizers, SecretsFinalizer)
	assert.Contains(t, updated.Finalizers, "other-finalizer")
}

func TestSecretManager_ReleaseSecret_NoFinalizer(t *testing.T) {
	scheme := newTestScheme()
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "test",
		},
		Data: map[string][]byte{
			"data": []byte("value"),
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()

	sm := NewSecretManager(t.Context(), logr.Discard(), fakeClient, fakeClient)

	// Should return nil (no-op) when finalizer is not present
	err := sm.ReleaseSecret(secret)
	require.NoError(t, err)
}

func TestSecretsFinalizer_Constant(t *testing.T) {
	assert.Equal(t, metal3api.BareMetalHostFinalizer+"/secret", SecretsFinalizer)
}
