package controllers

import (
	"encoding/base64"
	"errors"
	"fmt"
	"testing"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/secretutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// TestNoDataInSecretErrorAs verifies that errors.As works correctly with
// NoDataInSecretError. This test guards against incorrect patterns like
// errors.As(err, new(*NoDataInSecretError)) which don't work with value types.
func TestNoDataInSecretErrorAs(t *testing.T) {
	err := NoDataInSecretError{secret: "test-secret", key: "networkData"}

	// This is the correct pattern - use a variable and pass its address
	var target NoDataInSecretError
	if !errors.As(err, &target) {
		t.Fatal("errors.As should match NoDataInSecretError with &target pattern")
	}
	assert.Equal(t, "test-secret", target.secret)
	assert.Equal(t, "networkData", target.key)

	// Verify the incorrect pattern does NOT work (this is what the bug was)
	// Note: new(*NoDataInSecretError) creates **NoDataInSecretError, which
	// doesn't match a value type error
	if errors.As(err, new(*NoDataInSecretError)) {
		t.Fatal("errors.As with new(*NoDataInSecretError) should NOT match value type errors - if this passes, the Go behavior changed")
	}
}

func TestLabelSecrets(t *testing.T) {
	testCases := []struct {
		name     string
		getter   func(hcd *hostConfigData) (string, error)
		hostSpec *metal3api.BareMetalHostSpec
	}{
		{
			name: "user-data",
			getter: func(hcd *hostConfigData) (string, error) {
				return hcd.UserData(t.Context())
			},
			hostSpec: &metal3api.BareMetalHostSpec{
				UserData: &corev1.SecretReference{
					Name:      "user-data",
					Namespace: namespace,
				},
			},
		},
		{
			name: "meta-data",
			getter: func(hcd *hostConfigData) (string, error) {
				return hcd.MetaData(t.Context())
			},
			hostSpec: &metal3api.BareMetalHostSpec{
				MetaData: &corev1.SecretReference{
					Name:      "meta-data",
					Namespace: namespace,
				},
			},
		},
		{
			name: "network-data",
			getter: func(hcd *hostConfigData) (string, error) {
				return hcd.NetworkData(t.Context())
			},
			hostSpec: &metal3api.BareMetalHostSpec{
				NetworkData: &corev1.SecretReference{
					Name:      "network-data",
					Namespace: namespace,
				},
			},
		},
		{
			name: "preprovisioning-network-data",
			getter: func(hcd *hostConfigData) (string, error) {
				return hcd.PreprovisioningNetworkData(t.Context())
			},
			hostSpec: &metal3api.BareMetalHostSpec{
				PreprovisioningNetworkDataName: "preprovisioning-network-data",
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			host := newHost("host", tc.hostSpec)
			c := fakeclient.NewClientBuilder().Build()
			baselog := ctrl.Log.WithName("controllers").WithName("BareMetalHost")
			hcd := &hostConfigData{
				host:          host,
				log:           baselog.WithName("host_config_data"),
				secretManager: secretutils.NewSecretManager(baselog, c, c),
			}

			secret := newSecret(tc.name, map[string]string{"value": "somedata"})
			err := c.Create(t.Context(), secret)
			require.NoError(t, err)

			_, err = tc.getter(hcd)
			require.NoError(t, err)

			actualSecret := &corev1.Secret{}
			err = c.Get(t.Context(), types.NamespacedName{Name: tc.name, Namespace: namespace}, actualSecret)
			require.NoError(t, err)
			assert.Equal(t, "baremetal", actualSecret.Labels["environment.metal3.io"])
		})
	}
}

func TestAcquirePreprovisioningNetworkSecret(t *testing.T) {
	host := newHost("host", &metal3api.BareMetalHostSpec{
		PreprovisioningNetworkDataName: "preprov-net-data",
	})
	host.Status.Provisioning.State = metal3api.StateRegistering

	secret := newSecret("preprov-net-data", map[string]string{"networkData": "key: value"})
	c := fakeclient.NewClientBuilder().WithObjects(host, secret).Build()
	baselog := ctrl.Log.WithName("controllers").WithName("BareMetalHost")
	hcd := &hostConfigData{
		host:          host,
		log:           baselog.WithName("host_config_data"),
		secretManager: secretutils.NewSecretManager(baselog, c, c),
	}

	_, err := hcd.PreprovisioningNetworkData(t.Context())
	require.NoError(t, err)

	actualSecret := &corev1.Secret{}
	err = c.Get(t.Context(), types.NamespacedName{Name: "preprov-net-data", Namespace: namespace}, actualSecret)
	require.NoError(t, err)
	assert.Equal(t, secretutils.LabelEnvironmentValue, actualSecret.Labels[secretutils.LabelEnvironmentName])
	assert.Contains(t, actualSecret.Finalizers, secretutils.SecretsFinalizer)
	assert.Empty(t, actualSecret.OwnerReferences)
}

func TestPreprovisioningNetworkSecretNotFoundDuringDeletion(t *testing.T) {
	host := newHost("host", &metal3api.BareMetalHostSpec{
		PreprovisioningNetworkDataName: "missing-preprov-net",
	})
	host.Status.Provisioning.State = metal3api.StatePoweringOffBeforeDelete
	now := metav1.Now()
	host.DeletionTimestamp = &now
	host.Finalizers = []string{metal3api.BareMetalHostFinalizer}

	c := fakeclient.NewClientBuilder().WithObjects(host).Build()
	baselog := ctrl.Log.WithName("controllers").WithName("BareMetalHost")
	hcd := &hostConfigData{
		host:          host,
		log:           baselog.WithName("host_config_data"),
		secretManager: secretutils.NewSecretManager(baselog, c, c),
	}

	data, err := hcd.PreprovisioningNetworkData(t.Context())
	require.NoError(t, err)
	assert.Empty(t, data)
}

func TestProvisionWithHostConfig(t *testing.T) {
	testBMCSecret := newBMCCredsSecret(defaultSecretName, "User", "Pass")

	testCases := []struct {
		Scenario                           string
		Host                               *metal3api.BareMetalHost
		HostClaim                          *metal3api.HostClaim
		UserDataSecret                     *corev1.Secret
		PreprovNetworkDataSecret           *corev1.Secret
		NetworkDataSecret                  *corev1.Secret
		ExpectedUserData                   string
		ErrUserData                        bool
		ExpectedNetworkData                string
		ErrNetworkData                     bool
		ExpectedPreprovisioningNetworkData string
		ErrPreprovisioningNetworkData      bool
		ExpectedMetaData                   string
		ErrMetaData                        bool
	}{
		{
			Scenario: "host with user data only",
			Host: newHost("host-user-data",
				&metal3api.BareMetalHostSpec{
					BMC: metal3api.BMCDetails{
						Address:         "ipmi://192.168.122.1:6233",
						CredentialsName: defaultSecretName,
					},
					UserData: &corev1.SecretReference{
						Name:      "user-data",
						Namespace: namespace,
					},
				}),
			UserDataSecret:      newSecret("user-data", map[string]string{"userData": "somedata"}),
			ExpectedUserData:    base64.StdEncoding.EncodeToString([]byte("somedata")),
			ErrUserData:         false,
			ExpectedNetworkData: "",
			ErrNetworkData:      false,
		},
		{
			Scenario: "host with user data only, no namespace",
			Host: newHost("host-user-data",
				&metal3api.BareMetalHostSpec{
					BMC: metal3api.BMCDetails{
						Address:         "ipmi://192.168.122.1:6233",
						CredentialsName: defaultSecretName,
					},
					UserData: &corev1.SecretReference{
						Name: "user-data",
					},
				}),
			UserDataSecret:      newSecret("user-data", map[string]string{"userData": "somedata"}),
			ExpectedUserData:    base64.StdEncoding.EncodeToString([]byte("somedata")),
			ErrUserData:         false,
			ExpectedNetworkData: "",
			ErrNetworkData:      false,
		},
		{
			Scenario: "host with preprov network data only",
			Host: newHost("host-user-data",
				&metal3api.BareMetalHostSpec{
					BMC: metal3api.BMCDetails{
						Address:         "ipmi://192.168.122.1:6233",
						CredentialsName: defaultSecretName,
					},
					PreprovisioningNetworkDataName: "net-data",
				}),
			NetworkDataSecret:                  newSecret("net-data", map[string]string{"networkData": "key: value"}),
			ExpectedUserData:                   "",
			ErrUserData:                        false,
			ExpectedNetworkData:                base64.StdEncoding.EncodeToString([]byte("key: value")),
			ExpectedPreprovisioningNetworkData: base64.StdEncoding.EncodeToString([]byte("key: value")),
			ErrNetworkData:                     false,
		},
		{
			Scenario: "host with preprov and regular network data",
			Host: newHost("host-user-data",
				&metal3api.BareMetalHostSpec{
					BMC: metal3api.BMCDetails{
						Address:         "ipmi://192.168.122.1:6233",
						CredentialsName: defaultSecretName,
					},
					PreprovisioningNetworkDataName: "net-data2",
					NetworkData: &corev1.SecretReference{
						Name:      "net-data",
						Namespace: namespace,
					},
				}),
			NetworkDataSecret:                  newSecret("net-data", map[string]string{"networkData": "key: value"}),
			PreprovNetworkDataSecret:           newSecret("net-data2", map[string]string{"networkData": "key: value2"}),
			ExpectedUserData:                   "",
			ErrUserData:                        false,
			ExpectedNetworkData:                base64.StdEncoding.EncodeToString([]byte("key: value")),
			ExpectedPreprovisioningNetworkData: base64.StdEncoding.EncodeToString([]byte("key: value2")),
			ErrNetworkData:                     false,
		},
		{
			Scenario: "host with network data only",
			Host: newHost("host-user-data",
				&metal3api.BareMetalHostSpec{
					BMC: metal3api.BMCDetails{
						Address:         "ipmi://192.168.122.1:6233",
						CredentialsName: defaultSecretName,
					},
					NetworkData: &corev1.SecretReference{
						Name:      "net-data",
						Namespace: namespace,
					},
				}),
			NetworkDataSecret:   newSecret("net-data", map[string]string{"networkData": "key: value"}),
			ExpectedUserData:    "",
			ErrUserData:         false,
			ExpectedNetworkData: base64.StdEncoding.EncodeToString([]byte("key: value")),
			ErrNetworkData:      false,
		},
		{
			Scenario: "host with network data only, no namespace",
			Host: newHost("host-user-data",
				&metal3api.BareMetalHostSpec{
					BMC: metal3api.BMCDetails{
						Address:         "ipmi://192.168.122.1:6233",
						CredentialsName: defaultSecretName,
					},
					NetworkData: &corev1.SecretReference{
						Name: "net-data",
					},
				}),
			NetworkDataSecret:   newSecret("net-data", map[string]string{"networkData": "key: value"}),
			ExpectedUserData:    "",
			ErrUserData:         false,
			ExpectedNetworkData: base64.StdEncoding.EncodeToString([]byte("key: value")),
			ErrNetworkData:      false,
		},
		{
			Scenario: "host with metadata only",
			Host: newHost("host-meta-data",
				&metal3api.BareMetalHostSpec{
					BMC: metal3api.BMCDetails{
						Address:         "ipmi://192.168.122.1:6233",
						CredentialsName: defaultSecretName,
					},
					MetaData: &corev1.SecretReference{
						Name:      "meta-data",
						Namespace: namespace,
					},
				}),
			NetworkDataSecret: newSecret("meta-data", map[string]string{"metaData": "key: value"}),
			ExpectedMetaData:  base64.StdEncoding.EncodeToString([]byte("key: value")),
			ErrMetaData:       false,
		},
		{
			Scenario: "host with metadata only, no namespace",
			Host: newHost("host-meta-data",
				&metal3api.BareMetalHostSpec{
					BMC: metal3api.BMCDetails{
						Address:         "ipmi://192.168.122.1:6233",
						CredentialsName: defaultSecretName,
					},
					MetaData: &corev1.SecretReference{
						Name: "meta-data",
					},
				}),
			NetworkDataSecret: newSecret("meta-data", map[string]string{"metaData": "key: value"}),
			ExpectedMetaData:  base64.StdEncoding.EncodeToString([]byte("key: value")),
			ErrMetaData:       false,
		},
		{
			Scenario: "fall back to value",
			Host: newHost("host-user-data",
				&metal3api.BareMetalHostSpec{
					BMC: metal3api.BMCDetails{
						Address:         "ipmi://192.168.122.1:6233",
						CredentialsName: defaultSecretName,
					},
					UserData: &corev1.SecretReference{
						Name:      "user-data",
						Namespace: namespace,
					},
				}),
			UserDataSecret:      newSecret("user-data", map[string]string{"value": "somedata"}),
			ExpectedUserData:    base64.StdEncoding.EncodeToString([]byte("somedata")),
			ErrUserData:         false,
			ExpectedNetworkData: "",
			ErrNetworkData:      false,
		},
		{
			Scenario: "host with non-existent network data",
			Host: newHost("host-user-data",
				&metal3api.BareMetalHostSpec{
					BMC: metal3api.BMCDetails{
						Address:         "ipmi://192.168.122.1:6233",
						CredentialsName: defaultSecretName,
					},
					NetworkData: &corev1.SecretReference{
						Name:      "net-data",
						Namespace: namespace,
					},
				}),
			ExpectedUserData:    "",
			ErrUserData:         true,
			ExpectedNetworkData: "",
			ErrNetworkData:      true,
		},
		{
			Scenario: "host with wrong key in network data secret",
			Host: newHost("host-user-data",
				&metal3api.BareMetalHostSpec{
					BMC: metal3api.BMCDetails{
						Address:         "ipmi://192.168.122.1:6233",
						CredentialsName: defaultSecretName,
					},
					NetworkData: &corev1.SecretReference{
						Name:      "net-data",
						Namespace: namespace,
					},
				}),
			NetworkDataSecret:   newSecret("net-data", map[string]string{"wrong": "key: value"}),
			ExpectedUserData:    "",
			ErrUserData:         false,
			ExpectedNetworkData: "",
			ErrNetworkData:      true,
		},
		{
			Scenario: "host without keys in user data secret",
			Host: newHost("host-user-data",
				&metal3api.BareMetalHostSpec{
					BMC: metal3api.BMCDetails{
						Address:         "ipmi://192.168.122.1:6233",
						CredentialsName: defaultSecretName,
					},
					UserData: &corev1.SecretReference{
						Name:      "user-data",
						Namespace: namespace,
					},
				}),
			UserDataSecret:      newSecret("user-data", map[string]string{}),
			ExpectedUserData:    "",
			ErrUserData:         true,
			ExpectedNetworkData: "",
			ErrNetworkData:      true,
		},
		{
			Scenario: "user-data secret in different namespace",
			Host: newHost("host-user-data",
				&metal3api.BareMetalHostSpec{
					BMC: metal3api.BMCDetails{
						Address:         "ipmi://192.168.122.1:6233",
						CredentialsName: defaultSecretName,
					},
					UserData: &corev1.SecretReference{
						Name:      "user-data",
						Namespace: "other-namespace",
					},
				}),
			UserDataSecret: newSecretInNamespace("user-data", "other-namespace", map[string]string{"userData": "somedata"}),
			ErrUserData:    true,
		},
		{
			Scenario: "meta-data secret in different namespace",
			Host: newHost("host-user-data",
				&metal3api.BareMetalHostSpec{
					BMC: metal3api.BMCDetails{
						Address:         "ipmi://192.168.122.1:6233",
						CredentialsName: defaultSecretName,
					},
					MetaData: &corev1.SecretReference{
						Name:      "meta-data",
						Namespace: "other-namespace",
					},
				}),
			NetworkDataSecret: newSecretInNamespace("meta-data", "other-namespace", map[string]string{"metaData": "key: value"}),
			ErrMetaData:       true,
		},
		{
			Scenario: "network-data secret in different namespace",
			Host: newHost("host-user-data",
				&metal3api.BareMetalHostSpec{
					BMC: metal3api.BMCDetails{
						Address:         "ipmi://192.168.122.1:6233",
						CredentialsName: defaultSecretName,
					},
					NetworkData: &corev1.SecretReference{
						Name:      "net-data",
						Namespace: "other-namespace",
					},
				}),
			NetworkDataSecret: newSecretInNamespace("net-data", "other-namespace", map[string]string{"networkData": "key: value"}),
			ErrNetworkData:    true,
		},
		{
			Scenario: "preprov network data with wrong key returns empty (not error)",
			Host: newHost("host-preprov-wrong-key",
				&metal3api.BareMetalHostSpec{
					BMC: metal3api.BMCDetails{
						Address:         "ipmi://192.168.122.1:6233",
						CredentialsName: defaultSecretName,
					},
					PreprovisioningNetworkDataName: "preprov-net-data",
				}),
			// Secret exists but has wrong key - PreprovisioningNetworkData should return empty, not error
			PreprovNetworkDataSecret:           newSecret("preprov-net-data", map[string]string{"wrongkey": "some: data"}),
			ExpectedUserData:                   "",
			ErrUserData:                        false,
			ExpectedNetworkData:                "",
			ErrNetworkData:                     false,
			ExpectedPreprovisioningNetworkData: "",
			ErrPreprovisioningNetworkData:      false, // Should NOT error, should return empty
		},
		{
			Scenario: "host with user data only through hostclaim",
			Host: newHost("host-user-data",
				&metal3api.BareMetalHostSpec{
					BMC: metal3api.BMCDetails{
						Address:         "ipmi://192.168.122.1:6233",
						CredentialsName: defaultSecretName,
					},
					ConsumerRef: &corev1.ObjectReference{
						Kind:       "HostClaim",
						APIVersion: "metal3.io/v1alpha1",
						Namespace:  "hc-ns",
						Name:       "hc",
					},
					UserData: &corev1.SecretReference{
						Name:      "-hostclaim-",
						Namespace: "",
					},
				}),
			HostClaim: &metal3api.HostClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "hc",
					Namespace: "hc-ns",
				},
				Spec: metal3api.HostClaimSpec{
					UserData: &metal3api.SecretReference{
						Name: "user-data",
					},
				},
				Status: metal3api.HostClaimStatus{
					BareMetalHost: &metal3api.ObjectReference{
						Namespace: namespace,
						Name:      "host-user-data",
					},
				},
			},
			UserDataSecret:      newSecretInNamespace("user-data", "hc-ns", map[string]string{"userData": "somedata"}),
			ExpectedUserData:    base64.StdEncoding.EncodeToString([]byte("somedata")),
			ErrUserData:         false,
			ExpectedNetworkData: "",
			ErrNetworkData:      false,
		},
		{
			Scenario: "host with network data only  through hostclaim",
			Host: newHost("host-user-data",
				&metal3api.BareMetalHostSpec{
					BMC: metal3api.BMCDetails{
						Address:         "ipmi://192.168.122.1:6233",
						CredentialsName: defaultSecretName,
					},
					ConsumerRef: &corev1.ObjectReference{
						Kind:       "HostClaim",
						APIVersion: "metal3.io/v1alpha1",
						Namespace:  "hc-ns",
						Name:       "hc",
					},
					NetworkData: &corev1.SecretReference{
						Name: "-hostclaim-",
					},
				}),
			HostClaim: &metal3api.HostClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "hc",
					Namespace: "hc-ns",
				},
				Spec: metal3api.HostClaimSpec{
					NetworkData: &metal3api.SecretReference{
						Name: "net-data",
					},
				},
				Status: metal3api.HostClaimStatus{
					BareMetalHost: &metal3api.ObjectReference{
						Namespace: namespace,
						Name:      "host-user-data",
					},
				},
			},
			NetworkDataSecret:   newSecretInNamespace("net-data", "hc-ns", map[string]string{"networkData": "key: value"}),
			ExpectedUserData:    "",
			ErrUserData:         false,
			ExpectedNetworkData: base64.StdEncoding.EncodeToString([]byte("key: value")),
			ErrNetworkData:      false,
		},
		{
			Scenario: "host with metadata only through hostclaim",
			Host: newHost("host-meta-data",
				&metal3api.BareMetalHostSpec{
					BMC: metal3api.BMCDetails{
						Address:         "ipmi://192.168.122.1:6233",
						CredentialsName: defaultSecretName,
					},
					ConsumerRef: &corev1.ObjectReference{
						Kind:       "HostClaim",
						APIVersion: "metal3.io/v1alpha1",
						Namespace:  "hc-ns",
						Name:       "hc",
					},
					MetaData: &corev1.SecretReference{
						Name: "-hostclaim-",
					},
				}),
			HostClaim: &metal3api.HostClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "hc",
					Namespace: "hc-ns",
				},
				Spec: metal3api.HostClaimSpec{
					MetaData: &metal3api.SecretReference{
						Name: "meta-data",
					},
				},
				Status: metal3api.HostClaimStatus{
					BareMetalHost: &metal3api.ObjectReference{
						Namespace: namespace,
						Name:      "host-meta-data",
					},
				},
			},
			NetworkDataSecret: newSecretInNamespace("meta-data", "hc-ns", map[string]string{"metaData": "key: value"}),
			ExpectedMetaData:  base64.StdEncoding.EncodeToString([]byte("key: value")),
			ErrMetaData:       false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Scenario, func(t *testing.T) {
			tc.Host.Spec.Image = &metal3api.Image{
				URL:      "https://example.com/image-name",
				Checksum: "12345",
			}
			tc.Host.Spec.Online = true

			c := fakeclient.NewClientBuilder().WithObjects(tc.Host).Build()
			if tc.HostClaim != nil {
				_ = c.Create(t.Context(), tc.HostClaim)
			}
			_ = c.Create(t.Context(), testBMCSecret)
			_ = c.Create(t.Context(), tc.UserDataSecret)
			_ = c.Create(t.Context(), tc.NetworkDataSecret)
			_ = c.Create(t.Context(), tc.PreprovNetworkDataSecret)
			baselog := ctrl.Log.WithName("controllers").WithName("BareMetalHost")
			hcd := &hostConfigData{
				host:          tc.Host,
				log:           baselog.WithName("host_config_data"),
				secretManager: secretutils.NewSecretManager(baselog, c, c),
				hostClaim:     tc.HostClaim,
			}

			actualUserData, err := hcd.UserData(t.Context())
			if err != nil && !tc.ErrUserData {
				t.Fatal(err)
			}

			if actualUserData != tc.ExpectedUserData {
				t.Fatal(fmt.Errorf("Failed to assert UserData. Expected '%s' got '%s'", tc.ExpectedUserData, actualUserData))
			}

			actualNetworkData, err := hcd.NetworkData(t.Context())
			if err != nil && !tc.ErrNetworkData {
				t.Fatal(err)
			}

			if actualNetworkData != tc.ExpectedNetworkData {
				t.Fatal(fmt.Errorf("Failed to assert NetworkData. Expected '%s' got '%s'", tc.ExpectedNetworkData, actualNetworkData))
			}

			actualPreprovisioningNetworkData, err := hcd.PreprovisioningNetworkData(t.Context())
			if err != nil && !tc.ErrPreprovisioningNetworkData {
				t.Fatal(err)
			}

			if actualPreprovisioningNetworkData != tc.ExpectedPreprovisioningNetworkData {
				t.Fatal(fmt.Errorf("Failed to assert PreprovisioningNetworkData. Expected '%s' got '%s'", tc.ExpectedPreprovisioningNetworkData, actualPreprovisioningNetworkData))
			}

			actualMetaData, err := hcd.MetaData(t.Context())
			if err != nil && !tc.ErrMetaData {
				t.Fatal(err)
			}

			if actualMetaData != tc.ExpectedMetaData {
				t.Fatal(fmt.Errorf("Failed to assert MetaData. Expected '%s' got '%s'", tc.ExpectedMetaData, actualMetaData))
			}
		})
	}
}
