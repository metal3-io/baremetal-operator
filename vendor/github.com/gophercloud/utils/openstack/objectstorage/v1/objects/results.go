package objects

import (
	"io"
)

type DownloadResult struct {
	Action    string
	Container string
	Content   io.ReadCloser
	Object    string
	Path      string
	PseudoDir bool
	Success   bool
}

type UploadResult struct {
	Action      string
	Container   string
	LargeObject bool
	Object      string
	Path        string
	Status      string
	Success     bool
}
