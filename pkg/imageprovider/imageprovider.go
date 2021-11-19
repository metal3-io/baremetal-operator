package imageprovider

import (
	metal3 "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ImageData contains information about the image type being requested, and
// metadata about the request.
type ImageData struct {
	ImageMetadata     *metav1.ObjectMeta
	Format            metal3.ImageFormat
	Architecture      string
	NetworkDataStatus metal3.SecretStatus
}

type NetworkData map[string][]byte

type ImageProvider interface {
	// SupportsArchitecture returns whether the ImageProvider can provide
	// images for the given processor architecture.
	SupportsArchitecture(string) bool

	// SupportsFormat returns whether the ImageProvider can provide images in
	// the given format.
	SupportsFormat(metal3.ImageFormat) bool

	// BuildImage requests the ImageProvider to build an image with the
	// supplied network data and return a URL where it can be accessed.
	BuildImage(ImageData, NetworkData) (string, error)

	// DiscardImage notifies the ImageProvider that a previously built image
	// is no longer required.
	DiscardImage(ImageData) error
}
