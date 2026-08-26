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

// Package starscript loads Starlark scripts, validates required functions, and runs calls with panic recovery.
package starscript

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
)

// RequiredFunction is a Starlark entry point with the positional argument count
// the provisioner always calls it with, the host dict being the first.
type RequiredFunction struct {
	Name string
	Args int
}

// Starlark functions the provisioner calls, validated at factory load time. Args
// must match the call sites in provisioner.go or a mismatch survives to runtime.
var RequiredFunctions = []RequiredFunction{
	{"has_capacity", 1},
	{"register", 4},
	{"preprovisioning_image_formats", 1},
	{"inspect_hardware", 5},
	{"update_hardware_state", 1},
	{"adopt", 3},
	{"prepare", 4},
	{"service", 4},
	{"provision", 3},
	{"deprovision", 3},
	{"delete", 1},
	{"detach", 2},
	{"power_on", 2},
	{"power_off", 4},
	{"get_firmware_settings", 2},
	{"get_firmware_components", 1},
	{"add_bmc_event_subscription", 2},
	{"remove_bmc_event_subscription", 2},
	{"get_data_image_status", 1},
	{"attach_data_image", 2},
	{"detach_data_image", 1},
	{"has_power_failure", 1},
	{"get_health", 1},
}

// LoadScript reads and executes a Starlark script with the given predeclared
// builtins, routing print through onPrint and bounding module code by maxSteps.
func LoadScript(path string, predeclared starlark.StringDict, onPrint func(*starlark.Thread, string), maxSteps uint64) (starlark.StringDict, error) {
	// Path is user supplied by design. It comes from the operator's flag, not untrusted runtime input.
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading script %s: %w", path, err)
	}

	// Module level code runs here, so it needs the same print routing and step
	// budget the per call threads get.
	thread := &starlark.Thread{Name: filepath.Base(path), Print: onPrint}
	thread.SetMaxExecutionSteps(maxSteps)

	globals, err := starlark.ExecFileOptions(&syntax.FileOptions{}, thread, path, data, predeclared)
	if err != nil {
		return nil, fmt.Errorf("executing script %s: %w", path, err)
	}

	return globals, nil
}

// ArityRange renders an accepted positional argument count for an error message.
func ArityRange(required, positional int) string {
	if required == positional {
		return strconv.Itoa(required)
	}

	return strconv.Itoa(required) + " to " + strconv.Itoa(positional)
}

// ValidateArity reports whether fn can be called with exactly want positional arguments.
func ValidateArity(fn *starlark.Function, want int) error {
	// A *args or **kwargs signature absorbs any count, so there is nothing to check.
	if fn.HasVarargs() || fn.HasKwargs() {
		return nil
	}

	// NumParams counts keyword only params too, and those are never passed positionally.
	positional := fn.NumParams() - fn.NumKwonlyParams()

	required := 0

	for i := range positional {
		if fn.ParamDefault(i) == nil {
			required++
		}
	}

	if want < required || want > positional {
		noun := "arguments"
		if required == positional && positional == 1 {
			noun = "argument"
		}

		return fmt.Errorf("takes %s positional %s, called with %d", ArityRange(required, positional), noun, want)
	}

	return nil
}

// ValidateRequiredFunctions reports every required function that is absent from
// globals, not callable, or declared with an incompatible argument count.
func ValidateRequiredFunctions(globals starlark.StringDict) error {
	var missing, badArity []string

	for _, required := range RequiredFunctions {
		v, ok := globals[required.Name]
		if !ok {
			missing = append(missing, required.Name)

			continue
		}

		// Accept both user defined functions and builtins. Anything else shadowing the name is "missing".
		switch fn := v.(type) {
		case *starlark.Function:
			// Only a user defined function exposes its signature, so only it can be arity checked.
			if err := ValidateArity(fn, required.Args); err != nil {
				badArity = append(badArity, fmt.Sprintf("%s %s", required.Name, err))
			}
		case *starlark.Builtin:
			// callable, signature not introspectable
		default:
			missing = append(missing, required.Name)
		}
	}

	var errs []error
	if len(missing) > 0 {
		errs = append(errs, fmt.Errorf("missing required functions: %s", strings.Join(missing, ", ")))
	}

	if len(badArity) > 0 {
		errs = append(errs, fmt.Errorf("wrong signature: %s", strings.Join(badArity, "; ")))
	}

	return errors.Join(errs...)
}

// CallOnThread invokes a named function in globals on the given thread, converting panics into errors.
func CallOnThread(
	thread *starlark.Thread,
	globals starlark.StringDict,
	name string,
	args starlark.Tuple,
) (result starlark.Value, err error) {
	defer func() {
		if r := recover(); r != nil {
			result = starlark.None
			err = fmt.Errorf("%s: starlark call panicked: %v", name, r)
		}
	}()

	fn, ok := globals[name]
	if !ok {
		return starlark.None, fmt.Errorf("%s: function not defined in script", name)
	}

	v, callErr := starlark.Call(thread, fn, args, nil)
	if callErr != nil {
		return starlark.None, fmt.Errorf("%s: %w", name, callErr)
	}

	return v, nil
}
