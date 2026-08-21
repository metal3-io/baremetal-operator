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

package starlib

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.starlark.net/starlark"
)

// The positional argument order of every Redfish builtin is a public contract
// for user scripts, so these tests pin it with values that differ per slot.

const stubSystemURI = "/redfish/v1/Systems/1"

// redfishStub serves the smallest tree the builtins walk and records PATCH bodies.
func redfishStub(t *testing.T, patched *[]byte) *httptest.Server {
	t.Helper()

	docs := map[string]string{
		"/redfish/v1/": `{"@odata.id":"/redfish/v1/","Id":"RootService","Name":"Root Service",
			"Systems":{"@odata.id":"/redfish/v1/Systems"}}`,
		"/redfish/v1/Systems": `{"Members":[{"@odata.id":"` + stubSystemURI + `"}]}`,
		stubSystemURI: `{"@odata.id":"` + stubSystemURI + `","Id":"1","Name":"System",
			"Manufacturer":"Contoso","Model":"PowerServe R720","SerialNumber":"SN0123456789",
			"BiosVersion":"2.14.1",
			"MemorySummary":{"TotalSystemMemoryGiB":128},
			"ProcessorSummary":{"Count":2,"LogicalProcessorCount":64},
			"EthernetInterfaces":{"@odata.id":"/redfish/v1/Systems/1/EthernetInterfaces"}}`,
		"/redfish/v1/Systems/1/EthernetInterfaces": `{"Members":[
			{"@odata.id":"/redfish/v1/Systems/1/EthernetInterfaces/NIC.1"}]}`,
		"/redfish/v1/Systems/1/EthernetInterfaces/NIC.1": `{"Id":"NIC.1","Name":"Integrated NIC 1",
			"MACAddress":"aa:bb:cc:dd:ee:01"}`,
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			body, _ := io.ReadAll(r.Body)
			if patched != nil {
				*patched = body
			}

			w.WriteHeader(http.StatusNoContent)

			return
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

	return srv
}

func TestRedfishConnArgsPositionalOrder(t *testing.T) {
	// Slots for redfish_set_boot are endpoint, username, password, target,
	// persistent, efi and finally insecure.
	var conn RedfishConnArgs
	var target starlark.String
	var persistent, efi starlark.Bool

	setBoot := starlark.Tuple{
		starlark.String("https://bmc.example.com"),
		starlark.String("svc-user"),
		starlark.String("svc-pass"),
		starlark.String("pxe"),
		starlark.Bool(true),
		starlark.Bool(false),
		starlark.Bool(true),
	}

	if err := conn.Unpack("redfish_set_boot", setBoot, nil,
		"target", &target, "persistent?", &persistent, "efi?", &efi); err != nil {
		t.Fatalf("Unpack: %v", err)
	}

	endpoint, user, pass, insecure := conn.Strings()
	if endpoint != "https://bmc.example.com" || user != "svc-user" || pass != "svc-pass" {
		t.Errorf("connection = (%q, %q, %q), want (https://bmc.example.com, svc-user, svc-pass)", endpoint, user, pass)
	}

	// persistent and efi differ so a swap of the two bool slots is visible.
	if target != "pxe" || !bool(persistent) || bool(efi) || !insecure {
		t.Errorf("target = %q, persistent = %v, efi = %v, insecure = %v, want pxe, true, false, true",
			target, persistent, efi, insecure)
	}

	// Slots for redfish_inventory are endpoint, username, password, insecure
	// and finally fields.
	var inv RedfishConnArgs
	var fields *starlark.List

	inventory := starlark.Tuple{
		starlark.String("https://bmc.example.com"),
		starlark.String("svc-user"),
		starlark.String("svc-pass"),
		starlark.Bool(true),
		starlark.NewList([]starlark.Value{starlark.String("cpu")}),
	}

	if err := inv.UnpackInsecureFirst("redfish_inventory", inventory, nil, "fields?", &fields); err != nil {
		t.Fatalf("UnpackInsecureFirst: %v", err)
	}

	if _, _, _, invInsecure := inv.Strings(); !invInsecure {
		t.Errorf("insecure = false, want true bound from the fourth positional slot")
	}

	if fields == nil || fields.Len() != 1 {
		t.Errorf("fields = %v, want the single element list from the fifth positional slot", fields)
	}
}

func TestRedfishConnPositionalOrder(t *testing.T) {
	args := starlark.Tuple{
		starlark.String("https://bmc.example.com"),
		starlark.String("svc-user"),
		starlark.String("svc-pass"),
		starlark.Bool(true),
	}

	endpoint, user, pass, insecure, err := RedfishConn("redfish_power_on", args, nil)
	if err != nil {
		t.Fatalf("RedfishConn: %v", err)
	}

	if endpoint != "https://bmc.example.com" || user != "svc-user" || pass != "svc-pass" || !insecure {
		t.Errorf("got (%q, %q, %q, %v), want (https://bmc.example.com, svc-user, svc-pass, true)",
			endpoint, user, pass, insecure)
	}
}

// The unknown target error fires before any connection, so it pins the fourth
// positional slot of the real builtin without needing a server.
func TestRedfishSetBootPositionalTargetSlot(t *testing.T) {
	args := starlark.Tuple{
		starlark.String("https://bmc.invalid"),
		starlark.String("u"),
		starlark.String("p"),
		starlark.String("not-a-target"),
	}

	_, err := BuiltinRedfishSetBoot(&starlark.Thread{}, nil, args, nil)
	if err == nil {
		t.Fatal("BuiltinRedfishSetBoot accepted an unknown target")
	}

	if !strings.Contains(err.Error(), `unknown target "not-a-target"`) {
		t.Errorf("error = %v, want the fourth positional argument reported as the target", err)
	}
}

func TestRedfishSetBootPositionalAppliesBootOptions(t *testing.T) {
	var patched []byte
	srv := redfishStub(t, &patched)

	// Slots are endpoint, username, password, target, persistent, efi and insecure.
	// persistent and efi differ so swapping the two bool slots fails this test.
	args := starlark.Tuple{
		starlark.String(srv.URL),
		starlark.String("u"),
		starlark.String("p"),
		starlark.String("pxe"),
		starlark.Bool(true),
		starlark.Bool(false),
		starlark.Bool(false),
	}

	if _, err := BuiltinRedfishSetBoot(&starlark.Thread{}, nil, args, nil); err != nil {
		t.Fatalf("BuiltinRedfishSetBoot: %v", err)
	}

	var body struct {
		Boot struct {
			BootSourceOverrideTarget  string
			BootSourceOverrideEnabled string
			BootSourceOverrideMode    string
		}
	}

	if err := json.Unmarshal(patched, &body); err != nil {
		t.Fatalf("decode patch body %q, %v", patched, err)
	}

	if body.Boot.BootSourceOverrideTarget != "Pxe" {
		t.Errorf("target = %q, want Pxe", body.Boot.BootSourceOverrideTarget)
	}

	// persistent true must reach the BMC as Continuous, efi false as Legacy.
	if body.Boot.BootSourceOverrideEnabled != "Continuous" {
		t.Errorf("enabled = %q, want Continuous from the persistent slot", body.Boot.BootSourceOverrideEnabled)
	}

	if body.Boot.BootSourceOverrideMode != "Legacy" {
		t.Errorf("mode = %q, want Legacy from the efi slot", body.Boot.BootSourceOverrideMode)
	}
}

func TestRedfishSetBootEFIMode(t *testing.T) {
	var patched []byte
	srv := redfishStub(t, &patched)

	// efi true is the only way UEFI reaches the BMC, so it needs its own case.
	args := starlark.Tuple{
		starlark.String(srv.URL),
		starlark.String("u"),
		starlark.String("p"),
		starlark.String("cdrom"),
		starlark.Bool(false),
		starlark.Bool(true),
	}

	if _, err := BuiltinRedfishSetBoot(&starlark.Thread{}, nil, args, nil); err != nil {
		t.Fatalf("BuiltinRedfishSetBoot: %v", err)
	}

	var body struct {
		Boot struct {
			BootSourceOverrideTarget  string
			BootSourceOverrideEnabled string
			BootSourceOverrideMode    string
		}
	}

	if err := json.Unmarshal(patched, &body); err != nil {
		t.Fatalf("decode patch body %q, %v", patched, err)
	}

	if body.Boot.BootSourceOverrideTarget != "Cd" {
		t.Errorf("target = %q, want Cd", body.Boot.BootSourceOverrideTarget)
	}

	if body.Boot.BootSourceOverrideEnabled != "Once" {
		t.Errorf("enabled = %q, want Once when persistent is false", body.Boot.BootSourceOverrideEnabled)
	}

	if body.Boot.BootSourceOverrideMode != "UEFI" {
		t.Errorf("mode = %q, want UEFI when efi is true", body.Boot.BootSourceOverrideMode)
	}
}

func TestRedfishSetBootNonPersistentLegacyDefaults(t *testing.T) {
	var patched []byte
	srv := redfishStub(t, &patched)

	args := starlark.Tuple{
		starlark.String(srv.URL),
		starlark.String("u"),
		starlark.String("p"),
		starlark.String("disk"),
	}

	if _, err := BuiltinRedfishSetBoot(&starlark.Thread{}, nil, args, nil); err != nil {
		t.Fatalf("BuiltinRedfishSetBoot: %v", err)
	}

	var body struct {
		Boot struct {
			BootSourceOverrideTarget  string
			BootSourceOverrideEnabled string
			BootSourceOverrideMode    string
		}
	}

	if err := json.Unmarshal(patched, &body); err != nil {
		t.Fatalf("decode patch body %q, %v", patched, err)
	}

	if body.Boot.BootSourceOverrideTarget != "Hdd" ||
		body.Boot.BootSourceOverrideEnabled != "Once" ||
		body.Boot.BootSourceOverrideMode != "Legacy" {
		t.Errorf("boot = %+v, want Hdd, Once and Legacy when the optional flags are omitted", body.Boot)
	}
}

func TestRedfishInventoryPositionalFieldsSelectsSections(t *testing.T) {
	srv := redfishStub(t, nil)

	// Slots are endpoint, username, password, insecure and then fields.
	args := starlark.Tuple{
		starlark.String(srv.URL),
		starlark.String("u"),
		starlark.String("p"),
		starlark.Bool(false),
		starlark.NewList([]starlark.Value{starlark.String("cpu")}),
	}

	val, err := BuiltinRedfishInventory(&starlark.Thread{}, nil, args, nil)
	if err != nil {
		t.Fatalf("BuiltinRedfishInventory: %v", err)
	}

	inv, ok := ToGo(val).(map[string]any)
	if !ok {
		t.Fatalf("inventory is %T, want a map", ToGo(val))
	}

	// The fifth positional slot must reach the field selector, so nothing but cpu is collected.
	if len(inv) != 1 {
		t.Errorf("sections = %v, want only cpu", inv)
	}

	cpu, ok := inv["cpu"].(map[string]any)
	if !ok {
		t.Fatalf("cpu section is %T, want a map", inv["cpu"])
	}

	if cpu["count"] != int64(64) {
		t.Errorf("cpu count = %v, want 64 from LogicalProcessorCount", cpu["count"])
	}
}

func TestRedfishInventoryCollectsEverySection(t *testing.T) {
	srv := redfishStub(t, nil)

	args := starlark.Tuple{starlark.String(srv.URL), starlark.String("u"), starlark.String("p")}

	val, err := BuiltinRedfishInventory(&starlark.Thread{}, nil, args, nil)
	if err != nil {
		t.Fatalf("BuiltinRedfishInventory: %v", err)
	}

	inv, ok := ToGo(val).(map[string]any)
	if !ok {
		t.Fatalf("inventory is %T, want a map", ToGo(val))
	}

	if inv["ramMebibytes"] != int64(131072) {
		t.Errorf("ramMebibytes = %v, want 131072 from 128 GiB", inv["ramMebibytes"])
	}

	vendor, ok := inv["systemVendor"].(map[string]any)
	if !ok || vendor["manufacturer"] != "Contoso" || vendor["productName"] != "PowerServe R720" {
		t.Errorf("systemVendor = %v, want the manufacturer and model of the stub", inv["systemVendor"])
	}

	nics, ok := inv["nics"].([]any)
	if !ok || len(nics) != 1 {
		t.Fatalf("nics = %v, want one interface", inv["nics"])
	}

	nic, ok := nics[0].(map[string]any)
	if !ok || nic["mac"] != "aa:bb:cc:dd:ee:01" {
		t.Errorf("nic = %v, want the stub MAC", nics[0])
	}

	// The stub serves no UpdateService, so only the version from the system is reported.
	firmware, ok := inv["firmware"].(map[string]any)
	if !ok {
		t.Fatalf("firmware = %v, want a section carrying the BIOS version", inv["firmware"])
	}

	bios, ok := firmware["bios"].(map[string]any)
	if !ok || bios["version"] != "2.14.1" {
		t.Errorf("bios = %v, want version 2.14.1", firmware["bios"])
	}

	if _, present := bios["vendor"]; present {
		t.Errorf("bios vendor = %v, want no vendor without a firmware inventory", bios["vendor"])
	}

	// The stub serves no Storage or SimpleStorage link, so the section is absent.
	if _, present := inv["storage"]; present {
		t.Errorf("storage = %v, want no section when the BMC serves no drives", inv["storage"])
	}
}

func TestRedfishInventoryFailsWithoutSystem(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redfish/v1/" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"@odata.id":"/redfish/v1/","Id":"RootService","Name":"Root"}`))

			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	args := starlark.Tuple{starlark.String(srv.URL), starlark.String("u"), starlark.String("p")}

	if _, err := BuiltinRedfishInventory(&starlark.Thread{}, nil, args, nil); err == nil {
		t.Fatal("BuiltinRedfishInventory succeeded with no Systems link")
	}
}
