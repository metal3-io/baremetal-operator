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

// anacondaOnlyDep is a module the out-of-tree anaconda plugin needs and BMO
// does not. It is the marker this test tracks, so it has to stay a direct
// dependency of test/anaconda and of nothing else.
const anacondaOnlyDep = "github.com/stmcginnis/gofish"

// TestPluginOnlyDepsStayOutOfBMO pins the reason the anaconda plugin lives in
// its own module. A plugin may pull in whatever it likes, but the moment one of
// those deps reaches BMO's own graph it is linked into the operator binary and
// into every other module here, which is exactly what the nesting prevents.
// Folding test/anaconda into the root or the test module would break this.
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

// TestAnacondaPluginKeepsItsOwnDeps is the other half: the marker has to
// actually be there, or the test above passes for the wrong reason once the
// plugin stops using it.
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
