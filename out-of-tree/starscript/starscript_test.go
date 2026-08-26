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

package starscript

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
)

// loadFunc compiles a single def and returns it for arity checks.
func loadFunc(t *testing.T, src string) *starlark.Function {
	t.Helper()

	thread := &starlark.Thread{Name: "test"}

	globals, err := starlark.ExecFileOptions(&syntax.FileOptions{}, thread, "t.star", src, nil)
	if err != nil {
		t.Fatalf("exec %q: %v", src, err)
	}

	fn, ok := globals["f"].(*starlark.Function)
	if !ok {
		t.Fatalf("f is not a function in %q", src)
	}

	return fn
}

// load executes src as a script file and returns its globals.
func load(t *testing.T, src string) starlark.StringDict {
	t.Helper()

	globals, _, err := loadCapturingPrint(t, src, testMaxSteps)
	if err != nil {
		t.Fatalf("LoadScript: %v", err)
	}

	return globals
}

// testMaxSteps is small enough that a runaway module body stops quickly.
const testMaxSteps = 1_000_000

// loadCapturingPrint loads src and records whatever module level print emitted.
func loadCapturingPrint(t *testing.T, src string, maxSteps uint64) (starlark.StringDict, []string, error) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "s.star")
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatalf("write script: %v", err)
	}

	var printed []string

	globals, err := LoadScript(path, nil, func(_ *starlark.Thread, msg string) {
		printed = append(printed, msg)
	}, maxSteps)

	return globals, printed, err
}

// Module level code runs on its own thread, so it needs the same print routing
// and step budget the per call threads get.
func TestLoadScriptRoutesPrintAndBoundsSteps(t *testing.T) {
	_, printed, err := loadCapturingPrint(t, "print(\"from module level\")\n", testMaxSteps)
	if err != nil {
		t.Fatalf("LoadScript: %v", err)
	}

	if len(printed) != 1 || printed[0] != "from module level" {
		t.Errorf("printed = %v, want the module level print routed to the handler", printed)
	}

	// Without a budget this never returns.
	_, _, err = loadCapturingPrint(t, "total = 0\nfor i in range(1000000000):\n    total += i\n", testMaxSteps)
	if err == nil {
		t.Fatal("a runaway module body loaded successfully, the budget did not apply")
	}
}

// script builds a source file defining every required function with the declared
// argument count. Entries in overrides replace the generated definition.
func script(overrides map[string]string) string {
	var b strings.Builder

	for _, required := range RequiredFunctions {
		if src, ok := overrides[required.Name]; ok {
			fmt.Fprintf(&b, "%s\n", src)

			continue
		}

		params := make([]string, required.Args)
		for i := range params {
			params[i] = "a" + strconv.Itoa(i)
		}

		fmt.Fprintf(&b, "def %s(%s): return {}\n", required.Name, strings.Join(params, ", "))
	}

	return b.String()
}

func TestValidateArity(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want int
		ok   bool
	}{
		{"exact", "def f(a, b): pass", 2, true},
		{"too few params", "def f(a): pass", 2, false},
		{"too many params", "def f(a, b, c): pass", 2, false},
		{"default covers the gap", "def f(a, b = 1): pass", 1, true},
		{"default may be supplied", "def f(a, b = 1): pass", 2, true},
		{"default below required", "def f(a, b, c = 1): pass", 1, false},
		{"varargs absorbs any count", "def f(*args): pass", 4, true},
		{"kwargs is not checked", "def f(a, **kw): pass", 3, true},
		{"kwonly is not positional", "def f(a, *, b = 1): pass", 1, true},
		{"zero args", "def f(): pass", 0, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateArity(loadFunc(t, tc.src), tc.want)
			if tc.ok && err != nil {
				t.Errorf("ValidateArity(%q, %d) = %v, want nil", tc.src, tc.want, err)
			}

			if !tc.ok && err == nil {
				t.Errorf("ValidateArity(%q, %d) = nil, want error", tc.src, tc.want)
			}
		})
	}
}

func TestValidateRequiredFunctionsAcceptsMatchingScript(t *testing.T) {
	if err := ValidateRequiredFunctions(load(t, script(nil))); err != nil {
		t.Errorf("ValidateRequiredFunctions = %v, want nil", err)
	}
}

func TestValidateRequiredFunctionsRejectsWrongArity(t *testing.T) {
	// The historical bug had detach declared without the force argument the host passes.
	err := ValidateRequiredFunctions(load(t, script(map[string]string{"detach": "def detach(host): return {}"})))
	if err == nil {
		t.Fatal("ValidateRequiredFunctions = nil, want error")
	}

	if !strings.Contains(err.Error(), "detach") {
		t.Errorf("error %q does not name detach", err)
	}
}

func TestValidateRequiredFunctionsReportsMissingAndArityTogether(t *testing.T) {
	src := strings.ReplaceAll(script(map[string]string{"power_on": "def power_on(host): return {}"}),
		"def get_health(a0): return {}\n", "")

	err := ValidateRequiredFunctions(load(t, src))
	if err == nil {
		t.Fatal("ValidateRequiredFunctions = nil, want error")
	}

	for _, want := range []string{"get_health", "power_on"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %s", err, want)
		}
	}
}

// A name bound to a non callable value is reported as missing, not as bad arity.
func TestValidateRequiredFunctionsRejectsNonCallable(t *testing.T) {
	err := ValidateRequiredFunctions(load(t, script(map[string]string{"get_health": "get_health = 1"})))
	if err == nil {
		t.Fatal("ValidateRequiredFunctions = nil, want error")
	}

	if !strings.Contains(err.Error(), "missing required functions: get_health") {
		t.Errorf("error %q, want get_health reported as missing", err)
	}
}
