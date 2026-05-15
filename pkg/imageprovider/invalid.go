package imageprovider

type ImageBuildInvalidError struct {
	err error
}

func (ibf ImageBuildInvalidError) Error() string {
	return "Cannot generate image: " + ibf.err.Error()
}

func (ibf ImageBuildInvalidError) Unwrap() error {
	return ibf.err
}

func BuildInvalidError(err error) ImageBuildInvalidError {
	return ImageBuildInvalidError{err: err}
}

// ImageBuildInvalid has been deprecated in favor of ImageBuildInvalidError
//
// Deprecated: Use ImageBuildInvalidError instead.
type ImageBuildInvalid = ImageBuildInvalidError //nolint:errname

type ImageNotReadyError struct{}

func (inr ImageNotReadyError) Error() string {
	return "Image is not ready yet"
}

// ImageNotReady has been deprecated in favor of ImageNotReadyError
//
// Deprecated: Use ImageNotReadyError instead.
type ImageNotReady = ImageNotReadyError //nolint:errname
