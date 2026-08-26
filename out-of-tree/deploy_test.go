/*

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

// Cover for the shipped deployment manifest, so the kickstart templates in it stay
// in step with the variables redfish-inspect.star registers.

package starlark

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"
)

// manifestConfigMaps returns the ConfigMap data blocks in the deployment manifest.
func manifestConfigMaps(t *testing.T) map[string]map[string]string {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join("deploy", "redfish-inspect.yaml"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}

	out := map[string]map[string]string{}

	for _, doc := range bytes.Split(raw, []byte("\n---\n")) {
		var obj struct {
			Kind     string `json:"kind"`
			Metadata struct {
				Name string `json:"name"`
			} `json:"metadata"`
			Data map[string]string `json:"data"`
		}

		if err := yaml.Unmarshal(doc, &obj); err != nil {
			t.Fatalf("parse manifest document: %v", err)
		}

		if obj.Kind == "ConfigMap" {
			out[obj.Metadata.Name] = obj.Data
		}
	}

	return out
}

// The script registers the route with these vars, and RenderTemplate uses
// missingkey=error, so a template referencing anything else fails at install time.
func TestManifestKickstartRendersWithScriptVars(t *testing.T) {
	cms := manifestConfigMaps(t)

	ks, ok := cms["kickstart"]["ks.cfg"]
	if !ok {
		t.Fatal("manifest has no kickstart ConfigMap with a ks.cfg key")
	}

	vars := map[string]any{"Name": "node-1", "Namespace": "metal3-system", "UID": "uid-1"}

	out, err := RenderTemplate("kickstart", ks, vars)
	if err != nil {
		t.Fatalf("kickstart template does not render with the vars the script supplies: %v", err)
	}

	if !strings.Contains(out, "node-1") {
		t.Errorf("rendered kickstart never used the host name, got:\n%s", out)
	}
}

// The fallback is the one served to machines we cannot identify, so it must never
// carry a command that writes to disk.
func TestManifestFallbackKickstartIsInert(t *testing.T) {
	fb, ok := manifestConfigMaps(t)["kickstart-fallback"]["ks.cfg"]
	if !ok {
		t.Fatal("manifest has no kickstart-fallback ConfigMap with a ks.cfg key")
	}

	for _, forbidden := range []string{"clearpart", "autopart", "zerombr", "ignoredisk", "bootloader", "%packages"} {
		if strings.Contains(fb, forbidden) {
			t.Errorf("fallback kickstart contains %q, it must not touch the machine", forbidden)
		}
	}

	if !strings.Contains(fb, "poweroff") {
		t.Error("fallback kickstart does not power the machine off")
	}
}
