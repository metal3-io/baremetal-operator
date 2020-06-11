/*
Copyright The Helm Authors.

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

package v3

import (
	"os"

	"helm.sh/helm/v3/pkg/helmpath"
)

// ConfigDir returns the v2 config directory
func ConfigDir() string {
	if homeDir, exists := os.LookupEnv("HELM_V3_CONFIG"); exists {
		return homeDir
	}

	defaultDir := helmpath.ConfigPath()
	return defaultDir
}

// DataDir returns the v3 data directory
func DataDir() string {
	if homeDir, exists := os.LookupEnv("HELM_V3_DATA"); exists {
		return homeDir
	}

	defaultDir := helmpath.DataPath()
	return defaultDir
}

// CacheDir returns the v3 data directory
func CacheDir() string {
	if homeDir, exists := os.LookupEnv("HELM_V3_CACHE"); exists {
		return homeDir
	}

	defaultDir := helmpath.CachePath()
	return defaultDir
}
