package objects

import (
	"bufio"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/objectstorage/v1/objects"
)

// Manifest represents an object manifest.
type Manifest struct {
	Bytes        int64     `json:"bytes"`
	ContentType  string    `json:"content_type"`
	Hash         string    `json:"hash"`
	Name         string    `json:"name"`
	LastModified time.Time `json:"-"`
}

func (r *Manifest) UnmarshalJSON(b []byte) error {
	type tmp Manifest
	var s struct {
		tmp
		LastModified gophercloud.JSONRFC3339MilliNoZ `json:"last_modified"`
	}
	err := json.Unmarshal(b, &s)
	if err != nil {
		return err
	}

	*r = Manifest(s.tmp)

	r.LastModified = time.Time(s.LastModified)

	return nil
}

// ExtractMultipartManifest will extract a manifest returned when
// downloading an object using DownloadOpts.MultipartManifest = "get".
func ExtractMultipartManifest(body []byte) ([]Manifest, error) {
	var s []Manifest
	err := json.Unmarshal(body, &s)
	return s, err
}

type GetManifestOpts struct {
	ContainerName     string
	ContentLength     int64
	ETag              string
	ObjectManifest    string
	ObjectName        string
	Manifest          string
	StaticLargeObject bool
}

// https://github.com/openstack/python-swiftclient/blob/e65070964c7b1e04119c87e5f344d39358780d18/swiftclient/service.py#L1916
func GetManifest(client *gophercloud.ServiceClient, opts GetManifestOpts) ([]Manifest, error) {
	var manifest []Manifest

	// TODO: test this
	if opts.ObjectManifest != "" {
		v := strings.SplitN(opts.ObjectManifest, "/", 2)
		if len(v) != 2 {
			return nil, fmt.Errorf("unable to parse object manifest %s", opts.ObjectManifest)
		}

		sContainer := v[0]
		sPrefix := v[1]

		listOpts := objects.ListOpts{
			Prefix: sPrefix,
		}

		allPages, err := objects.List(client, sContainer, listOpts).AllPages()
		if err != nil {
			return nil, fmt.Errorf("unable to list %s: %s", sContainer, err)
		}

		allObjects, err := objects.ExtractNames(allPages)
		if err != nil {
			return nil, fmt.Errorf("unable to extract objects from %s: %s", sContainer, err)
		}

		for _, obj := range allObjects {
			objInfo, err := objects.Get(client, sContainer, obj, nil).Extract()
			if err != nil {
				return nil, fmt.Errorf("unable to get object %s:%s: %s", sContainer, obj, err)
			}

			m := Manifest{
				Bytes:        objInfo.ContentLength,
				ContentType:  objInfo.ContentType,
				Hash:         objInfo.ETag,
				LastModified: objInfo.LastModified,
				Name:         obj,
			}

			manifest = append(manifest, m)
		}

		return manifest, nil
	}

	if opts.StaticLargeObject {
		if opts.Manifest == "" {
			downloadOpts := objects.DownloadOpts{
				MultipartManifest: "get",
			}
			res := objects.Download(client, opts.ContainerName, opts.ObjectName, downloadOpts)
			if res.Err != nil {
				return nil, res.Err
			}

			body, err := res.ExtractContent()
			if err != nil {
				return nil, err
			}

			multipartManifest, err := ExtractMultipartManifest(body)
			if err != nil {
				return nil, err
			}

			for _, obj := range multipartManifest {
				// TODO: support sub_slo
				m := Manifest{
					Bytes:        obj.Bytes,
					ContentType:  obj.ContentType,
					Hash:         obj.Hash,
					LastModified: obj.LastModified,
					Name:         obj.Name,
				}
				manifest = append(manifest, m)
			}
		}

		return manifest, nil
	}

	m := Manifest{
		Hash:  opts.ETag,
		Bytes: opts.ContentLength,
	}

	manifest = append(manifest, m)

	return manifest, nil
}

func IsIdentical(manifest []Manifest, path string) (bool, error) {
	if path == "" {
		return false, nil
	}

	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()

	reader := bufio.NewReader(f)

	for _, data := range manifest {
		hash := md5.New()
		buf := make([]byte, data.Bytes)
		n, err := reader.Read(buf)
		if err != nil && err != io.EOF {
			return false, err
		}

		hash.Write(buf[:n])
		checksum := fmt.Sprintf("%x", hash.Sum(nil))
		if checksum != data.Hash {
			return false, nil
		}
	}

	// Do one last read to see if the end of file was reached.
	buf := make([]byte, 1)
	_, err = reader.Read(buf)
	if err == io.EOF {
		return true, nil
	}

	return false, nil
}
