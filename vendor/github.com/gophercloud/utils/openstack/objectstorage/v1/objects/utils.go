package objects

import (
	"bufio"
	"crypto/md5"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	emptyETag  = "d41d8cd98f00b204e9800998ecf8427e"
	diskBuffer = 65536
)

var (
	knownDirMarkers = []string{
		"application/directory",
		"text/directory",
	}
)

func ContainerPartition(containerName string) (string, string) {
	var pseudoFolder string

	parts := strings.SplitN(containerName, "/", 2)
	if len(parts) == 2 {
		containerName = parts[0]
		pseudoFolder = strings.TrimSuffix(parts[1], "/")
	}

	return containerName, pseudoFolder
}

// https://github.com/holys/checksum/blob/master/md5/md5.go
func FileMD5Sum(filename string) (string, error) {
	if _, err := os.Stat(filename); err != nil {
		return "", err
	}

	file, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := md5.New()

	for buf, reader := make([]byte, diskBuffer), bufio.NewReader(file); ; {
		n, err := reader.Read(buf)
		if err != nil {
			if err == io.EOF {
				break
			}

			return "", err
		}

		hash.Write(buf[:n])
	}

	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func GetContentType(ct string) string {
	v := strings.SplitN(ct, ";", 2)
	return v[0]
}
