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

// End to end cover for scripts/redfish-inspect.star against a stub Redfish
// service carrying the minimum a BMC must serve for inspection to be useful.

package starlark

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/hardwareutils/bmc"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	"github.com/s3rj1k/starlark-provisioner/starlib"
	"github.com/s3rj1k/starlark-provisioner/starscript"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BMC credentials the stub service demands, proving they reach the builtin.
const (
	stubUser = "admin"
	stubPass = "s3cret"
)

// inspectDocs is the Redfish tree the collector walks, keyed by request path.
var inspectDocs = map[string]string{
	"/redfish/v1/": `{"@odata.id":"/redfish/v1/","Id":"RootService","Name":"Root Service",
		"RedfishVersion":"1.6.0","Systems":{"@odata.id":"/redfish/v1/Systems"}}`,
	"/redfish/v1/Systems": `{"Members@odata.count":1,
		"Members":[{"@odata.id":"/redfish/v1/Systems/1"}]}`,
	"/redfish/v1/Systems/1": `{"@odata.id":"/redfish/v1/Systems/1","Id":"1","Name":"System",
		"Manufacturer":"Contoso","Model":"PowerServe R720","SerialNumber":"SN0123456789",
		"BiosVersion":"2.14.1","PowerState":"On","Status":{"Health":"OK"},
		"MemorySummary":{"TotalSystemMemoryGiB":128},
		"ProcessorSummary":{"Count":2,"LogicalProcessorCount":64},
		"Processors":{"@odata.id":"/redfish/v1/Systems/1/Processors"},
		"EthernetInterfaces":{"@odata.id":"/redfish/v1/Systems/1/EthernetInterfaces"},
		"Storage":{"@odata.id":"/redfish/v1/Systems/1/Storage"}}`,
	"/redfish/v1/Systems/1/Processors": `{"Members":[
		{"@odata.id":"/redfish/v1/Systems/1/Processors/CPU.1"}]}`,
	"/redfish/v1/Systems/1/Processors/CPU.1": `{"Id":"CPU.1","Name":"Processor 1",
		"InstructionSet":"x86-64","Model":"Contoso Xeon 6338 32C"}`,
	"/redfish/v1/Systems/1/EthernetInterfaces": `{"Members":[
		{"@odata.id":"/redfish/v1/Systems/1/EthernetInterfaces/NIC.1"}]}`,
	"/redfish/v1/Systems/1/EthernetInterfaces/NIC.1": `{"Id":"NIC.1",
		"Name":"Integrated NIC 1 Port 1","MACAddress":"aa:bb:cc:dd:ee:01"}`,
	"/redfish/v1/Systems/1/Storage": `{"Members":[
		{"@odata.id":"/redfish/v1/Systems/1/Storage/RAID.1"}]}`,
	"/redfish/v1/Systems/1/Storage/RAID.1": `{"Id":"RAID.1","Name":"Storage Controller",
		"Drives":[{"@odata.id":"/redfish/v1/Systems/1/Storage/RAID.1/Drives/Disk.0"}]}`,
	"/redfish/v1/Systems/1/Storage/RAID.1/Drives/Disk.0": `{"Id":"Disk.0","Name":"Disk 0",
		"CapacityBytes":960197124096,"MediaType":"SSD","Protocol":"NVMe",
		"Model":"Contoso NVMe 960G","Manufacturer":"Contoso","SerialNumber":"S3W1NA0M700001",
		"Identifiers":[{"DurableName":"naa.5000c500a1b2c3d4","DurableNameFormat":"NAA"}]}`,
}

// redfishService serves inspectDocs, requiring credentials everywhere except the
// service root, which DSP0266 and the collector both expect to be anonymous.
func redfishService(t *testing.T) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/redfish/v1/" {
			user, pass, ok := r.BasicAuth()
			if !ok || user != stubUser || pass != stubPass {
				w.WriteHeader(http.StatusUnauthorized)

				return
			}
		}

		body, ok := inspectDocs[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)

			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	return srv
}

// inspectProvisioner loads the shipped script and points it at srv over the
// redfish+http scheme, the form a BareMetalHost carries for a plaintext BMC.
func inspectProvisioner(t *testing.T, srv *httptest.Server, events *[]string) *starlarkProvisioner {
	t.Helper()

	globals, err := starscript.LoadScript(filepath.Join("scripts", "redfish-inspect.star"), starlib.Builtins(), starlib.ThreadPrint(logr.Discard()), starlib.MaxExecutionSteps)
	if err != nil {
		t.Fatalf("LoadScript: %v", err)
	}

	authority := strings.TrimPrefix(srv.URL, "http://")

	return &starlarkProvisioner{
		globals: globals,
		log:     logr.Discard(),
		publisher: func(reason, message string) {
			if events != nil {
				*events = append(*events, reason+": "+message)
			}
		},
		hostData: provisioner.HostData{
			ObjectMeta:     metav1.ObjectMeta{Namespace: "ns", Name: "node-1", UID: "bmh-uid"},
			BMCAddress:     "redfish+http://" + authority + "/redfish/v1/Systems/1",
			BMCCredentials: bmc.Credentials{Username: stubUser, Password: stubPass},
			ProvisionerID:  "bmh-uid",
		},
	}
}

func TestRedfishInspectCollectsHardwareDetails(t *testing.T) {
	var events []string

	p := inspectProvisioner(t, redfishService(t), &events)

	result, started, details, err := p.InspectHardware(context.Background(),
		provisioner.InspectData{InspectionMode: "fast"}, false, false, false)
	if err != nil {
		t.Fatalf("InspectHardware: %v", err)
	}

	if details == nil {
		t.Fatal("details are nil, want an inventory")
	}

	// Out of band inspection finishes in one pass, so it never reports started
	// or asks to be requeued.
	if started || result.Dirty || result.ErrorMessage != "" {
		t.Errorf("result = %+v, started = %v, want a clean single pass", result, started)
	}

	if details.RAMMebibytes != 131072 {
		t.Errorf("RAMMebibytes = %d, want 131072 from 128 GiB", details.RAMMebibytes)
	}

	if details.CPU.Count != 64 {
		t.Errorf("CPU.Count = %d, want 64 from LogicalProcessorCount", details.CPU.Count)
	}

	if details.CPU.Arch != "x86_64" {
		t.Errorf("CPU.Arch = %q, want x86_64 mapped from x86-64", details.CPU.Arch)
	}

	if details.SystemVendor.Manufacturer != "Contoso" || details.SystemVendor.ProductName != "PowerServe R720" {
		t.Errorf("SystemVendor = %+v, want the stub vendor", details.SystemVendor)
	}

	if details.Firmware.BIOS.Version != "2.14.1" {
		t.Errorf("BIOS.Version = %q, want 2.14.1", details.Firmware.BIOS.Version)
	}

	if len(details.NIC) != 1 || details.NIC[0].MAC != "aa:bb:cc:dd:ee:01" {
		t.Fatalf("NIC = %+v, want the stub interface", details.NIC)
	}

	if len(details.Storage) != 1 {
		t.Fatalf("Storage = %+v, want one drive", details.Storage)
	}

	disk := details.Storage[0]
	if disk.Type != "NVME" || disk.SizeBytes != 960197124096 || disk.WWN != "naa.5000c500a1b2c3d4" {
		t.Errorf("Storage[0] = %+v, want the NVMe drive of the stub", disk)
	}

	if disk.Rotational {
		t.Error("Storage[0].Rotational is true, want false for an SSD")
	}

	// A complete inventory reports completion and nothing about gaps.
	var complete, incomplete bool

	for _, e := range events {
		if strings.HasPrefix(e, "InspectionComplete") {
			complete = true
		}

		if strings.HasPrefix(e, "InspectionIncomplete") {
			incomplete = true
		}
	}

	if !complete || incomplete {
		t.Errorf("events = %v, want completion and no gap report", events)
	}
}

// The BMC serves only the system, so every collection link is missing and the
// script records the partial inventory while naming the gaps.
func TestRedfishInspectReportsPartialInventory(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		docs := map[string]string{
			"/redfish/v1/":          inspectDocs["/redfish/v1/"],
			"/redfish/v1/Systems":   inspectDocs["/redfish/v1/Systems"],
			"/redfish/v1/Systems/1": `{"Id":"1","Manufacturer":"Contoso","BiosVersion":"2.14.1"}`,
		}

		body, ok := docs[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)

			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	var events []string

	p := inspectProvisioner(t, srv, &events)

	_, _, details, err := p.InspectHardware(context.Background(), provisioner.InspectData{}, false, false, false)
	if err != nil {
		t.Fatalf("InspectHardware: %v", err)
	}

	if details == nil || details.SystemVendor.Manufacturer != "Contoso" {
		t.Fatalf("details = %+v, want the vendor the BMC did serve", details)
	}

	if details.RAMMebibytes != 0 || len(details.NIC) != 0 {
		t.Errorf("details = %+v, want no fabricated memory or NICs", details)
	}

	var reported string

	for _, e := range events {
		if strings.HasPrefix(e, "InspectionIncomplete") {
			reported = e
		}
	}

	for _, want := range []string{"ramMebibytes", "cpu.count", "nics", "storage"} {
		if !strings.Contains(reported, want) {
			t.Errorf("gap report %q does not name %s", reported, want)
		}
	}
}

func TestRedfishInspectPowerAndHealth(t *testing.T) {
	p := inspectProvisioner(t, redfishService(t), nil)
	ctx := context.Background()

	state, err := p.UpdateHardwareState(ctx)
	if err != nil {
		t.Fatalf("UpdateHardwareState: %v", err)
	}

	if state.PoweredOn == nil || !*state.PoweredOn {
		t.Errorf("PoweredOn = %v, want true from PowerState On", state.PoweredOn)
	}

	if health := p.GetHealth(ctx); health != provisioner.HealthOK {
		t.Errorf("GetHealth = %q, want %q from a Health rollup of OK", health, provisioner.HealthOK)
	}

	if p.HasPowerFailure(ctx) {
		t.Error("HasPowerFailure is true, want false")
	}
}

// A host already powered on needs no reset call, so the script reports clean.
func TestRedfishInspectPowerOnIsIdempotent(t *testing.T) {
	p := inspectProvisioner(t, redfishService(t), nil)

	result, err := p.PowerOn(context.Background(), false)
	if err != nil {
		t.Fatalf("PowerOn: %v", err)
	}

	if result.Dirty || result.ErrorMessage != "" {
		t.Errorf("result = %+v, want a no op for an already powered host", result)
	}
}

func TestRedfishInspectRejectsNonRedfishBMC(t *testing.T) {
	p := inspectProvisioner(t, redfishService(t), nil)
	p.hostData.BMCAddress = "ipmi://192.168.0.1"

	result, started, details, err := p.InspectHardware(context.Background(),
		provisioner.InspectData{}, false, false, false)
	if err == nil {
		t.Fatal("InspectHardware accepted an IPMI address")
	}

	if !strings.Contains(err.Error(), "unsupported BMC scheme") {
		t.Errorf("error = %v, want an unsupported scheme rejection", err)
	}

	if details != nil || started || result.Dirty {
		t.Errorf("details = %+v, started = %v, result = %+v, want nothing on rejection", details, started, result)
	}
}

// Provisioning is out of scope, and the script has to say so rather than
// silently reporting success and stranding the host.
func TestRedfishInspectRefusesProvisioning(t *testing.T) {
	p := inspectProvisioner(t, redfishService(t), nil)

	result, err := p.Provision(context.Background(), provisioner.ProvisionData{}, false)
	if err != nil {
		t.Fatalf("Provision: %v", err)
	}

	if !strings.Contains(result.ErrorMessage, "not supported") {
		t.Errorf("ErrorMessage = %q, want an unsupported report", result.ErrorMessage)
	}
}

// Teardown must not depend on the BMC, otherwise a host with a broken address
// keeps its finalizer and cannot be deleted.
func TestRedfishInspectTeardownIgnoresBrokenBMC(t *testing.T) {
	p := inspectProvisioner(t, redfishService(t), nil)
	p.hostData.BMCAddress = "not-a-url"

	ctx := context.Background()

	cases := map[string]func() (provisioner.Result, error){
		"deprovision": func() (provisioner.Result, error) { return p.Deprovision(ctx, false, "") },
		"delete":      func() (provisioner.Result, error) { return p.Delete(ctx) },
		"detach":      func() (provisioner.Result, error) { return p.Detach(ctx, false) },
	}

	for name, call := range cases {
		t.Run(name, func(t *testing.T) {
			result, err := call()
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}

			if result.Dirty || result.ErrorMessage != "" {
				t.Errorf("result = %+v, want a clean no op", result)
			}
		})
	}
}

func TestRedfishInspectReportsBIOSComponent(t *testing.T) {
	p := inspectProvisioner(t, redfishService(t), nil)

	components, err := p.GetFirmwareComponents(context.Background())
	if err != nil {
		t.Fatalf("GetFirmwareComponents: %v", err)
	}

	if len(components) != 1 {
		t.Fatalf("components = %+v, want the BIOS entry", components)
	}

	if components[0].Component != "bios" || components[0].InitialVersion != "2.14.1" {
		t.Errorf("component = %+v, want bios at 2.14.1", components[0])
	}
}

// Four sub resource controllers build HostData with BuildHostDataNoBMC, so these
// methods run with no BMC address and refusing there strands the CR forever.
func TestShippedScriptsSurviveHostDataWithNoBMC(t *testing.T) {
	for _, path := range shippedScripts(t) {
		t.Run(filepath.Base(path), func(t *testing.T) {
			globals, err := starscript.LoadScript(path, starlib.Builtins(), starlib.ThreadPrint(logr.Discard()), starlib.MaxExecutionSteps)
			if err != nil {
				t.Fatalf("LoadScript: %v", err)
			}

			// Exactly what BuildHostDataNoBMC produces, no address and no credentials.
			p := &starlarkProvisioner{
				globals: globals,
				log:     logr.Discard(),
				hostData: provisioner.HostData{
					ObjectMeta:    metav1.ObjectMeta{Namespace: "ns", Name: "node-1", UID: "uid-1"},
					ProvisionerID: "prov-1",
				},
			}

			ctx := context.Background()

			if _, err := p.GetDataImageStatus(ctx); err != nil {
				t.Errorf("GetDataImageStatus with no BMC: %v", err)
			}

			if err := p.DetachDataImage(ctx); err != nil {
				t.Errorf("DetachDataImage with no BMC: %v", err)
			}

			if _, _, err := p.GetFirmwareSettings(ctx, false); err != nil {
				t.Errorf("GetFirmwareSettings with no BMC: %v", err)
			}

			if _, err := p.GetFirmwareComponents(ctx); err != nil {
				t.Errorf("GetFirmwareComponents with no BMC: %v", err)
			}

			if _, err := p.RemoveBMCEventSubscriptionForNode(ctx, metal3api.BMCEventSubscription{}); err != nil {
				t.Errorf("RemoveBMCEventSubscriptionForNode with no BMC: %v", err)
			}
		})
	}
}

// power_off runs in StatePoweringOffBeforeDelete, so a host whose BMC address is
// gone must still power off cleanly or deletion never completes.
func TestRedfishInspectPowerOffWithoutBMCDoesNotBlockDeletion(t *testing.T) {
	p := inspectProvisioner(t, redfishService(t), nil)
	p.hostData.BMCAddress = ""

	result, err := p.PowerOff(context.Background(), "hard", false, "")
	if err != nil {
		t.Fatalf("PowerOff with no BMC address: %v", err)
	}

	if result.Dirty || result.ErrorMessage != "" {
		t.Errorf("result = %+v, want a clean no op", result)
	}
}
