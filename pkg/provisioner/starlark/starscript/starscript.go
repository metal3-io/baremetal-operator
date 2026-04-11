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
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
)

// Starlark function names the provisioner calls; validated at factory load time.
var requiredFunctions = []string{
	"try_init",
	"has_capacity",
	"register",
	"preprovisioning_image_formats",
	"inspect_hardware",
	"update_hardware_state",
	"adopt",
	"prepare",
	"service",
	"provision",
	"deprovision",
	"delete",
	"detach",
	"power_on",
	"power_off",
	"get_firmware_settings",
	"get_firmware_components",
	"add_bmc_event_subscription",
	"remove_bmc_event_subscription",
	"get_data_image_status",
	"attach_data_image",
	"detach_data_image",
	"has_power_failure",
	"get_health",
}

// LoadScript reads and executes a Starlark script with the given predeclared builtins.
func LoadScript(path string, predeclared starlark.StringDict) (starlark.StringDict, error) {
	// Path is user-supplied by design — it comes from the operator's flag, not untrusted runtime input.
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading script %s: %w", path, err)
	}

	thread := &starlark.Thread{Name: filepath.Base(path)}

	globals, err := starlark.ExecFileOptions(&syntax.FileOptions{}, thread, path, data, predeclared)
	if err != nil {
		return nil, fmt.Errorf("executing script %s: %w", path, err)
	}

	return globals, nil
}

// ValidateRequiredFunctions reports every required function absent from globals or not callable.
func ValidateRequiredFunctions(globals starlark.StringDict) error {
	var missing []string

	for _, name := range requiredFunctions {
		v, ok := globals[name]
		if !ok {
			missing = append(missing, name)

			continue
		}
		// Accept both user-defined functions and builtins; anything else shadowing the name is "missing".
		switch v.(type) {
		case *starlark.Function, *starlark.Builtin:
			// callable
		default:
			missing = append(missing, name)
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("missing required functions: %s", strings.Join(missing, ", "))
	}

	return nil
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
