package objects

import (
	"bytes"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/objectstorage/v1/containers"
	"github.com/gophercloud/gophercloud/openstack/objectstorage/v1/objects"
)

// UploadOpts represents options used for uploading an object.
type UploadOpts struct {
	// Changed will prevent an upload if the mtime and size of the source
	// and destination objects are the same.
	Changed bool

	// Checksum will enforce a comparison of the md5sum/etag between the
	// local and remote object to ensure integrity.
	Checksum bool

	// Content is an io.Reader which can be used to upload a object via an
	// open file descriptor or any other type of stream.
	Content io.Reader

	// DirMarker will create a directory marker.
	DirMarker bool

	// LeaveSegments will cause old segments of an object to be left in a
	// container.
	LeaveSegments bool

	// Metadata is optional metadata to place on the object.
	Metadata map[string]string

	// Path is a local filesystem path of an object to be uploaded.
	Path string

	// Segment container is a custom container name to store object segments.
	// If one is not specified, then "containerName_segments" will be used.
	SegmentContainer string

	// SegmentSize is the size of each segment. An object will be split into
	// pieces (segments) of this size.
	SegmentSize int64

	// SkipIdentical is a more thorough check than "Changed". It will compare
	// the md5sum/etag of the object as a comparison.
	SkipIdentical bool

	// StoragePolicy represents a storage policy of where the object should be
	// uploaded.
	StoragePolicy string

	// UseSLO will have the object uploaded using Static Large Object support.
	UseSLO bool
}

// originalObject is an interal structure used to store information about an
//  existing object.
type originalObject struct {
	headers  *objects.GetHeader
	metadata map[string]string
}

// uploadSegmentOpts is an internal structure used for handling the upload
// of an object's segment.
type uploadSegmentOpts struct {
	Checksum         bool
	ContainerName    string
	Content          io.Reader
	Path             string
	ObjectName       string
	SegmentContainer string
	SegmentName      string
	SegmentSize      int64
	SegmentStart     int64
	SegmentIndex     int
}

// uploadSegmentResult is an internal structure that represents the result
// result of a segment upload.
type uploadSegmentResult struct {
	Complete bool
	ETag     string
	Index    int
	Location string
	Size     int64
	Success  bool
}

// uploadSLOManifestOpts is an internal structure that represents
// options used for creating an SLO manifest.
type uploadSLOManifestOpts struct {
	Results       []uploadSegmentResult
	ContainerName string
	ObjectName    string
	Metadata      map[string]string
}

// sloManifest represents an SLO manifest.
type sloManifest struct {
	Path      string `json:"path"`
	ETag      string `json:"etag"`
	SizeBytes int64  `json:"size_bytes"`
}

// Upload uploads a single object to swift.
//
// https://github.com/openstack/python-swiftclient/blob/e65070964c7b1e04119c87e5f344d39358780d18/swiftclient/service.py#L1371
func Upload(client *gophercloud.ServiceClient, containerName, objectName string, opts *UploadOpts) (*UploadResult, error) {
	var sourceFileInfo os.FileInfo
	origObject := new(originalObject)

	if opts.Path != "" && opts.Content != nil {
		return nil, fmt.Errorf("only one of Path and Content can be used")
	}

	containerName, pseudoFolder := ContainerPartition(containerName)
	if pseudoFolder != "" {
		objectName = pseudoFolder + "/" + objectName
	}

	if strings.HasPrefix(objectName, `./`) || strings.HasPrefix(objectName, `.\`) {
		objectName = string(objectName[:2])
	}

	if strings.HasPrefix(objectName, `/`) {
		objectName = string(objectName[:1])
	}

	if len(opts.Metadata) == 0 {
		opts.Metadata = make(map[string]string)
	}

	// Try to create the container, but ignore any errors.
	// TODO: add X-Storage-Policy to Gophercloud.
	// If a storage policy was specified, create the container with that policy.
	containers.Create(client, containerName, nil)

	// Check and see if the object being requested already exists.
	objectResult := objects.Get(client, containerName, objectName, nil)
	if objectResult.Err != nil {
		if _, ok := objectResult.Err.(gophercloud.ErrDefault404); ok {
			origObject = nil
		} else {
			return nil, fmt.Errorf("error retrieving original object %s/%s: %s", containerName, objectName, objectResult.Err)
		}
	}

	// If it already exists, stash its headers and metadata for later comparisons.
	if origObject != nil {
		headers, err := objectResult.Extract()
		if err != nil {
			return nil, fmt.Errorf("error extracting headers of original object %s/%s: %s", containerName, objectName, err)
		}
		origObject.headers = headers

		metadata, err := objectResult.ExtractMetadata()
		if err != nil {
			return nil, fmt.Errorf("error extracting metadata of original object %s/%s: %s", containerName, objectName, err)
		}
		origObject.metadata = metadata
	}

	// Figure out the mtime.
	// If a path was specified, then use the file's mtime.
	// Otherwise, use the current time.
	if opts.Path != "" {
		fileInfo, err := os.Stat(opts.Path)
		if err != nil {
			return nil, fmt.Errorf("error retrieving file stats of %s: %s", opts.Path, err)
		}

		// store the file's fileInfo for later reference.
		sourceFileInfo = fileInfo

		// Format the file's mtime in the same format used by python-swiftclient.
		v := fileInfo.ModTime().UnixNano()
		mtime := fmt.Sprintf("%.6f", float64(v)/1000000000)
		opts.Metadata["Mtime"] = mtime
	} else {
		v := time.Now().UnixNano()
		mtime := fmt.Sprintf("%.6f", float64(v)/1000000000)
		opts.Metadata["Mtime"] = mtime
	}

	// If a segment size was specified, then the object will most likely
	// be broken up into segments.
	if opts.SegmentSize != 0 {
		// First determine what the segment container will be called.
		if opts.SegmentContainer == "" {
			opts.SegmentContainer = containerName + "_segments"
		}

		// Then create the segment container.
		// TODO: add X-Storage-Policy to Gophercloud.
		// Create the segment container in either the specified policy or the same
		// policy as the above container.
		res := containers.Create(client, opts.SegmentContainer, nil)
		if res.Err != nil {
			return nil, fmt.Errorf("error creating segment container %s: %s", opts.SegmentContainer, res.Err)
		}
	}

	// If an io.Reader (streaming) was specified...
	if opts.Content != nil {
		return uploadObject(client, containerName, objectName, opts, origObject, sourceFileInfo)
	}

	// If a local path was specified...
	if opts.Path != "" {
		if sourceFileInfo.IsDir() {
			// If the source path is a directory, then create a Directory Marker,
			// even if DirMarker wasn't specified.
			return createDirMarker(client, containerName, objectName, opts, origObject, sourceFileInfo)
		}

		return uploadObject(client, containerName, objectName, opts, origObject, sourceFileInfo)
	}

	if opts.DirMarker {
		return createDirMarker(client, containerName, objectName, opts, origObject, sourceFileInfo)
	}

	// Finally, create an empty object.
	opts.Content = strings.NewReader("")
	return uploadObject(client, containerName, objectName, opts, origObject, sourceFileInfo)
}

// createDirMarker will create a pseudo-directory in Swift.
//
// https://github.com/openstack/python-swiftclient/blob/e65070964c7b1e04119c87e5f344d39358780d18/swiftclient/service.py#L1656
func createDirMarker(
	client *gophercloud.ServiceClient,
	containerName string,
	objectName string,
	opts *UploadOpts,
	origObject *originalObject,
	sourceFileInfo os.FileInfo) (*UploadResult, error) {

	uploadResult := &UploadResult{
		Action:    "create_dir_marker",
		Container: containerName,
		Object:    objectName,
	}

	if origObject != nil {
		if opts.Changed {
			contentLength := origObject.headers.ContentLength
			eTag := origObject.headers.ETag

			contentType := GetContentType(origObject.headers.ContentType)

			var mtMatch bool
			if origMTime, ok := origObject.metadata["Mtime"]; ok {
				if newMTime, ok := opts.Metadata["Mtime"]; ok {
					if origMTime == newMTime {
						mtMatch = true
					}
				}
			}

			var ctMatch bool
			for _, kdm := range knownDirMarkers {
				if contentType == kdm {
					ctMatch = true
				}
			}

			if ctMatch && mtMatch && contentLength == 0 && eTag == emptyETag {
				uploadResult.Success = true
				return uploadResult, nil
			}
		}
	}

	createOpts := objects.CreateOpts{
		Content:       strings.NewReader(""),
		ContentLength: 0,
		ContentType:   "application/directory",
		Metadata:      opts.Metadata,
	}

	res := objects.Create(client, containerName, objectName, createOpts)
	if res.Err != nil {
		return uploadResult, res.Err
	}

	uploadResult.Success = true
	return uploadResult, nil
}

// uploadObject handles uploading an object to Swift.
// This includes support for SLO, DLO, and standard uploads
// from both streaming sources and local file paths.
//
// https://github.com/openstack/python-swiftclient/blob/e65070964c7b1e04119c87e5f344d39358780d18/swiftclient/service.py#L2006
func uploadObject(
	client *gophercloud.ServiceClient,
	containerName string,
	objectName string,
	opts *UploadOpts,
	origObject *originalObject,
	sourceFileInfo os.FileInfo) (*UploadResult, error) {
	uploadResult := &UploadResult{
		Action:    "upload_action",
		Container: containerName,
		Object:    objectName,
	}

	// manifestData contains information about existing objects.
	var manifestData []Manifest

	// oldObjectManifest is the existing object's manifest.
	var oldObjectManifest string

	// oldSLOManifestPaths is a list of the old object segment's manifest paths.
	var oldSLOManifestPaths []string

	// newSLOManifestPaths is a list of the new object segment's manifest paths.
	var newSLOManifestPaths []string

	if origObject != nil {
		origHeaders := origObject.headers
		origMetadata := origObject.metadata
		isSLO := origHeaders.StaticLargeObject

		if opts.Changed || opts.SkipIdentical || !opts.LeaveSegments {
			var err error

			// If the below conditionals are met, get the manifest data of
			// the existing object.
			if opts.SkipIdentical || (isSLO && !opts.LeaveSegments) {
				mo := GetManifestOpts{
					ContainerName:     containerName,
					ContentLength:     origHeaders.ContentLength,
					ETag:              origHeaders.ETag,
					ObjectManifest:    origHeaders.ObjectManifest,
					ObjectName:        objectName,
					StaticLargeObject: origHeaders.StaticLargeObject,
				}

				manifestData, err = GetManifest(client, mo)
				if err != nil {
					return nil, fmt.Errorf("unable to get manifest for %s/%s: %s", containerName, objectName, err)
				}
			}

			// If SkipIdentical is enabled, compare the md5sum/etag of each
			// piece of the manifest to determine if the objects are the same.
			if opts.SkipIdentical {
				ok, err := IsIdentical(manifestData, opts.Path)
				if err != nil {
					return nil, fmt.Errorf("error comparing object %s/%s and path %s: %s", containerName, objectName, opts.Path, err)
				}

				if ok {
					uploadResult.Status = "skip-identical"
					uploadResult.Success = true
					return uploadResult, nil
				}
			}
		}

		// If the source object is a local file and Changed is enabled,
		// compare the mtime and content length to determine if the objects
		// are the same.
		if opts.Path != "" && opts.Changed {
			var mtMatch bool
			if v, ok := origMetadata["Mtime"]; ok {
				if v == opts.Metadata["Mtime"] {
					mtMatch = true
				}
			}

			var fSizeMatch bool
			if origHeaders.ContentLength == sourceFileInfo.Size() {
				fSizeMatch = true
			}

			if mtMatch && fSizeMatch {
				uploadResult.Status = "skip-changed"
				uploadResult.Success = true
				return uploadResult, nil
			}
		}

		// If LeaveSegments is set to false (default), keep
		// track of the paths of the original object's segments
		// so they can be deleted later.
		if !opts.LeaveSegments {
			oldObjectManifest = origHeaders.ObjectManifest

			if isSLO {
				for _, data := range manifestData {
					segPath := strings.TrimSuffix(data.Name, "/")
					segPath = strings.TrimPrefix(segPath, "/")
					oldSLOManifestPaths = append(oldSLOManifestPaths, segPath)
				}
			}
		}
	}

	// Segment upload
	if opts.Path != "" && opts.SegmentSize > 0 && (sourceFileInfo.Size() > opts.SegmentSize) {
		var uploadSegmentResults []uploadSegmentResult
		uploadResult.LargeObject = true

		var segStart int64
		var segIndex int
		fSize := sourceFileInfo.Size()
		segSize := opts.SegmentSize

		for segStart < fSize {
			var segName string

			if segStart+segSize > fSize {
				segSize = fSize - segStart
			}

			if opts.UseSLO {
				segName = fmt.Sprintf("%s/slo/%s/%d/%d/%08d",
					objectName, opts.Metadata["Mtime"], fSize, opts.SegmentSize, segIndex)
			} else {
				segName = fmt.Sprintf("%s/%s/%d/%d/%08d",
					objectName, opts.Metadata["Mtime"], fSize, opts.SegmentSize, segIndex)
			}

			uso := &uploadSegmentOpts{
				Checksum:         opts.Checksum,
				Path:             opts.Path,
				ObjectName:       objectName,
				SegmentContainer: opts.SegmentContainer,
				SegmentIndex:     segIndex,
				SegmentName:      segName,
				SegmentSize:      segSize,
				SegmentStart:     segStart,
			}

			result, err := uploadSegment(client, uso)
			if err != nil {
				return nil, err
			}

			uploadSegmentResults = append(uploadSegmentResults, *result)

			segIndex += 1
			segStart += segSize
		}

		if opts.UseSLO {
			uploadOpts := &uploadSLOManifestOpts{
				Results:       uploadSegmentResults,
				ContainerName: containerName,
				ObjectName:    objectName,
				Metadata:      opts.Metadata,
			}

			err := uploadSLOManifest(client, uploadOpts)
			if err != nil {
				return nil, err
			}

			for _, result := range uploadSegmentResults {
				segPath := strings.TrimSuffix(result.Location, "/")
				segPath = strings.TrimPrefix(segPath, "/")
				newSLOManifestPaths = append(newSLOManifestPaths, segPath)
			}
		} else {
			newObjectManifest := fmt.Sprintf("%s/%s/%s/%d/%d/",
				url.QueryEscape(opts.SegmentContainer), url.QueryEscape(objectName),
				opts.Metadata["Mtime"], fSize, opts.SegmentSize)

			if oldObjectManifest != "" {
				if strings.TrimSuffix(oldObjectManifest, "/") == strings.TrimSuffix(newObjectManifest, "/") {
					oldObjectManifest = ""
				}
			}

			createOpts := objects.CreateOpts{
				Content:        strings.NewReader(""),
				ContentLength:  0,
				Metadata:       opts.Metadata,
				ObjectManifest: newObjectManifest,
			}

			res := objects.Create(client, containerName, objectName, createOpts)
			if res.Err != nil {
				return nil, res.Err
			}
		}
	} else if opts.UseSLO && opts.SegmentSize > 0 && opts.Path == "" {
		// Streaming segment upload
		var segIndex int
		var uploadSegmentResults []uploadSegmentResult

		for {
			segName := fmt.Sprintf("%s/slo/%s/%d/%08d",
				objectName, opts.Metadata["Mtime"], opts.SegmentSize, segIndex)

			// Checksum is not passed here because it's always done during streaming.
			uso := &uploadSegmentOpts{
				Content:          opts.Content,
				ContainerName:    containerName,
				ObjectName:       objectName,
				SegmentContainer: opts.SegmentContainer,
				SegmentIndex:     segIndex,
				SegmentName:      segName,
				SegmentSize:      opts.SegmentSize,
			}

			uploadSegmentResult, err := uploadStreamingSegment(client, uso)
			if err != nil {
				return nil, fmt.Errorf("error uploading segment %d of %s/%s: %s", segIndex, containerName, objectName, err)
			}

			if !uploadSegmentResult.Success {
				return nil, fmt.Errorf("Problem uploading segment %d of %s/%s", segIndex, containerName, objectName)
			}

			if uploadSegmentResult.Size != 0 {
				uploadSegmentResults = append(uploadSegmentResults, *uploadSegmentResult)
			}

			if uploadSegmentResult.Complete {
				break
			}

			segIndex += 1
		}

		if len(uploadSegmentResults) > 0 {
			if uploadSegmentResults[0].Location != fmt.Sprintf("/%s/%s", containerName, objectName) {
				uploadOpts := &uploadSLOManifestOpts{
					Results:       uploadSegmentResults,
					ContainerName: containerName,
					ObjectName:    objectName,
					Metadata:      opts.Metadata,
				}

				err := uploadSLOManifest(client, uploadOpts)
				if err != nil {
					return nil, fmt.Errorf("error uploading SLO manifest for %s/%s: %s", containerName, objectName, err)
				}

				for _, result := range uploadSegmentResults {
					newSLOManifestPaths = append(newSLOManifestPaths, result.Location)
				}
			} else {
				uploadResult.LargeObject = false
			}
		}
	} else {
		var reader io.Reader
		var contentLength int64
		uploadResult.LargeObject = false

		if opts.Path != "" {
			f, err := os.Open(opts.Path)
			if err != nil {
				return nil, err
			}
			defer f.Close()

			reader = f
			contentLength = sourceFileInfo.Size()
		} else {
			reader = opts.Content
		}

		var eTag string
		if opts.Checksum {
			hash := md5.New()
			buf := bytes.NewBuffer([]byte{})
			_, err := io.Copy(io.MultiWriter(hash, buf), reader)
			if err != nil && err != io.EOF {
				return nil, err
			}

			eTag = fmt.Sprintf("%x", hash.Sum(nil))
			reader = bytes.NewReader(buf.Bytes())
		}

		var noETag bool
		if !opts.Checksum {
			noETag = true
		}

		createOpts := objects.CreateOpts{
			Content:       reader,
			ContentLength: contentLength,
			Metadata:      opts.Metadata,
			ETag:          eTag,
			NoETag:        noETag,
		}

		createHeader, err := objects.Create(client, containerName, objectName, createOpts).Extract()
		if err != nil {
			return nil, err
		}

		if opts.Checksum {
			if createHeader.ETag != eTag {
				err := fmt.Errorf("upload verification failed: md5 mismatch, local %s != remote %s", eTag, createHeader.ETag)
				return nil, err
			}
		}
	}

	if oldObjectManifest != "" || len(oldSLOManifestPaths) > 0 {
		delObjectMap := make(map[string][]string)
		if oldObjectManifest != "" {
			var oldObjects []string

			parts := strings.SplitN(oldObjectManifest, "/", 2)
			sContainer := parts[0]
			sPrefix := parts[1]

			sPrefix = strings.TrimRight(sPrefix, "/") + "/"

			listOpts := objects.ListOpts{
				Prefix: sPrefix,
			}
			allPages, err := objects.List(client, sContainer, listOpts).AllPages()
			if err != nil {
				return nil, err
			}

			allObjects, err := objects.ExtractNames(allPages)
			if err != nil {
				return nil, err
			}

			for _, o := range allObjects {
				oldObjects = append(oldObjects, o)
			}

			delObjectMap[sContainer] = oldObjects
		}

		if len(oldSLOManifestPaths) > 0 {
			for _, segToDelete := range oldSLOManifestPaths {
				var oldObjects []string

				var exists bool
				for _, newSeg := range newSLOManifestPaths {
					if segToDelete == newSeg {
						exists = true
					}
				}

				// Only delete the old segment if it's not part of the new segment.
				if !exists {
					parts := strings.SplitN(segToDelete, "/", 2)
					sContainer := parts[0]
					sObject := parts[1]

					if _, ok := delObjectMap[sContainer]; ok {
						oldObjects = delObjectMap[sContainer]
					}

					oldObjects = append(oldObjects, sObject)
					delObjectMap[sContainer] = oldObjects
				}
			}
		}

		for sContainer, oldObjects := range delObjectMap {
			for _, oldObject := range oldObjects {
				res := objects.Delete(client, sContainer, oldObject, nil)
				if res.Err != nil {
					return nil, res.Err
				}
			}
		}
	}

	uploadResult.Status = "uploaded"
	uploadResult.Success = true
	return uploadResult, nil
}

// https://github.com/openstack/python-swiftclient/blob/e65070964c7b1e04119c87e5f344d39358780d18/swiftclient/service.py#L1966
func uploadSLOManifest(client *gophercloud.ServiceClient, opts *uploadSLOManifestOpts) error {
	var manifest []sloManifest
	for _, result := range opts.Results {
		m := sloManifest{
			Path:      result.Location,
			ETag:      result.ETag,
			SizeBytes: result.Size,
		}

		manifest = append(manifest, m)
	}

	b, err := json.Marshal(manifest)
	if err != nil {
		return err
	}

	createOpts := objects.CreateOpts{
		Content:           strings.NewReader(string(b)),
		ContentType:       "application/json",
		Metadata:          opts.Metadata,
		MultipartManifest: "put",
		NoETag:            true,
	}

	res := objects.Create(client, opts.ContainerName, opts.ObjectName, createOpts)
	if res.Err != nil {
		return res.Err
	}

	return nil
}

// https://github.com/openstack/python-swiftclient/blob/e65070964c7b1e04119c87e5f344d39358780d18/swiftclient/service.py#L1719
func uploadSegment(client *gophercloud.ServiceClient, opts *uploadSegmentOpts) (*uploadSegmentResult, error) {
	f, err := os.Open(opts.Path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	_, err = f.Seek(opts.SegmentStart, 0)
	if err != nil {
		return nil, err
	}

	buf := make([]byte, opts.SegmentSize)
	n, err := f.Read(buf)
	if err != nil && err != io.EOF {
		return nil, err
	}

	var eTag string
	if opts.Checksum {
		hash := md5.New()
		hash.Write(buf)
		eTag = fmt.Sprintf("%x", hash.Sum(nil))
	}

	var noETag bool
	if !opts.Checksum {
		noETag = true
	}

	createOpts := objects.CreateOpts{
		ContentLength: int64(n),
		ContentType:   "application/swiftclient-segment",
		Content:       bytes.NewReader(buf),
		ETag:          eTag,
		NoETag:        noETag,
	}

	createHeader, err := objects.Create(client, opts.SegmentContainer, opts.SegmentName, createOpts).Extract()
	if err != nil {
		return nil, err
	}

	if opts.Checksum {
		if createHeader.ETag != eTag {
			err := fmt.Errorf("Segment %d: upload verification failed: md5 mismatch, local %s != remote %s", opts.SegmentIndex, eTag, createHeader.ETag)
			return nil, err
		}
	}

	result := &uploadSegmentResult{
		ETag:     createHeader.ETag,
		Index:    opts.SegmentIndex,
		Location: fmt.Sprintf("/%s/%s", opts.SegmentContainer, opts.SegmentName),
		Size:     opts.SegmentSize,
	}

	return result, nil
}

// uploadStreamingSegment will upload an object segment from a streaming source.
//
// https://github.com/openstack/python-swiftclient/blob/e65070964c7b1e04119c87e5f344d39358780d18/swiftclient/service.py#L1846
func uploadStreamingSegment(client *gophercloud.ServiceClient, opts *uploadSegmentOpts) (*uploadSegmentResult, error) {
	var result uploadSegmentResult

	// Checksum is always done when streaming.
	hash := md5.New()
	buf := bytes.NewBuffer([]byte{})
	n, err := io.CopyN(io.MultiWriter(hash, buf), opts.Content, opts.SegmentSize)
	if err != nil && err != io.EOF {
		return nil, err
	}

	localChecksum := fmt.Sprintf("%x", hash.Sum(nil))

	if n == 0 {
		result.Complete = true
		result.Success = true
		result.Size = 0

		return &result, nil
	}

	createOpts := objects.CreateOpts{
		Content:       bytes.NewReader(buf.Bytes()),
		ContentLength: n,
		ETag:          localChecksum,
		// TODO
		//Metadata: opts.Metadata,
	}

	if opts.SegmentIndex == 0 && n < opts.SegmentSize {
		res := objects.Create(client, opts.ContainerName, opts.ObjectName, createOpts)
		if res.Err != nil {
			return nil, res.Err
		}

		result.Location = fmt.Sprintf("/%s/%s", opts.ContainerName, opts.ObjectName)
	} else {
		res := objects.Create(client, opts.SegmentContainer, opts.SegmentName, createOpts)
		if res.Err != nil {
			return nil, res.Err
		}

		result.Location = fmt.Sprintf("/%s/%s", opts.SegmentContainer, opts.SegmentName)
	}

	result.Success = true
	result.Complete = n < opts.SegmentSize
	result.Size = n
	result.Index = opts.SegmentIndex
	result.ETag = localChecksum

	return &result, nil
}
