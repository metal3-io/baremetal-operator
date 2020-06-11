package baremetalhost

import (
	goctx "context"
	"encoding/base64"
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"

	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	metal3v1alpha1 "github.com/metal3-io/baremetal-operator/pkg/apis/metal3/v1alpha1"
)

func TestProvisionWithHostConfig(t *testing.T) {
	testBMCSecret := newBMCCredsSecret(defaultSecretName, "User", "Pass")

	testCases := []struct {
		Scenario            string
		Host                *metal3v1alpha1.BareMetalHost
		UserDataSecret      *corev1.Secret
		NetworkDataSecret   *corev1.Secret
		ExpectedUserData    string
		ErrUserData         bool
		ExpectedNetworkData string
		ErrNetworkData      bool
		ExpectedMetaData    string
		ErrMetaData         bool
	}{
		{
			Scenario: "host with user data only",
			Host: newHost("host-user-data",
				&metal3v1alpha1.BareMetalHostSpec{
					BMC: metal3v1alpha1.BMCDetails{
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
				&metal3v1alpha1.BareMetalHostSpec{
					BMC: metal3v1alpha1.BMCDetails{
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
			Scenario: "host with network data only",
			Host: newHost("host-user-data",
				&metal3v1alpha1.BareMetalHostSpec{
					BMC: metal3v1alpha1.BMCDetails{
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
				&metal3v1alpha1.BareMetalHostSpec{
					BMC: metal3v1alpha1.BMCDetails{
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
				&metal3v1alpha1.BareMetalHostSpec{
					BMC: metal3v1alpha1.BMCDetails{
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
				&metal3v1alpha1.BareMetalHostSpec{
					BMC: metal3v1alpha1.BMCDetails{
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
				&metal3v1alpha1.BareMetalHostSpec{
					BMC: metal3v1alpha1.BMCDetails{
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
				&metal3v1alpha1.BareMetalHostSpec{
					BMC: metal3v1alpha1.BMCDetails{
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
				&metal3v1alpha1.BareMetalHostSpec{
					BMC: metal3v1alpha1.BMCDetails{
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
				&metal3v1alpha1.BareMetalHostSpec{
					BMC: metal3v1alpha1.BMCDetails{
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
	}

	for _, tc := range testCases {
		t.Run(tc.Scenario, func(t *testing.T) {
			tc.Host.Spec.Image = &metal3v1alpha1.Image{
				URL:      "https://example.com/image-name",
				Checksum: "12345",
			}
			tc.Host.Spec.Online = true

			c := fakeclient.NewFakeClient(tc.Host)
			c.Create(goctx.TODO(), testBMCSecret)
			c.Create(goctx.TODO(), tc.UserDataSecret)
			c.Create(goctx.TODO(), tc.NetworkDataSecret)
			hcd := &hostConfigData{
				host:   tc.Host,
				log:    log.WithValues("Test", "test"),
				client: c,
			}

			actualUserData, err := hcd.UserData()
			if err != nil && !tc.ErrUserData {
				t.Fatal(err)
			}

			if actualUserData != tc.ExpectedUserData {
				t.Fatal(fmt.Errorf("Failed to assert UserData. Expected '%s' got '%s'", tc.ExpectedUserData, actualUserData))
			}

			actualNetworkData, err := hcd.NetworkData()
			if err != nil && !tc.ErrNetworkData {
				t.Fatal(err)
			}

			if actualNetworkData != tc.ExpectedNetworkData {
				t.Fatal(fmt.Errorf("Failed to assert NetworkData. Expected '%s' got '%s'", actualNetworkData, tc.ExpectedNetworkData))
			}

			actualMetaData, err := hcd.MetaData()
			if err != nil && !tc.ErrMetaData {
				t.Fatal(err)
			}

			if actualMetaData != tc.ExpectedMetaData {
				t.Fatal(fmt.Errorf("Failed to assert MetaData. Expected '%s' got '%s'", actualMetaData, tc.ExpectedMetaData))
			}
		})
	}
}
