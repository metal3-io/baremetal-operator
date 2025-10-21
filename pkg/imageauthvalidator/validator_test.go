package imageauthvalidator

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"

	metal3v1 "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestValidate_NoAuthSecret(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = metal3v1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	validator := New(c, recorder)

	bmh := &metal3v1.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-host",
			Namespace: "default",
		},
		Spec: metal3v1.BareMetalHostSpec{
			Image: &metal3v1.Image{
				URL: "oci://registry.example.com/repo/image:tag",
				// AuthSecretName is nil
			},
		},
	}

	result, err := validator.Validate(context.TODO(), bmh)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Reason != ReasonNotRequired {
		t.Errorf("expected reason %s, got %s", ReasonNotRequired, result.Reason)
	}
	if result.Valid {
		t.Error("expected Valid to be false")
	}
	if !result.OCIRelevant {
		t.Error("expected OCIRelevant to be true for oci:// URL")
	}
}

func TestValidate_SecretNotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = metal3v1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	validator := New(c, recorder)

	secretName := "my-secret"
	bmh := &metal3v1.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-host",
			Namespace: "default",
		},
		Spec: metal3v1.BareMetalHostSpec{
			Image: &metal3v1.Image{
				URL:            "oci://registry.example.com/repo/image:tag",
				AuthSecretName: &secretName,
			},
		},
	}

	result, err := validator.Validate(context.TODO(), bmh)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Reason != ReasonSecretNotFound {
		t.Errorf("expected reason %s, got %s", ReasonSecretNotFound, result.Reason)
	}
	if result.Valid {
		t.Error("expected Valid to be false")
	}
}

func TestValidate_WrongSecretType(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = metal3v1.AddToScheme(scheme)
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
	validator := New(c, recorder)

	bmh := &metal3v1.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-host",
			Namespace: "default",
		},
		Spec: metal3v1.BareMetalHostSpec{
			Image: &metal3v1.Image{
				URL:            "oci://registry.example.com/repo/image:tag",
				AuthSecretName: &secretName,
			},
		},
	}

	result, err := validator.Validate(context.TODO(), bmh)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Reason != ReasonWrongType {
		t.Errorf("expected reason %s, got %s", ReasonWrongType, result.Reason)
	}
	if result.Valid {
		t.Error("expected Valid to be false")
	}

	// Check that warning event was recorded
	select {
	case event := <-recorder.Events:
		if event != "Warning ImageAuthFormatUnsupported Secret \"my-secret\" has unsupported type \"Opaque\"" {
			t.Errorf("unexpected event: %s", event)
		}
	default:
		t.Error("expected warning event to be recorded")
	}
}

func TestValidate_ValidDockerConfigJSON(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = metal3v1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	// Create a valid docker config JSON
	dockerConfig := map[string]interface{}{
		"auths": map[string]interface{}{
			"registry.example.com": map[string]interface{}{
				"username": "testuser",
				"password": "testpass",
			},
		},
	}
	dockerConfigJSON, _ := json.Marshal(dockerConfig)

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
	validator := New(c, recorder)

	bmh := &metal3v1.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-host",
			Namespace: "default",
		},
		Spec: metal3v1.BareMetalHostSpec{
			Image: &metal3v1.Image{
				URL:            "oci://registry.example.com/repo/image:tag",
				AuthSecretName: &secretName,
			},
		},
	}

	result, err := validator.Validate(context.TODO(), bmh)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Valid {
		t.Errorf("expected Valid to be true, got false: %s", result.Message)
	}
	if result.Reason != ReasonValid {
		t.Errorf("expected reason %s, got %s", ReasonValid, result.Reason)
	}
	if result.Credentials == "" {
		t.Error("expected credentials to be populated")
	}

	// Verify credentials are base64 encoded
	decoded, err := base64.StdEncoding.DecodeString(result.Credentials)
	if err != nil {
		t.Fatalf("credentials are not valid base64: %v", err)
	}

	// Verify credentials contain username:password format
	if string(decoded) != "testuser:testpass" {
		t.Errorf("expected credentials to be 'testuser:testpass', got '%s'", string(decoded))
	}
}

func TestValidate_RegistryNotInSecret(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = metal3v1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	// Create a docker config JSON with different registry
	dockerConfig := map[string]interface{}{
		"auths": map[string]interface{}{
			"different-registry.com": map[string]interface{}{
				"username": "testuser",
				"password": "testpass",
			},
		},
	}
	dockerConfigJSON, _ := json.Marshal(dockerConfig)

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
	validator := New(c, recorder)

	bmh := &metal3v1.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-host",
			Namespace: "default",
		},
		Spec: metal3v1.BareMetalHostSpec{
			Image: &metal3v1.Image{
				URL:            "oci://registry.example.com/repo/image:tag",
				AuthSecretName: &secretName,
			},
		},
	}

	result, err := validator.Validate(context.TODO(), bmh)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Valid {
		t.Error("expected Valid to be false")
	}
	if result.Reason != ReasonRegistryEntryMissing {
		t.Errorf("expected reason %s, got %s", ReasonRegistryEntryMissing, result.Reason)
	}
}

func TestValidate_NonOCIImageWithSecret(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = metal3v1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	dockerConfig := map[string]interface{}{
		"auths": map[string]interface{}{
			"registry.example.com": map[string]interface{}{
				"username": "testuser",
				"password": "testpass",
			},
		},
	}
	dockerConfigJSON, _ := json.Marshal(dockerConfig)

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
	validator := New(c, recorder)

	bmh := &metal3v1.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-host",
			Namespace: "default",
		},
		Spec: metal3v1.BareMetalHostSpec{
			Image: &metal3v1.Image{
				URL:            "http://example.com/image.qcow2", // Non-OCI URL
				AuthSecretName: &secretName,
			},
		},
	}

	result, err := validator.Validate(context.TODO(), bmh)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Valid {
		t.Error("expected Valid to be true (secret is valid, just not relevant)")
	}
	if result.OCIRelevant {
		t.Error("expected OCIRelevant to be false for non-OCI URL")
	}
	if result.Credentials != "" {
		t.Error("expected credentials to be empty for non-OCI images")
	}

	// Check that warning event was recorded
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
	_ = metal3v1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	validator := New(c, recorder)

	bmh := &metal3v1.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-host",
			Namespace: "default",
		},
		Spec: metal3v1.BareMetalHostSpec{
			Image: nil,
		},
	}

	result, err := validator.Validate(context.TODO(), bmh)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Valid {
		t.Error("expected Valid to be false")
	}
	if result.Message != "image URL not set" {
		t.Errorf("unexpected message: %s", result.Message)
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
			result := isOCI(tt.url)
			if result != tt.expected {
				t.Errorf("isOCI(%q) = %v, expected %v", tt.url, result, tt.expected)
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

// Helper function to get a client with the given objects
func getFakeClientWithSecretAndBMH(t *testing.T, secretType corev1.SecretType, secretData map[string][]byte, imageURL string) (client.Client, *metal3v1.BareMetalHost, *corev1.Secret) {
	scheme := runtime.NewScheme()
	_ = metal3v1.AddToScheme(scheme)
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

	bmh := &metal3v1.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-host",
			Namespace: "default",
		},
		Spec: metal3v1.BareMetalHostSpec{
			Image: &metal3v1.Image{
				URL:            imageURL,
				AuthSecretName: &secretName,
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret, bmh).Build()
	return c, bmh, secret
}

// TestIntegration_ValidateAndExtractCredentials tests the full flow
func TestIntegration_ValidateAndExtractCredentials(t *testing.T) {
	dockerConfig := map[string]interface{}{
		"auths": map[string]interface{}{
			"quay.io": map[string]interface{}{
				"auth": base64.StdEncoding.EncodeToString([]byte("myuser:mypassword")),
			},
		},
	}
	dockerConfigJSON, _ := json.Marshal(dockerConfig)

	c, bmh, _ := getFakeClientWithSecretAndBMH(
		t,
		corev1.SecretTypeDockerConfigJson,
		map[string][]byte{
			corev1.DockerConfigJsonKey: dockerConfigJSON,
		},
		"oci://quay.io/metal3-io/ironic:latest",
	)

	recorder := record.NewFakeRecorder(10)
	validator := New(c, recorder)

	result, err := validator.Validate(context.TODO(), bmh)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Valid {
		t.Fatalf("expected validation to succeed, got: %s", result.Message)
	}

	if result.Credentials == "" {
		t.Fatal("expected credentials to be populated")
	}

	// Verify the credentials can be decoded
	decoded, err := base64.StdEncoding.DecodeString(result.Credentials)
	if err != nil {
		t.Fatalf("failed to decode credentials: %v", err)
	}

	if string(decoded) != "myuser:mypassword" {
		t.Errorf("expected decoded credentials to be 'myuser:mypassword', got '%s'", string(decoded))
	}
}
