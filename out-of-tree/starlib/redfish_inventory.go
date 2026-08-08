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

// Optional Redfish inventory collection shaped to the metal3api HardwareDetails object.

package starlib

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	gofish "github.com/stmcginnis/gofish"
	"github.com/stmcginnis/gofish/schemas"
	"go.starlark.net/starlark"
)

// mebibytesPerGibibyte converts the summary GiB value to the MiB field HardwareDetails wants.
const mebibytesPerGibibyte = 1024

// redfishFieldSelector returns a predicate reporting whether a section should be collected.
// An empty or missing list selects every section.
func redfishFieldSelector(fields *starlark.List) func(string) bool {
	if fields == nil || fields.Len() == 0 {
		return func(string) bool { return true }
	}

	set := make(map[string]bool, fields.Len())
	for i := range fields.Len() {
		if s, ok := starlark.AsString(fields.Index(i)); ok {
			set[s] = true
		}
	}

	return func(key string) bool { return set[key] }
}

// redfishSystemVendor reports the non empty manufacturer, product, and serial of the whole system.
func redfishSystemVendor(sys *schemas.ComputerSystem) map[string]any {
	vendor := map[string]any{}
	PutNonZero(vendor, "manufacturer", sys.Manufacturer)
	PutNonZero(vendor, "productName", sys.Model)
	PutNonZero(vendor, "serialNumber", sys.SerialNumber)

	return vendor
}

// redfishIsBIOSEntry reports whether a firmware inventory entry describes the system BIOS.
// It matches a whole bios token so names like BIOSConnect are not mistaken for the BIOS.
func redfishIsBIOSEntry(s *schemas.SoftwareInventory) bool {
	fields := strings.FieldsFunc(strings.ToLower(s.Name+" "+s.ID+" "+s.SoftwareID), func(r rune) bool {
		return r == ' ' || r == '.' || r == '_' || r == '-'
	})

	return slices.Contains(fields, "bios")
}

// redfishBIOSFirmware finds the BIOS entry in the firmware inventory and returns its vendor and date.
func redfishBIOSFirmware(c *gofish.APIClient) (vendor, date string) {
	us, err := c.Service.UpdateService()
	if err != nil || us == nil {
		return "", ""
	}

	items, err := us.FirmwareInventory()
	if err != nil {
		return "", ""
	}

	for _, s := range items {
		if s != nil && redfishIsBIOSEntry(s) {
			return s.Manufacturer, s.ReleaseDate
		}
	}

	return "", ""
}

// redfishBIOS reports the BIOS version, plus vendor and date when the firmware inventory exposes them.
func redfishBIOS(c *gofish.APIClient, sys *schemas.ComputerSystem) map[string]any {
	bios := map[string]any{}
	PutNonZero(bios, "version", sys.BiosVersion)

	vendor, date := redfishBIOSFirmware(c)
	PutNonZero(bios, "vendor", vendor)
	PutNonZero(bios, "date", date)

	return bios
}

// redfishMemoryMiB reports total memory in Mebibytes from the summary, or the sum of the modules.
func redfishMemoryMiB(sys *schemas.ComputerSystem) int {
	if g, ok := Positive(sys.MemorySummary.TotalSystemMemoryGiB); ok {
		return int(g * mebibytesPerGibibyte)
	}

	mods, err := sys.Memory()
	if err != nil {
		return 0
	}

	total := 0
	for _, m := range mods {
		if m == nil {
			continue
		}

		if v, ok := Positive(m.CapacityMiB); ok {
			total += v
		}
	}

	return total
}

// redfishCPUArch maps a Redfish instruction set to the arch string Ironic uses.
func redfishCPUArch(is schemas.InstructionSet) string {
	switch is {
	case schemas.X8664InstructionSet:
		return "x86_64"
	case schemas.ARMA64InstructionSet:
		return "aarch64"
	default:
		return string(is)
	}
}

// redfishCPU reports processor count, arch, and model best effort.
func redfishCPU(sys *schemas.ComputerSystem) map[string]any {
	cpu := map[string]any{}

	if c, ok := Positive(sys.ProcessorSummary.LogicalProcessorCount); ok {
		cpu["count"] = int(c)
	} else if c, ok := Positive(sys.ProcessorSummary.Count); ok {
		cpu["count"] = int(c)
	}

	if procs, err := sys.Processors(); err == nil {
		for _, p := range procs {
			if p == nil {
				continue
			}

			PutNonZero(cpu, "arch", redfishCPUArch(p.InstructionSet))
			PutNonZero(cpu, "model", p.Model)

			break
		}
	}

	return cpu
}

// redfishName returns the friendly name, falling back to the resource id.
func redfishName(name, id string) string {
	return cmp.Or(name, id)
}

// redfishNICs returns one entry per interface that exposes a MAC address.
func redfishNICs(sys *schemas.ComputerSystem) []any {
	ifaces, err := sys.EthernetInterfaces()
	if err != nil {
		return nil
	}

	var out []any

	for _, ni := range ifaces {
		if ni == nil {
			continue
		}

		mac := cmp.Or(ni.MACAddress, ni.PermanentMACAddress)
		if mac == "" {
			continue
		}

		nic := map[string]any{"mac": mac}
		PutNonZero(nic, "name", redfishName(ni.Name, ni.ID))

		out = append(out, nic)
	}

	return out
}

// redfishDriveType maps the Redfish media type and protocol to a metal3api DiskType.
func redfishDriveType(d *schemas.Drive) string {
	if d.Protocol == schemas.NVMeProtocol {
		return "NVME"
	}

	switch d.MediaType {
	case schemas.HDDMediaType, schemas.SMRMediaType:
		return "HDD"
	case schemas.SSDMediaType:
		return "SSD"
	default:
		return ""
	}
}

// redfishDriveWWN returns the drive WWN from its NAA durable name when present.
func redfishDriveWWN(d *schemas.Drive) string {
	for _, id := range d.Identifiers {
		if id.DurableNameFormat == schemas.NAADurableNameFormat && id.DurableName != "" {
			return id.DurableName
		}
	}

	return ""
}

// redfishDrive maps one Redfish drive to a HardwareDetails storage entry.
func redfishDrive(d *schemas.Drive) map[string]any {
	drive := map[string]any{"rotational": d.MediaType == schemas.HDDMediaType || d.MediaType == schemas.SMRMediaType}

	PutNonZero(drive, "name", redfishName(d.Name, d.ID))
	PutNonZero(drive, "type", redfishDriveType(d))
	PutNonZero(drive, "model", d.Model)
	PutNonZero(drive, "vendor", d.Manufacturer)
	PutNonZero(drive, "serialNumber", d.SerialNumber)
	PutNonZero(drive, "wwn", redfishDriveWWN(d))

	if v, ok := Positive(d.CapacityBytes); ok {
		drive["sizeBytes"] = int64(v)
	}

	return drive
}

// redfishSimpleStorage reports devices from the legacy SimpleStorage resource.
func redfishSimpleStorage(sys *schemas.ComputerSystem) []any {
	simples, err := sys.SimpleStorage()
	if err != nil {
		return nil
	}

	var out []any

	for _, s := range simples {
		if s == nil {
			continue
		}

		for _, dev := range s.Devices {
			entry := map[string]any{}
			PutNonZero(entry, "name", dev.Name)
			PutNonZero(entry, "model", dev.Model)
			PutNonZero(entry, "vendor", dev.Manufacturer)

			if v, ok := Positive(dev.CapacityBytes); ok {
				entry["sizeBytes"] = int64(v)
			}

			if len(entry) > 0 {
				out = append(out, entry)
			}
		}
	}

	return out
}

// redfishStorage reports drives from the Storage resources, falling back to SimpleStorage.
func redfishStorage(sys *schemas.ComputerSystem) []any {
	var out []any

	if storages, err := sys.Storage(); err == nil {
		for _, s := range storages {
			if s == nil {
				continue
			}

			drives, derr := s.Drives()
			if derr != nil {
				continue
			}

			for _, d := range drives {
				if d != nil {
					out = append(out, redfishDrive(d))
				}
			}
		}
	}

	if len(out) > 0 {
		return out
	}

	return redfishSimpleStorage(sys)
}

// builtinRedfishInventory collects the requested inspection sections from what Redfish serves, shaped to HardwareDetails.
// The optional fields list selects sections like cpu, nics, storage, and an empty list collects everything.
func builtinRedfishInventory(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var endpoint, user, pass starlark.String
	var insecure starlark.Bool
	var fields *starlark.List

	if err := starlark.UnpackArgs("redfish_inventory", args, kwargs,
		"endpoint", &endpoint, "username", &user, "password", &pass,
		"insecure?", &insecure, "fields?", &fields); err != nil {
		return starlark.None, err
	}

	want := redfishFieldSelector(fields)
	inv := map[string]any{}

	err := redfishWithClient(contextFromThread(thread), string(endpoint), string(user), string(pass), bool(insecure), func(c *gofish.APIClient) error {
		sys, serr := redfishFirstSystem(c)
		if serr != nil {
			return serr
		}

		if want("systemVendor") {
			if vendor := redfishSystemVendor(sys); len(vendor) > 0 {
				inv["systemVendor"] = vendor
			}
		}

		if want("firmware") {
			if bios := redfishBIOS(c, sys); len(bios) > 0 {
				inv["firmware"] = map[string]any{"bios": bios}
			}
		}

		if want("ramMebibytes") {
			if mib := redfishMemoryMiB(sys); mib > 0 {
				inv["ramMebibytes"] = mib
			}
		}

		if want("cpu") {
			if cpu := redfishCPU(sys); len(cpu) > 0 {
				inv["cpu"] = cpu
			}
		}

		if want("nics") {
			if nics := redfishNICs(sys); len(nics) > 0 {
				inv["nics"] = nics
			}
		}

		if want("storage") {
			if storage := redfishStorage(sys); len(storage) > 0 {
				inv["storage"] = storage
			}
		}

		return nil
	})
	if err != nil {
		return starlark.None, fmt.Errorf("redfish_inventory: %w", err)
	}

	return GoToStarlark(inv), nil
}
