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
	"context"
	"errors"
	"testing"

	"github.com/go-logr/logr"
	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/secretutils"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

const (
	testSwitchConfigsSecretName    = "metal3-switch-configs"
	testSwitchCredentialSecretName = "metal3-switch-credentials"
	testSwitchCredentialPath       = "/etc/ironic/switch-credentials"
)

// newMixedAuthSwitchObjects creates a pair of switches (one password, one publickey)
// with their corresponding credential secrets for testing.
func newMixedAuthSwitchObjects(pwName, pkName, pwSecretName, pkSecretName string, pwCreds, pkCreds map[string][]byte) []client.Object {
	return []client.Object{
		&metal3api.BareMetalSwitch{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pwName,
				Namespace: "test-ns",
			},
			Spec: metal3api.BareMetalSwitchSpec{
				Address:    "192.168.1.1",
				MACAddress: "00:00:5e:00:53:01",
				DeviceType: "cisco_ios",
				Credentials: metal3api.SwitchCredentials{
					Type:       "password",
					SecretName: pwSecretName,
				},
			},
		},
		&metal3api.BareMetalSwitch{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pkName,
				Namespace: "test-ns",
			},
			Spec: metal3api.BareMetalSwitchSpec{
				Address:    "192.168.1.2",
				MACAddress: "aa:bb:cc:dd:ee:ff",
				DeviceType: "netmiko_dell_os10",
				Credentials: metal3api.SwitchCredentials{
					Type:       "publickey",
					SecretName: pkSecretName,
				},
			},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pwSecretName,
				Namespace: "test-ns",
			},
			Data: pwCreds,
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pkSecretName,
				Namespace: "test-ns",
			},
			Data: pkCreds,
		},
	}
}

func TestWriteSwitchEntry(t *testing.T) {
	g := NewWithT(t)
	scheme := runtime.NewScheme()
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())
	g.Expect(metal3api.AddToScheme(scheme)).To(Succeed())

	tests := []struct {
		name           string
		sw             *metal3api.BareMetalSwitch
		secret         *corev1.Secret
		credentialPath string
		expectedConfig string
		expectedKeys   map[string][]byte
		expectError    bool
	}{
		{
			name: "basic switch with password auth",
			sw: &metal3api.BareMetalSwitch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "switch1",
					Namespace: "test-ns",
				},
				Spec: metal3api.BareMetalSwitchSpec{
					Address:    "192.168.1.1",
					MACAddress: "00:00:5e:00:53:01",
					DeviceType: "cisco_ios",
					Credentials: metal3api.SwitchCredentials{
						Type:       "password",
						SecretName: "switch1-creds",
					},
				},
			},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "switch1-creds",
					Namespace: "test-ns",
				},
				Data: map[string][]byte{
					"username": []byte("admin"),
					"password": []byte("secret123"),
				},
			},
			expectedConfig: `[switch:switch1]
address=192.168.1.1
mac_address=00:00:5e:00:53:01
driver_type=generic-switch
device_type=cisco_ios
username=admin
password=secret123

`,
			expectedKeys: map[string][]byte{},
			expectError:  false,
		},
		{
			name: "switch with all optional fields",
			sw: &metal3api.BareMetalSwitch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "switch2",
					Namespace: "test-ns",
				},
				Spec: metal3api.BareMetalSwitchSpec{
					Address:                        "switch.example.com",
					DeviceType:                     "dell_os10",
					Driver:                         "generic-switch",
					Port:                           ptr.To(int32(8443)),
					MACAddress:                     "aa:bb:cc:dd:ee:ff",
					DisableCertificateVerification: ptr.To(true),
					Credentials: metal3api.SwitchCredentials{
						Type:       "password",
						SecretName: "switch2-creds",
					},
				},
			},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "switch2-creds",
					Namespace: "test-ns",
				},
				Data: map[string][]byte{
					"username": []byte("switchuser"),
					"password": []byte("p@ssw0rd"),
				},
			},
			expectedConfig: `[switch:switch2]
address=switch.example.com
mac_address=aa:bb:cc:dd:ee:ff
port=8443
driver_type=generic-switch
device_type=dell_os10
insecure=true
username=switchuser
password=p@ssw0rd

`,
			expectedKeys: map[string][]byte{},
			expectError:  false,
		},
		{
			name: "switch with publickey auth",
			sw: &metal3api.BareMetalSwitch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "switch-pubkey",
					Namespace: "test-ns",
				},
				Spec: metal3api.BareMetalSwitchSpec{
					Address:    "192.168.1.10",
					MACAddress: "aa:bb:cc:dd:ee:ff",
					DeviceType: "netmiko_dell_os10",
					Credentials: metal3api.SwitchCredentials{
						Type:       "publickey",
						SecretName: "switch-pubkey-creds",
					},
				},
			},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "switch-pubkey-creds",
					Namespace: "test-ns",
				},
				Data: map[string][]byte{
					"username":       []byte("admin"),
					"ssh-privatekey": []byte("-----BEGIN RSA PRIVATE KEY-----\ntest-key-data\n-----END RSA PRIVATE KEY-----\n"),
				},
			},
			credentialPath: testSwitchCredentialPath,
			expectedConfig: `[switch:switch-pubkey]
address=192.168.1.10
mac_address=aa:bb:cc:dd:ee:ff
driver_type=generic-switch
device_type=netmiko_dell_os10
username=admin
key_file=/etc/ironic/switch-credentials/aa-bb-cc-dd-ee-ff.key

`,
			expectedKeys: map[string][]byte{
				"aa-bb-cc-dd-ee-ff.key": []byte("-----BEGIN RSA PRIVATE KEY-----\ntest-key-data\n-----END RSA PRIVATE KEY-----\n"),
			},
			expectError: false,
		},
		{
			name: "switch with enable-secret",
			sw: &metal3api.BareMetalSwitch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "switch-enable",
					Namespace: "test-ns",
				},
				Spec: metal3api.BareMetalSwitchSpec{
					Address:    "192.168.1.20",
					MACAddress: "00:00:5e:00:53:20",
					DeviceType: "cisco_ios",
					Credentials: metal3api.SwitchCredentials{
						Type:       "password",
						SecretName: "switch-enable-creds",
					},
				},
			},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "switch-enable-creds",
					Namespace: "test-ns",
				},
				Data: map[string][]byte{
					"username":      []byte("admin"),
					"password":      []byte("secret123"),
					"enable-secret": []byte("enablepass"),
				},
			},
			expectedConfig: `[switch:switch-enable]
address=192.168.1.20
mac_address=00:00:5e:00:53:20
driver_type=generic-switch
device_type=cisco_ios
username=admin
password=secret123
enable_secret=enablepass

`,
			expectedKeys: map[string][]byte{},
			expectError:  false,
		},
		{
			name: "publickey auth missing ssh-privatekey",
			sw: &metal3api.BareMetalSwitch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "switch-bad",
					Namespace: "test-ns",
				},
				Spec: metal3api.BareMetalSwitchSpec{
					Address:    "192.168.1.11",
					MACAddress: "11:22:33:44:55:66",
					DeviceType: "cisco_ios",
					Credentials: metal3api.SwitchCredentials{
						Type:       "publickey",
						SecretName: "switch-bad-creds",
					},
				},
			},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "switch-bad-creds",
					Namespace: "test-ns",
				},
				Data: map[string][]byte{
					"username": []byte("admin"),
				},
			},
			credentialPath: testSwitchCredentialPath,
			expectError:    true,
		},
		{
			name: "missing username in secret",
			sw: &metal3api.BareMetalSwitch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "switch3",
					Namespace: "test-ns",
				},
				Spec: metal3api.BareMetalSwitchSpec{
					Address:    "192.168.1.3",
					DeviceType: "cisco_ios",
					Credentials: metal3api.SwitchCredentials{
						Type:       "password",
						SecretName: "switch3-creds",
					},
				},
			},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "switch3-creds",
					Namespace: "test-ns",
				},
				Data: map[string][]byte{
					"password": []byte("secret123"),
				},
			},
			expectError: true,
		},
		{
			name: "missing password in secret",
			sw: &metal3api.BareMetalSwitch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "switch4",
					Namespace: "test-ns",
				},
				Spec: metal3api.BareMetalSwitchSpec{
					Address:    "192.168.1.4",
					DeviceType: "cisco_ios",
					Credentials: metal3api.SwitchCredentials{
						Type:       "password",
						SecretName: "switch4-creds",
					},
				},
			},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "switch4-creds",
					Namespace: "test-ns",
				},
				Data: map[string][]byte{
					"username": []byte("admin"),
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tt.secret).Build()
			sm := secretutils.NewSecretManager(logr.Discard(), c, c)

			configEntries := make(map[string][]byte)
			keyFiles := make(map[string][]byte)
			err := writeSwitchEntry(t.Context(), sm, tt.sw, tt.credentialPath, configEntries, keyFiles)

			if tt.expectError {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(configEntries).To(HaveKey(tt.sw.Name))
				g.Expect(string(configEntries[tt.sw.Name])).To(Equal(tt.expectedConfig))
				g.Expect(keyFiles).To(Equal(tt.expectedKeys))
			}
		})
	}
}

func TestGenerateSwitchConfig(t *testing.T) {
	g := NewWithT(t)
	scheme := runtime.NewScheme()
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())
	g.Expect(metal3api.AddToScheme(scheme)).To(Succeed())

	tests := []struct {
		name                  string
		namespace             string
		objects               []client.Object
		credentialPath        string
		expectedConfigEntries map[string]string
		expectedKeys          map[string][]byte
		expectError           bool
	}{
		{
			name:                  "no switches configured",
			namespace:             "test-ns",
			objects:               []client.Object{},
			expectedConfigEntries: map[string]string{},
			expectedKeys:          map[string][]byte{},
			expectError:           false,
		},
		{
			name:      "single password switch",
			namespace: "test-ns",
			objects: []client.Object{
				&metal3api.BareMetalSwitch{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "switch1",
						Namespace: "test-ns",
					},
					Spec: metal3api.BareMetalSwitchSpec{
						Address:    "192.168.1.1",
						MACAddress: "00:00:5e:00:53:01",
						DeviceType: "cisco_ios",
						Credentials: metal3api.SwitchCredentials{
							Type:       "password",
							SecretName: "switch1-creds",
						},
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "switch1-creds",
						Namespace: "test-ns",
					},
					Data: map[string][]byte{
						"username": []byte("admin"),
						"password": []byte("secret123"),
					},
				},
			},
			expectedConfigEntries: map[string]string{
				"switch1": `[switch:switch1]
address=192.168.1.1
mac_address=00:00:5e:00:53:01
driver_type=generic-switch
device_type=cisco_ios
username=admin
password=secret123

`,
			},
			expectedKeys: map[string][]byte{},
			expectError:  false,
		},
		{
			name:      "mixed password and publickey switches",
			namespace: "test-ns",
			objects: newMixedAuthSwitchObjects(
				"switch1", "switch2", "switch1-creds", "switch2-creds",
				map[string][]byte{"username": []byte("admin1"), "password": []byte("secret1")},
				map[string][]byte{"username": []byte("admin2"), "ssh-privatekey": []byte("private-key-data")},
			),
			credentialPath: testSwitchCredentialPath,
			expectedConfigEntries: map[string]string{
				"switch1": `[switch:switch1]
address=192.168.1.1
mac_address=00:00:5e:00:53:01
driver_type=generic-switch
device_type=cisco_ios
username=admin1
password=secret1

`,
				"switch2": `[switch:switch2]
address=192.168.1.2
mac_address=aa:bb:cc:dd:ee:ff
driver_type=generic-switch
device_type=netmiko_dell_os10
username=admin2
key_file=/etc/ironic/switch-credentials/aa-bb-cc-dd-ee-ff.key

`,
			},
			expectedKeys: map[string][]byte{
				"aa-bb-cc-dd-ee-ff.key": []byte("private-key-data"),
			},
			expectError: false,
		},
		{
			name:      "valid switches produced when one switch has missing credentials",
			namespace: "test-ns",
			objects: []client.Object{
				&metal3api.BareMetalSwitch{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "good-switch",
						Namespace: "test-ns",
					},
					Spec: metal3api.BareMetalSwitchSpec{
						Address:    "192.168.1.1",
						MACAddress: "00:00:5e:00:53:01",
						DeviceType: "cisco_ios",
						Credentials: metal3api.SwitchCredentials{
							Type:       "password",
							SecretName: "good-creds",
						},
					},
				},
				&metal3api.BareMetalSwitch{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "bad-switch",
						Namespace: "test-ns",
					},
					Spec: metal3api.BareMetalSwitchSpec{
						Address:    "192.168.1.2",
						MACAddress: "00:00:5e:00:53:02",
						DeviceType: "cisco_ios",
						Credentials: metal3api.SwitchCredentials{
							Type:       "password",
							SecretName: "missing-creds",
						},
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "good-creds",
						Namespace: "test-ns",
					},
					Data: map[string][]byte{
						"username": []byte("admin"),
						"password": []byte("secret123"),
					},
				},
			},
			expectedConfigEntries: map[string]string{
				"good-switch": `[switch:good-switch]
address=192.168.1.1
mac_address=00:00:5e:00:53:01
driver_type=generic-switch
device_type=cisco_ios
username=admin
password=secret123

`,
			},
			expectedKeys: map[string][]byte{},
			expectError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tt.objects...).Build()
			sm := secretutils.NewSecretManager(logr.Discard(), c, c)

			result, err := generateSwitchConfig(t.Context(), c, sm, tt.namespace, tt.credentialPath, logr.Discard())

			if tt.expectError {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(result.configEntries).To(HaveLen(len(tt.expectedConfigEntries)))
				for name, expected := range tt.expectedConfigEntries {
					g.Expect(result.configEntries).To(HaveKey(name))
					g.Expect(string(result.configEntries[name])).To(Equal(expected))
				}
				g.Expect(result.keyFiles).To(Equal(tt.expectedKeys))
			}
		})
	}

	t.Run("propagates API errors instead of skipping", func(t *testing.T) {
		g := NewWithT(t)
		apiErr := errors.New("connection refused")
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
			&metal3api.BareMetalSwitch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "switch1",
					Namespace: "test-ns",
				},
				Spec: metal3api.BareMetalSwitchSpec{
					Address:    "192.168.1.1",
					MACAddress: "00:00:5e:00:53:01",
					DeviceType: "cisco_ios",
					Credentials: metal3api.SwitchCredentials{
						Type:       "password",
						SecretName: "switch1-creds",
					},
				},
			},
		).WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, client client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if _, ok := obj.(*corev1.Secret); ok {
					return apiErr
				}
				return client.Get(ctx, key, obj, opts...)
			},
		}).Build()
		sm := secretutils.NewSecretManager(logr.Discard(), c, c)

		_, err := generateSwitchConfig(t.Context(), c, sm, "test-ns", "", logr.Discard())
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("connection refused"))
	})
}

func TestUpdateSwitchConfigSecret(t *testing.T) {
	g := NewWithT(t)
	scheme := runtime.NewScheme()
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())
	g.Expect(metal3api.AddToScheme(scheme)).To(Succeed())

	// Both secrets are always present (created by the deployment operator)
	newConfigSecret := func() *corev1.Secret {
		return &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      testSwitchConfigsSecretName,
				Namespace: "test-ns",
			},
			Data: map[string][]byte{},
		}
	}
	newCredentialSecret := func() *corev1.Secret {
		return &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      testSwitchCredentialSecretName,
				Namespace: "test-ns",
			},
			Data: map[string][]byte{},
		}
	}

	tests := []struct {
		name                     string
		namespace                string
		objects                  []client.Object
		expectError              bool
		expectedConfig           string
		expectedCredentialKeys   map[string][]byte
		expectedCredentialErrors map[string]error
	}{
		{
			name:      "updates existing secret with generated config",
			namespace: "test-ns",
			objects: []client.Object{
				&metal3api.BareMetalSwitch{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "switch1",
						Namespace: "test-ns",
					},
					Spec: metal3api.BareMetalSwitchSpec{
						Address:    "192.168.1.1",
						MACAddress: "00:00:5e:00:53:01",
						DeviceType: "cisco_ios",
						Credentials: metal3api.SwitchCredentials{
							Type:       "password",
							SecretName: "switch1-creds",
						},
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "switch1-creds",
						Namespace: "test-ns",
					},
					Data: map[string][]byte{
						"username": []byte("admin"),
						"password": []byte("secret123"),
					},
				},
			},
			expectError: false,
			expectedConfig: `# This file is managed by the Baremetal Operator

[switch:switch1]
address=192.168.1.1
mac_address=00:00:5e:00:53:01
driver_type=generic-switch
device_type=cisco_ios
username=admin
password=secret123

`,
			expectedCredentialKeys: map[string][]byte{},
		},
		{
			name:                   "no-op when no switches exist",
			namespace:              "test-ns",
			objects:                []client.Object{},
			expectError:            false,
			expectedConfig:         "# This file is managed by the Baremetal Operator\n\n",
			expectedCredentialKeys: map[string][]byte{},
		},
		{
			name:      "skips switch when credentials secret is missing",
			namespace: "test-ns",
			objects: []client.Object{
				&metal3api.BareMetalSwitch{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "switch3",
						Namespace: "test-ns",
					},
					Spec: metal3api.BareMetalSwitchSpec{
						Address:    "192.168.1.3",
						MACAddress: "00:00:5e:00:53:03",
						DeviceType: "cisco_ios",
						Credentials: metal3api.SwitchCredentials{
							Type:       "password",
							SecretName: "missing-secret",
						},
					},
				},
			},
			expectError:            false,
			expectedConfig:         "# This file is managed by the Baremetal Operator\n\n",
			expectedCredentialKeys: map[string][]byte{},
			expectedCredentialErrors: map[string]error{
				"switch3": &credentialSecretNotFoundError{msg: "credentials secret test-ns/missing-secret not found"},
			},
		},
		{
			name:      "mixed password and publickey updates both secrets",
			namespace: "test-ns",
			objects: newMixedAuthSwitchObjects(
				"switch-pw", "switch-pk", "switch-pw-creds", "switch-pk-creds",
				map[string][]byte{"username": []byte("admin"), "password": []byte("pass123")},
				map[string][]byte{"username": []byte("sshuser"), "ssh-privatekey": []byte("my-private-key")},
			),
			expectError: false,
			expectedConfig: `# This file is managed by the Baremetal Operator

[switch:switch-pk]
address=192.168.1.2
mac_address=aa:bb:cc:dd:ee:ff
driver_type=generic-switch
device_type=netmiko_dell_os10
username=sshuser
key_file=/etc/ironic/switch-credentials/aa-bb-cc-dd-ee-ff.key

[switch:switch-pw]
address=192.168.1.1
mac_address=00:00:5e:00:53:01
driver_type=generic-switch
device_type=cisco_ios
username=admin
password=pass123

`,
			expectedCredentialKeys: map[string][]byte{
				"aa-bb-cc-dd-ee-ff.key": []byte("my-private-key"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			objects := make([]client.Object, 0, len(tt.objects)+2)
			objects = append(objects, tt.objects...)
			objects = append(objects, newConfigSecret(), newCredentialSecret())

			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
			sm := secretutils.NewSecretManager(logr.Discard(), c, c)

			result, err := updateSwitchConfigSecret(t.Context(), c, sm, tt.namespace, testSwitchConfigsSecretName, testSwitchCredentialSecretName, testSwitchCredentialPath, logr.Discard())

			if tt.expectError {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())

				// Verify credential errors
				g.Expect(result.credentialErrors).To(HaveLen(len(tt.expectedCredentialErrors)))
				for name, expected := range tt.expectedCredentialErrors {
					g.Expect(result.credentialErrors).To(HaveKey(name))
					g.Expect(result.credentialErrors[name].Error()).To(Equal(expected.Error()))
				}

				// Verify the config secret data
				configSecret := &corev1.Secret{}
				err := c.Get(t.Context(), client.ObjectKey{
					Namespace: tt.namespace,
					Name:      testSwitchConfigsSecretName,
				}, configSecret)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(string(configSecret.Data[switchConfigKey])).To(Equal(tt.expectedConfig))

				// Verify the credentials secret data
				credSecret := &corev1.Secret{}
				err = c.Get(t.Context(), client.ObjectKey{
					Namespace: tt.namespace,
					Name:      testSwitchCredentialSecretName,
				}, credSecret)
				g.Expect(err).ToNot(HaveOccurred())
				if len(tt.expectedCredentialKeys) == 0 {
					g.Expect(credSecret.Data).To(BeEmpty())
				} else {
					g.Expect(credSecret.Data).To(Equal(tt.expectedCredentialKeys))
				}
			}
		})
	}
}

func TestReconcileConditions(t *testing.T) {
	g := NewWithT(t)
	scheme := runtime.NewScheme()
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())
	g.Expect(metal3api.AddToScheme(scheme)).To(Succeed())

	configSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testSwitchConfigsSecretName,
			Namespace: "test-ns",
		},
		Data: map[string][]byte{},
	}
	credentialSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testSwitchCredentialSecretName,
			Namespace: "test-ns",
		},
		Data: map[string][]byte{},
	}

	tests := []struct {
		name          string
		switch_       *metal3api.BareMetalSwitch
		objects       []client.Object
		expectRequeue bool
		expectStatus  metav1.ConditionStatus
		expectReason  string
		expectMessage string
	}{
		{
			name: "sets Reconciled=True on successful reconciliation",
			switch_: &metal3api.BareMetalSwitch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "good-switch",
					Namespace: "test-ns",
				},
				Spec: metal3api.BareMetalSwitchSpec{
					Address:    "192.168.1.1",
					MACAddress: "00:00:5e:00:53:01",
					DeviceType: "cisco_ios",
					Credentials: metal3api.SwitchCredentials{
						Type:       metal3api.SwitchCredentialTypePassword,
						SecretName: "good-creds",
					},
				},
			},
			objects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "good-creds",
						Namespace: "test-ns",
					},
					Data: map[string][]byte{
						"username": []byte("admin"),
						"password": []byte("secret"),
					},
				},
			},
			expectRequeue: false,
			expectStatus:  metav1.ConditionTrue,
			expectReason:  "Reconciled",
			expectMessage: "Switch configuration has been successfully reconciled into the config secret",
		},
		{
			name: "sets Reconciled=False when credential secret is missing",
			switch_: &metal3api.BareMetalSwitch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "missing-secret-switch",
					Namespace: "test-ns",
				},
				Spec: metal3api.BareMetalSwitchSpec{
					Address:    "192.168.1.1",
					MACAddress: "00:00:5e:00:53:01",
					DeviceType: "cisco_ios",
					Credentials: metal3api.SwitchCredentials{
						Type:       metal3api.SwitchCredentialTypePassword,
						SecretName: "nonexistent-creds",
					},
				},
			},
			objects:       []client.Object{},
			expectRequeue: true,
			expectStatus:  metav1.ConditionFalse,
			expectReason:  "CredentialError",
			expectMessage: "credentials secret test-ns/nonexistent-creds not found",
		},
		{
			name: "sets Reconciled=False when username key is missing",
			switch_: &metal3api.BareMetalSwitch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "bad-secret-switch",
					Namespace: "test-ns",
				},
				Spec: metal3api.BareMetalSwitchSpec{
					Address:    "192.168.1.1",
					MACAddress: "00:00:5e:00:53:01",
					DeviceType: "cisco_ios",
					Credentials: metal3api.SwitchCredentials{
						Type:       metal3api.SwitchCredentialTypePassword,
						SecretName: "bad-creds",
					},
				},
			},
			objects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "bad-creds",
						Namespace: "test-ns",
					},
					Data: map[string][]byte{
						"password": []byte("secret"),
					},
				},
			},
			expectRequeue: false,
			expectStatus:  metav1.ConditionFalse,
			expectReason:  "CredentialError",
			expectMessage: "missing 'username' key",
		},
		{
			name: "sets Reconciled=False when password key is missing",
			switch_: &metal3api.BareMetalSwitch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "no-password-switch",
					Namespace: "test-ns",
				},
				Spec: metal3api.BareMetalSwitchSpec{
					Address:    "192.168.1.1",
					MACAddress: "00:00:5e:00:53:01",
					DeviceType: "cisco_ios",
					Credentials: metal3api.SwitchCredentials{
						Type:       metal3api.SwitchCredentialTypePassword,
						SecretName: "no-password-creds",
					},
				},
			},
			objects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "no-password-creds",
						Namespace: "test-ns",
					},
					Data: map[string][]byte{
						"username": []byte("admin"),
					},
				},
			},
			expectRequeue: false,
			expectStatus:  metav1.ConditionFalse,
			expectReason:  "CredentialError",
			expectMessage: "missing 'password' key",
		},
		{
			name: "sets Reconciled=False when ssh-privatekey is missing for publickey auth",
			switch_: &metal3api.BareMetalSwitch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "no-key-switch",
					Namespace: "test-ns",
				},
				Spec: metal3api.BareMetalSwitchSpec{
					Address:    "192.168.1.1",
					MACAddress: "00:00:5e:00:53:01",
					DeviceType: "cisco_ios",
					Credentials: metal3api.SwitchCredentials{
						Type:       metal3api.SwitchCredentialTypePublicKey,
						SecretName: "no-key-creds",
					},
				},
			},
			objects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "no-key-creds",
						Namespace: "test-ns",
					},
					Data: map[string][]byte{
						"username": []byte("admin"),
					},
				},
			},
			expectRequeue: false,
			expectStatus:  metav1.ConditionFalse,
			expectReason:  "CredentialError",
			expectMessage: "missing 'ssh-privatekey' key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			objects := make([]client.Object, 0, 3+len(tt.objects))
			objects = append(objects, tt.switch_, configSecret.DeepCopy(), credentialSecret.DeepCopy())
			objects = append(objects, tt.objects...)

			c := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(objects...).
				WithStatusSubresource(&metal3api.BareMetalSwitch{}).
				Build()

			r := &BareMetalSwitchReconciler{
				Client:                     c,
				Log:                        logr.Discard(),
				APIReader:                  c,
				SwitchConfigsSecretName:    testSwitchConfigsSecretName,
				SwitchCredentialSecretName: testSwitchCredentialSecretName,
				SwitchCredentialPath:       testSwitchCredentialPath,
			}

			result, err := r.Reconcile(t.Context(), ctrl.Request{
				NamespacedName: client.ObjectKeyFromObject(tt.switch_),
			})
			g.Expect(err).ToNot(HaveOccurred())

			if tt.expectRequeue {
				g.Expect(result.RequeueAfter).To(BeNumerically(">", 0))
			} else {
				g.Expect(result.RequeueAfter).To(BeZero())
			}

			// Re-fetch the switch to get updated status
			updatedSwitch := &metal3api.BareMetalSwitch{}
			g.Expect(c.Get(t.Context(), client.ObjectKeyFromObject(tt.switch_), updatedSwitch)).To(Succeed())

			cond := meta.FindStatusCondition(updatedSwitch.Status.Conditions, string(metal3api.SwitchConditionReconciled))
			g.Expect(cond).ToNot(BeNil(), "Reconciled condition should be set")
			g.Expect(cond.Status).To(Equal(tt.expectStatus))
			g.Expect(cond.Reason).To(Equal(tt.expectReason))
			g.Expect(cond.Message).To(ContainSubstring(tt.expectMessage))
			g.Expect(cond.ObservedGeneration).To(Equal(updatedSwitch.Generation))
		})
	}
}

func TestSecretDataEqual(t *testing.T) {
	tests := []struct {
		name     string
		a        map[string][]byte
		b        map[string][]byte
		expected bool
	}{
		{
			name:     "both nil",
			a:        nil,
			b:        nil,
			expected: true,
		},
		{
			name:     "both empty",
			a:        map[string][]byte{},
			b:        map[string][]byte{},
			expected: true,
		},
		{
			name:     "equal data",
			a:        map[string][]byte{"key": []byte("value")},
			b:        map[string][]byte{"key": []byte("value")},
			expected: true,
		},
		{
			name:     "different values",
			a:        map[string][]byte{"key": []byte("value1")},
			b:        map[string][]byte{"key": []byte("value2")},
			expected: false,
		},
		{
			name:     "different keys",
			a:        map[string][]byte{"key1": []byte("value")},
			b:        map[string][]byte{"key2": []byte("value")},
			expected: false,
		},
		{
			name:     "different lengths",
			a:        map[string][]byte{"key": []byte("value")},
			b:        map[string][]byte{"key": []byte("value"), "key2": []byte("value2")},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(secretDataEqual(tt.a, tt.b)).To(Equal(tt.expected))
		})
	}
}
