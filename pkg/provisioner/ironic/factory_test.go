package ironic

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

type EnvFixture struct {
	ironicEndpoint    string
	inspectorEndpoint string
	kernelURL         string
	ramdiskURL        string
	isoURL            string
}

func tearDown() {
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "IRONIC_") && !strings.HasPrefix(e, "DEPLOY_") {
			continue
		}
		parts := strings.SplitN(e, "=", 2)
		os.Unsetenv(parts[0])
	}
}

func (f EnvFixture) SetUp() {
	tearDown()

	if f.ironicEndpoint != "" {
		os.Setenv("IRONIC_ENDPOINT", f.ironicEndpoint)
	}
	if f.inspectorEndpoint != "" {
		os.Setenv("IRONIC_INSPECTOR_ENDPOINT", f.inspectorEndpoint)
	}
	if f.kernelURL != "" {
		os.Setenv("DEPLOY_KERNEL_URL", f.kernelURL)
	}
	if f.ramdiskURL != "" {
		os.Setenv("DEPLOY_RAMDISK_URL", f.ramdiskURL)
	}
	if f.isoURL != "" {
		os.Setenv("DEPLOY_ISO_URL", f.isoURL)
	}
}

func (f EnvFixture) Verify(t *testing.T) {
	assert.Equal(t, f.ironicEndpoint, ironicEndpoint)
	assert.Equal(t, f.inspectorEndpoint, inspectorEndpoint)
	assert.Equal(t, f.kernelURL, deployKernelURL)
	assert.Equal(t, f.ramdiskURL, deployRamdiskURL)
	assert.Equal(t, f.isoURL, deployISOURL)
}

func TestLoadFromEnv(t *testing.T) {
	defer tearDown()

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
