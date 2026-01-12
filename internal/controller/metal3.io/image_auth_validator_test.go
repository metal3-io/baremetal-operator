package controllers

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/secretutils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func testLogger(t *testing.T) logr.Logger {
	t.Helper()
	return logr.Discard()
}

func TestValidate_NoAuthSecret(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = metal3api.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	secretManager := secretutils.NewSecretManager(t.Context(), testLogger(t), c, c)
	validator := NewImageAuthValidator(recorder)

	bmh := &metal3api.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-host",
			Namespace: "default",
		},
		Spec: metal3api.BareMetalHostSpec{
			Image: &metal3api.Image{
				URL: "oci://registry.example.com/repo/image:tag",
				// AuthSecretName is nil
			},
		},
	}

	result, err := validator.Validate(t.Context(), bmh, secretManager)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Valid {
		t.Error("expected Valid to be false when no auth secret is configured")
	}
	if result.Credentials != "" {
		t.Error("expected empty credentials when no auth secret is configured")
	}
}

func TestValidate_SecretNotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = metal3api.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	secretManager := secretutils.NewSecretManager(t.Context(), testLogger(t), c, c)
	validator := NewImageAuthValidator(recorder)

	secretName := "my-secret"
	bmh := &metal3api.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-host",
			Namespace: "default",
		},
		Spec: metal3api.BareMetalHostSpec{
			Image: &metal3api.Image{
				URL:            "oci://registry.example.com/repo/image:tag",
				AuthSecretName: &secretName,
			},
		},
	}

	result, err := validator.Validate(t.Context(), bmh, secretManager)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Valid {
		t.Error("expected Valid to be false when secret is not found")
	}
	if result.Credentials != "" {
		t.Error("expected empty credentials when secret is not found")
	}
}

func TestValidate_WrongSecretType(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = metal3api.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	secretName := "my-secret"
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: "default",
		},
		Type: corev1.SecretTypeOpaque, // Wrong type
		Data: map[string][]byte{
			"username": []byte("user"),
			"password": []byte("pass"),
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()
	recorder := record.NewFakeRecorder(10)
	secretManager := secretutils.NewSecretManager(t.Context(), testLogger(t), c, c)
	validator := NewImageAuthValidator(recorder)

	bmh := &metal3api.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-host",
			Namespace: "default",
		},
		Spec: metal3api.BareMetalHostSpec{
			Image: &metal3api.Image{
				URL:            "oci://registry.example.com/repo/image:tag",
				AuthSecretName: &secretName,
			},
		},
	}

	result, err := validator.Validate(t.Context(), bmh, secretManager)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Valid {
		t.Error("expected Valid to be false for wrong secret type")
	}
	if result.Credentials != "" {
		t.Error("expected empty credentials for wrong secret type")
	}

	// Assert that warning event was recorded.
	select {
	case event := <-recorder.Events:
		expectedEvent := "Warning ImageAuthFormatUnsupported Secret \"my-secret\" has unsupported type \"Opaque\""
		if event != expectedEvent {
			t.Errorf("expected event %q, got %q", expectedEvent, event)
		}
	default:
		t.Error("expected warning event to be recorded")
	}
}

func TestValidate_ValidDockerConfigJSON(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = metal3api.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	// Create a valid docker config JSON.
	dockerConfig := map[string]interface{}{
		"auths": map[string]interface{}{
			"registry.example.com": map[string]interface{}{
				"auth": base64.StdEncoding.EncodeToString([]byte("testuser:testpass")),
			},
		},
	}
	dockerConfigJSON, err := json.Marshal(dockerConfig)
	if err != nil {
		t.Fatalf("failed to marshal docker config: %v", err)
	}

	secretName := "my-secret"
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: "default",
		},
		Type: corev1.SecretTypeDockerConfigJson,
		Data: map[string][]byte{
			corev1.DockerConfigJsonKey: dockerConfigJSON,
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()
	recorder := record.NewFakeRecorder(10)
	secretManager := secretutils.NewSecretManager(t.Context(), testLogger(t), c, c)
	validator := NewImageAuthValidator(recorder)

	bmh := &metal3api.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-host",
			Namespace: "default",
		},
		Spec: metal3api.BareMetalHostSpec{
			Image: &metal3api.Image{
				URL:            "oci://registry.example.com/repo/image:tag",
				AuthSecretName: &secretName,
			},
		},
	}

	result, err := validator.Validate(t.Context(), bmh, secretManager)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Valid {
		t.Error("expected Valid to be true for valid docker config")
	}
	if result.Credentials == "" {
		t.Error("expected credentials to be populated")
	}

	// Verify credentials are base64 encoded.
	decoded, err := base64.StdEncoding.DecodeString(result.Credentials)
	if err != nil {
		t.Fatalf("credentials are not valid base64: %v", err)
	}

	// Verify credentials contain username:password format.
	if string(decoded) != "testuser:testpass" {
		t.Errorf("expected credentials to be 'testuser:testpass', got '%s'", string(decoded))
	}

	// No event should be emitted on success (validator only emits warnings).
	select {
	case event := <-recorder.Events:
		t.Errorf("unexpected event emitted: %q", event)
	default:
		// Expected: no events for successful validation.
	}
}

func TestValidate_RegistryNotInSecret(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = metal3api.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	// Create a docker config JSON with different registry.
	dockerConfig := map[string]interface{}{
		"auths": map[string]interface{}{
			"different-registry.com": map[string]interface{}{
				"auth": base64.StdEncoding.EncodeToString([]byte("testuser:testpass")),
			},
		},
	}
	dockerConfigJSON, err := json.Marshal(dockerConfig)
	if err != nil {
		t.Fatalf("failed to marshal docker config: %v", err)
	}

	secretName := "my-secret"
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: "default",
		},
		Type: corev1.SecretTypeDockerConfigJson,
		Data: map[string][]byte{
			corev1.DockerConfigJsonKey: dockerConfigJSON,
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()
	recorder := record.NewFakeRecorder(10)
	secretManager := secretutils.NewSecretManager(t.Context(), testLogger(t), c, c)
	validator := NewImageAuthValidator(recorder)

	bmh := &metal3api.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-host",
			Namespace: "default",
		},
		Spec: metal3api.BareMetalHostSpec{
			Image: &metal3api.Image{
				URL:            "oci://registry.example.com/repo/image:tag",
				AuthSecretName: &secretName,
			},
		},
	}

	result, err := validator.Validate(t.Context(), bmh, secretManager)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Valid {
		t.Error("expected Valid to be false when registry is not in secret")
	}
	if result.Credentials != "" {
		t.Error("expected empty credentials when registry is not in secret")
	}

	// Assert warning event was recorded.
	select {
	case event := <-recorder.Events:
		if !strings.Contains(event, "Warning") || !strings.Contains(event, "ImageAuthParseError") {
			t.Errorf("expected Warning ImageAuthParseError event, got: %q", event)
		}
	default:
		t.Error("expected warning event to be recorded")
	}
}

func TestValidate_NonOCIImageWithSecret(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = metal3api.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	dockerConfig := map[string]interface{}{
		"auths": map[string]interface{}{
			"registry.example.com": map[string]interface{}{
				"auth": base64.StdEncoding.EncodeToString([]byte("testuser:testpass")),
			},
		},
	}
	dockerConfigJSON, err := json.Marshal(dockerConfig)
	if err != nil {
		t.Fatalf("failed to marshal docker config: %v", err)
	}

	secretName := "my-secret"
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: "default",
		},
		Type: corev1.SecretTypeDockerConfigJson,
		Data: map[string][]byte{
			corev1.DockerConfigJsonKey: dockerConfigJSON,
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()
	recorder := record.NewFakeRecorder(10)
	secretManager := secretutils.NewSecretManager(t.Context(), testLogger(t), c, c)
	validator := NewImageAuthValidator(recorder)

	bmh := &metal3api.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-host",
			Namespace: "default",
		},
		Spec: metal3api.BareMetalHostSpec{
			Image: &metal3api.Image{
				URL:            "http://example.com/image.qcow2", // Non-OCI URL
				AuthSecretName: &secretName,
			},
		},
	}

	result, err := validator.Validate(t.Context(), bmh, secretManager)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Valid {
		t.Error("expected Valid to be true (secret is valid, just not relevant)")
	}
	if result.Credentials != "" {
		t.Error("expected credentials to be empty for non-OCI images")
	}

	// Check that warning event was recorded.
	select {
	case event := <-recorder.Events:
		if event != "Warning ImageAuthIrrelevant authSecretName=\"my-secret\" is set but image URL is not oci:// (http://example.com/image.qcow2)" {
			t.Errorf("unexpected event: %s", event)
		}
	default:
		t.Error("expected warning event to be recorded")
	}
}

func TestValidate_NilImage(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = metal3api.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	secretManager := secretutils.NewSecretManager(t.Context(), testLogger(t), c, c)
	validator := NewImageAuthValidator(recorder)

	bmh := &metal3api.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-host",
			Namespace: "default",
		},
		Spec: metal3api.BareMetalHostSpec{
			Image: nil,
		},
	}

	result, err := validator.Validate(t.Context(), bmh, secretManager)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Valid {
		t.Error("expected Valid to be false when image is nil")
	}
	if result.Credentials != "" {
		t.Error("expected empty credentials when image is nil")
	}
}

func TestIsOCI(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected bool
	}{
		{"OCI lowercase", "oci://registry.example.com/image:tag", true},
		{"OCI uppercase", "OCI://registry.example.com/image:tag", true},
		{"OCI mixed case", "Oci://registry.example.com/image:tag", true},
		{"HTTP", "http://example.com/image.qcow2", false},
		{"HTTPS", "https://example.com/image.qcow2", false},
		{"Empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			img := &metal3api.Image{URL: tt.url}
			result := img.IsOCI()
			if result != tt.expected {
				t.Errorf("IsOCI(%q) = %v, expected %v", tt.url, result, tt.expected)
			}
		})
	}
}

func TestIsAllowedDockerConfigType(t *testing.T) {
	tests := []struct {
		name     string
		typ      corev1.SecretType
		expected bool
	}{
		{"dockerconfigjson", corev1.SecretTypeDockerConfigJson, true},
		{"dockercfg", corev1.SecretTypeDockercfg, true},
		{"opaque", corev1.SecretTypeOpaque, false},
		{"tls", corev1.SecretTypeTLS, false},
		{"basic-auth", corev1.SecretTypeBasicAuth, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isAllowedDockerConfigType(tt.typ)
			if result != tt.expected {
				t.Errorf("isAllowedDockerConfigType(%v) = %v, expected %v", tt.typ, result, tt.expected)
			}
		})
	}
}

// Helper function to get a client with the given objects.
func getFakeClientWithSecretAndBMH(t *testing.T, secretType corev1.SecretType, secretData map[string][]byte, imageURL string) (client.Client, *metal3api.BareMetalHost, *corev1.Secret) {
	t.Helper()
	scheme := runtime.NewScheme()
	_ = metal3api.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	secretName := "test-secret"
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: "default",
		},
		Type: secretType,
		Data: secretData,
	}

	bmh := &metal3api.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-host",
			Namespace: "default",
		},
		Spec: metal3api.BareMetalHostSpec{
			Image: &metal3api.Image{
				URL:            imageURL,
				AuthSecretName: &secretName,
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret, bmh).Build()
	return c, bmh, secret
}

// TestIntegration_ValidateAndExtractCredentials tests the full flow.
func TestIntegration_ValidateAndExtractCredentials(t *testing.T) {
	dockerConfig := map[string]interface{}{
		"auths": map[string]interface{}{
			"quay.io": map[string]interface{}{
				"auth": base64.StdEncoding.EncodeToString([]byte("myuser:mypassword")),
			},
		},
	}
	dockerConfigJSON, err := json.Marshal(dockerConfig)
	if err != nil {
		t.Fatalf("failed to marshal docker config: %v", err)
	}

	c, bmh, _ := getFakeClientWithSecretAndBMH(
		t,
		corev1.SecretTypeDockerConfigJson,
		map[string][]byte{
			corev1.DockerConfigJsonKey: dockerConfigJSON,
		},
		"oci://quay.io/metal3-io/ironic:latest",
	)

	recorder := record.NewFakeRecorder(10)
	secretManager := secretutils.NewSecretManager(t.Context(), testLogger(t), c, c)
	validator := NewImageAuthValidator(recorder)

	result, err := validator.Validate(t.Context(), bmh, secretManager)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Valid {
		t.Fatal("expected validation to succeed for Docker Hub")
	}

	if result.Credentials == "" {
		t.Fatal("expected credentials to be populated")
	}

	// Verify the credentials can be decoded.
	decoded, err := base64.StdEncoding.DecodeString(result.Credentials)
	if err != nil {
		t.Fatalf("failed to decode credentials: %v", err)
	}

	if string(decoded) != "myuser:mypassword" {
		t.Errorf("expected decoded credentials to be 'myuser:mypassword', got '%s'", string(decoded))
	}
}
