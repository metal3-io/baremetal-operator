package imageprovider

import (
	"testing"

	"github.com/go-logr/logr"
	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
)

func TestBuildImageInitRDArchSpecific(t *testing.T) {
	testCases := []struct {
		Scenario          string
		Arch              string
		EnvVars           map[string]string
		ExpectedKernelURL string
		ExpectedImageURL  string
	}{
		{
			Scenario: "default arch uses default env vars",
			Arch:     "x86_64",
			EnvVars: map[string]string{
				"DEPLOY_KERNEL_URL":  "http://example.com/kernel",
				"DEPLOY_RAMDISK_URL": "http://example.com/ramdisk",
			},
			ExpectedKernelURL: "http://example.com/kernel",
			ExpectedImageURL:  "http://example.com/ramdisk",
		},
		{
			Scenario: "aarch64 with arch-specific env vars",
			Arch:     "aarch64",
			EnvVars: map[string]string{
				"DEPLOY_KERNEL_URL":          "http://example.com/kernel",
				"DEPLOY_RAMDISK_URL":         "http://example.com/ramdisk",
				"DEPLOY_KERNEL_URL_AARCH64":  "http://example.com/kernel-aarch64",
				"DEPLOY_RAMDISK_URL_AARCH64": "http://example.com/ramdisk-aarch64",
			},
			ExpectedKernelURL: "http://example.com/kernel-aarch64",
			ExpectedImageURL:  "http://example.com/ramdisk-aarch64",
		},
		{
			Scenario: "aarch64 falls back to default when no arch-specific env vars",
			Arch:     "aarch64",
			EnvVars: map[string]string{
				"DEPLOY_KERNEL_URL":  "http://example.com/kernel",
				"DEPLOY_RAMDISK_URL": "http://example.com/ramdisk",
			},
			ExpectedKernelURL: "http://example.com/kernel",
			ExpectedImageURL:  "http://example.com/ramdisk",
		},
		{
			Scenario: "empty arch uses default env vars",
			Arch:     "",
			EnvVars: map[string]string{
				"DEPLOY_KERNEL_URL":  "http://example.com/kernel",
				"DEPLOY_RAMDISK_URL": "http://example.com/ramdisk",
			},
			ExpectedKernelURL: "http://example.com/kernel",
			ExpectedImageURL:  "http://example.com/ramdisk",
		},
		{
			Scenario: "only kernel is arch-specific",
			Arch:     "aarch64",
			EnvVars: map[string]string{
				"DEPLOY_KERNEL_URL":         "http://example.com/kernel",
				"DEPLOY_RAMDISK_URL":        "http://example.com/ramdisk",
				"DEPLOY_KERNEL_URL_AARCH64": "http://example.com/kernel-aarch64",
			},
			ExpectedKernelURL: "http://example.com/kernel-aarch64",
			ExpectedImageURL:  "http://example.com/ramdisk",
		},
		{
			Scenario: "aarch64 via BY_ARCH variables",
			Arch:     "aarch64",
			EnvVars: map[string]string{
				"DEPLOY_KERNEL_URL":      "http://example.com/kernel",
				"DEPLOY_RAMDISK_URL":     "http://example.com/ramdisk",
				"DEPLOY_KERNEL_BY_ARCH":  "x86_64:http://example.com/kernel-x86,aarch64:http://example.com/kernel-aarch64",
				"DEPLOY_RAMDISK_BY_ARCH": "x86_64:http://example.com/ramdisk-x86,aarch64:http://example.com/ramdisk-aarch64",
			},
			ExpectedKernelURL: "http://example.com/kernel-aarch64",
			ExpectedImageURL:  "http://example.com/ramdisk-aarch64",
		},
		{
			Scenario: "per-arch variable takes priority over BY_ARCH",
			Arch:     "aarch64",
			EnvVars: map[string]string{
				"DEPLOY_KERNEL_URL":          "http://example.com/kernel",
				"DEPLOY_RAMDISK_URL":         "http://example.com/ramdisk",
				"DEPLOY_KERNEL_URL_AARCH64":  "http://example.com/kernel-per-arch",
				"DEPLOY_KERNEL_BY_ARCH":      "aarch64:http://example.com/kernel-by-arch",
				"DEPLOY_RAMDISK_URL_AARCH64": "http://example.com/ramdisk-per-arch",
				"DEPLOY_RAMDISK_BY_ARCH":     "aarch64:http://example.com/ramdisk-by-arch",
			},
			ExpectedKernelURL: "http://example.com/kernel-per-arch",
			ExpectedImageURL:  "http://example.com/ramdisk-per-arch",
		},
		{
			Scenario: "BY_ARCH falls back to base when arch not listed",
			Arch:     "ppc64le",
			EnvVars: map[string]string{
				"DEPLOY_KERNEL_URL":     "http://example.com/kernel",
				"DEPLOY_RAMDISK_URL":    "http://example.com/ramdisk",
				"DEPLOY_KERNEL_BY_ARCH": "x86_64:http://example.com/kernel-x86,aarch64:http://example.com/kernel-aarch64",
			},
			ExpectedKernelURL: "http://example.com/kernel",
			ExpectedImageURL:  "http://example.com/ramdisk",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Scenario, func(t *testing.T) {
			for k, v := range tc.EnvVars {
				t.Setenv(k, v)
			}

			provider := envImageProvider{
				initrdURL: tc.EnvVars["DEPLOY_RAMDISK_URL"],
			}

			image, err := provider.BuildImage(
				ImageData{
					Format:       metal3api.ImageFormatInitRD,
					Architecture: tc.Arch,
				},
				nil,
				logr.Discard(),
			)

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if image.KernelURL != tc.ExpectedKernelURL {
				t.Errorf("expected KernelURL %q, got %q", tc.ExpectedKernelURL, image.KernelURL)
			}
			if image.ImageURL != tc.ExpectedImageURL {
				t.Errorf("expected ImageURL %q, got %q", tc.ExpectedImageURL, image.ImageURL)
			}
		})
	}
}

func TestLookupByArch(t *testing.T) {
	testCases := []struct {
		Scenario string
		Value    string
		Arch     string
		Expected string
	}{
		{
			Scenario: "match first entry",
			Value:    "x86_64:http://example.com/kernel-x86,aarch64:http://example.com/kernel-arm",
			Arch:     "x86_64",
			Expected: "http://example.com/kernel-x86",
		},
		{
			Scenario: "match second entry",
			Value:    "x86_64:http://example.com/kernel-x86,aarch64:http://example.com/kernel-arm",
			Arch:     "aarch64",
			Expected: "http://example.com/kernel-arm",
		},
		{
			Scenario: "no match",
			Value:    "x86_64:http://example.com/kernel-x86,aarch64:http://example.com/kernel-arm",
			Arch:     "ppc64le",
			Expected: "",
		},
		{
			Scenario: "empty value",
			Value:    "",
			Arch:     "x86_64",
			Expected: "",
		},
		{
			Scenario: "empty arch",
			Value:    "x86_64:http://example.com/kernel-x86",
			Arch:     "",
			Expected: "",
		},
		{
			Scenario: "single entry",
			Value:    "aarch64:http://example.com/kernel-arm",
			Arch:     "aarch64",
			Expected: "http://example.com/kernel-arm",
		},
		{
			Scenario: "file URL with triple slash",
			Value:    "x86_64:file:///shared/html/images/ipa_x86.kernel,aarch64:file:///shared/html/images/ipa_arm64.kernel",
			Arch:     "aarch64",
			Expected: "file:///shared/html/images/ipa_arm64.kernel",
		},
		{
			Scenario: "case-insensitive arch match",
			Value:    "AARCH64:http://example.com/kernel-arm",
			Arch:     "aarch64",
			Expected: "http://example.com/kernel-arm",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Scenario, func(t *testing.T) {
			result := lookupByArch(tc.Value, tc.Arch)
			if result != tc.Expected {
				t.Errorf("expected %q, got %q", tc.Expected, result)
			}
		})
	}
}

func TestBuildImageISOArchSpecific(t *testing.T) {
	testCases := []struct {
		Scenario         string
		Arch             string
		EnvVars          map[string]string
		ExpectedImageURL string
	}{
		{
			Scenario: "default arch uses default ISO URL",
			Arch:     "x86_64",
			EnvVars: map[string]string{
				"DEPLOY_ISO_URL": "http://example.com/image.iso",
			},
			ExpectedImageURL: "http://example.com/image.iso",
		},
		{
			Scenario: "aarch64 with arch-specific ISO URL",
			Arch:     "aarch64",
			EnvVars: map[string]string{
				"DEPLOY_ISO_URL":         "http://example.com/image.iso",
				"DEPLOY_ISO_URL_AARCH64": "http://example.com/image-aarch64.iso",
			},
			ExpectedImageURL: "http://example.com/image-aarch64.iso",
		},
		{
			Scenario: "aarch64 falls back to default ISO URL",
			Arch:     "aarch64",
			EnvVars: map[string]string{
				"DEPLOY_ISO_URL": "http://example.com/image.iso",
			},
			ExpectedImageURL: "http://example.com/image.iso",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Scenario, func(t *testing.T) {
			for k, v := range tc.EnvVars {
				t.Setenv(k, v)
			}

			provider := envImageProvider{
				isoURL: tc.EnvVars["DEPLOY_ISO_URL"],
			}

			image, err := provider.BuildImage(
				ImageData{
					Format:       metal3api.ImageFormatISO,
					Architecture: tc.Arch,
				},
				nil,
				logr.Discard(),
			)

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if image.ImageURL != tc.ExpectedImageURL {
				t.Errorf("expected ImageURL %q, got %q", tc.ExpectedImageURL, image.ImageURL)
			}
		})
	}
}
