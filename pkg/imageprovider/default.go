package imageprovider

import (
	"fmt"
	"os"
	"strings"

	"github.com/go-logr/logr"
	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
)

type envImageProvider struct {
	isoURL    string
	initrdURL string
}

func NewDefaultImageProvider() ImageProvider {
	return envImageProvider{
		isoURL:    os.Getenv("DEPLOY_ISO_URL"),
		initrdURL: os.Getenv("DEPLOY_RAMDISK_URL"),
	}
}

// lookupByArch parses a BY_ARCH variable value (comma-separated arch:url pairs,
// e.g. "x86_64:http://example.com/kernel,aarch64:http://example.com/kernel-arm")
// and returns the URL for the given architecture, or empty string if not found.
func lookupByArch(byArchValue, arch string) string {
	if byArchValue == "" || arch == "" {
		return ""
	}
	for _, entry := range strings.Split(byArchValue, ",") {
		entryArch, entryURL, found := strings.Cut(entry, ":")
		if found && strings.EqualFold(entryArch, arch) {
			return entryURL
		}
	}
	return ""
}

// envWithArchFallback returns the URL for the given architecture by checking
// multiple environment variable formats in order of priority, matching the
// conventions used by ironic-image:
//
//  1. Per-arch variable: DEPLOY_KERNEL_URL_AARCH64
//  2. BY_ARCH variable:  DEPLOY_KERNEL_BY_ARCH=aarch64:http://...
//  3. Base variable:     DEPLOY_KERNEL_URL
//
// The BY_ARCH variable name is derived by replacing the _URL suffix with
// _BY_ARCH (e.g. DEPLOY_KERNEL_URL -> DEPLOY_KERNEL_BY_ARCH).
func envWithArchFallback(base, arch string) string {
	if arch != "" {
		// 1. Per-arch variable (e.g. DEPLOY_KERNEL_URL_AARCH64)
		if v := os.Getenv(base + "_" + strings.ToUpper(arch)); v != "" {
			return v
		}

		// 2. BY_ARCH variable (e.g. DEPLOY_KERNEL_BY_ARCH)
		byArchVar := strings.TrimSuffix(base, "_URL") + "_BY_ARCH"
		if v := lookupByArch(os.Getenv(byArchVar), arch); v != "" {
			return v
		}
	}

	// 3. Base variable (e.g. DEPLOY_KERNEL_URL)
	return os.Getenv(base)
}

func (eip envImageProvider) SupportsArchitecture(_ string) bool {
	return true
}

func (eip envImageProvider) SupportsFormat(format metal3api.ImageFormat) bool {
	switch format {
	case metal3api.ImageFormatISO:
		return eip.isoURL != ""
	case metal3api.ImageFormatInitRD:
		// Assume we are running inside the same process as the BMH controller -
		// if there is no kernel URL then it will be unable to use the initrd.
		if os.Getenv("DEPLOY_KERNEL_URL") == "" {
			return false
		}
		return eip.initrdURL != ""
	default:
		return false
	}
}

func (eip envImageProvider) BuildImage(data ImageData, _ NetworkData, _ logr.Logger) (image GeneratedImage, err error) {
	switch data.Format {
	case metal3api.ImageFormatISO:
		image.ImageURL = envWithArchFallback("DEPLOY_ISO_URL", data.Architecture)
	case metal3api.ImageFormatInitRD:
		image.KernelURL = envWithArchFallback("DEPLOY_KERNEL_URL", data.Architecture)
		image.ImageURL = envWithArchFallback("DEPLOY_RAMDISK_URL", data.Architecture)
	default:
		err = BuildInvalidError(fmt.Errorf("unsupported image format \"%s\"", data.Format))
	}
	return
}

func (eip envImageProvider) DiscardImage(_ ImageData) error {
	return nil
}
