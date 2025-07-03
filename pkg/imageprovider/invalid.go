package imageprovider

type ImageBuildInvalid struct {
	err error
}

func (ibf ImageBuildInvalid) Error() string {
	return "Cannot generate image: " + ibf.err.Error()
}

func (ibf ImageBuildInvalid) Unwrap() error {
	return ibf.err
}

func BuildInvalidError(err error) ImageBuildInvalid {
	return ImageBuildInvalid{err: err}
}

type ImageNotReady struct{}

func (inr ImageNotReady) Error() string {
	return "Image is not ready yet"
}
