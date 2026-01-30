package secretutils

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/cpuguy83/dockercfg"
	corev1 "k8s.io/api/core/v1"
)

// ExtractRegistryCredentials extracts the registry credentials from a Kubernetes secret
// for the registry associated with the given image URL.
// It supports both kubernetes.io/dockerconfigjson and kubernetes.io/dockercfg secret types.
// Returns ONLY the minimal credential in the format expected by Ironic:
// base64-encoded "username:password" (NOT the entire Docker config JSON).
// This is what Ironic accepts in instance_info[image_pull_secret].
func ExtractRegistryCredentials(secret *corev1.Secret, imageURL string) (string, error) {
	if secret == nil {
		return "", errors.New("secret is nil")
	}

	registryHost, err := extractRegistryHost(imageURL)
	if err != nil {
		return "", fmt.Errorf("failed to extract registry host from image URL: %w", err)
	}

	// Use dockercfg library to parse Docker config
	var cfg dockercfg.Config
	var data []byte
	var ok bool

	// Try parsing as dockerconfigjson format first (newer format)
	if data, ok = secret.Data[corev1.DockerConfigJsonKey]; ok {
		if parseErr := json.Unmarshal(data, &cfg); parseErr != nil {
			return "", fmt.Errorf("failed to parse dockerconfigjson: %w", parseErr)
		}
	} else if data, ok = secret.Data[corev1.DockerConfigKey]; ok {
		// Try parsing as dockercfg format (legacy format) - it's just the AuthConfigs map
		if parseErr := json.Unmarshal(data, &cfg.AuthConfigs); parseErr != nil {
			return "", fmt.Errorf("failed to parse dockercfg: %w", parseErr)
		}
	} else {
		return "", fmt.Errorf("secret does not contain %s or %s key", corev1.DockerConfigJsonKey, corev1.DockerConfigKey)
	}

	// Get credentials for the registry using the library's built-in resolution
	// Use ResolveRegistryHost to handle Docker Hub resolution (docker.io -> index.docker.io)
	resolvedHost := dockercfg.ResolveRegistryHost(registryHost)
	username, password, err := cfg.GetRegistryCredentials(resolvedHost)
	if err != nil {
		return "", fmt.Errorf("failed to get credentials for registry %s: %w", registryHost, err)
	}

	if username == "" && password == "" {
		// Empty credentials means the registry was not found in the config
		return "", fmt.Errorf("registry %s not found in auth config", registryHost)
	}

	// Return credentials in the format expected by Ironic (base64-encoded "username:password")
	credentials := fmt.Sprintf("%s:%s", username, password)
	return base64.StdEncoding.EncodeToString([]byte(credentials)), nil
}

// extractRegistryHost extracts the registry hostname from an OCI image URL.
// For example, "oci://registry.example.com/repo/image:tag" returns "registry.example.com".
func extractRegistryHost(imageURL string) (string, error) {
	parsed, err := url.Parse(imageURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse image URL: %w", err)
	}

	if !strings.EqualFold(parsed.Scheme, "oci") {
		return "", fmt.Errorf("image URL does not have oci:// scheme: %s", imageURL)
	}

	if parsed.Host == "" {
		return "", fmt.Errorf("failed to extract hostname from image URL: %s", imageURL)
	}

	return parsed.Host, nil
}
