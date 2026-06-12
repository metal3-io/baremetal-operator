/*
Copyright 2025 The Metal3 Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package webhooks

import (
	"fmt"
	"net/url"
	"slices"
)

// validateURL validates the given URL. The URL is assumed to come from HTTP
// request. On success, no error is returned. Empty string also returns an error.
func validateURL(input string) (error) {
	url_obj, err := url.ParseRequestURI(input)
	if err != nil {
		return err
	}

	// Check the URL scheme
	allowed := []string{"http", "https", "ftp", "gopher" }
	if !slices.Contains(allowed, url_obj.Scheme) {
		return fmt.Errorf("invalid scheme in URL, \"%s\" not allowed", url_obj.Scheme)
	}

	return nil
}
