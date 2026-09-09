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
	"os"
	"strings"
	"testing"
)

// anacondaOnlyDep is a module the anaconda plugin needs and BMO does not, the
// marker this test tracks. It stays a direct dep of test/anaconda and nothing else.
const anacondaOnlyDep = "github.com/stmcginnis/gofish"

// TestPluginOnlyDepsStayOutOfBMO pins why the plugin lives in its own module.
// A dep that reaches BMO's graph is linked into the operator and every module.
func TestPluginOnlyDepsStayOutOfBMO(t *testing.T) {
	for _, path := range []string{
		"../../go.mod",
		"../../go.sum",
		"../../test/go.mod",
		"../../test/go.sum",
	} {
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}

		if strings.Contains(string(contents), anacondaOnlyDep) {
			t.Errorf("%s names %s, which belongs only to the out-of-tree plugin module", path, anacondaOnlyDep)
		}
	}
}

// TestAnacondaPluginKeepsItsOwnDeps is the other half. The marker has to be
// there, or the test above passes for the wrong reason if the plugin drops it.
func TestAnacondaPluginKeepsItsOwnDeps(t *testing.T) {
	const path = "../../test/anaconda/go.mod"

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}

	if !strings.Contains(string(contents), anacondaOnlyDep) {
		t.Errorf("%s no longer requires %s, so the isolation test proves nothing", path, anacondaOnlyDep)
	}
}
