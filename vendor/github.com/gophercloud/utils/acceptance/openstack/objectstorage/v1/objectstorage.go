package v1

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/acceptance/tools"
	"github.com/gophercloud/gophercloud/openstack/objectstorage/v1/containers"
	"github.com/gophercloud/gophercloud/openstack/objectstorage/v1/objects"
)

// CompareFiles will compare two files
func CompareFiles(t *testing.T, file1, file2 string) (bool, error) {
	f1, err := os.Open(file1)
	if err != nil {
		return false, fmt.Errorf("unable to open %s: %s", file1, err)
	}
	defer f1.Close()

	f2, err := os.Open(file2)
	if err != nil {
		return false, fmt.Errorf("unable to open %s: %s", file2, err)
	}
	defer f2.Close()

	contents1, err := ioutil.ReadAll(f1)
	if err != nil {
		return false, fmt.Errorf("unable to read %s: %s", file1, err)
	}

	contents2, err := ioutil.ReadAll(f2)
	if err != nil {
		return false, fmt.Errorf("unable to read %s: %s", file2, err)
	}

	equal := bytes.Equal(contents1, contents2)

	return equal, nil
}

// CreateContainer will create a container with a random name.
func CreateContainer(t *testing.T, client *gophercloud.ServiceClient) (string, error) {
	cName := tools.RandomString("test-container-", 8)
	res := containers.Create(client, cName, nil)

	t.Logf("creating container: %s", cName)

	return cName, res.Err
}

// CreateRandomFile will create a file with random content.
func CreateRandomFile(t *testing.T, parentDir string) (string, error) {
	tmpfile, err := CreateTempFile(t, parentDir)
	if err != nil {
		return "", fmt.Errorf("unable to create random file: %s", err)
	}

	content := tools.RandomString("", 256)
	tmpfile.Write([]byte(content))
	tmpfile.Close()

	return tmpfile.Name(), nil
}

// CreateTempDir will create and return a temp directory.
func CreateTempDir(t *testing.T, parentDir string) (string, error) {
	dirName, err := ioutil.TempDir(parentDir, "test-dir-")
	if err != nil {
		return "", err
	}

	t.Logf("creating tempdir: %s", dirName)
	return dirName, nil
}

// CreateTempFile will create and return a temp file.
func CreateTempFile(t *testing.T, dir string) (*os.File, error) {
	fileName := tools.RandomString("test-file-", 8)
	t.Logf("creating tempfile: %s", fileName)
	return ioutil.TempFile(dir, fileName)
}

// DeleteContainer will delete a container. A fatal error will occur if the
// container failed to be deleted. This works best when used as a deferred
// function.
func DeleteContainer(t *testing.T, client *gophercloud.ServiceClient, cName string) {
	t.Logf("deleting container %s", cName)

	allPages, err := objects.List(client, cName, nil).AllPages()
	if err != nil {
		t.Fatalf("unable to list container %s: %s", cName, err)
	}

	allObjects, err := objects.ExtractNames(allPages)
	if err != nil {
		t.Fatalf("unable to extract container %s: %s", cName, err)
	}

	for _, oName := range allObjects {
		res := objects.Delete(client, cName, oName, nil)
		if res.Err != nil {
			t.Fatalf("unable to delete object: %s/%s: %s", cName, oName, oName)
		}
	}

	res := containers.Delete(client, cName)
	if res.Err != nil {
		t.Fatalf("unable to delete container %s: %s", cName, res.Err)
	}
}

// DeleteObject will delete an object. A fatal error will occur if the object
// failed to be deleted. This works best when used as a deferred function.
func DeleteObject(t *testing.T, client *gophercloud.ServiceClient, cName, oName string) {
	t.Logf("deleting object %s/%s", cName, oName)

	res := objects.Delete(client, cName, oName, nil)
	if res.Err != nil {
		t.Fatalf("unable to delete object %s/%s: %s", cName, oName, res.Err)
	}
}

// DeleteTempFile will delete a temporary file. A fatal error will occur if the
// file could not be deleted. This works best when used as a deferred function.
func DeleteTempFile(t *testing.T, fileName string) {
	t.Logf("deleting tempfile %s", fileName)

	if err := os.Remove(fileName); err != nil {
		t.Fatalf("unable to delete tempfile %s: %s", fileName, err)
	}
}

// DeleteTempDir will delete a temporary directory. A fatal error will occur if
// the directory could not be deleted. This works best when used as a deferred
// function.
func DeleteTempDir(t *testing.T, dirName string) {
	t.Logf("deleting tempdir %s", dirName)

	if err := os.RemoveAll(dirName); err != nil {
		t.Fatalf("unable to delete tempdir %s: %s", dirName, err)
	}
}

// GetObject is an alias to objects.GetObject so we don't have to import
// gophercloud/gophercloud into objects_test.go and make things confusing.
func GetObject(client *gophercloud.ServiceClient, cName, oName string) (*objects.GetHeader, error) {
	return objects.Get(client, cName, oName, nil).Extract()
}
