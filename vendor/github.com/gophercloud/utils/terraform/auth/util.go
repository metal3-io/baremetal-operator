package auth

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/mitchellh/go-homedir"
)

// List of headers that need to be redacted
var REDACT_HEADERS = []string{"x-auth-token", "x-auth-key", "x-service-token",
	"x-storage-token", "x-account-meta-temp-url-key", "x-account-meta-temp-url-key-2",
	"x-container-meta-temp-url-key", "x-container-meta-temp-url-key-2", "set-cookie",
	"x-subject-token"}

// RedactHeaders processes a headers object, returning a redacted list
func RedactHeaders(headers http.Header) (processedHeaders []string) {
	ignoreCase := true

	for name, header := range headers {
		for _, v := range header {
			if sliceContainsStr(REDACT_HEADERS, name, ignoreCase) {
				processedHeaders = append(processedHeaders, fmt.Sprintf("%v: %v", name, "***"))
			} else {
				processedHeaders = append(processedHeaders, fmt.Sprintf("%v: %v", name, v))
			}
		}
	}
	return
}

// FormatHeaders processes a headers object plus a deliminator, returning a string
func FormatHeaders(headers http.Header, seperator string) string {
	redactedHeaders := RedactHeaders(headers)
	sort.Strings(redactedHeaders)

	return strings.Join(redactedHeaders, seperator)
}

func sliceContainsStr(haystack []string, needle string, ignoreCase bool) bool {
	if ignoreCase {
		needle = strings.ToLower(needle)
	}

	for _, v := range haystack {
		if ignoreCase {
			v = strings.ToLower(v)
		}

		if v == needle {
			return true
		}

	}
	return false
}

// This is copied directly from Terraform in order to remove a single legacy
// vendor dependency.
// https://github.com/hashicorp/terraform/tree/master/helper/pathorcontents
func pathOrContents(poc string) (string, bool, error) {
	if len(poc) == 0 {
		return poc, false, nil
	}

	path := poc
	if path[0] == '~' {
		var err error
		path, err = homedir.Expand(path)
		if err != nil {
			return path, true, err
		}
	}

	if _, err := os.Stat(path); err == nil {
		contents, err := ioutil.ReadFile(path)
		if err != nil {
			return string(contents), true, err
		}
		return string(contents), true, nil
	}

	return poc, false, nil
}
