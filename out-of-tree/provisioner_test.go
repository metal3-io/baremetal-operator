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
	"strings"
	"testing"

	"github.com/go-logr/logr"
	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/s3rj1k/starlark-provisioner/starlib"
	"github.com/s3rj1k/starlark-provisioner/starscript"
)

func TestAddBMCEventSubscriptionSetsID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "s.star")

	script := "def add_bmc_event_subscription(host, sub):\n" +
		"    return {\"subscriptionID\": \"sub-123\"}\n"
	if err := os.WriteFile(path, []byte(script), 0o600); err != nil {
		t.Fatalf("write script: %v", err)
	}

	globals, err := starscript.LoadScript(path, starlib.Builtins(), starlib.ThreadPrint(logr.Discard()), starlib.MaxExecutionSteps)
	if err != nil {
		t.Fatalf("load script: %v", err)
	}

	p := &starlarkProvisioner{globals: globals, log: logr.Discard()}
	sub := &metal3api.BMCEventSubscription{}

	if _, err := p.AddBMCEventSubscriptionForNode(context.Background(), sub, nil); err != nil {
		t.Fatalf("AddBMCEventSubscriptionForNode: %v", err)
	}

	if sub.Status.SubscriptionID != "sub-123" {
		t.Errorf("SubscriptionID = %q, want sub-123", sub.Status.SubscriptionID)
	}
}

// A script reports a read failure as a dict carrying an error, so that message
// must reach the operator instead of a generic shape mismatch.
func TestGetFirmwareComponentsSurfacesScriptError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "s.star")

	script := "def get_firmware_components(host):\n" +
		"    return {\"error\": \"BMC unreachable, connection refused\"}\n"
	if err := os.WriteFile(path, []byte(script), 0o600); err != nil {
		t.Fatalf("write script: %v", err)
	}

	globals, err := starscript.LoadScript(path, starlib.Builtins(), starlib.ThreadPrint(logr.Discard()), starlib.MaxExecutionSteps)
	if err != nil {
		t.Fatalf("load script: %v", err)
	}

	p := &starlarkProvisioner{globals: globals, log: logr.Discard()}

	out, err := p.GetFirmwareComponents(context.Background())
	if err == nil {
		t.Fatalf("GetFirmwareComponents = %+v, want the script error", out)
	}

	if !strings.Contains(err.Error(), "BMC unreachable") {
		t.Errorf("error = %v, want the script's own message", err)
	}
}

// The unsupported sentinel still means no components rather than an error.
func TestGetFirmwareComponentsUnsupportedSentinel(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "s.star")

	script := "def get_firmware_components(host):\n" +
		"    return {\"error\": \"" + SentinelFirmwareUnsupported + "\"}\n"
	if err := os.WriteFile(path, []byte(script), 0o600); err != nil {
		t.Fatalf("write script: %v", err)
	}

	globals, err := starscript.LoadScript(path, starlib.Builtins(), starlib.ThreadPrint(logr.Discard()), starlib.MaxExecutionSteps)
	if err != nil {
		t.Fatalf("load script: %v", err)
	}

	p := &starlarkProvisioner{globals: globals, log: logr.Discard()}

	out, err := p.GetFirmwareComponents(context.Background())
	if err != nil || out != nil {
		t.Errorf("GetFirmwareComponents = (%+v, %v), want (nil, nil)", out, err)
	}
}
