package ironic

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

type EnvFixture struct {
	ironicEndpoint                   string
	kernelURL                        string
	ramdiskURL                       string
	isoURL                           string
	liveISOForcePersistentBootDevice string

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
}

func (f EnvFixture) VerifyConfig(t *testing.T, c ironicConfig, _ string) {
	t.Helper()
	assert.Equal(t, f.kernelURL, c.deployKernelURL)
	assert.Equal(t, f.ramdiskURL, c.deployRamdiskURL)
	assert.Equal(t, f.isoURL, c.deployISOURL)
	assert.Equal(t, f.liveISOForcePersistentBootDevice, c.liveISOForcePersistentBootDevice)
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
			name:          "no deploy info",
			env:           EnvFixture{},
			expectedError: "either DEPLOY_KERNEL_URL and DEPLOY_RAMDISK_URL or DEPLOY_ISO_URL must be set",
		},
		{
			name: "only kernel",
			env: EnvFixture{
				kernelURL: "http://kernel",
			},
			expectedError: "either DEPLOY_KERNEL_URL and DEPLOY_RAMDISK_URL or DEPLOY_ISO_URL must be set",
		},
		{
			name: "only ramdisk",
			env: EnvFixture{
				ramdiskURL: "http://ramdisk",
			},
			expectedError:         "either DEPLOY_KERNEL_URL and DEPLOY_RAMDISK_URL or DEPLOY_ISO_URL must be set",
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
			name: "Force Persistent Default",
			env: EnvFixture{
				isoURL:                           "http://iso",
				liveISOForcePersistentBootDevice: "Default",
			},
			forcePersistent: "Default",
		},
		{
			name: "Force Persistent Never",
			env: EnvFixture{
				isoURL:                           "http://iso",
				liveISOForcePersistentBootDevice: "Never",
			},
			forcePersistent: "Never",
		},
		{
			name: "Force Persistent Always",
			env: EnvFixture{
				isoURL:                           "http://iso",
				liveISOForcePersistentBootDevice: "Always",
			},
			forcePersistent: "Always",
		},
		{
			name: "Force Persistent Invalid",
			env: EnvFixture{
				isoURL:                           "http://iso",
				liveISOForcePersistentBootDevice: "NotAValidOption",
			},
			expectedError:         "invalid value for variable LIVE_ISO_FORCE_PERSISTENT_BOOT_DEVICE",
			expectedImgBuildError: "invalid value for variable LIVE_ISO_FORCE_PERSISTENT_BOOT_DEVICE",
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
					assert.Nil(t, err)
					tc.env.VerifyConfig(t, config, tc.forcePersistent)
				}
			})
		}
	}
}

func TestLoadTLSConfigFromEnv(t *testing.T) {
	const (
		TLSKeyFilePath  = "/opt/metal3/certs/client/tls.key"
		TLSCertFilePath = "/opt/metal3/certs/client/tls.crt"
	)

	os.Setenv("IRONIC_CACERT_FILE", "/path/to/ca.crt")
	os.Setenv("IRONIC_CLIENT_CERT_FILE", "/path/to/client.crt")
	os.Setenv("IRONIC_CLIENT_PRIVATE_KEY_FILE", "/path/to/client.key")
	os.Setenv("IRONIC_INSECURE", "false")
	os.Setenv("IRONIC_SKIP_CLIENT_SAN_VERIFY", "true")
	defer func() {
		os.Unsetenv("IRONIC_CACERT_FILE")
		os.Unsetenv("IRONIC_CLIENT_CERT_FILE")
		os.Unsetenv("IRONIC_CLIENT_PRIVATE_KEY_FILE")
		os.Unsetenv("IRONIC_INSECURE")
		os.Unsetenv("IRONIC_SKIP_CLIENT_SAN_VERIFY")
	}()

	tlsConfig := loadTLSConfigFromEnv()

	if tlsConfig.TrustedCAFile != "/path/to/ca.crt" ||
		tlsConfig.ClientCertificateFile != "/path/to/client.crt" ||
		tlsConfig.ClientPrivateKeyFile != "/path/to/client.key" ||
		tlsConfig.InsecureSkipVerify != false ||
		tlsConfig.SkipClientSANVerify != true {
		t.Errorf("Unexpected TLS config values")
	}

	os.Setenv("IRONIC_CACERT_FILE", "/path/to/ca.crt")
	os.Unsetenv("IRONIC_CLIENT_CERT_FILE")
	os.Unsetenv("IRONIC_CLIENT_PRIVATE_KEY_FILE")
	os.Unsetenv("IRONIC_INSECURE")
	os.Unsetenv("IRONIC_SKIP_CLIENT_SAN_VERIFY")
	defer func() {
		os.Unsetenv("IRONIC_CACERT_FILE")
		os.Unsetenv("IRONIC_CLIENT_CERT_FILE")
		os.Unsetenv("IRONIC_CLIENT_PRIVATE_KEY_FILE")
		os.Unsetenv("IRONIC_INSECURE")
		os.Unsetenv("IRONIC_SKIP_CLIENT_SAN_VERIFY")
	}()

	tlsConfig = loadTLSConfigFromEnv()

	if tlsConfig.TrustedCAFile != "/path/to/ca.crt" ||
		tlsConfig.ClientCertificateFile != TLSCertFilePath ||
		tlsConfig.ClientPrivateKeyFile != TLSKeyFilePath ||
		tlsConfig.InsecureSkipVerify != false ||
		tlsConfig.SkipClientSANVerify != false {
		t.Errorf("Unexpected TLS config values")
	}

	os.Setenv("IRONIC_CACERT_FILE", "")
	os.Setenv("IRONIC_CLIENT_CERT_FILE", "")
	os.Setenv("IRONIC_CLIENT_PRIVATE_KEY_FILE", "")
	os.Setenv("IRONIC_INSECURE", "")
	os.Setenv("IRONIC_SKIP_CLIENT_SAN_VERIFY", "")
	defer func() {
		os.Unsetenv("IRONIC_CACERT_FILE")
		os.Unsetenv("IRONIC_CLIENT_CERT_FILE")
		os.Unsetenv("IRONIC_CLIENT_PRIVATE_KEY_FILE")
		os.Unsetenv("IRONIC_INSECURE")
		os.Unsetenv("IRONIC_SKIP_CLIENT_SAN_VERIFY")
	}()

	tlsConfig = loadTLSConfigFromEnv()

	if tlsConfig.TrustedCAFile != "/opt/metal3/certs/ca/tls.crt" ||
		tlsConfig.ClientCertificateFile != TLSCertFilePath ||
		tlsConfig.ClientPrivateKeyFile != TLSKeyFilePath ||
		tlsConfig.InsecureSkipVerify != false ||
		tlsConfig.SkipClientSANVerify != false {
		t.Errorf("Unexpected TLS config values")
	}
}
