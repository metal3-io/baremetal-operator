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
	"fmt"
	"testing"

	"github.com/go-logr/logr"
	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	testSwitchConfigsSecretName    = "metal3-switch-configs"
	testSwitchCredentialSecretName = "metal3-switch-credentials"
	testSwitchCredentialPath       = "/etc/ironic/switch-credentials"
)

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

			configEntries := make(map[string][]byte)
			keyFiles := make(map[string][]byte)
			err := writeSwitchEntry(t.Context(), c, tt.sw, tt.credentialPath, configEntries, keyFiles)

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
				&metal3api.BareMetalSwitch{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "switch2",
						Namespace: "test-ns",
					},
					Spec: metal3api.BareMetalSwitchSpec{
						Address:    "192.168.1.2",
						MACAddress: "aa:bb:cc:dd:ee:ff",
						DeviceType: "netmiko_dell_os10",
						Credentials: metal3api.SwitchCredentials{
							Type:       "publickey",
							SecretName: "switch2-creds",
						},
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "switch1-creds",
						Namespace: "test-ns",
					},
					Data: map[string][]byte{
						"username": []byte("admin1"),
						"password": []byte("secret1"),
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "switch2-creds",
						Namespace: "test-ns",
					},
					Data: map[string][]byte{
						"username":       []byte("admin2"),
						"ssh-privatekey": []byte("private-key-data"),
					},
				},
			},
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

			result, err := generateSwitchConfig(t.Context(), c, tt.namespace, tt.credentialPath, logr.Discard())

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
		apiErr := fmt.Errorf("connection refused")
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

		_, err := generateSwitchConfig(t.Context(), c, "test-ns", "", logr.Discard())
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
		name                   string
		namespace              string
		objects                []client.Object
		expectError            bool
		expectedConfig         string
		expectedCredentialKeys map[string][]byte
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
			expectedConfig: "# This file is managed by the Baremetal Operator\n\n",
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
		},
		{
			name:      "mixed password and publickey updates both secrets",
			namespace: "test-ns",
			objects: []client.Object{
				&metal3api.BareMetalSwitch{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "switch-pw",
						Namespace: "test-ns",
					},
					Spec: metal3api.BareMetalSwitchSpec{
						Address:    "192.168.1.1",
						MACAddress: "00:00:5e:00:53:01",
						DeviceType: "cisco_ios",
						Credentials: metal3api.SwitchCredentials{
							Type:       "password",
							SecretName: "switch-pw-creds",
						},
					},
				},
				&metal3api.BareMetalSwitch{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "switch-pk",
						Namespace: "test-ns",
					},
					Spec: metal3api.BareMetalSwitchSpec{
						Address:    "192.168.1.2",
						MACAddress: "aa:bb:cc:dd:ee:ff",
						DeviceType: "netmiko_dell_os10",
						Credentials: metal3api.SwitchCredentials{
							Type:       "publickey",
							SecretName: "switch-pk-creds",
						},
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "switch-pw-creds",
						Namespace: "test-ns",
					},
					Data: map[string][]byte{
						"username": []byte("admin"),
						"password": []byte("pass123"),
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "switch-pk-creds",
						Namespace: "test-ns",
					},
					Data: map[string][]byte{
						"username":       []byte("sshuser"),
						"ssh-privatekey": []byte("my-private-key"),
					},
				},
			},
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

			err := updateSwitchConfigSecret(t.Context(), c, tt.namespace, testSwitchConfigsSecretName, testSwitchCredentialSecretName, testSwitchCredentialPath, logr.Discard())

			if tt.expectError {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())

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

func TestFindSwitchesForSecret(t *testing.T) {
	g := NewWithT(t)
	scheme := runtime.NewScheme()
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())
	g.Expect(metal3api.AddToScheme(scheme)).To(Succeed())

	tests := []struct {
		name             string
		secret           *corev1.Secret
		switches         []client.Object
		expectedRequests []reconcile.Request
	}{
		{
			name: "matches switches referencing the secret",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "shared-creds",
					Namespace: "test-ns",
				},
			},
			switches: []client.Object{
				&metal3api.BareMetalSwitch{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "switch1",
						Namespace: "test-ns",
					},
					Spec: metal3api.BareMetalSwitchSpec{
						Address:    "192.168.1.1",
						DeviceType: "cisco_ios",
						Credentials: metal3api.SwitchCredentials{
							SecretName: "shared-creds",
						},
					},
				},
				&metal3api.BareMetalSwitch{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "switch2",
						Namespace: "test-ns",
					},
					Spec: metal3api.BareMetalSwitchSpec{
						Address:    "192.168.1.2",
						DeviceType: "dell_os10",
						Credentials: metal3api.SwitchCredentials{
							SecretName: "other-creds",
						},
					},
				},
				&metal3api.BareMetalSwitch{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "switch3",
						Namespace: "test-ns",
					},
					Spec: metal3api.BareMetalSwitchSpec{
						Address:    "192.168.1.3",
						DeviceType: "cisco_ios",
						Credentials: metal3api.SwitchCredentials{
							SecretName: "shared-creds",
						},
					},
				},
			},
			expectedRequests: []reconcile.Request{
				{NamespacedName: types.NamespacedName{Name: "switch1", Namespace: "test-ns"}},
				{NamespacedName: types.NamespacedName{Name: "switch3", Namespace: "test-ns"}},
			},
		},
		{
			name: "no matches when no switches reference the secret",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "unused-creds",
					Namespace: "test-ns",
				},
			},
			switches: []client.Object{
				&metal3api.BareMetalSwitch{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "switch1",
						Namespace: "test-ns",
					},
					Spec: metal3api.BareMetalSwitchSpec{
						Address:    "192.168.1.1",
						DeviceType: "cisco_ios",
						Credentials: metal3api.SwitchCredentials{
							SecretName: "other-creds",
						},
					},
				},
			},
			expectedRequests: nil,
		},
		{
			name: "no matches when no switches exist",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "some-creds",
					Namespace: "test-ns",
				},
			},
			switches:         []client.Object{},
			expectedRequests: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tt.switches...).Build()
			r := &BareMetalSwitchReconciler{
				Client: c,
				Log:    ctrl.Log.WithName("test"),
			}

			requests := r.findSwitchesForSecret(t.Context(), tt.secret)
			g.Expect(requests).To(Equal(tt.expectedRequests))
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
