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

package starlark

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	"github.com/s3rj1k/starlark-provisioner/starlib"
	"github.com/s3rj1k/starlark-provisioner/starscript"
)

// shippedScripts returns every reference script the image ships.
func shippedScripts(t *testing.T) []string {
	t.Helper()

	paths, err := filepath.Glob(filepath.Join("scripts", "*.star"))
	if err != nil {
		t.Fatalf("glob scripts: %v", err)
	}

	if len(paths) == 0 {
		t.Fatal("no scripts found under scripts/")
	}

	return paths
}

// The shipped reference scripts must satisfy the same contract the factory
// enforces at load time, so a signature drift fails here and not on a live host.
func TestShippedScriptsSatisfyContract(t *testing.T) {
	for _, path := range shippedScripts(t) {
		t.Run(filepath.Base(path), func(t *testing.T) {
			globals, err := starscript.LoadScript(path, starlib.Builtins())
			if err != nil {
				t.Fatalf("LoadScript: %v", err)
			}

			if err := starscript.ValidateRequiredFunctions(globals); err != nil {
				t.Errorf("ValidateRequiredFunctions: %v", err)
			}
		})
	}
}

// Mirrors the plugin-load-test gate the image build runs over every script,
// minus the dlopen, so a script that cannot back a provisioner fails in go test.
func TestShippedScriptsBuildAProvisioner(t *testing.T) {
	for _, path := range shippedScripts(t) {
		t.Run(filepath.Base(path), func(t *testing.T) {
			factory, err := NewProvisionerFactory(path, nil)
			if err != nil {
				t.Fatalf("NewProvisionerFactory: %v", err)
			}

			if factory == nil {
				t.Fatal("NewProvisionerFactory returned a nil factory")
			}

			ctx := context.Background()

			prov, err := factory.NewProvisioner(ctx, provisioner.HostData{}, nil)
			if err != nil {
				t.Fatalf("NewProvisioner: %v", err)
			}

			if prov == nil {
				t.Fatal("NewProvisioner returned nil")
			}

			// GetHealth swallows script errors by contract, so with no BMC behind
			// the empty HostData this only has to return rather than panic.
			valid := []string{"", provisioner.HealthOK, provisioner.HealthWarning, provisioner.HealthCritical}
			if health := prov.GetHealth(ctx); !slices.Contains(valid, health) {
				t.Errorf("GetHealth = %q, want a health string the controller understands", health)
			}
		})
	}
}

// A missing script would only surface when the image starts, so the default the
// Dockerfile bakes in has to name a script that is actually shipped.
func TestDockerfileDefaultScriptIsShipped(t *testing.T) {
	const key = "ENV STARLARK_PROVISIONER_SCRIPT="

	raw, err := os.ReadFile("Dockerfile")
	if err != nil {
		t.Fatalf("read Dockerfile: %v", err)
	}

	var value string

	for line := range strings.Lines(string(raw)) {
		if after, ok := strings.CutPrefix(strings.TrimSpace(line), key); ok {
			value = strings.TrimSpace(after)
		}
	}

	if value == "" {
		t.Fatalf("Dockerfile has no %s line", key)
	}

	// The image serves scripts out of /scripts, copied from the module directory.
	name, ok := strings.CutPrefix(value, "/scripts/")
	if !ok {
		t.Fatalf("default script %q is not under /scripts", value)
	}

	if _, err := os.Stat(filepath.Join("scripts", name)); err != nil {
		t.Errorf("default script %q is not shipped, %v", value, err)
	}
}
