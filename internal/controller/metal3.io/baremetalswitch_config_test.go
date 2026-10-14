/*
Copyright 2026 The Metal3 Authors.

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
	"time"

	"github.com/go-logr/logr"
	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/secretutils"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
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
				Credentials: &corev1.SecretReference{
					Name: pwSecretName,
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
				Credentials: &corev1.SecretReference{
					Name: pkSecretName,
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

func newDuplicateTestSwitch(name, namespace, address, macAddress, credentialsName string, uid types.UID, creationTimestamp metav1.Time) *metal3api.BareMetalSwitch {
	return &metal3api.BareMetalSwitch{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         namespace,
			UID:               uid,
			CreationTimestamp: creationTimestamp,
		},
		Spec: metal3api.BareMetalSwitchSpec{
			Address:    address,
			MACAddress: macAddress,
			DeviceType: "cisco_ios",
			Credentials: &corev1.SecretReference{
				Name: credentialsName,
			},
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
					Credentials: &corev1.SecretReference{
						Name: "switch1-creds",
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
					DisableCertificateVerification: true,
					Credentials: &corev1.SecretReference{
						Name: "switch2-creds",
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
					Credentials: &corev1.SecretReference{
						Name: "switch-pubkey-creds",
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
					"password":       []byte("unused-password"),
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
			name: "switch with admin-password",
			sw: &metal3api.BareMetalSwitch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "switch-enable",
					Namespace: "test-ns",
				},
				Spec: metal3api.BareMetalSwitchSpec{
					Address:    "192.168.1.20",
					MACAddress: "00:00:5e:00:53:20",
					DeviceType: "cisco_ios",
					Credentials: &corev1.SecretReference{
						Name: "switch-enable-creds",
					},
				},
			},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "switch-enable-creds",
					Namespace: "test-ns",
				},
				Data: map[string][]byte{
					"username":       []byte("admin"),
					"password":       []byte("secret123"),
					"admin-password": []byte("enablepass"),
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
			name: "missing username in secret",
			sw: &metal3api.BareMetalSwitch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "switch3",
					Namespace: "test-ns",
				},
				Spec: metal3api.BareMetalSwitchSpec{
					Address:    "192.168.1.3",
					DeviceType: "cisco_ios",
					Credentials: &corev1.SecretReference{
						Name: "switch3-creds",
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
			name: "missing password and ssh-privatekey in secret",
			sw: &metal3api.BareMetalSwitch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "switch4",
					Namespace: "test-ns",
				},
				Spec: metal3api.BareMetalSwitchSpec{
					Address:    "192.168.1.4",
					DeviceType: "cisco_ios",
					Credentials: &corev1.SecretReference{
						Name: "switch4-creds",
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

func TestValidateSingleLineINIValue(t *testing.T) {
	tests := []struct {
		name    string
		field   string
		value   string
		wantErr bool
	}{
		{name: "plain value", field: "username", value: "admin"},
		{name: "punctuation", field: "password", value: "p@ss=word!"},
		{name: "line feed", field: "username", value: "admin\nrole=admin", wantErr: true},
		{name: "carriage return", field: "password", value: "secret\rrole=admin", wantErr: true},
		{name: "null byte", field: "admin-password", value: "enable\x00secret", wantErr: true},
		{name: "delete character", field: "username", value: "admin\x7f", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			err := validateSingleLineINIValue(tt.field, tt.value)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(ContainSubstring(tt.field))
				g.Expect(err.Error()).To(ContainSubstring("control characters"))
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
		})
	}
}

func TestWriteSwitchEntryRejectsControlCharactersInCredentialValues(t *testing.T) {
	g := NewWithT(t)
	scheme := runtime.NewScheme()
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())
	g.Expect(metal3api.AddToScheme(scheme)).To(Succeed())

	for _, field := range []string{"username", "password", "admin-password"} {
		t.Run(field, func(t *testing.T) {
			g := NewWithT(t)
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "switch-creds",
					Namespace: "test-ns",
				},
				Data: map[string][]byte{
					"username":       []byte("admin"),
					"password":       []byte("secret"),
					"admin-password": []byte("enable-secret"),
				},
			}
			secret.Data[field] = []byte("bad\r\nvalue")
			if field == "password" || field == "admin-password" {
				secret.Data["ssh-privatekey"] = []byte("private-key")
			}

			sw := &metal3api.BareMetalSwitch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "switch1",
					Namespace: "test-ns",
				},
				Spec: metal3api.BareMetalSwitchSpec{
					Credentials: &corev1.SecretReference{Name: secret.Name},
				},
			}
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()
			sm := secretutils.NewSecretManager(logr.Discard(), c, c)

			configEntries := make(map[string][]byte)
			keyFiles := make(map[string][]byte)
			err := writeSwitchEntry(t.Context(), sm, sw, testSwitchCredentialPath, configEntries, keyFiles)

			g.Expect(err).To(HaveOccurred())
			g.Expect(err.Error()).To(ContainSubstring(secret.Name))
			g.Expect(err.Error()).To(ContainSubstring(field))
			g.Expect(configEntries).To(BeEmpty())
			g.Expect(keyFiles).To(BeEmpty())
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
		objects               []client.Object
		credentialPath        string
		expectedConfigEntries map[string]string
		expectedKeys          map[string][]byte
		expectError           bool
	}{
		{
			name:                  "no switches configured",
			objects:               []client.Object{},
			expectedConfigEntries: map[string]string{},
			expectedKeys:          map[string][]byte{},
			expectError:           false,
		},
		{
			name: "single password switch",
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
						Credentials: &corev1.SecretReference{
							Name: "switch1-creds",
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
			name: "mixed password and publickey switches",
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
			name: "valid switches produced when one switch has missing credentials",
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
						Credentials: &corev1.SecretReference{
							Name: "good-creds",
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
						Credentials: &corev1.SecretReference{
							Name: "missing-creds",
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

			result, err := generateSwitchConfig(t.Context(), c, sm, tt.credentialPath, logr.Discard())

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
					Credentials: &corev1.SecretReference{
						Name: "switch1-creds",
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

		_, err := generateSwitchConfig(t.Context(), c, sm, "", logr.Discard())
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("connection refused"))
	})
}

func TestGenerateSwitchConfigRejectsDuplicateNamesAndMACAddresses(t *testing.T) {
	g := NewWithT(t)
	scheme := runtime.NewScheme()
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())
	g.Expect(metal3api.AddToScheme(scheme)).To(Succeed())

	createdAt := func(day int) metav1.Time {
		return metav1.NewTime(time.Date(2026, time.January, day, 0, 0, 0, 0, time.UTC))
	}
	newCredentials := func(name, namespace string, data map[string][]byte) *corev1.Secret {
		return &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Data:       data,
		}
	}

	t.Run("same name keeps the oldest switch across namespaces", func(t *testing.T) {
		g := NewWithT(t)
		oldest := newDuplicateTestSwitch("shared", "namespace-a", "192.168.1.1", "00:00:5e:00:53:01", "old-creds", "old-uid", createdAt(1))
		duplicate := newDuplicateTestSwitch("shared", "namespace-b", "192.168.1.2", "00:00:5e:00:53:02", "duplicate-creds", "duplicate-uid", createdAt(2))
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
			oldest,
			duplicate,
			newCredentials("old-creds", "namespace-a", map[string][]byte{"username": []byte("old"), "password": []byte("old-password")}),
			newCredentials("duplicate-creds", "namespace-b", map[string][]byte{"username": []byte("new"), "password": []byte("new-password")}),
		).Build()
		sm := secretutils.NewSecretManager(logr.Discard(), c, c)

		result, err := generateSwitchConfig(t.Context(), c, sm, "", logr.Discard())
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(result.configEntries).To(HaveLen(1))
		g.Expect(string(result.configEntries["shared"])).To(ContainSubstring("address=192.168.1.1"))
		g.Expect(string(result.configEntries["shared"])).NotTo(ContainSubstring("192.168.1.2"))
		g.Expect(result.errors).To(HaveLen(1))
		g.Expect(result.errors[duplicate.UID].Error()).To(ContainSubstring(`name "shared" conflicts with namespace-a/shared`))
	})

	t.Run("MAC comparison ignores case and unrelated switches retain their config and key files", func(t *testing.T) {
		g := NewWithT(t)
		oldest := newDuplicateTestSwitch("switch-oldest", "namespace-a", "192.168.1.1", "aa:bb:cc:dd:ee:01", "old-creds", "old-uid", createdAt(1))
		duplicate := newDuplicateTestSwitch("switch-duplicate", "namespace-b", "192.168.1.2", "AA:BB:CC:DD:EE:01", "duplicate-creds", "duplicate-uid", createdAt(2))
		unrelated := newDuplicateTestSwitch("switch-unrelated", "namespace-c", "192.168.1.3", "00:00:5e:00:53:03", "unrelated-creds", "unrelated-uid", createdAt(3))
		publicKey := func(value string) map[string][]byte {
			return map[string][]byte{"username": []byte("admin"), "ssh-privatekey": []byte(value)}
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
			oldest,
			duplicate,
			unrelated,
			newCredentials("old-creds", "namespace-a", publicKey("old-key")),
			newCredentials("duplicate-creds", "namespace-b", publicKey("duplicate-key")),
			newCredentials("unrelated-creds", "namespace-c", publicKey("unrelated-key")),
		).Build()
		sm := secretutils.NewSecretManager(logr.Discard(), c, c)

		result, err := generateSwitchConfig(t.Context(), c, sm, testSwitchCredentialPath, logr.Discard())
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(result.configEntries).To(HaveLen(2))
		g.Expect(result.configEntries).To(HaveKey("switch-oldest"))
		g.Expect(result.configEntries).To(HaveKey("switch-unrelated"))
		g.Expect(result.configEntries).NotTo(HaveKey("switch-duplicate"))
		g.Expect(result.keyFiles).To(Equal(map[string][]byte{
			"aa-bb-cc-dd-ee-01.key": []byte("old-key"),
			"00-00-5e-00-53-03.key": []byte("unrelated-key"),
		}))
		g.Expect(result.errors).To(HaveLen(1))
		g.Expect(result.errors[duplicate.UID].Error()).To(ContainSubstring(`MAC address "AA:BB:CC:DD:EE:01" conflicts with namespace-a/switch-oldest`))
	})
}

func TestCompareBareMetalSwitchesBreaksCreationTimestampTies(t *testing.T) {
	g := NewWithT(t)
	timestamp := metav1.NewTime(time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC))

	byNamespace := metal3api.BareMetalSwitch{ObjectMeta: metav1.ObjectMeta{Namespace: "namespace-a", Name: "z-switch", CreationTimestamp: timestamp}}
	otherNamespace := metal3api.BareMetalSwitch{ObjectMeta: metav1.ObjectMeta{Namespace: "namespace-b", Name: "a-switch", CreationTimestamp: timestamp}}
	g.Expect(compareBareMetalSwitches(byNamespace, otherNamespace)).To(BeNumerically("<", 0))

	byName := metal3api.BareMetalSwitch{ObjectMeta: metav1.ObjectMeta{Namespace: "namespace-a", Name: "a-switch", CreationTimestamp: timestamp}}
	g.Expect(compareBareMetalSwitches(byName, byNamespace)).To(BeNumerically("<", 0))
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
		expectedErrors         map[types.UID]error
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
						Credentials: &corev1.SecretReference{
							Name: "switch1-creds",
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
						UID:       "switch3-uid",
					},
					Spec: metal3api.BareMetalSwitchSpec{
						Address:    "192.168.1.3",
						MACAddress: "00:00:5e:00:53:03",
						DeviceType: "cisco_ios",
						Credentials: &corev1.SecretReference{
							Name: "missing-secret",
						},
					},
				},
			},
			expectError:            false,
			expectedConfig:         "# This file is managed by the Baremetal Operator\n\n",
			expectedCredentialKeys: map[string][]byte{},
			expectedErrors: map[types.UID]error{
				"switch3-uid": &credentialSecretNotFoundError{msg: "credentials secret test-ns/missing-secret not found"},
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
				g.Expect(result.errors).To(HaveLen(len(tt.expectedErrors)))
				for uid, expected := range tt.expectedErrors {
					g.Expect(result.errors).To(HaveKey(uid))
					g.Expect(result.errors[uid].Error()).To(Equal(expected.Error()))
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
		name               string
		switch_            *metal3api.BareMetalSwitch
		objects            []client.Object
		expectRequeueAfter time.Duration
		expectStatus       metav1.ConditionStatus
		expectReason       string
		expectMessage      string
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
					Credentials: &corev1.SecretReference{
						Name: "good-creds",
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
			expectRequeueAfter: bareMetalSwitchReconcileInterval,
			expectStatus:       metav1.ConditionTrue,
			expectReason:       "Reconciled",
			expectMessage:      "Switch configuration has been successfully reconciled into the config secret",
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
					Credentials: &corev1.SecretReference{
						Name: "nonexistent-creds",
					},
				},
			},
			objects:            []client.Object{},
			expectRequeueAfter: credentialErrorRequeueDelay,
			expectStatus:       metav1.ConditionFalse,
			expectReason:       "CredentialError",
			expectMessage:      "credentials secret test-ns/nonexistent-creds not found",
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
					Credentials: &corev1.SecretReference{
						Name: "bad-creds",
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
			expectRequeueAfter: bareMetalSwitchReconcileInterval,
			expectStatus:       metav1.ConditionFalse,
			expectReason:       "CredentialError",
			expectMessage:      "missing 'username' key",
		},
		{
			name: "sets Reconciled=False when password and ssh-privatekey keys are missing",
			switch_: &metal3api.BareMetalSwitch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "no-password-switch",
					Namespace: "test-ns",
				},
				Spec: metal3api.BareMetalSwitchSpec{
					Address:    "192.168.1.1",
					MACAddress: "00:00:5e:00:53:01",
					DeviceType: "cisco_ios",
					Credentials: &corev1.SecretReference{
						Name: "no-password-creds",
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
			expectRequeueAfter: bareMetalSwitchReconcileInterval,
			expectStatus:       metav1.ConditionFalse,
			expectReason:       "CredentialError",
			expectMessage:      "missing 'password' or 'ssh-privatekey' key",
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
				SwitchSecretsNamespace:     "test-ns",
			}

			result, err := r.Reconcile(t.Context(), ctrl.Request{
				NamespacedName: client.ObjectKeyFromObject(tt.switch_),
			})
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(result.RequeueAfter).To(Equal(tt.expectRequeueAfter))

			// Re-fetch the switch to get updated status
			updatedSwitch := &metal3api.BareMetalSwitch{}
			g.Expect(c.Get(t.Context(), client.ObjectKeyFromObject(tt.switch_), updatedSwitch)).To(Succeed())

			cond := meta.FindStatusCondition(updatedSwitch.Status.Conditions, string(metal3api.SwitchConditionValid))
			g.Expect(cond).ToNot(BeNil(), "Reconciled condition should be set")
			g.Expect(cond.Status).To(Equal(tt.expectStatus))
			g.Expect(cond.Reason).To(Equal(tt.expectReason))
			g.Expect(cond.Message).To(ContainSubstring(tt.expectMessage))
			g.Expect(cond.ObservedGeneration).To(Equal(updatedSwitch.Generation))
		})
	}
}

func TestReconcileDuplicateMACAddressMarksOnlyDuplicateInvalid(t *testing.T) {
	g := NewWithT(t)
	scheme := runtime.NewScheme()
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())
	g.Expect(metal3api.AddToScheme(scheme)).To(Succeed())

	createdAt := func(day int) metav1.Time {
		return metav1.NewTime(time.Date(2026, time.January, day, 0, 0, 0, 0, time.UTC))
	}
	canonical := newDuplicateTestSwitch("switch-canonical", "namespace-a", "192.168.1.1", "aa:bb:cc:dd:ee:01", "canonical-creds", "canonical-uid", createdAt(1))
	duplicate := newDuplicateTestSwitch("switch-duplicate", "namespace-b", "192.168.1.2", "AA:BB:CC:DD:EE:01", "duplicate-creds", "duplicate-uid", createdAt(2))
	configSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: testSwitchConfigsSecretName, Namespace: "test-ns"},
		Data:       map[string][]byte{},
	}
	credentialSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: testSwitchCredentialSecretName, Namespace: "test-ns"},
		Data:       map[string][]byte{},
	}
	keyCredentials := func(name, namespace, key string) *corev1.Secret {
		return &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Data:       map[string][]byte{"username": []byte("admin"), "ssh-privatekey": []byte(key)},
		}
	}
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(
			// Add the duplicate first to ensure generation order is based on the
			// creation timestamps rather than the API list order.
			duplicate,
			canonical,
			keyCredentials("canonical-creds", "namespace-a", "canonical-key"),
			keyCredentials("duplicate-creds", "namespace-b", "duplicate-key"),
			configSecret,
			credentialSecret,
		).
		WithStatusSubresource(&metal3api.BareMetalSwitch{}).
		Build()
	r := &BareMetalSwitchReconciler{
		Client:                     c,
		Log:                        logr.Discard(),
		APIReader:                  c,
		SwitchSecretsNamespace:     "test-ns",
		SwitchConfigsSecretName:    testSwitchConfigsSecretName,
		SwitchCredentialSecretName: testSwitchCredentialSecretName,
		SwitchCredentialPath:       testSwitchCredentialPath,
	}

	_, err := r.Reconcile(t.Context(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(canonical)})
	g.Expect(err).NotTo(HaveOccurred())
	result, err := r.Reconcile(t.Context(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(duplicate)})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(result.RequeueAfter).To(Equal(time.Minute))

	getCondition := func(sw *metal3api.BareMetalSwitch) *metav1.Condition {
		updated := &metal3api.BareMetalSwitch{}
		g.Expect(c.Get(t.Context(), client.ObjectKeyFromObject(sw), updated)).To(Succeed())
		return meta.FindStatusCondition(updated.Status.Conditions, string(metal3api.SwitchConditionValid))
	}
	canonicalCondition := getCondition(canonical)
	g.Expect(canonicalCondition).NotTo(BeNil())
	g.Expect(canonicalCondition.Status).To(Equal(metav1.ConditionTrue))
	duplicateCondition := getCondition(duplicate)
	g.Expect(duplicateCondition).NotTo(BeNil())
	g.Expect(duplicateCondition.Status).To(Equal(metav1.ConditionFalse))
	g.Expect(duplicateCondition.Reason).To(Equal("Duplicate"))
	g.Expect(duplicateCondition.Message).To(ContainSubstring("MAC address"))
	g.Expect(duplicateCondition.Message).To(ContainSubstring("namespace-a/switch-canonical"))

	updatedConfig := &corev1.Secret{}
	g.Expect(c.Get(t.Context(), client.ObjectKeyFromObject(configSecret), updatedConfig)).To(Succeed())
	g.Expect(string(updatedConfig.Data[switchConfigKey])).To(ContainSubstring("[switch:switch-canonical]"))
	g.Expect(string(updatedConfig.Data[switchConfigKey])).NotTo(ContainSubstring("[switch:switch-duplicate]"))
	updatedCredentials := &corev1.Secret{}
	g.Expect(c.Get(t.Context(), client.ObjectKeyFromObject(credentialSecret), updatedCredentials)).To(Succeed())
	g.Expect(updatedCredentials.Data).To(Equal(map[string][]byte{
		"aa-bb-cc-dd-ee-01.key": []byte("canonical-key"),
	}))

	// When the canonical switch is removed, the duplicate's periodic reconcile
	// should admit it and clear its Duplicate condition.
	g.Expect(c.Delete(t.Context(), canonical)).To(Succeed())
	result, err = r.Reconcile(t.Context(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(duplicate)})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(result.RequeueAfter).To(Equal(bareMetalSwitchReconcileInterval))
	duplicateCondition = getCondition(duplicate)
	g.Expect(duplicateCondition.Status).To(Equal(metav1.ConditionTrue))
	g.Expect(duplicateCondition.Reason).To(Equal("Reconciled"))

	updatedConfig = &corev1.Secret{}
	g.Expect(c.Get(t.Context(), client.ObjectKeyFromObject(configSecret), updatedConfig)).To(Succeed())
	g.Expect(string(updatedConfig.Data[switchConfigKey])).To(ContainSubstring("[switch:switch-duplicate]"))
	g.Expect(string(updatedConfig.Data[switchConfigKey])).NotTo(ContainSubstring("[switch:switch-canonical]"))
	updatedCredentials = &corev1.Secret{}
	g.Expect(c.Get(t.Context(), client.ObjectKeyFromObject(credentialSecret), updatedCredentials)).To(Succeed())
	g.Expect(updatedCredentials.Data).To(Equal(map[string][]byte{
		"AA-BB-CC-DD-EE-01.key": []byte("duplicate-key"),
	}))
}

func TestPeriodicReconcileUpdatesSwitchesAffectedByMACAddressChanges(t *testing.T) {
	g := NewWithT(t)
	scheme := runtime.NewScheme()
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())
	g.Expect(metal3api.AddToScheme(scheme)).To(Succeed())

	createdAt := func(day int) metav1.Time {
		return metav1.NewTime(time.Date(2026, time.January, day, 0, 0, 0, 0, time.UTC))
	}
	canonical := newDuplicateTestSwitch("switch-canonical", "namespace-a", "192.168.1.1", "aa:bb:cc:dd:ee:01", "canonical-creds", "canonical-uid", createdAt(1))
	peer := newDuplicateTestSwitch("switch-peer", "namespace-b", "192.168.1.2", "aa:bb:cc:dd:ee:02", "peer-creds", "peer-uid", createdAt(2))
	configSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: testSwitchConfigsSecretName, Namespace: "test-ns"},
		Data:       map[string][]byte{},
	}
	credentialSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: testSwitchCredentialSecretName, Namespace: "test-ns"},
		Data:       map[string][]byte{},
	}
	credentials := func(name, namespace string) *corev1.Secret {
		return &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Data:       map[string][]byte{"username": []byte("admin"), "password": []byte("secret")},
		}
	}
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(
			canonical,
			peer,
			credentials("canonical-creds", "namespace-a"),
			credentials("peer-creds", "namespace-b"),
			configSecret,
			credentialSecret,
		).
		WithStatusSubresource(&metal3api.BareMetalSwitch{}).
		Build()
	r := &BareMetalSwitchReconciler{
		Client:                     c,
		Log:                        logr.Discard(),
		APIReader:                  c,
		SwitchSecretsNamespace:     "test-ns",
		SwitchConfigsSecretName:    testSwitchConfigsSecretName,
		SwitchCredentialSecretName: testSwitchCredentialSecretName,
		SwitchCredentialPath:       testSwitchCredentialPath,
	}

	requestFor := func(sw *metal3api.BareMetalSwitch) ctrl.Request {
		return ctrl.Request{NamespacedName: client.ObjectKeyFromObject(sw)}
	}
	canonicalKey := client.ObjectKeyFromObject(canonical)
	getValidCondition := func(sw *metal3api.BareMetalSwitch) *metav1.Condition {
		updated := &metal3api.BareMetalSwitch{}
		g.Expect(c.Get(t.Context(), client.ObjectKeyFromObject(sw), updated)).To(Succeed())
		return meta.FindStatusCondition(updated.Status.Conditions, string(metal3api.SwitchConditionValid))
	}
	for _, sw := range []*metal3api.BareMetalSwitch{canonical, peer} {
		result, err := r.Reconcile(t.Context(), requestFor(sw))
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(result.RequeueAfter).To(Equal(bareMetalSwitchReconcileInterval))
		condition := getValidCondition(sw)
		g.Expect(condition).NotTo(BeNil())
		g.Expect(condition.Status).To(Equal(metav1.ConditionTrue))
	}

	// A periodic reconcile of the peer observes the conflict after the older
	// switch changes to its MAC.
	canonical = &metal3api.BareMetalSwitch{}
	g.Expect(c.Get(t.Context(), canonicalKey, canonical)).To(Succeed())
	canonical.Spec.MACAddress = peer.Spec.MACAddress
	g.Expect(c.Update(t.Context(), canonical)).To(Succeed())

	result, err := r.Reconcile(t.Context(), requestFor(peer))
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(result.RequeueAfter).To(Equal(bareMetalSwitchReconcileInterval))
	condition := getValidCondition(peer)
	g.Expect(condition.Status).To(Equal(metav1.ConditionFalse))
	g.Expect(condition.Reason).To(Equal("Duplicate"))

	// The peer's next periodic reconcile observes that the older switch moved
	// away and clears its duplicate condition.
	canonical = &metal3api.BareMetalSwitch{}
	g.Expect(c.Get(t.Context(), canonicalKey, canonical)).To(Succeed())
	canonical.Spec.MACAddress = "aa:bb:cc:dd:ee:03"
	g.Expect(c.Update(t.Context(), canonical)).To(Succeed())

	result, err = r.Reconcile(t.Context(), requestFor(peer))
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(result.RequeueAfter).To(Equal(bareMetalSwitchReconcileInterval))
	condition = getValidCondition(peer)
	g.Expect(condition.Status).To(Equal(metav1.ConditionTrue))
	g.Expect(condition.Reason).To(Equal("Reconciled"))
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
