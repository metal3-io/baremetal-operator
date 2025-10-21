package secretutils

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestExtractRegistryHost(t *testing.T) {
	tests := []struct {
		name        string
		imageURL    string
		expected    string
		expectError bool
	}{
		{
			name:     "simple OCI URL",
			imageURL: "oci://registry.example.com/repo/image:tag",
			expected: "registry.example.com",
		},
		{
			name:     "OCI URL with port",
			imageURL: "oci://registry.example.com:5000/repo/image:tag",
			expected: "registry.example.com:5000",
		},
		{
			name:     "OCI URL without tag",
			imageURL: "oci://quay.io/metal3/image",
			expected: "quay.io",
		},
		{
			name:     "OCI URL with nested path",
			imageURL: "oci://gcr.io/project/subfolder/image:v1.0.0",
			expected: "gcr.io",
		},
		{
			name:        "non-OCI URL",
			imageURL:    "https://example.com/image.qcow2",
			expectError: true,
		},
		{
			name:        "empty URL",
			imageURL:    "",
			expectError: true,
		},
		{
			name:        "malformed OCI URL",
			imageURL:    "oci://",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := extractRegistryHost(tt.imageURL)
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
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestExtractCredentials(t *testing.T) {
	tests := []struct {
		name           string
		authConfig     *DockerAuthConfig
		expectedUser   string
		expectedPass   string
		expectError    bool
	}{
		{
			name: "explicit username and password",
			authConfig: &DockerAuthConfig{
				Username: "testuser",
				Password: "testpass",
			},
			expectedUser: "testuser",
			expectedPass: "testpass",
		},
		{
			name: "base64 encoded auth",
			authConfig: &DockerAuthConfig{
				Auth: base64.StdEncoding.EncodeToString([]byte("user123:pass456")),
			},
			expectedUser: "user123",
			expectedPass: "pass456",
		},
		{
			name: "auth field with colon in password",
			authConfig: &DockerAuthConfig{
				Auth: base64.StdEncoding.EncodeToString([]byte("myuser:pass:with:colons")),
			},
			expectedUser: "myuser",
			expectedPass: "pass:with:colons",
		},
		{
			name: "username and password take precedence over auth",
			authConfig: &DockerAuthConfig{
				Username: "explicituser",
				Password: "explicitpass",
				Auth:     base64.StdEncoding.EncodeToString([]byte("authuser:authpass")),
			},
			expectedUser: "explicituser",
			expectedPass: "explicitpass",
		},
		{
			name:        "empty auth config",
			authConfig:  &DockerAuthConfig{},
			expectError: true,
		},
		{
			name: "invalid auth format - no colon",
			authConfig: &DockerAuthConfig{
				Auth: base64.StdEncoding.EncodeToString([]byte("invalidformat")),
			},
			expectError: true,
		},
		{
			name: "invalid base64",
			authConfig: &DockerAuthConfig{
				Auth: "not-valid-base64!!!",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, pass, err := extractCredentials(tt.authConfig)
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
			if user != tt.expectedUser {
				t.Errorf("expected user %q, got %q", tt.expectedUser, user)
			}
			if pass != tt.expectedPass {
				t.Errorf("expected password %q, got %q", tt.expectedPass, pass)
			}
		})
	}
}

func TestParseDockerConfigJSON(t *testing.T) {
	tests := []struct {
		name         string
		data         []byte
		registryHost string
		expectError  bool
		checkAuth    bool
	}{
		{
			name: "valid dockerconfigjson",
			data: []byte(`{
				"auths": {
					"registry.example.com": {
						"username": "testuser",
						"password": "testpass"
					}
				}
			}`),
			registryHost: "registry.example.com",
			checkAuth:    true,
		},
		{
			name: "registry with https prefix",
			data: []byte(`{
				"auths": {
					"https://registry.example.com": {
						"auth": "` + base64.StdEncoding.EncodeToString([]byte("user:pass")) + `"
					}
				}
			}`),
			registryHost: "registry.example.com",
			checkAuth:    true,
		},
		{
			name: "Docker Hub format",
			data: []byte(`{
				"auths": {
					"https://index.docker.io/v1/": {
						"auth": "` + base64.StdEncoding.EncodeToString([]byte("dockeruser:dockerpass")) + `"
					}
				}
			}`),
			registryHost: "docker.io",
			checkAuth:    true,
		},
		{
			name: "registry not found",
			data: []byte(`{
				"auths": {
					"registry.example.com": {
						"username": "testuser",
						"password": "testpass"
					}
				}
			}`),
			registryHost: "different.registry.com",
			expectError:  true,
		},
		{
			name:         "invalid JSON",
			data:         []byte(`{invalid json`),
			registryHost: "registry.example.com",
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authConfig, err := parseDockerConfigJSON(tt.data, tt.registryHost)
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
			if tt.checkAuth && authConfig == nil {
				t.Errorf("expected auth config but got nil")
			}
		})
	}
}

func TestParseDockerConfig(t *testing.T) {
	tests := []struct {
		name         string
		data         []byte
		registryHost string
		expectError  bool
		checkAuth    bool
	}{
		{
			name: "valid dockercfg",
			data: []byte(`{
				"registry.example.com": {
					"username": "testuser",
					"password": "testpass",
					"email": "test@example.com"
				}
			}`),
			registryHost: "registry.example.com",
			checkAuth:    true,
		},
		{
			name: "registry with auth field",
			data: []byte(`{
				"quay.io": {
					"auth": "` + base64.StdEncoding.EncodeToString([]byte("quayuser:quaypass")) + `",
					"email": "quay@example.com"
				}
			}`),
			registryHost: "quay.io",
			checkAuth:    true,
		},
		{
			name: "registry not found",
			data: []byte(`{
				"registry.example.com": {
					"username": "testuser",
					"password": "testpass"
				}
			}`),
			registryHost: "other.registry.com",
			expectError:  true,
		},
		{
			name:         "invalid JSON",
			data:         []byte(`not valid json`),
			registryHost: "registry.example.com",
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authConfig, err := parseDockerConfig(tt.data, tt.registryHost)
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
			if tt.checkAuth && authConfig == nil {
				t.Errorf("expected auth config but got nil")
			}
		})
	}
}

func TestExtractRegistryCredentials(t *testing.T) {
	// Helper function to create docker config JSON
	createDockerConfigJSON := func(host, username, password string) []byte {
		config := DockerConfigJSON{
			Auths: map[string]DockerAuthConfig{
				host: {
					Username: username,
					Password: password,
				},
			},
		}
		data, _ := json.Marshal(config)
		return data
	}

	// Helper function to create legacy docker config
	createDockerConfig := func(host, username, password string) []byte {
		config := DockerConfig{
			host: {
				Username: username,
				Password: password,
			},
		}
		data, _ := json.Marshal(config)
		return data
	}

	tests := []struct {
		name             string
		secret           *corev1.Secret
		imageURL         string
		expectError      bool
		validateResult   bool
	}{
		{
			name: "dockerconfigjson secret with exact match",
			secret: &corev1.Secret{
				Data: map[string][]byte{
					corev1.DockerConfigJsonKey: createDockerConfigJSON("registry.example.com", "user1", "pass1"),
				},
			},
			imageURL:       "oci://registry.example.com/repo/image:tag",
			validateResult: true,
		},
		{
			name: "dockerconfigjson secret with https prefix",
			secret: &corev1.Secret{
				Data: map[string][]byte{
					corev1.DockerConfigJsonKey: createDockerConfigJSON("https://registry.example.com", "user2", "pass2"),
				},
			},
			imageURL:       "oci://registry.example.com/repo/image:tag",
			validateResult: true,
		},
		{
			name: "dockercfg secret (legacy format)",
			secret: &corev1.Secret{
				Data: map[string][]byte{
					corev1.DockerConfigKey: createDockerConfig("quay.io", "quayuser", "quaypass"),
				},
			},
			imageURL:       "oci://quay.io/org/image:v1",
			validateResult: true,
		},
		{
			name: "secret with auth field instead of username/password",
			secret: &corev1.Secret{
				Data: map[string][]byte{
					corev1.DockerConfigJsonKey: []byte(`{
						"auths": {
							"gcr.io": {
								"auth": "` + base64.StdEncoding.EncodeToString([]byte("_json_key:keydata")) + `"
							}
						}
					}`),
				},
			},
			imageURL:       "oci://gcr.io/project/image:latest",
			validateResult: true,
		},
		{
			name: "Docker Hub with legacy format",
			secret: &corev1.Secret{
				Data: map[string][]byte{
					corev1.DockerConfigJsonKey: []byte(`{
						"auths": {
							"https://index.docker.io/v1/": {
								"username": "dockeruser",
								"password": "dockerpass"
							}
						}
					}`),
				},
			},
			imageURL:       "oci://docker.io/library/image:tag",
			validateResult: true,
		},
		{
			name:        "nil secret",
			secret:      nil,
			imageURL:    "oci://registry.example.com/image:tag",
			expectError: true,
		},
		{
			name: "secret missing required keys",
			secret: &corev1.Secret{
				Data: map[string][]byte{
					"wrong-key": []byte("data"),
				},
			},
			imageURL:    "oci://registry.example.com/image:tag",
			expectError: true,
		},
		{
			name: "non-OCI image URL",
			secret: &corev1.Secret{
				Data: map[string][]byte{
					corev1.DockerConfigJsonKey: createDockerConfigJSON("registry.example.com", "user", "pass"),
				},
			},
			imageURL:    "https://example.com/image.qcow2",
			expectError: true,
		},
		{
			name: "registry not in secret",
			secret: &corev1.Secret{
				Data: map[string][]byte{
					corev1.DockerConfigJsonKey: createDockerConfigJSON("registry.example.com", "user", "pass"),
				},
			},
			imageURL:    "oci://different.registry.com/image:tag",
			expectError: true,
		},
		{
			name: "secret with port in registry",
			secret: &corev1.Secret{
				Data: map[string][]byte{
					corev1.DockerConfigJsonKey: createDockerConfigJSON("registry.example.com:5000", "user", "pass"),
				},
			},
			imageURL:       "oci://registry.example.com:5000/repo/image:tag",
			validateResult: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ExtractRegistryCredentials(tt.secret, tt.imageURL)
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
			if tt.validateResult {
				// Verify the result is valid base64
				decoded, err := base64.StdEncoding.DecodeString(result)
				if err != nil {
					t.Errorf("result is not valid base64: %v", err)
					return
				}
				// Verify it contains a colon (username:password format)
				if !strings.Contains(string(decoded), ":") {
					t.Errorf("decoded result does not contain colon separator: %s", decoded)
				}
			}
		})
	}
}

func TestFindAuthConfig(t *testing.T) {
	auths := map[string]DockerAuthConfig{
		"registry.example.com": {
			Username: "user1",
			Password: "pass1",
		},
		"https://quay.io": {
			Username: "user2",
			Password: "pass2",
		},
		"gcr.io/v2/": {
			Username: "user3",
			Password: "pass3",
		},
		"https://index.docker.io/v1/": {
			Username: "user4",
			Password: "pass4",
		},
	}

	tests := []struct {
		name         string
		registryHost string
		expectFound  bool
		expectedUser string
	}{
		{
			name:         "exact match",
			registryHost: "registry.example.com",
			expectFound:  true,
			expectedUser: "user1",
		},
		{
			name:         "match with https prefix in config",
			registryHost: "quay.io",
			expectFound:  true,
			expectedUser: "user2",
		},
		{
			name:         "match with /v2/ suffix in config",
			registryHost: "gcr.io",
			expectFound:  true,
			expectedUser: "user3",
		},
		{
			name:         "Docker Hub special case",
			registryHost: "docker.io",
			expectFound:  true,
			expectedUser: "user4",
		},
		{
			name:         "not found",
			registryHost: "notfound.registry.com",
			expectFound:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authConfig, err := findAuthConfig(auths, tt.registryHost)
			if !tt.expectFound {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if authConfig == nil {
				t.Errorf("expected auth config but got nil")
				return
			}
			if authConfig.Username != tt.expectedUser {
				t.Errorf("expected username %q, got %q", tt.expectedUser, authConfig.Username)
			}
		})
	}
}

