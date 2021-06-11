package ironic

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

type EnvFixture struct {
	ironicEndpoint    string
	inspectorEndpoint string
	kernelURL         string
	ramdiskURL        string
	isoURL            string

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
	f.replace("IRONIC_INSPECTOR_ENDPOINT", f.inspectorEndpoint)
	f.replace("DEPLOY_KERNEL_URL", f.kernelURL)
	f.replace("DEPLOY_RAMDISK_URL", f.ramdiskURL)
	f.replace("DEPLOY_ISO_URL", f.isoURL)
}

func (f EnvFixture) Verify(t *testing.T) {
	assert.Equal(t, f.ironicEndpoint, ironicEndpoint)
	assert.Equal(t, f.inspectorEndpoint, inspectorEndpoint)
	assert.Equal(t, f.kernelURL, deployKernelURL)
	assert.Equal(t, f.ramdiskURL, deployRamdiskURL)
	assert.Equal(t, f.isoURL, deployISOURL)
}

func TestLoadFromEnv(t *testing.T) {

	cases := []struct {
		name          string
		env           EnvFixture
		expectedError string
	}{
		{
			name: "kernel and ramdisk",
			env: EnvFixture{
				ironicEndpoint:    "http://ironic",
				inspectorEndpoint: "http://inspector",
				kernelURL:         "http://kernel",
				ramdiskURL:        "http://ramdisk",
			},
		},
		{
			name: "ISO only",
			env: EnvFixture{
				ironicEndpoint:    "http://ironic",
				inspectorEndpoint: "http://inspector",
				isoURL:            "http://iso",
			},
		},
		{
			name: "ISO and kernel/ramdisk",
			env: EnvFixture{
				ironicEndpoint:    "http://ironic",
				inspectorEndpoint: "http://inspector",
				isoURL:            "http://iso",
				kernelURL:         "http://kernel",
				ramdiskURL:        "http://ramdisk",
			},
		},
		{
			name: "no deploy info",
			env: EnvFixture{
				ironicEndpoint:    "http://ironic",
				inspectorEndpoint: "http://inspector",
			},
			expectedError: "Either DEPLOY_KERNEL_URL and DEPLOY_RAMDISK_URL or DEPLOY_ISO_URL must be set",
		},
		{
			name: "only kernel",
			env: EnvFixture{
				ironicEndpoint:    "http://ironic",
				inspectorEndpoint: "http://inspector",
				kernelURL:         "http://kernel",
			},
			expectedError: "Either DEPLOY_KERNEL_URL and DEPLOY_RAMDISK_URL or DEPLOY_ISO_URL must be set",
		},
		{
			name: "only ramdisk",
			env: EnvFixture{
				ironicEndpoint:    "http://ironic",
				inspectorEndpoint: "http://inspector",
				ramdiskURL:        "http://ramdisk",
			},
			expectedError: "Either DEPLOY_KERNEL_URL and DEPLOY_RAMDISK_URL or DEPLOY_ISO_URL must be set",
		},
		{
			name: "ISO and kernel",
			env: EnvFixture{
				ironicEndpoint:    "http://ironic",
				inspectorEndpoint: "http://inspector",
				kernelURL:         "http://kernel",
				isoURL:            "http://iso",
			},
			expectedError: "DEPLOY_KERNEL_URL and DEPLOY_RAMDISK_URL can only be set together",
		},
		{
			name: "ISO and ramdisk",
			env: EnvFixture{
				ironicEndpoint:    "http://ironic",
				inspectorEndpoint: "http://inspector",
				ramdiskURL:        "http://ramdisk",
				isoURL:            "http://iso",
			},
			expectedError: "DEPLOY_KERNEL_URL and DEPLOY_RAMDISK_URL can only be set together",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer tc.env.TearDown()
			tc.env.SetUp()
			err := loadConfigFromEnv()
			if tc.expectedError != "" {
				assert.Regexp(t, tc.expectedError, err, "error message")
			} else {
				assert.Nil(t, err)
				tc.env.Verify(t)
			}
		})
	}
}
