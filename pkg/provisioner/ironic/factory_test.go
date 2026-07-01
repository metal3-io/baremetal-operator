package ironic

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"math/big"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/noauth"
	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/hardwareutils/bmc"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/clients"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/testserver"
	ironicv1alpha1 "github.com/metal3-io/ironic-standalone-operator/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

type EnvFixture struct {
	ironicEndpoint                        string
	kernelURL                             string
	ramdiskURL                            string
	isoURL                                string
	liveISOForcePersistentBootDevice      string
	directDeployForcePersistentBootDevice string
	ironicCACertFile                      string
	ironicClientCertFile                  string
	ironicClientPrivateKeyFile            string
	ironicInsecure                        string
	ironicSkipClientSANVerify             string

	origEnv map[string]string
}

func (f *EnvFixture) TearDown() {
	for e, v := range f.origEnv {
		if v == "" {
			os.Unsetenv(e)
		} else {
			os.Setenv(e, v)
		}
	}
}

func (f *EnvFixture) replace(env, value string) {
	f.origEnv[env] = os.Getenv(env)
	if value == "" {
		os.Unsetenv(env)
	} else {
		os.Setenv(env, value)
	}
}

func (f *EnvFixture) SetUp() {
	f.origEnv = map[string]string{}
	f.replace("IRONIC_ENDPOINT", f.ironicEndpoint)
	f.replace("DEPLOY_KERNEL_URL", f.kernelURL)
	f.replace("DEPLOY_RAMDISK_URL", f.ramdiskURL)
	f.replace("DEPLOY_ISO_URL", f.isoURL)
	f.replace("LIVE_ISO_FORCE_PERSISTENT_BOOT_DEVICE", f.liveISOForcePersistentBootDevice)
	f.replace("DIRECT_DEPLOY_FORCE_PERSISTENT_BOOT_DEVICE", f.directDeployForcePersistentBootDevice)
	f.replace("IRONIC_CACERT_FILE", f.ironicCACertFile)
	f.replace("IRONIC_CLIENT_CERT_FILE", f.ironicClientCertFile)
	f.replace("IRONIC_CLIENT_PRIVATE_KEY_FILE", f.ironicClientPrivateKeyFile)
	f.replace("IRONIC_INSECURE", f.ironicInsecure)
	f.replace("IRONIC_SKIP_CLIENT_SAN_VERIFY", f.ironicSkipClientSANVerify)
}
func (f EnvFixture) VerifyConfig(t *testing.T, c ironicConfig, _ string) {
	t.Helper()
	assert.Equal(t, f.kernelURL, c.deployKernelURL)
	assert.Equal(t, f.ramdiskURL, c.deployRamdiskURL)
	assert.Equal(t, f.isoURL, c.deployISOURL)
	assert.Equal(t, f.liveISOForcePersistentBootDevice, c.liveISOForcePersistentBootDevice)
	assert.Equal(t, f.directDeployForcePersistentBootDevice, c.directDeployForcePersistentBootDevice)
}

func (f EnvFixture) VerifyEndpoints(t *testing.T, ironic string) {
	t.Helper()
	assert.Equal(t, f.ironicEndpoint, ironic)
}

func TestLoadConfigFromEnv(t *testing.T) {
	cases := []struct {
		name                  string
		env                   EnvFixture
		expectedError         string
		expectedImgBuildError string
		forcePersistent       string
	}{
		{
			name: "kernel and ramdisk",
			env: EnvFixture{
				kernelURL:  "http://kernel",
				ramdiskURL: "http://ramdisk",
			},
		},
		{
			name: "ISO only",
			env: EnvFixture{
				isoURL: "http://iso",
			},
		},
		{
			name: "ISO and kernel/ramdisk",
			env: EnvFixture{
				isoURL:     "http://iso",
				kernelURL:  "http://kernel",
				ramdiskURL: "http://ramdisk",
			},
		},
		{
			name: "only kernel",
			env: EnvFixture{
				kernelURL: "http://kernel",
			},
			expectedError: "DEPLOY_KERNEL_URL and DEPLOY_RAMDISK_URL can only be set together",
		},
		{
			name: "only ramdisk",
			env: EnvFixture{
				ramdiskURL: "http://ramdisk",
			},
			expectedError:         "DEPLOY_KERNEL_URL and DEPLOY_RAMDISK_URL can only be set together",
			expectedImgBuildError: "DEPLOY_RAMDISK_URL requires DEPLOY_KERNEL_URL to be set also",
		},
		{
			name: "ISO and kernel",
			env: EnvFixture{
				kernelURL: "http://kernel",
				isoURL:    "http://iso",
			},
			expectedError: "DEPLOY_KERNEL_URL and DEPLOY_RAMDISK_URL can only be set together",
		},
		{
			name: "ISO and ramdisk",
			env: EnvFixture{
				ramdiskURL: "http://ramdisk",
				isoURL:     "http://iso",
			},
			expectedError:         "DEPLOY_KERNEL_URL and DEPLOY_RAMDISK_URL can only be set together",
			expectedImgBuildError: "DEPLOY_RAMDISK_URL requires DEPLOY_KERNEL_URL to be set also",
		},
		{
			name: "ISO Force Persistent Default",
			env: EnvFixture{
				isoURL:                           "http://iso",
				liveISOForcePersistentBootDevice: "Default",
			},
			forcePersistent: "Default",
		},
		{
			name: "ISO Force Persistent Never",
			env: EnvFixture{
				isoURL:                           "http://iso",
				liveISOForcePersistentBootDevice: "Never",
			},
			forcePersistent: "Never",
		},
		{
			name: "ISO Force Persistent Always",
			env: EnvFixture{
				isoURL:                           "http://iso",
				liveISOForcePersistentBootDevice: "Always",
			},
			forcePersistent: "Always",
		},
		{
			name: "ISO Force Persistent Invalid",
			env: EnvFixture{
				isoURL:                           "http://iso",
				liveISOForcePersistentBootDevice: "NotAValidOption",
			},
			expectedError:         "invalid value for variable LIVE_ISO_FORCE_PERSISTENT_BOOT_DEVICE",
			expectedImgBuildError: "invalid value for variable LIVE_ISO_FORCE_PERSISTENT_BOOT_DEVICE",
		},
		{
			name: "kernel/ramdisk Force Persistent Default",
			env: EnvFixture{
				kernelURL:                             "http://kernel",
				ramdiskURL:                            "http://ramdisk",
				directDeployForcePersistentBootDevice: "Default",
			},
			forcePersistent: "Default",
		},
		{
			name: "kernel/ramdisk Force Persistent Never",
			env: EnvFixture{
				kernelURL:                             "http://kernel",
				ramdiskURL:                            "http://ramdisk",
				directDeployForcePersistentBootDevice: "Never",
			},
			forcePersistent: "Never",
		},
		{
			name: "kernel/ramdisk Force Persistent Always",
			env: EnvFixture{
				kernelURL:                             "http://kernel",
				ramdiskURL:                            "http://ramdisk",
				directDeployForcePersistentBootDevice: "Always",
			},
			forcePersistent: "Always",
		},
		{
			name: "kernel/ramdisk Force Persistent Invalid",
			env: EnvFixture{
				kernelURL:                             "http://kernel",
				ramdiskURL:                            "http://ramdisk",
				directDeployForcePersistentBootDevice: "NotAValidOption",
			},
			expectedError:         "invalid value for variable DIRECT_DEPLOY_FORCE_PERSISTENT_BOOT_DEVICE",
			expectedImgBuildError: "invalid value for variable DIRECT_DEPLOY_FORCE_PERSISTENT_BOOT_DEVICE",
		},
	}

	for _, tt := range []string{"", " (with img builder)"} {
		for _, tc := range cases {
			t.Run(tc.name+tt, func(t *testing.T) {
				defer tc.env.TearDown()
				tc.env.SetUp()
				imgBuild := tt != ""
				config, err := loadConfigFromEnv(imgBuild)
				expectedError := tc.expectedError
				if imgBuild {
					expectedError = tc.expectedImgBuildError
				}
				if expectedError != "" {
					assert.Regexp(t, expectedError, err)
				} else {
					require.NoError(t, err)
					tc.env.VerifyConfig(t, config, tc.forcePersistent)
				}
			})
		}
	}
}

func TestLoadEndpointsFromEnv(t *testing.T) {
	cases := []struct {
		name        string
		env         EnvFixture
		expectError bool
	}{
		{
			name: "with-ironic",
			env: EnvFixture{
				ironicEndpoint: "http://ironic.test",
			},
		}, {
			name:        "without-ironic",
			env:         EnvFixture{},
			expectError: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer tc.env.TearDown()
			tc.env.SetUp()
			i, err := loadEndpointsFromEnv()
			if tc.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				tc.env.VerifyEndpoints(t, i)
			}
		})
	}
}
func TestLoadTLSConfigFromEnv(t *testing.T) {
	cases := []struct {
		name              string
		env               EnvFixture
		expectedTLSConfig clients.TLSConfig
	}{
		{
			name: "default values",
			env:  EnvFixture{},
			expectedTLSConfig: clients.TLSConfig{
				TrustedCAFile:         "/opt/metal3/certs/ca/tls.crt",
				ClientCertificateFile: "/opt/metal3/certs/client/tls.crt",
				ClientPrivateKeyFile:  "/opt/metal3/certs/client/tls.key",
				InsecureSkipVerify:    false,
				SkipClientSANVerify:   false,
			},
		},
		{
			name: "custom file paths",
			env: EnvFixture{
				ironicCACertFile:           "/custom/ca.crt",
				ironicClientCertFile:       "/custom/client.crt",
				ironicClientPrivateKeyFile: "/custom/client.key",
			},
			expectedTLSConfig: clients.TLSConfig{
				TrustedCAFile:         "/custom/ca.crt",
				ClientCertificateFile: "/custom/client.crt",
				ClientPrivateKeyFile:  "/custom/client.key",
				InsecureSkipVerify:    false,
				SkipClientSANVerify:   false,
			},
		},
		{
			name: "insecure true",
			env: EnvFixture{
				ironicInsecure: "true",
			},
			expectedTLSConfig: clients.TLSConfig{
				TrustedCAFile:         "/opt/metal3/certs/ca/tls.crt",
				ClientCertificateFile: "/opt/metal3/certs/client/tls.crt",
				ClientPrivateKeyFile:  "/opt/metal3/certs/client/tls.key",
				InsecureSkipVerify:    true,
				SkipClientSANVerify:   false,
			},
		},
		{
			name: "skip client SAN verify true",
			env: EnvFixture{
				ironicSkipClientSANVerify: "true",
			},
			expectedTLSConfig: clients.TLSConfig{
				TrustedCAFile:         "/opt/metal3/certs/ca/tls.crt",
				ClientCertificateFile: "/opt/metal3/certs/client/tls.crt",
				ClientPrivateKeyFile:  "/opt/metal3/certs/client/tls.key",
				InsecureSkipVerify:    false,
				SkipClientSANVerify:   true,
			},
		},
		{
			name: "case insensitive boolean values",
			env: EnvFixture{
				ironicInsecure:            "TRUE",
				ironicSkipClientSANVerify: "True",
			}, expectedTLSConfig: clients.TLSConfig{
				TrustedCAFile:         "/opt/metal3/certs/ca/tls.crt",
				ClientCertificateFile: "/opt/metal3/certs/client/tls.crt",
				ClientPrivateKeyFile:  "/opt/metal3/certs/client/tls.key",
				InsecureSkipVerify:    true,
				SkipClientSANVerify:   true,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer tc.env.TearDown()
			tc.env.SetUp()

			result := loadTLSConfigFromEnv()
			assert.Equal(t, tc.expectedTLSConfig, result)
		})
	}
}

func newFakeIronicClient(endpoint string) *gophercloud.ServiceClient {
	client, err := noauth.NewBareMetalNoAuth(noauth.EndpointOpts{
		IronicEndpoint: endpoint,
	})
	if err != nil {
		panic(err)
	}
	return client
}

func newTestCacheFactory(cacheTTL time.Duration) ironicProvisionerFactory {
	return ironicProvisionerFactory{
		log:      logr.Discard(),
		cache:    new(ironicProvisionerCache),
		cacheTTL: cacheTTL,
	}
}

func TestRefreshCachePopulatesOnFirstCall(t *testing.T) {
	ironic := testserver.NewIronic(t).WithDrivers()
	ironic.Start()
	defer ironic.Stop()

	factory := newTestCacheFactory(5 * time.Second)
	client := newFakeIronicClient(ironic.Endpoint())

	ironicClient, features, err := factory.refreshCache(t.Context(), func() (*gophercloud.ServiceClient, error) {
		return client, nil
	})
	require.NoError(t, err)
	assert.Equal(t, 95, features.MaxVersion)
	assert.NotNil(t, ironicClient)
	assert.True(t, time.Now().Before(factory.cache.expiresAt))
}

func TestRefreshCacheReturnsCachedValues(t *testing.T) {
	ironic := testserver.NewIronic(t).WithDrivers()
	ironic.Start()
	defer ironic.Stop()

	factory := newTestCacheFactory(5 * time.Second)
	client := newFakeIronicClient(ironic.Endpoint())

	callCount := 0
	createClient := func() (*gophercloud.ServiceClient, error) {
		callCount++
		return client, nil
	}

	_, _, err := factory.refreshCache(t.Context(), createClient)
	require.NoError(t, err)
	assert.Equal(t, 1, callCount)

	ironicClient, features, err := factory.refreshCache(t.Context(), createClient)
	require.NoError(t, err)
	assert.Equal(t, 1, callCount, "createClient should not be called again while cache is fresh")
	assert.Equal(t, 95, features.MaxVersion)
	assert.NotNil(t, ironicClient)
}

func TestRefreshCacheRefreshesAfterExpiry(t *testing.T) {
	ironic := testserver.NewIronic(t).WithDrivers()
	ironic.Start()
	defer ironic.Stop()

	factory := newTestCacheFactory(1 * time.Millisecond)
	client := newFakeIronicClient(ironic.Endpoint())

	callCount := 0
	createClient := func() (*gophercloud.ServiceClient, error) {
		callCount++
		return client, nil
	}

	_, _, err := factory.refreshCache(t.Context(), createClient)
	require.NoError(t, err)
	assert.Equal(t, 1, callCount)

	time.Sleep(2 * time.Millisecond)

	_, _, err = factory.refreshCache(t.Context(), createClient)
	require.NoError(t, err)
	assert.Equal(t, 2, callCount, "createClient should be called again after expiry")
}

func TestRefreshCacheDoesNotCacheOnError(t *testing.T) {
	factory := newTestCacheFactory(5 * time.Second)

	callCount := 0
	createClient := func() (*gophercloud.ServiceClient, error) {
		callCount++
		return nil, errors.New("connection refused")
	}

	_, _, err := factory.refreshCache(t.Context(), createClient)
	require.Error(t, err)
	assert.Equal(t, 1, callCount)
	assert.True(t, factory.cache.expiresAt.IsZero(), "cache expiry should not be set on error")

	_, _, err = factory.refreshCache(t.Context(), createClient)
	require.Error(t, err)
	assert.Equal(t, 2, callCount, "should retry after previous failure")
}

func TestRefreshCacheDoesNotCacheOnIronicFailure(t *testing.T) {
	ironic := testserver.NewIronic(t).NotReady(http.StatusServiceUnavailable)
	ironic.Start()
	defer ironic.Stop()

	factory := newTestCacheFactory(5 * time.Second)
	client := newFakeIronicClient(ironic.Endpoint())

	_, _, err := factory.refreshCache(t.Context(), func() (*gophercloud.ServiceClient, error) {
		return client, nil
	})
	require.ErrorIs(t, err, provisioner.ErrNotReady)
	assert.True(t, factory.cache.expiresAt.IsZero())
}

func TestRefreshCacheDoesNotCacheOnNoDrivers(t *testing.T) {
	ironic := testserver.NewIronic(t).WithNoDrivers()
	ironic.Start()
	defer ironic.Stop()

	factory := newTestCacheFactory(5 * time.Second)
	client := newFakeIronicClient(ironic.Endpoint())

	_, _, err := factory.refreshCache(t.Context(), func() (*gophercloud.ServiceClient, error) {
		return client, nil
	})
	require.ErrorIs(t, err, provisioner.ErrNotReady)
	assert.True(t, factory.cache.expiresAt.IsZero())
}

func TestRefreshCacheDisabled(t *testing.T) {
	ironic := testserver.NewIronic(t).WithDrivers()
	ironic.Start()
	defer ironic.Stop()

	factory := newTestCacheFactory(0)
	client := newFakeIronicClient(ironic.Endpoint())

	callCount := 0
	createClient := func() (*gophercloud.ServiceClient, error) {
		callCount++
		return client, nil
	}

	_, features, err := factory.refreshCache(t.Context(), createClient)
	require.NoError(t, err)
	assert.Equal(t, 95, features.MaxVersion)

	_, _, err = factory.refreshCache(t.Context(), createClient)
	require.NoError(t, err)
	assert.Equal(t, 2, callCount, "createClient should always be called when cache is disabled")
	assert.True(t, factory.cache.expiresAt.IsZero(), "cache should not be populated when disabled")
}

func generateTestCACert(t *testing.T) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
}

func TestIronicProvisionerFromCR_CleansTempCAFile(t *testing.T) {
	env := EnvFixture{}
	defer env.TearDown()
	env.SetUp()

	scheme := runtime.NewScheme()
	require.NoError(t, ironicv1alpha1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	ironicCR := &ironicv1alpha1.Ironic{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-ironic",
			Namespace:  "test-ns",
			Generation: 1,
		},
		Spec: ironicv1alpha1.IronicSpec{
			TLS: ironicv1alpha1.TLS{
				CertificateName: "ironic-tls-cert",
			},
			APICredentialsName: "ironic-auth",
		},
		Status: ironicv1alpha1.IronicStatus{
			Conditions: []metav1.Condition{
				{
					Type:               string(ironicv1alpha1.IronicStatusReady),
					Status:             metav1.ConditionTrue,
					ObservedGeneration: 1,
				},
			},
		},
	}

	tlsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ironic-tls-cert",
			Namespace: "test-ns",
		},
		Data: map[string][]byte{
			"tls.crt": generateTestCACert(t),
		},
	}

	authSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ironic-auth",
			Namespace: "test-ns",
		},
		Data: map[string][]byte{
			"username": []byte("admin"),
			"password": []byte("password"),
		},
	}

	k8sClient := fakeclient.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(ironicCR, tlsSecret, authSecret).
		WithStatusSubresource(ironicCR).
		Build()

	factory := ironicProvisionerFactory{
		log:             logf.Log,
		config:          ironicConfig{maxBusyHosts: 20},
		cache:           new(ironicProvisionerCache),
		cacheTTL:        5 * time.Second,
		k8sClient:       k8sClient,
		apiReader:       k8sClient,
		ironicName:      "test-ironic",
		ironicNamespace: "test-ns",
	}

	host := metal3api.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-host",
			Namespace: "test-ns",
			UID:       "test-uid",
		},
		Spec: metal3api.BareMetalHostSpec{
			BMC: metal3api.BMCDetails{
				Address: "test://test.bmc/",
			},
		},
	}
	hostData := provisioner.BuildHostData(host, bmc.Credentials{})

	tempDir := t.TempDir()
	t.Setenv("TMPDIR", tempDir)

	getTempCAFiles := func() (result []string) {
		files, err := os.ReadDir(tempDir)
		require.NoError(t, err)
		for _, f := range files {
			if strings.HasPrefix(f.Name(), "ironic-ca-") {
				result = append(result, f.Name())
			}
		}
		return
	}

	require.Empty(t, getTempCAFiles())

	// ironicProvisioner will fail because the Ironic CR endpoint is not
	// reachable, but the createClient closure must still clean up the
	// temp CA file via defer.
	_, err := factory.ironicProvisioner(t.Context(), hostData, nullEventPublisher)
	require.Error(t, err)
	// ErrNotReady means the client was created successfully (the CA file
	// was read and the TLS transport configured), but Ironic is unreachable.
	require.ErrorIs(t, err, provisioner.ErrNotReady)

	assert.Empty(t, getTempCAFiles(), "temp CA file was not cleaned up")
}
