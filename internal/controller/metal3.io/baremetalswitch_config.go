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
	"strings"
	"text/template"

	"github.com/go-logr/logr"
	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/secretutils"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const switchConfigKey = "switch-configs.conf"

// credentialConfigError indicates a switch's credentials secret exists but
// is misconfigured (e.g., required key missing). The Owns() watch will
// trigger a reconcile when the user fixes the secret data.
type credentialConfigError struct {
	msg string
}

func (e *credentialConfigError) Error() string { return e.msg }

// credentialSecretNotFoundError indicates a switch's credentials secret
// does not exist. The Owns() watch cannot detect creation of a secret
// that has no owner reference, so the controller must requeue.
type credentialSecretNotFoundError struct {
	msg string
}

func (e *credentialSecretNotFoundError) Error() string { return e.msg }

// switchConfigResult holds the per-switch config entries and any collected
// SSH private key files for publickey-authenticated switches. Both maps
// are keyed per-switch: configEntries by switch name, keyFiles by MAC address.
type switchConfigResult struct {
	// configEntries maps switch name to its INI-format configuration section.
	configEntries map[string][]byte
	// keyFiles maps "<mac-address>.key" to SSH private key bytes for
	// publickey-authenticated switches.
	keyFiles map[string][]byte
	// credentialErrors maps switch name to the credential error that caused
	// the switch to be skipped during config generation. Values are either
	// *credentialConfigError or *credentialSecretNotFoundError.
	credentialErrors map[string]error
}

// generateSwitchConfig generates the INI-format switch configuration for ironic-networking.
// It returns per-switch config entries and a map of key files for publickey switches.
// Switches with credential errors are skipped with a warning log instead of failing
// the entire config generation.
func generateSwitchConfig(ctx context.Context, c client.Client, sm secretutils.SecretManager, namespace, credentialsPath string, logger logr.Logger) (*switchConfigResult, error) {
	// List all BareMetalSwitch resources in the namespace
	switchList := &metal3api.BareMetalSwitchList{}
	if err := c.List(ctx, switchList, client.InNamespace(namespace)); err != nil {
		return nil, fmt.Errorf("failed to list BareMetalSwitch resources: %w", err)
	}

	result := &switchConfigResult{
		configEntries:    make(map[string][]byte),
		keyFiles:         make(map[string][]byte),
		credentialErrors: make(map[string]error),
	}

	// Generate config for each switch
	for i := range switchList.Items {
		if err := writeSwitchEntry(ctx, sm, &switchList.Items[i], credentialsPath, result.configEntries, result.keyFiles); err != nil {
			if errors.As(err, new(*credentialConfigError)) || errors.As(err, new(*credentialSecretNotFoundError)) {
				logger.Info("skipping switch due to credential error",
					"switch", switchList.Items[i].Name, "error", err)
				result.credentialErrors[switchList.Items[i].Name] = err
				continue
			}
			return nil, fmt.Errorf("failed to generate config for switch %s: %w", switchList.Items[i].Name, err)
		}
	}

	return result, nil
}

// switchConfigData holds the template data for generating a switch's INI config entry.
type switchConfigData struct {
	Name         string
	Address      string
	MACAddress   string
	Port         *int32
	DriverType   string
	DeviceType   string
	Insecure     *bool
	Username     string
	Password     string //nolint: gosec
	KeyFile      string
	EnableSecret string
}

// switchConfigTemplate is the INI-format template for a single switch config section.
var switchConfigTemplate = template.Must(template.New("switchConfig").Parse(
	`[switch:{{.Name}}]
address={{.Address}}
mac_address={{.MACAddress}}
{{- if .Port}}
port={{.Port}}
{{- end}}
driver_type={{.DriverType}}
device_type={{.DeviceType}}
{{- if .Insecure}}
insecure={{.Insecure}}
{{- end}}
username={{.Username}}
{{- if .KeyFile}}
key_file={{.KeyFile}}
{{- else}}
password={{.Password}}
{{- end}}
{{- if .EnableSecret}}
enable_secret={{.EnableSecret}}
{{- end}}

`))

// writeSwitchEntry generates a single switch's INI config entry and adds it
// to configEntries (keyed by switch name). For publickey-authenticated switches,
// it also adds the SSH private key to keyFiles (keyed by "<mac-address>.key")
// and emits a key_file= directive in the config instead of password=.
func writeSwitchEntry(ctx context.Context, sm secretutils.SecretManager, sw *metal3api.BareMetalSwitch, credentialsPath string, configEntries map[string][]byte, keyFiles map[string][]byte) error {
	// Fetch credentials from secret
	secretKey := types.NamespacedName{
		Namespace: sw.Namespace,
		Name:      sw.Spec.Credentials.SecretName,
	}
	secret, err := sm.AcquireSecret(ctx, secretKey, sw, false)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return &credentialSecretNotFoundError{
				msg: fmt.Sprintf("credentials secret %s/%s not found", sw.Namespace, sw.Spec.Credentials.SecretName),
			}
		}
		return fmt.Errorf("failed to get credentials secret %s/%s: %w",
			sw.Namespace, sw.Spec.Credentials.SecretName, err)
	}

	// Username (required for all credential types)
	username, ok := secret.Data["username"]
	if !ok {
		return &credentialConfigError{msg: fmt.Sprintf("credentials secret %s missing 'username' key", sw.Spec.Credentials.SecretName)}
	}

	// Driver type (CRD defaults to "generic-switch", but defend against empty)
	driverType := sw.Spec.Driver
	if driverType == "" {
		driverType = "generic-switch"
	}

	data := switchConfigData{
		Name:       sw.Name,
		Address:    sw.Spec.Address,
		MACAddress: sw.Spec.MACAddress,
		Port:       sw.Spec.Port,
		DriverType: driverType,
		DeviceType: sw.Spec.DeviceType,
		Insecure:   sw.Spec.DisableCertificateVerification,
		Username:   string(username),
	}

	// Credentials: branch on type
	if sw.Spec.Credentials.Type == metal3api.SwitchCredentialTypePublicKey {
		privateKey, ok := secret.Data["ssh-privatekey"]
		if !ok {
			return &credentialConfigError{msg: fmt.Sprintf("credentials secret %s missing 'ssh-privatekey' key for publickey auth", sw.Spec.Credentials.SecretName)}
		}

		// Store the private key in the key files map, keyed by MAC address.
		// Replace colons with dashes because colons are not valid in
		// Kubernetes secret data keys.
		keyFileName := strings.ReplaceAll(sw.Spec.MACAddress, ":", "-") + ".key"
		keyFiles[keyFileName] = privateKey

		data.KeyFile = filepath.Join(credentialsPath, keyFileName)
	} else {
		password, ok := secret.Data["password"]
		if !ok {
			return &credentialConfigError{msg: fmt.Sprintf("credentials secret %s missing 'password' key", sw.Spec.Credentials.SecretName)}
		}
		data.Password = string(password)
	}

	if enableSecret, ok := secret.Data["enable-secret"]; ok {
		data.EnableSecret = string(enableSecret)
	}

	var buf bytes.Buffer
	if err := switchConfigTemplate.Execute(&buf, data); err != nil {
		return fmt.Errorf("failed to render switch config template for %s: %w", sw.Name, err)
	}

	configEntries[sw.Name] = buf.Bytes()
	return nil
}

// updateSwitchConfigSecret generates switch configuration from BareMetalSwitch CRDs
// and updates both the switch config secret and the switch credentials secret.
// It returns the switchConfigResult so the caller can inspect per-switch credential errors.
func updateSwitchConfigSecret(ctx context.Context, c client.Client, sm secretutils.SecretManager, namespace, configSecretName, credentialSecretName, credentialPath string, logger logr.Logger) (*switchConfigResult, error) {
	// Generate the switch config from BareMetalSwitch CRDs
	result, err := generateSwitchConfig(ctx, c, sm, namespace, credentialPath, logger)
	if err != nil {
		return nil, err
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
	if err := updateSecretData(ctx, c, sm, namespace, configSecretName, map[string][]byte{
		switchConfigKey: configBuf.Bytes(),
	}); err != nil {
		return nil, fmt.Errorf("failed to update switch configs secret: %w", err)
	}

	// Update the switch credentials secret
	if err := updateSecretData(ctx, c, sm, namespace, credentialSecretName, result.keyFiles); err != nil {
		return nil, fmt.Errorf("failed to update switch credentials secret: %w", err)
	}

	return result, nil
}

// updateSecretData updates a secret's data if it has changed. The secret's
// data is fully replaced with newData. The SecretManager is used to fetch the
// secret so that unlabelled secrets are found via the API reader and
// automatically labelled for the filtered cache.
func updateSecretData(ctx context.Context, c client.Client, sm secretutils.SecretManager, namespace, secretName string, newData map[string][]byte) error {
	secret, err := sm.ObtainSecret(ctx, types.NamespacedName{
		Name:      secretName,
		Namespace: namespace,
	})
	if err != nil {
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
