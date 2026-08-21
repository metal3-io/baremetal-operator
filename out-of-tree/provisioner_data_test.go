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
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-logr/logr"
	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	"github.com/s3rj1k/starlark-provisioner/starlib"
	"github.com/s3rj1k/starlark-provisioner/starscript"
)

// newScriptProvisioner builds a provisioner over an inline script.
func newScriptProvisioner(t *testing.T, script string) *starlarkProvisioner {
	t.Helper()

	path := filepath.Join(t.TempDir(), "s.star")
	if err := os.WriteFile(path, []byte(script), 0o600); err != nil {
		t.Fatalf("write script: %v", err)
	}

	globals, err := starscript.LoadScript(path, starlib.Builtins())
	if err != nil {
		t.Fatalf("load script: %v", err)
	}

	return &starlarkProvisioner{globals: globals, log: logr.Discard()}
}

// Prepare and Service hand scripts the upstream struct field names verbatim, so
// this pins that contract and fails loudly when an upstream field is added or removed.
func TestPrepareDataKeysReachScript(t *testing.T) {
	script := `
WANT = ["ActualFirmwareSettings", "ActualRAIDConfig", "RootDeviceHints",
        "TargetFirmwareComponents", "TargetFirmwareSettings", "TargetRAIDConfig"]

def prepare(host, data, unprepared, restart_on_failure):
    got = sorted(data.keys())
    if got != WANT:
        return {"error": "prepare data keys are " + str(got)}
    return {"started": True}
`

	p := newScriptProvisioner(t, script)

	result, started, err := p.Prepare(context.Background(), provisioner.PrepareData{}, true, false)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	if result.ErrorMessage != "" {
		t.Fatalf("script rejected the data map, %s", result.ErrorMessage)
	}

	if !started {
		t.Error("started = false, want true")
	}
}

// The deprecated FirmwareConfig field was dropped upstream and the plugin no
// longer strips it, so this fails if a dependency bump brings it back.
func TestPrepareAndServiceHideFirmwareConfig(t *testing.T) {
	script := `
def prepare(host, data, unprepared, restart_on_failure):
    if "FirmwareConfig" in data:
        return {"error": "prepare data leaked FirmwareConfig"}
    return {"started": True}

def service(host, data, unprepared, restart_on_failure):
    if "FirmwareConfig" in data:
        return {"error": "service data leaked FirmwareConfig"}
    return {"started": True}
`

	p := newScriptProvisioner(t, script)

	prepared, _, err := p.Prepare(context.Background(), provisioner.PrepareData{}, true, false)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	if prepared.ErrorMessage != "" {
		t.Error(prepared.ErrorMessage)
	}

	serviced, started, err := p.Service(context.Background(), provisioner.ServicingData{}, true, false)
	if err != nil {
		t.Fatalf("Service: %v", err)
	}

	if serviced.ErrorMessage != "" {
		t.Error(serviced.ErrorMessage)
	}

	if !started {
		t.Error("started = false, want true")
	}
}

func TestRegisterReturnsProvID(t *testing.T) {
	script := `
def register(host, data, credentials_changed, restart_on_failure):
    if not credentials_changed:
        return {"error": "credentials_changed did not reach the script"}
    if data.get("CPUArchitecture") != "x86_64":
        return {"error": "data did not reach the script, got " + str(data)}
    return {"provID": "node-uuid-1", "dirty": True}
`

	p := newScriptProvisioner(t, script)

	data := provisioner.ManagementAccessData{CPUArchitecture: "x86_64"}

	result, provID, err := p.Register(context.Background(), data, true, false)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	if result.ErrorMessage != "" {
		t.Fatalf("script reported %s", result.ErrorMessage)
	}

	if provID != "node-uuid-1" {
		t.Errorf("provID = %q, want node-uuid-1", provID)
	}

	if !result.Dirty {
		t.Error("Dirty = false, want true")
	}
}

func TestRegisterNeedsPreprovisioningSentinel(t *testing.T) {
	script := `
def register(host, data, credentials_changed, restart_on_failure):
    return {"error": "needs-preprovisioning-image", "provID": "node-uuid-1"}
`

	p := newScriptProvisioner(t, script)

	result, provID, err := p.Register(context.Background(), provisioner.ManagementAccessData{}, false, false)
	if !errors.Is(err, provisioner.ErrNeedsPreprovisioningImage) {
		t.Fatalf("err = %v, want ErrNeedsPreprovisioningImage", err)
	}

	// The sentinel is consumed, so it must not surface as a host error message.
	if result.ErrorMessage != "" {
		t.Errorf("ErrorMessage = %q, want empty once the sentinel is translated", result.ErrorMessage)
	}

	if provID != "node-uuid-1" {
		t.Errorf("provID = %q, want node-uuid-1", provID)
	}
}

// InspectData carries the InspectionMode added upstream, so scripts can choose
// between a ramdisk inspection and an out of band one.
func TestInspectHardwareParsesDetails(t *testing.T) {
	script := `
def inspect_hardware(host, data, restart_on_failure, refresh, force_reboot):
    if "InspectionMode" not in data:
        return {"error": "InspectData is missing InspectionMode"}
    if data["InspectionMode"] != "fast":
        return {"error": "InspectionMode is " + str(data["InspectionMode"])}
    return {
        "started": True,
        "hardwareDetails": {
            "ramMebibytes": 131072,
            "cpu": {"count": 64, "arch": "x86_64"},
            "nics": [{"mac": "aa:bb:cc:dd:ee:01"}],
        },
    }
`

	p := newScriptProvisioner(t, script)

	data := provisioner.InspectData{InspectionMode: "fast"}

	result, started, details, err := p.InspectHardware(context.Background(), data, false, false, false)
	if err != nil {
		t.Fatalf("InspectHardware: %v", err)
	}

	if result.ErrorMessage != "" {
		t.Fatalf("script reported %s", result.ErrorMessage)
	}

	if !started {
		t.Error("started = false, want true")
	}

	if details == nil {
		t.Fatal("details = nil, want parsed hardware details")
	}

	if details.RAMMebibytes != 131072 {
		t.Errorf("RAMMebibytes = %d, want 131072", details.RAMMebibytes)
	}

	if details.CPU.Count != 64 || details.CPU.Arch != "x86_64" {
		t.Errorf("CPU = %+v, want count 64 and arch x86_64", details.CPU)
	}

	if len(details.NIC) != 1 || details.NIC[0].MAC != "aa:bb:cc:dd:ee:01" {
		t.Errorf("NIC = %+v, want the single MAC from the script", details.NIC)
	}
}

func TestInspectHardwareWithoutDetails(t *testing.T) {
	script := `
def inspect_hardware(host, data, restart_on_failure, refresh, force_reboot):
    return {"started": True}
`

	p := newScriptProvisioner(t, script)

	_, started, details, err := p.InspectHardware(context.Background(), provisioner.InspectData{}, false, false, false)
	if err != nil {
		t.Fatalf("InspectHardware: %v", err)
	}

	if !started {
		t.Error("started = false, want true")
	}

	if details != nil {
		t.Errorf("details = %+v, want nil when the script reports none", details)
	}
}

func TestInspectHardwareRejectsBadDetails(t *testing.T) {
	script := `
def inspect_hardware(host, data, restart_on_failure, refresh, force_reboot):
    return {"hardwareDetails": {"ramMebibytes": "plenty"}}
`

	p := newScriptProvisioner(t, script)

	_, _, details, err := p.InspectHardware(context.Background(), provisioner.InspectData{}, false, false, false)
	if err == nil {
		t.Fatal("InspectHardware accepted a string where an int is required")
	}

	if details != nil {
		t.Errorf("details = %+v, want nil on a parse failure", details)
	}
}

// RemoveBMCEventSubscriptionForNode is the sixth CallWithData caller and the
// only one whose argument is a value rather than a pointer.
func TestRemoveBMCEventSubscription(t *testing.T) {
	script := `
def remove_bmc_event_subscription(host, sub):
    if sub.get("status", {}).get("subscriptionID") != "sub-123":
        return {"error": "subscription did not reach the script, got " + str(sub)}
    return {"dirty": True}
`

	p := newScriptProvisioner(t, script)

	sub := metal3api.BMCEventSubscription{
		Status: metal3api.BMCEventSubscriptionStatus{SubscriptionID: "sub-123"},
	}

	result, err := p.RemoveBMCEventSubscriptionForNode(context.Background(), sub)
	if err != nil {
		t.Fatalf("RemoveBMCEventSubscriptionForNode: %v", err)
	}

	if result.ErrorMessage != "" {
		t.Fatalf("script reported %s", result.ErrorMessage)
	}

	if !result.Dirty {
		t.Error("Dirty = false, want true")
	}
}

func TestAdoptPassesRestartFlag(t *testing.T) {
	script := `
def adopt(host, data, restart_on_failure):
    if not restart_on_failure:
        return {"error": "restart_on_failure did not reach the script"}
    return {"dirty": True, "requeue_after_seconds": 5}
`

	p := newScriptProvisioner(t, script)

	result, err := p.Adopt(context.Background(), provisioner.AdoptData{}, true)
	if err != nil {
		t.Fatalf("Adopt: %v", err)
	}

	if result.ErrorMessage != "" {
		t.Fatalf("script reported %s", result.ErrorMessage)
	}

	if result.RequeueAfter.Seconds() != 5 {
		t.Errorf("RequeueAfter = %v, want 5s", result.RequeueAfter)
	}

	// A requeue delay implies dirty so the controller actually comes back.
	if !result.Dirty {
		t.Error("Dirty = false, want true alongside a requeue delay")
	}
}
