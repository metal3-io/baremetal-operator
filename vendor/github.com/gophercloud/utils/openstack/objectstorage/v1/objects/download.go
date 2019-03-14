package objects

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/objectstorage/v1/containers"
	"github.com/gophercloud/gophercloud/openstack/objectstorage/v1/objects"
	"github.com/gophercloud/gophercloud/pagination"
)

// DownloadOpts represents options used for downloading an object.
type DownloadOpts struct {
	// Delimiter is a delimiter to specify for listing objects.
	Delimiter string

	// IgnoreMtime won't update the downloaded file's mtime.
	IgnoreMtime bool

	// NoDownload won't actually download the object.
	NoDownload bool

	// OutDirectory is a directory to save the objects to.
	OutDirectory string

	// OutFile is a file to save the object to.
	OutFile string

	// Prefix is a prefix string for a container.
	Prefix string

	// RemovePrefix will remove the prefix from the container.
	RemovePrefix bool

	// SkipIdentical will skip identical objects already downloaded.
	SkipIdentical bool

	// YesAll will download everything.
	YesAll bool
}

// Download downloads one or more objects from an Object Storage account.
// It is roughly based on the python-swiftclient implementation:
//
// https://github.com/openstack/python-swiftclient/blob/e65070964c7b1e04119c87e5f344d39358780d18/swiftclient/service.py#L1024
func Download(client *gophercloud.ServiceClient, containerName string, objectNames []string, opts *DownloadOpts) ([]DownloadResult, error) {
	var downloadResults []DownloadResult

	if strings.Contains(containerName, "/") {
		return nil, fmt.Errorf("container name %s contains a /", containerName)
	}

	if containerName == "" {
		if opts.YesAll {
			// Download everything
			listOpts := containers.ListOpts{
				Full:      true,
				Delimiter: opts.Delimiter,
				Prefix:    opts.Prefix,
			}

			err := containers.List(client, listOpts).EachPage(func(page pagination.Page) (bool, error) {
				containerList, err := containers.ExtractInfo(page)
				if err != nil {
					return false, fmt.Errorf("error listing containers: %s", err)
				}

				for _, c := range containerList {
					results, err := downloadContainer(client, c.Name, opts)
					if err != nil {
						return false, fmt.Errorf("error downloading container %s: %s", c.Name, err)
					}

					downloadResults = append(downloadResults, results...)
				}

				return true, nil
			})

			if err != nil {
				return nil, fmt.Errorf("error downloading container %s: %s", containerName, err)
			}

			return downloadResults, nil
		}
	}

	if len(objectNames) == 0 {
		results, err := downloadContainer(client, containerName, opts)
		if err != nil {
			return nil, fmt.Errorf("error downloading container %s: %s", containerName, err)
		}
		downloadResults = append(downloadResults, results...)

		return downloadResults, nil
	}

	for _, objectName := range objectNames {
		result, err := downloadObject(client, containerName, objectName, opts)
		if err != nil {
			return nil, fmt.Errorf("error downloading object %s/%s: %s", containerName, objectName, err)
		}
		downloadResults = append(downloadResults, *result)
	}

	return downloadResults, nil
}

// downloadObject will download a specified object.
func downloadObject(client *gophercloud.ServiceClient, containerName string, objectName string, opts *DownloadOpts) (*DownloadResult, error) {
	var objectDownloadOpts objects.DownloadOpts
	var pseudoDir bool

	// Perform a get on the object in order to get its metadata.
	originalObject := objects.Get(client, containerName, objectName, nil)
	if originalObject.Err != nil {
		return nil, fmt.Errorf("error retrieving object %s/%s: %s", containerName, objectName, originalObject.Err)
	}

	originalMetadata, err := originalObject.ExtractMetadata()
	if err != nil {
		return nil, fmt.Errorf("error extracting object metadata for %s/%s: %s", containerName, objectName, err)
	}

	objectPath := objectName
	if opts.YesAll {
		objectPath = path.Join(containerName, objectName)
	}

	// SkipIdentical is not possible when stdout has been specified.
	opts.SkipIdentical = opts.SkipIdentical && opts.OutFile != "-"

	if opts.Prefix != "" && opts.RemovePrefix {
		objectPath = string(objectPath[len(opts.Prefix):])
	}

	if opts.OutDirectory != "" {
		objectPath = path.Join(opts.OutDirectory, objectName)
	}

	filename := objectPath
	if opts.OutFile != "" && opts.OutFile != "-" {
		filename = opts.OutFile
	}

	// SkipIdentical will get the md5sum of the existing local file.
	// It'll use it in the If-None-Match header.
	if opts.SkipIdentical {
		objectDownloadOpts.MultipartManifest = "get"

		md5, err := FileMD5Sum(filename)
		if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("error getting md5sum of file %s: %s", filename, err)
		}

		if md5 != "" {
			objectDownloadOpts.IfNoneMatch = md5
		}
	}

	// Attempt to download the object
	res := objects.Download(client, containerName, objectName, objectDownloadOpts)
	if res.Err != nil {
		// Ignore the error if SkipIdentical is set.
		// This is because a second attempt to download the object will happen later.
		if !opts.SkipIdentical {
			return nil, fmt.Errorf("error getting object %s/%s: %s", containerName, objectName, res.Err)
		}
	}

	headers, err := res.Extract()
	if err != nil {
		return nil, fmt.Errorf("error extracting headers from %s: %s", objectName, err)
	}

	if opts.SkipIdentical {
		// Determine if the downloaded object has a manifest or is a Static Large
		// Object.
		//
		// This is a little odd. It should be doing the same thing that
		// python-swiftclient is doing, though.
		var hasManifest bool
		var manifest string
		if headers.ObjectManifest != "" {
			hasManifest = true
			manifest = ""
		}

		if headers.StaticLargeObject {
			hasManifest = true
			manifest = "[]"
		}

		if hasManifest {
			mo := GetManifestOpts{
				ContainerName:     containerName,
				ContentLength:     headers.ContentLength,
				ETag:              headers.ETag,
				ObjectName:        objectName,
				ObjectManifest:    headers.ObjectManifest,
				Manifest:          manifest,
				StaticLargeObject: headers.StaticLargeObject,
			}

			manifestData, err := GetManifest(client, mo)
			if err != nil {
				return nil, fmt.Errorf("unable to get manifest for %s/%s: %s", containerName, objectName, err)
			}

			if len(manifestData) > 0 {
				ok, err := IsIdentical(manifestData, filename)
				if err != nil {
					return nil, fmt.Errorf("error comparing object %s/%s and path %s: %s", containerName, objectName, filename, err)
				}

				if ok {
					downloadResult := &DownloadResult{
						Action:    "download_object",
						Container: containerName,
						Object:    objectName,
						Path:      objectPath,
						PseudoDir: pseudoDir,
						Success:   true,
					}

					return downloadResult, nil
				}

				// This is a Large object
				objectDownloadOpts.MultipartManifest = ""
				res = objects.Download(client, containerName, objectName, objectDownloadOpts)
				if res.Err != nil {
					return nil, fmt.Errorf("error downloading object %s/%s: %s", containerName, objectName, err)
				}
			}
		}
	}

	if opts.OutFile == "-" && !opts.NoDownload {
		downloadResult := &DownloadResult{
			Action:    "download_object",
			Container: containerName,
			Content:   res.Body,
			Object:    objectName,
			Path:      objectPath,
			PseudoDir: pseudoDir,
			Success:   true,
		}

		return downloadResult, nil
	}

	contentType := GetContentType(headers.ContentType)
	var ctMatch bool
	for _, kdm := range knownDirMarkers {
		if contentType == kdm {
			pseudoDir = true
			ctMatch = true
		}
	}

	// I'm not sure if 0777 is appropriate here.
	// It looks to be what python-swiftclient and python os.makedirs is doing.
	if ctMatch && opts.OutFile != "-" && !opts.NoDownload {
		if err := os.MkdirAll(objectPath, 0777); err != nil {
			return nil, fmt.Errorf("error creating directory %s: %s", objectPath, err)
		}
	} else {
		mkdir := !(opts.NoDownload || opts.OutFile == "")

		if mkdir {
			dir := filepath.Dir(objectPath)
			if _, err := os.Stat(dir); err != nil && os.IsNotExist(err) {
				if err := os.MkdirAll(dir, 0777); err != nil {
					return nil, fmt.Errorf("error creating directory %s: %s", dir, err)
				}
			}
		}

		var file string
		if !opts.NoDownload {
			if opts.OutFile != "" {
				file = opts.OutFile
			} else {
				if strings.HasSuffix(objectPath, "/") {
					pseudoDir = true
				} else {
					file = objectPath
				}
			}
		}

		if file != "" {
			f, err := os.Create(file)
			if err != nil {
				return nil, fmt.Errorf("error creating file %s: %s", file, err)
			}
			defer f.Close()

			buf := make([]byte, diskBuffer)
			for {
				chunk, err := res.Body.Read(buf)
				if err != nil && err != io.EOF {
					return nil, fmt.Errorf("error reading object %s/%s: %s", containerName, objectName, err)
				}

				if chunk == 0 {
					break
				}

				if _, err := f.Write(buf[:chunk]); err != nil {
					return nil, fmt.Errorf("error writing file %s: %s", file, err)
				}
			}
			f.Close()
		}

		if !opts.NoDownload && !opts.IgnoreMtime {
			if v, ok := originalMetadata["Mtime"]; ok {
				epoch, err := strconv.ParseInt(v, 10, 64)
				if err == nil {
					epoch = epoch * 1000000000
					mtime := time.Unix(epoch, 0)
					if err := os.Chtimes(file, mtime, mtime); err != nil {
						return nil, fmt.Errorf("error updating mtime for %s: %s", file, err)
					}
				}
			}
		}
	}

	downloadResult := &DownloadResult{
		Action:    "download_object",
		Success:   true,
		Container: containerName,
		Object:    objectName,
		Path:      objectPath,
		PseudoDir: pseudoDir,
	}

	return downloadResult, nil
}

// downloadContainer will download all objects in a given container.
func downloadContainer(client *gophercloud.ServiceClient, containerName string, opts *DownloadOpts) ([]DownloadResult, error) {
	listOpts := objects.ListOpts{
		Full:      true,
		Prefix:    opts.Prefix,
		Delimiter: opts.Delimiter,
	}

	var downloadResults []DownloadResult
	err := objects.List(client, containerName, listOpts).EachPage(func(page pagination.Page) (bool, error) {
		objectList, err := objects.ExtractNames(page)
		if err != nil {
			return false, fmt.Errorf("error listing container %s: %s", containerName, err)
		}

		for _, objectName := range objectList {
			result, err := downloadObject(client, containerName, objectName, opts)
			if err != nil {
				return false, fmt.Errorf("error downloading object %s/%s: %s", containerName, objectName, err)
			}

			downloadResults = append(downloadResults, *result)
		}

		return true, nil
	})

	if err != nil {
		return nil, fmt.Errorf("error downloading container %s: %s", containerName, err)
	}

	return downloadResults, nil
}
