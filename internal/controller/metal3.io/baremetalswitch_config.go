/*
Copyright 2025 The Metal3 Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/go-logr/logr"
	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const switchConfigKey = "switch-configs.conf"

// credentialConfigError indicates a switch's credentials are misconfigured
// (e.g., secret not found or required key missing). These errors are safe
// to skip — the switch will be reconciled when the secret is created or updated.
type credentialConfigError struct {
	msg string
}

func (e *credentialConfigError) Error() string { return e.msg }

// switchConfigResult holds the per-switch config entries and any collected
// SSH private key files for publickey-authenticated switches. Both maps
// are keyed per-switch: configEntries by switch name, keyFiles by MAC address.
type switchConfigResult struct {
	// configEntries maps switch name to its INI-format configuration section.
	configEntries map[string][]byte
	// keyFiles maps "<mac-address>.key" to SSH private key bytes for
	// publickey-authenticated switches.
	keyFiles map[string][]byte
}

// generateSwitchConfig generates the INI-format switch configuration for ironic-networking.
// It returns per-switch config entries and a map of key files for publickey switches.
// Switches with credential errors are skipped with a warning log instead of failing
// the entire config generation.
func generateSwitchConfig(ctx context.Context, c client.Client, namespace, credentialsPath string, logger logr.Logger) (*switchConfigResult, error) {
	// List all BareMetalSwitch resources in the namespace
	switchList := &metal3api.BareMetalSwitchList{}
	if err := c.List(ctx, switchList, client.InNamespace(namespace)); err != nil {
		return nil, fmt.Errorf("failed to list BareMetalSwitch resources: %w", err)
	}

	result := &switchConfigResult{
		configEntries: make(map[string][]byte),
		keyFiles:      make(map[string][]byte),
	}

	// Generate config for each switch
	for i := range switchList.Items {
		if err := writeSwitchEntry(ctx, c, &switchList.Items[i], credentialsPath, result.configEntries, result.keyFiles); err != nil {
			var credErr *credentialConfigError
			if errors.As(err, &credErr) {
				logger.Info("skipping switch due to credential error",
					"switch", switchList.Items[i].Name, "error", err)
				continue
			}
			return nil, fmt.Errorf("failed to generate config for switch %s: %w", switchList.Items[i].Name, err)
		}
	}

	return result, nil
}

// writeSwitchEntry generates a single switch's INI config entry and adds it
// to configEntries (keyed by switch name). For publickey-authenticated switches,
// it also adds the SSH private key to keyFiles (keyed by "<mac-address>.key")
// and emits a key_file= directive in the config instead of password=.
func writeSwitchEntry(ctx context.Context, c client.Client, sw *metal3api.BareMetalSwitch, credentialsPath string, configEntries map[string][]byte, keyFiles map[string][]byte) error {
	var buf bytes.Buffer

	// Section header: [switch:name]
	fmt.Fprintf(&buf, "[switch:%s]\n", sw.Name)

	// Switch address
	fmt.Fprintf(&buf, "address=%s\n", sw.Spec.Address)

	// MAC address
	fmt.Fprintf(&buf, "mac_address=%s\n", sw.Spec.MACAddress)

	// Port (optional, defaults based on device type)
	if sw.Spec.Port != nil {
		fmt.Fprintf(&buf, "port=%d\n", *sw.Spec.Port)
	}

	// Driver type (CRD defaults to "generic-switch", but defend against empty)
	driverType := sw.Spec.Driver
	if driverType == "" {
		driverType = "generic-switch"
	}
	fmt.Fprintf(&buf, "driver_type=%s\n", driverType)

	// Device type (required)
	fmt.Fprintf(&buf, "device_type=%s\n", sw.Spec.DeviceType)

	// Certificate verification (optional)
	if sw.Spec.DisableCertificateVerification != nil {
		fmt.Fprintf(&buf, "insecure=%s\n",
			strconv.FormatBool(*sw.Spec.DisableCertificateVerification))
	}

	// Fetch credentials from secret
	secret := &corev1.Secret{}
	secretKey := types.NamespacedName{
		Namespace: sw.Namespace,
		Name:      sw.Spec.Credentials.SecretName,
	}
	if err := c.Get(ctx, secretKey, secret); err != nil {
		if k8serrors.IsNotFound(err) {
			return &credentialConfigError{msg: fmt.Sprintf("credentials secret %s/%s not found",
				sw.Namespace, sw.Spec.Credentials.SecretName)}
		}
		return fmt.Errorf("failed to get credentials secret %s/%s: %w",
			sw.Namespace, sw.Spec.Credentials.SecretName, err)
	}

	// Username (required for all credential types)
	username, ok := secret.Data["username"]
	if !ok {
		return &credentialConfigError{msg: fmt.Sprintf("credentials secret %s missing 'username' key", sw.Spec.Credentials.SecretName)}
	}
	fmt.Fprintf(&buf, "username=%s\n", string(username))

	// Credentials: branch on type
	if sw.Spec.Credentials.Type == metal3api.SwitchCredentialTypePublicKey {
		// Public key authentication
		privateKey, ok := secret.Data["ssh-privatekey"]
		if !ok {
			return &credentialConfigError{msg: fmt.Sprintf("credentials secret %s missing 'ssh-privatekey' key for publickey auth", sw.Spec.Credentials.SecretName)}
		}

		// Store the private key in the key files map, keyed by MAC address.
		// Replace colons with dashes because colons are not valid in
		// Kubernetes secret data keys.
		keyFileName := strings.ReplaceAll(sw.Spec.MACAddress, ":", "-") + ".key"
		keyFiles[keyFileName] = privateKey

		// Emit key_file= with the absolute path where the credentials secret will be mounted
		fmt.Fprintf(&buf, "key_file=%s\n", filepath.Join(credentialsPath, keyFileName))
	} else {
		// Password authentication (default)
		password, ok := secret.Data["password"]
		if !ok {
			return &credentialConfigError{msg: fmt.Sprintf("credentials secret %s missing 'password' key", sw.Spec.Credentials.SecretName)}
		}
		fmt.Fprintf(&buf, "password=%s\n", string(password))
	}

	// Blank line between sections
	fmt.Fprintln(&buf)

	configEntries[sw.Name] = buf.Bytes()
	return nil
}

// updateSwitchConfigSecret generates switch configuration from BareMetalSwitch CRDs
// and updates both the switch config secret and the switch credentials secret.
func updateSwitchConfigSecret(ctx context.Context, c client.Client, namespace, configSecretName, credentialSecretName, credentialPath string, logger logr.Logger) error {
	// Generate the switch config from BareMetalSwitch CRDs
	result, err := generateSwitchConfig(ctx, c, namespace, credentialPath, logger)
	if err != nil {
		return err
	}

	// Concatenate per-switch config entries into a single INI blob,
	// sorted by switch name for deterministic output.
	switchNames := make([]string, 0, len(result.configEntries))
	for name := range result.configEntries {
		switchNames = append(switchNames, name)
	}
	sort.Strings(switchNames)

	var configBuf bytes.Buffer
	configBuf.WriteString("# This file is managed by the Baremetal Operator\n\n")
	for _, name := range switchNames {
		configBuf.Write(result.configEntries[name])
	}

	// Update the switch configs secret
	if err := updateSecretData(ctx, c, namespace, configSecretName, map[string][]byte{
		switchConfigKey: configBuf.Bytes(),
	}); err != nil {
		return fmt.Errorf("failed to update switch configs secret: %w", err)
	}

	// Update the switch credentials secret
	if err := updateSecretData(ctx, c, namespace, credentialSecretName, result.keyFiles); err != nil {
		return fmt.Errorf("failed to update switch credentials secret: %w", err)
	}

	return nil
}

// updateSecretData updates a secret's data if it has changed. The secret's
// data is fully replaced with newData.
func updateSecretData(ctx context.Context, c client.Client, namespace, secretName string, newData map[string][]byte) error {
	secret := &corev1.Secret{}
	if err := c.Get(ctx, types.NamespacedName{
		Name:      secretName,
		Namespace: namespace,
	}, secret); err != nil {
		return fmt.Errorf("failed to get secret %s: %w", secretName, err)
	}

	// Only update if the data has changed
	if secretDataEqual(secret.Data, newData) {
		return nil
	}

	secret.Data = newData

	if err := c.Update(ctx, secret); err != nil {
		return fmt.Errorf("failed to update secret %s: %w", secretName, err)
	}

	return nil
}

// secretDataEqual compares two secret data maps for equality.
func secretDataEqual(a, b map[string][]byte) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || !bytes.Equal(v, bv) {
			return false
		}
	}
	return true
}
