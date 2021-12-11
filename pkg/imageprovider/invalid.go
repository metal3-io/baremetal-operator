package imageprovider

import "fmt"

type ImageBuildInvalid struct {
	err error
}

func (ibf ImageBuildInvalid) Error() string {
	return fmt.Sprintf("Cannot generate image: %s", ibf.err.Error())
}

func (ibf ImageBuildInvalid) Unwrap() error {
	return ibf.err
}

func BuildInvalidError(err error) ImageBuildInvalid {
	return ImageBuildInvalid{err: err}
}
