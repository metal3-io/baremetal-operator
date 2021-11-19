package imageprovider

import (
	"fmt"
	"os"

	metal3 "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
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

func (eip envImageProvider) SupportsArchitecture(arch string) bool {
	return true
}

func (eip envImageProvider) SupportsFormat(format metal3.ImageFormat) bool {
	switch format {
	case metal3.ImageFormatISO:
		return eip.isoURL != ""
	case metal3.ImageFormatInitRD:
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

func (eip envImageProvider) BuildImage(format metal3.ImageFormat) (url string, err error) {
	switch format {
	case metal3.ImageFormatISO:
		url = eip.isoURL
	case metal3.ImageFormatInitRD:
		url = eip.initrdURL
	}
	if url == "" {
		err = fmt.Errorf("Unsupported image format \"%s\"", format)
	}
	return
}
