package secretutils

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// credentialsTestCase defines a test case for ExtractRegistryCredentials.
type credentialsTestCase struct {
	name          string
	secret        *corev1.Secret
	imageURL      string
	expectError   bool
	errorContains string
}

// runCredentialsTest is a helper that runs a credentials test case.
func runCredentialsTest(t *testing.T, tc credentialsTestCase) {
	t.Helper()
	result, err := ExtractRegistryCredentials(tc.secret, tc.imageURL)

	if tc.expectError {
		if err == nil {
			t.Errorf("expected error but got none")
			return
		}
		if tc.errorContains != "" && !strings.Contains(err.Error(), tc.errorContains) {
			t.Errorf("expected error to contain %q, got: %v", tc.errorContains, err)
		}
		return
	}

	if err != nil {
		t.Errorf("unexpected error: %v", err)
		return
	}

	if result == "" {
		t.Error("expected non-empty credentials")
		return
	}

	// Verify credentials are base64 encoded.
	decoded, decodeErr := base64.StdEncoding.DecodeString(result)
	if decodeErr != nil {
		t.Errorf("credentials are not valid base64: %v", decodeErr)
		return
	}

	// Verify credentials contain username:password format.
	const credentialParts = 2
	parts := strings.SplitN(string(decoded), ":", credentialParts)
	if len(parts) != credentialParts {
		t.Errorf("expected credentials in username:password format, got: %s", string(decoded))
	}
}

func TestExtractRegistryCredentials(t *testing.T) {
	tests := []credentialsTestCase{
		{
			name: "dockerconfigjson secret with exact match",
			secret: createDockerConfigJSONSecret("test-secret", map[string]map[string]string{
				"registry.example.com": {
					"username": "testuser",
					"password": "testpass",
				},
			}),
			imageURL:    "oci://registry.example.com/repo/image:tag",
			expectError: false,
		},
		{
			name: "dockerconfigjson secret with port",
			secret: createDockerConfigJSONSecret("test-secret", map[string]map[string]string{
				"registry.example.com:5000": {
					"username": "testuser",
					"password": "testpass",
				},
			}),
			imageURL:    "oci://registry.example.com:5000/repo/image:tag",
			expectError: false,
		},
		{
			name: "quay.io registry",
			secret: createDockerConfigJSONSecret("test-secret", map[string]map[string]string{
				"quay.io": {
					"username": "quayuser",
					"password": "quaypass",
				},
			}),
			imageURL:    "oci://quay.io/repo/image:tag",
			expectError: false,
		},
		{
			name:          "nil secret",
			secret:        nil,
			imageURL:      "oci://registry.example.com/repo/image:tag",
			expectError:   true,
			errorContains: "secret is nil",
		},
		{
			name: "secret missing required keys",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "default",
				},
				Type: corev1.SecretTypeDockerConfigJson,
				Data: map[string][]byte{
					"wrong-key": []byte("data"),
				},
			},
			imageURL:      "oci://registry.example.com/repo/image:tag",
			expectError:   true,
			errorContains: "does not contain",
		},
		{
			name:          "non-OCI image URL",
			secret:        createDockerConfigJSONSecret("test-secret", map[string]map[string]string{}),
			imageURL:      "http://example.com/image.iso",
			expectError:   true,
			errorContains: "does not have oci:// scheme",
		},
		{
			name: "registry not in secret",
			secret: createDockerConfigJSONSecret("test-secret", map[string]map[string]string{
				"different-registry.com": {
					"username": "user",
					"password": "pass",
				},
			}),
			imageURL:      "oci://registry.example.com/repo/image:tag",
			expectError:   true,
			errorContains: "not found in auth config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runCredentialsTest(t, tt)
		})
	}
}

func TestExtractRegistryHost(t *testing.T) {
	tests := []struct {
		name         string
		imageURL     string
		expectedHost string
		expectError  bool
	}{
		{
			name:         "simple OCI URL",
			imageURL:     "oci://registry.example.com/repo/image:tag",
			expectedHost: "registry.example.com",
			expectError:  false,
		},
		{
			name:         "OCI URL with port",
			imageURL:     "oci://registry.example.com:5000/repo/image:tag",
			expectedHost: "registry.example.com:5000",
			expectError:  false,
		},
		{
			name:         "OCI URL without tag",
			imageURL:     "oci://registry.example.com/repo/image",
			expectedHost: "registry.example.com",
			expectError:  false,
		},
		{
			name:         "OCI URL with nested path",
			imageURL:     "oci://registry.example.com/org/team/repo/image:tag",
			expectedHost: "registry.example.com",
			expectError:  false,
		},
		{
			name:         "non-OCI URL",
			imageURL:     "http://example.com/image.iso",
			expectedHost: "",
			expectError:  true,
		},
		{
			name:         "empty URL",
			imageURL:     "",
			expectedHost: "",
			expectError:  true,
		},
		{
			name:         "malformed OCI URL",
			imageURL:     "oci://",
			expectedHost: "",
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, err := extractRegistryHost(tt.imageURL)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if host != tt.expectedHost {
				t.Errorf("expected host %q, got %q", tt.expectedHost, host)
			}
		})
	}
}

func TestExtractRegistryCredentials_LegacyDockerCfg(t *testing.T) {
	tests := []credentialsTestCase{
		{
			name: "legacy dockercfg secret with exact match",
			secret: createLegacyDockerCfgSecret("test-secret", map[string]map[string]string{
				"registry.example.com": {
					"username": "testuser",
					"password": "testpass",
				},
			}),
			imageURL:    "oci://registry.example.com/repo/image:tag",
			expectError: false,
		},
		{
			name: "legacy dockercfg secret with port",
			secret: createLegacyDockerCfgSecret("test-secret", map[string]map[string]string{
				"registry.example.com:5000": {
					"username": "testuser",
					"password": "testpass",
				},
			}),
			imageURL:    "oci://registry.example.com:5000/repo/image:tag",
			expectError: false,
		},
		{
			name: "legacy dockercfg quay.io registry",
			secret: createLegacyDockerCfgSecret("test-secret", map[string]map[string]string{
				"quay.io": {
					"username": "quayuser",
					"password": "quaypass",
				},
			}),
			imageURL:    "oci://quay.io/repo/image:tag",
			expectError: false,
		},
		{
			name: "legacy dockercfg registry not in secret",
			secret: createLegacyDockerCfgSecret("test-secret", map[string]map[string]string{
				"different-registry.com": {
					"username": "user",
					"password": "pass",
				},
			}),
			imageURL:      "oci://registry.example.com/repo/image:tag",
			expectError:   true,
			errorContains: "not found in auth config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runCredentialsTest(t, tt)
		})
	}
}

// Helper function to create a dockerconfigjson secret.
func createDockerConfigJSONSecret(name string, auths map[string]map[string]string) *corev1.Secret {
	dockerAuths := make(map[string]interface{})
	for registry, creds := range auths {
		username := creds["username"]
		password := creds["password"]
		// Encode credentials as base64("username:password") in the Auth field
		// This is the standard Docker config format
		auth := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
		dockerAuths[registry] = map[string]string{
			"auth": auth,
		}
	}

	dockerConfig := map[string]interface{}{
		"auths": dockerAuths,
	}
	dockerConfigJSON, err := json.Marshal(dockerConfig)
	if err != nil {
		panic(err)
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

// Helper function to create a legacy dockercfg secret (kubernetes.io/dockercfg).
// This format does not have the "auths" wrapper - it's just the registry map directly.
func createLegacyDockerCfgSecret(name string, auths map[string]map[string]string) *corev1.Secret {
	dockerAuths := make(map[string]interface{})
	for registry, creds := range auths {
		username := creds["username"]
		password := creds["password"]
		// Encode credentials as base64("username:password") in the Auth field
		auth := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
		dockerAuths[registry] = map[string]string{
			"auth": auth,
		}
	}

	// Legacy format: the config IS the auths map directly (no "auths" wrapper)
	dockerConfigJSON, err := json.Marshal(dockerAuths)
	if err != nil {
		panic(err)
	}

	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Type: corev1.SecretTypeDockercfg,
		Data: map[string][]byte{
			corev1.DockerConfigKey: dockerConfigJSON,
		},
	}
}
