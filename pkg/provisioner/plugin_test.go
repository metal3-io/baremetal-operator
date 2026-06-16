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

package provisioner_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
)

func TestPluginConfigHasFeature(t *testing.T) {
	cfg := provisioner.PluginConfig{
		Features: []provisioner.HostFeature{provisioner.FeaturePreprovisioningImage},
	}

	if !cfg.HasFeature(provisioner.FeaturePreprovisioningImage) {
		t.Errorf("expected HasFeature(PreprovisioningImage) = true")
	}
	if cfg.HasFeature(provisioner.HostFeature("Other")) {
		t.Errorf("expected HasFeature(Other) = false")
	}

	empty := provisioner.PluginConfig{}
	if empty.HasFeature(provisioner.FeaturePreprovisioningImage) {
		t.Errorf("empty config should not report any feature")
	}
}

func TestOpenMissingFile(t *testing.T) {
	skipIfPluginsUnsupported(t)
	_, err := provisioner.Open(filepath.Join(t.TempDir(), "nonexistent.so"), "nonexistent")
	if err == nil {
		t.Fatal("expected error opening missing plugin, got nil")
	}

	if !strings.Contains(err.Error(), "failed to open plugin") {
		t.Errorf("error should mention 'failed to open plugin', got: %v", err)
	}
}

// Subtests share one build because Go's plugin loader rejects loading the
// same plugin (by package path) from two different file paths in one process.
func TestOpenDemoPlugin(t *testing.T) {
	skipIfPluginsUnsupported(t)
	soPath := buildPlugin(t, "./demo/plugin", "demo-provisioner.so")

	t.Run("success", func(t *testing.T) {
		p, err := provisioner.Open(soPath, "demo")
		if err != nil {
			t.Fatalf("Open returned unexpected error: %v", err)
		}

		if p.Name() != "demo" {
			t.Errorf("Name() = %q, want %q", p.Name(), "demo")
		}
		if p.Path() != soPath {
			t.Errorf("Path() = %q, want %q", p.Path(), soPath)
		}

		factory, err := p.NewFactory(provisioner.PluginConfig{})
		if err != nil {
			t.Fatalf("NewFactory returned unexpected error: %v", err)
		}
		if factory == nil {
			t.Fatal("NewFactory returned nil factory")
		}
	})

	t.Run("name mismatch", func(t *testing.T) {
		_, err := provisioner.Open(soPath, "not-demo")
		if err == nil {
			t.Fatal("expected name mismatch error, got nil")
		}

		if !strings.Contains(err.Error(), "PluginName()") || !strings.Contains(err.Error(), "not-demo") {
			t.Errorf("error should mention PluginName() and the expected name, got: %v", err)
		}
	})

	t.Run("host configure absent", func(t *testing.T) {
		p, err := provisioner.Open(soPath, "demo")
		if err != nil {
			t.Fatalf("Open returned unexpected error: %v", err)
		}

		reqs, err := p.HostConfigure(provisioner.HostConfigureInput{})
		if err != nil {
			t.Fatalf("HostConfigure returned unexpected error: %v", err)
		}

		if reqs.AddToScheme != nil || reqs.CacheByObject != nil {
			t.Errorf("expected zero HostRequirements when plugin omits HostConfigure, got %+v", reqs)
		}
	})
}

func skipIfPluginsUnsupported(t *testing.T) {
	t.Helper()
	switch runtime.GOOS {
	case "linux", "darwin", "freebsd":
	default:
		t.Skipf("plugins not supported on %s", runtime.GOOS)
	}
	// -cover instruments the test binary but not the plugin, so plugin.Open
	// fails the package-version check.
	if testing.CoverMode() != "" {
		t.Skip("plugin load tests are incompatible with -cover instrumentation")
	}
}

func buildPlugin(t *testing.T, pkgPath, outName string) string {
	t.Helper()
	soPath := filepath.Join(t.TempDir(), outName)

	cmd := exec.Command("go", "build", "-buildmode=plugin", "-o", soPath, pkgPath)
	cmd.Env = append(os.Environ(), "CGO_ENABLED=1")

	out, err := cmd.CombinedOutput()
	if err != nil {
		// Missing C toolchain is an environment issue, not a code regression.
		var execErr *exec.Error
		if errors.As(err, &execErr) {
			t.Skipf("go build unavailable: %v", err)
		}

		if strings.Contains(string(out), "C compiler") || strings.Contains(string(out), "cgo:") {
			t.Skipf("C toolchain unavailable for plugin build:\n%s", out)
		}
		t.Fatalf("failed to build plugin %s: %v\n%s", pkgPath, err, out)
	}

	return soPath
}
