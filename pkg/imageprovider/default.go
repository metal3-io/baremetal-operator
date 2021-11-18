package imageprovider

import (
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

func (eip envImageProvider) BuildImage(acceptFormats []metal3.ImageFormat) (url string, format metal3.ImageFormat, errorMessage string) {
	for _, fmt := range acceptFormats {
		switch fmt {
		case metal3.ImageFormatISO:
			if iso := eip.isoURL; iso != "" {
				return iso, fmt, ""
			}
			if errorMessage == "" {
				format = fmt
				errorMessage = "No DEPLOY_ISO_URL specified"
			}
		case metal3.ImageFormatInitRD:
			if initrd := eip.initrdURL; initrd != "" {
				return initrd, fmt, ""
			}
			if errorMessage == "" {
				format = fmt
				errorMessage = "No DEPLOY_RAMDISK_URL specified"
			}
		}
	}
	return
}
