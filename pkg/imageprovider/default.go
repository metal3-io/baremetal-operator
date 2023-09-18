package imageprovider

import (
	"fmt"
	"os"

	"github.com/go-logr/logr"

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

func (eip envImageProvider) SupportsArchitecture(_ string) bool {
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

func (eip envImageProvider) BuildImage(data ImageData, _ NetworkData, _ logr.Logger) (image GeneratedImage, err error) {
	switch data.Format {
	case metal3.ImageFormatISO:
		image.ImageURL = eip.isoURL
	case metal3.ImageFormatInitRD:
		image.ImageURL = eip.initrdURL
	default:
		err = BuildInvalidError(fmt.Errorf("Unsupported image format \"%s\"", data.Format))
	}
	return
}

func (eip envImageProvider) DiscardImage(_ ImageData) error {
	return nil
}
