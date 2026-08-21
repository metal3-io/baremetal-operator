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
	"testing"

	"github.com/stmcginnis/gofish/schemas"
	"go.starlark.net/starlark"
)

func TestRedfishCPUArch(t *testing.T) {
	cases := map[schemas.InstructionSet]string{
		schemas.X8664InstructionSet:   "x86_64",
		schemas.ARMA64InstructionSet:  "aarch64",
		schemas.InstructionSet("x86"): "x86",
		schemas.InstructionSet(""):    "",
	}

	for in, want := range cases {
		if got := RedfishCPUArch(in); got != want {
			t.Errorf("RedfishCPUArch(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRedfishDriveType(t *testing.T) {
	cases := []struct {
		name  string
		drive *schemas.Drive
		want  string
	}{
		{"nvme by protocol", &schemas.Drive{Protocol: schemas.NVMeProtocol, MediaType: schemas.SSDMediaType}, "NVME"},
		{"hdd", &schemas.Drive{MediaType: schemas.HDDMediaType}, "HDD"},
		{"smr is hdd", &schemas.Drive{MediaType: schemas.SMRMediaType}, "HDD"},
		{"ssd", &schemas.Drive{MediaType: schemas.SSDMediaType}, "SSD"},
		{"unknown", &schemas.Drive{}, ""},
	}

	for _, c := range cases {
		if got := RedfishDriveType(c.drive); got != c.want {
			t.Errorf("%s: RedfishDriveType = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestRedfishIsBIOSEntry(t *testing.T) {
	cases := []struct {
		name string
		inv  *schemas.SoftwareInventory
		want bool
	}{
		{"system bios name", &schemas.SoftwareInventory{Entity: schemas.Entity{Name: "System BIOS"}}, true},
		{"bios id", &schemas.SoftwareInventory{Entity: schemas.Entity{ID: "BIOS"}}, true},
		{"biosconnect is not bios", &schemas.SoftwareInventory{Entity: schemas.Entity{Name: "BIOSConnect"}}, false},
		{"unrelated", &schemas.SoftwareInventory{Entity: schemas.Entity{Name: "iDRAC"}}, false},
	}

	for _, c := range cases {
		if got := RedfishIsBIOSEntry(c.inv); got != c.want {
			t.Errorf("%s: RedfishIsBIOSEntry = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestRedfishName(t *testing.T) {
	if got := RedfishName("friendly", "id"); got != "friendly" {
		t.Errorf("name preference wrong, got %q", got)
	}

	if got := RedfishName("", "id"); got != "id" {
		t.Errorf("id fallback wrong, got %q", got)
	}
}

func TestRedfishFieldSelector(t *testing.T) {
	all := RedfishFieldSelector(nil)
	if !all("cpu") || !all("storage") {
		t.Error("nil list should select every section")
	}

	if empty := RedfishFieldSelector(starlark.NewList(nil)); !empty("cpu") {
		t.Error("empty list should select every section")
	}

	sel := RedfishFieldSelector(starlark.NewList([]starlark.Value{
		starlark.String("cpu"),
		starlark.String("nics"),
	}))

	if !sel("cpu") || !sel("nics") {
		t.Error("listed sections should be selected")
	}

	if sel("storage") || sel("firmware") {
		t.Error("unlisted sections should be excluded")
	}
}
