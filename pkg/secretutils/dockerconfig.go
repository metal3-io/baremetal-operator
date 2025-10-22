package secretutils

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

// DockerConfigJSON represents the structure of a kubernetes.io/dockerconfigjson secret.
type DockerConfigJSON struct {
	Auths map[string]DockerAuthConfig `json:"auths"`
}

// DockerAuthConfig contains authorization information for a docker registry.
type DockerAuthConfig struct {
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	Auth     string `json:"auth,omitempty"`
	Email    string `json:"email,omitempty"`
}

// DockerConfig represents the structure of a kubernetes.io/dockercfg secret (legacy format).
type DockerConfig map[string]DockerAuthConfig

// ExtractRegistryCredentials extracts the registry credentials from a Kubernetes secret
// for the registry associated with the given image URL.
// It supports both kubernetes.io/dockerconfigjson and kubernetes.io/dockercfg secret types.
// Returns the credentials in base64-encoded "username:password" format as expected by Ironic.
func ExtractRegistryCredentials(secret *corev1.Secret, imageURL string) (string, error) {
	if secret == nil {
		return "", errors.New("secret is nil")
	}

	registryHost, err := extractRegistryHost(imageURL)
	if err != nil {
		return "", fmt.Errorf("failed to extract registry host from image URL: %w", err)
	}

	var authConfig *DockerAuthConfig

	// Try parsing as dockerconfigjson format first (newer format)
	if data, ok := secret.Data[corev1.DockerConfigJsonKey]; ok {
		authConfig, err = parseDockerConfigJSON(data, registryHost)
		if err != nil {
			return "", fmt.Errorf("failed to parse dockerconfigjson: %w", err)
		}
	} else if data, ok := secret.Data[corev1.DockerConfigKey]; ok {
		// Try parsing as dockercfg format (legacy format)
		authConfig, err = parseDockerConfig(data, registryHost)
		if err != nil {
			return "", fmt.Errorf("failed to parse dockercfg: %w", err)
		}
	} else {
		return "", fmt.Errorf("secret does not contain %s or %s key", corev1.DockerConfigJsonKey, corev1.DockerConfigKey)
	}

	if authConfig == nil {
		return "", fmt.Errorf("no credentials found for registry %s", registryHost)
	}

	// Extract username and password from the auth config
	username, password, err := extractCredentials(authConfig)
	if err != nil {
		return "", fmt.Errorf("failed to extract credentials: %w", err)
	}

	// Return credentials in the format expected by Ironic (base64-encoded "username:password")
	credentials := fmt.Sprintf("%s:%s", username, password)
	return base64.StdEncoding.EncodeToString([]byte(credentials)), nil
}

// extractRegistryHost extracts the registry hostname from an OCI image URL.
// For example, "oci://registry.example.com/repo/image:tag" returns "registry.example.com".
func extractRegistryHost(imageURL string) (string, error) {
	if !strings.HasPrefix(imageURL, "oci://") {
		return "", fmt.Errorf("image URL does not have oci:// scheme: %s", imageURL)
	}

	// Remove the oci:// prefix
	urlWithoutScheme := strings.TrimPrefix(imageURL, "oci://")

	// Parse the remaining part as a URL to extract the host
	// Add http:// temporarily to help with parsing
	parsedURL, err := url.Parse("http://" + urlWithoutScheme)
	if err != nil {
		return "", fmt.Errorf("failed to parse image URL: %w", err)
	}

	host := parsedURL.Hostname()
	if host == "" {
		return "", fmt.Errorf("failed to extract hostname from image URL: %s", imageURL)
	}

	// Include port if present
	if parsedURL.Port() != "" {
		host = parsedURL.Host
	}

	return host, nil
}

// parseDockerConfigJSON parses a kubernetes.io/dockerconfigjson secret data
// and returns the auth config for the specified registry.
func parseDockerConfigJSON(data []byte, registryHost string) (*DockerAuthConfig, error) {
	var config DockerConfigJSON
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal docker config JSON: %w", err)
	}

	return findAuthConfig(config.Auths, registryHost)
}

// parseDockerConfig parses a kubernetes.io/dockercfg secret data (legacy format)
// and returns the auth config for the specified registry.
func parseDockerConfig(data []byte, registryHost string) (*DockerAuthConfig, error) {
	var config DockerConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal docker config: %w", err)
	}

	return findAuthConfig(config, registryHost)
}

// findAuthConfig searches for the auth config matching the registry host.
// It tries several variations of the registry host to handle different formats.
// Returns a clear RegistryEntryMissing error when no entry matches.
func findAuthConfig(auths map[string]DockerAuthConfig, registryHost string) (*DockerAuthConfig, error) {
	// Try exact match first (handles both "host" and "host:port")
	if authConfig, ok := auths[registryHost]; ok {
		return &authConfig, nil
	}

	// Try with https:// prefix
	if authConfig, ok := auths["https://"+registryHost]; ok {
		return &authConfig, nil
	}

	// Try with http:// prefix
	if authConfig, ok := auths["http://"+registryHost]; ok {
		return &authConfig, nil
	}

	// Try with /v1/ suffix (Docker Hub legacy format)
	if authConfig, ok := auths[registryHost+"/v1/"]; ok {
		return &authConfig, nil
	}

	// Try with /v2/ suffix
	if authConfig, ok := auths[registryHost+"/v2/"]; ok {
		return &authConfig, nil
	}

	// Try with https:// prefix and /v1/ suffix
	if authConfig, ok := auths["https://"+registryHost+"/v1/"]; ok {
		return &authConfig, nil
	}

	// Try with https:// prefix and /v2/ suffix
	if authConfig, ok := auths["https://"+registryHost+"/v2/"]; ok {
		return &authConfig, nil
	}

	// Special handling for Docker Hub
	// Docker Hub can appear as: docker.io, index.docker.io, https://index.docker.io/v1/, etc.
	if registryHost == "docker.io" || registryHost == "index.docker.io" ||
		strings.HasPrefix(registryHost, "docker.io:") || strings.HasPrefix(registryHost, "index.docker.io:") {
		dockerHubKeys := []string{
			"https://index.docker.io/v1/",
			"index.docker.io",
			"docker.io",
			"https://docker.io",
			"https://index.docker.io",
			registryHost, // Already tried but keep for clarity
		}
		for _, key := range dockerHubKeys {
			if authConfig, ok := auths[key]; ok {
				return &authConfig, nil
			}
		}
	}

	// Return clear error when no entry matches
	return nil, fmt.Errorf("registry %s not found in auth config", registryHost)
}

// extractCredentials extracts username and password from a DockerAuthConfig.
// It handles both the explicit username/password fields and the base64-encoded auth field.
func extractCredentials(authConfig *DockerAuthConfig) (username, password string, err error) {
	// If username and password are explicitly provided, use them
	if authConfig.Username != "" && authConfig.Password != "" {
		return authConfig.Username, authConfig.Password, nil
	}

	// If auth field is present, decode it
	if authConfig.Auth != "" {
		decoded, err := base64.StdEncoding.DecodeString(authConfig.Auth)
		if err != nil {
			return "", "", fmt.Errorf("failed to decode auth field: %w", err)
		}

		const credentialParts = 2
		parts := strings.SplitN(string(decoded), ":", credentialParts)
		if len(parts) != credentialParts {
			return "", "", errors.New("invalid auth format: expected username:password")
		}

		return parts[0], parts[1], nil
	}

	return "", "", errors.New("no credentials found in auth config")
}
