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
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	goipmi "github.com/bougou/go-ipmi"
	"go.starlark.net/starlark"
)

// ipmiTimeout bounds a full connect and command cycle to the BMC.
const ipmiTimeout = 30 * time.Second

// ipmiCloseTimeout bounds the detached session teardown so shutdown is not held up by a hung BMC.
const ipmiCloseTimeout = 5 * time.Second

// ipmiArgc is the fixed connection arg count host, port, username, password.
const ipmiArgc = 4

// ipmiBootDevices maps script boot device names to IPMI selectors.
var ipmiBootDevices = map[string]goipmi.BootDeviceSelector{
	"none":   goipmi.BootDeviceSelectorNoOverride,
	"pxe":    goipmi.BootDeviceSelectorForcePXE,
	"disk":   goipmi.BootDeviceSelectorForceHardDrive,
	"safe":   goipmi.BootDeviceSelectorForceHardDriveSafe,
	"diag":   goipmi.BootDeviceSelectorForceDiagnosticPartition,
	"cdrom":  goipmi.BootDeviceSelectorForceCDROM,
	"bios":   goipmi.BootDeviceSelectorForceBIOSSetup,
	"floppy": goipmi.BootDeviceSelectorForceFloppy,
}

// ipmiWithClient opens a session to the BMC and runs fn, closing afterward.
// The caller ctx bounds the whole cycle so a canceled reconcile aborts it.
func ipmiWithClient(ctx context.Context, host string, port int, user, pass string, fn func(context.Context, *goipmi.Client) error) error {
	client, err := goipmi.NewClient(host, port, user, pass)
	if err != nil {
		return err
	}

	callCtx, cancel := context.WithTimeout(ctx, ipmiTimeout)
	defer cancel()

	if err := client.Connect(callCtx); err != nil {
		return err
	}
	// Close on a deadline detached from callCtx so a cancelled or expired call cannot skip socket teardown.
	defer func() {
		closeCtx, closeCancel := context.WithTimeout(context.WithoutCancel(ctx), ipmiCloseTimeout)
		defer closeCancel()

		_ = client.Close(closeCtx)
	}()

	return fn(callCtx, client)
}

// ipmiPort converts a starlark int port to a Go int.
func ipmiPort(name string, port starlark.Int) (int, error) {
	p, ok := port.Int64()
	if !ok {
		return 0, fmt.Errorf("%s: port out of range", name)
	}

	return int(p), nil
}

// ipmiConn unpacks the fixed host, port, username, password connection args.
func ipmiConn(name string, args starlark.Tuple) (host string, port int, user, pass string, err error) {
	var h, u, p starlark.String
	var portArg starlark.Int

	if uerr := starlark.UnpackPositionalArgs(name, args, nil, ipmiArgc, &h, &portArg, &u, &p); uerr != nil {
		return "", 0, "", "", uerr
	}

	port, err = ipmiPort(name, portArg)
	if err != nil {
		return "", 0, "", "", err
	}

	return string(h), port, string(u), string(p), nil
}

// ipmiBootDeviceName maps an IPMI selector back to a script boot device name.
func ipmiBootDeviceName(sel goipmi.BootDeviceSelector) string {
	for name, s := range ipmiBootDevices {
		if s == sel {
			return name
		}
	}

	return sel.String()
}

// ipmiFRUVendor extracts manufacturer, product name, and serial from a FRU.
// It prefers the product info area and falls back to the board info area.
func ipmiFRUVendor(fru *goipmi.FRU) (manufacturer, product, serial string) {
	if fru == nil {
		return "", "", ""
	}

	if p := fru.ProductInfoArea; p != nil {
		manufacturer = strings.TrimSpace(string(p.Manufacturer))
		product = strings.TrimSpace(string(p.Name))
		serial = strings.TrimSpace(string(p.SerialNumber))
	}

	if b := fru.BoardInfoArea; b != nil {
		if manufacturer == "" {
			manufacturer = strings.TrimSpace(string(b.Manufacturer))
		}
		if product == "" {
			product = strings.TrimSpace(string(b.ProductName))
		}
		if serial == "" {
			serial = strings.TrimSpace(string(b.SerialNumber))
		}
	}

	return manufacturer, product, serial
}

// ipmiSensorCritical reports whether a sensor threshold status is critical.
func ipmiSensorCritical(status goipmi.SensorThresholdStatus) bool {
	switch status {
	case goipmi.SensorThresholdStatus_LCR, goipmi.SensorThresholdStatus_LNR,
		goipmi.SensorThresholdStatus_UCR, goipmi.SensorThresholdStatus_UNR:
		return true
	default:
		return false
	}
}

// ipmiPowerBuiltin builds a starlark call name(host, port, username, password)
// that applies one chassis control action over IPMI LAN.
func ipmiPowerBuiltin(name string, control goipmi.ChassisControl) *starlark.Builtin {
	return starlark.NewBuiltin(name, func(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
		host, port, user, pass, err := ipmiConn(name, args)
		if err != nil {
			return starlark.None, err
		}

		err = ipmiWithClient(contextFromThread(thread), host, port, user, pass, func(ctx context.Context, c *goipmi.Client) error {
			_, cerr := c.ChassisControl(ctx, control)
			return cerr
		})
		if err != nil {
			return starlark.None, fmt.Errorf("%s: %w", name, err)
		}

		return starlark.None, nil
	})
}

// builtinIPMIPowerStatus reports BMC chassis power state over IPMI LAN.
// Starlark call is ipmi_power_status(host, port, username, password).
func builtinIPMIPowerStatus(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	host, port, user, pass, err := ipmiConn("ipmi_power_status", args)
	if err != nil {
		return starlark.None, err
	}

	var resp *goipmi.GetChassisStatusResponse
	err = ipmiWithClient(contextFromThread(thread), host, port, user, pass, func(ctx context.Context, c *goipmi.Client) error {
		r, rerr := c.GetChassisStatus(ctx)
		resp = r
		return rerr
	})
	if err != nil {
		return starlark.None, fmt.Errorf("ipmi_power_status: %w", err)
	}

	out := starlark.NewDict(0)
	_ = out.SetKey(starlark.String("power_on"), starlark.Bool(resp.PowerIsOn))
	_ = out.SetKey(starlark.String("power_fault"), starlark.Bool(resp.PowerFault))
	_ = out.SetKey(starlark.String("power_overload"), starlark.Bool(resp.PowerOverload))

	return out, nil
}

// builtinIPMISetBootDevice sets the next boot device over IPMI LAN. Starlark call
// is ipmi_set_boot_device(host, port, username, password, device, persistent, efi).
func builtinIPMISetBootDevice(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var host, user, pass, device starlark.String
	var port starlark.Int
	var persistent, efi starlark.Bool

	if err := starlark.UnpackArgs("ipmi_set_boot_device", args, kwargs,
		"host", &host, "port", &port, "username", &user, "password", &pass,
		"device", &device, "persistent?", &persistent, "efi?", &efi); err != nil {
		return starlark.None, err
	}

	selector, ok := ipmiBootDevices[strings.ToLower(string(device))]
	if !ok {
		return starlark.None, fmt.Errorf("ipmi_set_boot_device: unknown device %q", string(device))
	}

	portInt, err := ipmiPort("ipmi_set_boot_device", port)
	if err != nil {
		return starlark.None, err
	}

	bootType := goipmi.BIOSBootTypeLegacy
	if bool(efi) {
		bootType = goipmi.BIOSBootTypeEFI
	}

	err = ipmiWithClient(contextFromThread(thread), string(host), portInt, string(user), string(pass), func(ctx context.Context, c *goipmi.Client) error {
		return c.SetBootDevice(ctx, selector, bootType, bool(persistent))
	})
	if err != nil {
		return starlark.None, fmt.Errorf("ipmi_set_boot_device: %w", err)
	}

	return starlark.None, nil
}

// builtinIPMIGetBootDevice reports the configured boot device over IPMI LAN.
// Starlark call is ipmi_get_boot_device(host, port, username, password).
func builtinIPMIGetBootDevice(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	host, port, user, pass, err := ipmiConn("ipmi_get_boot_device", args)
	if err != nil {
		return starlark.None, err
	}

	var params *goipmi.BootOptionsParams
	err = ipmiWithClient(contextFromThread(thread), host, port, user, pass, func(ctx context.Context, c *goipmi.Client) error {
		p, perr := c.GetSystemBootOptionsParams(ctx)
		params = p
		return perr
	})
	if err != nil {
		return starlark.None, fmt.Errorf("ipmi_get_boot_device: %w", err)
	}

	device, persistent, efi := "none", false, false
	if params != nil && params.BootFlags != nil {
		device = ipmiBootDeviceName(params.BootFlags.BootDeviceSelector)
		persistent = params.BootFlags.Persist
		efi = params.BootFlags.BIOSBootType == goipmi.BIOSBootTypeEFI
	}

	out := starlark.NewDict(0)
	_ = out.SetKey(starlark.String("device"), starlark.String(device))
	_ = out.SetKey(starlark.String("persistent"), starlark.Bool(persistent))
	_ = out.SetKey(starlark.String("efi"), starlark.Bool(efi))

	return out, nil
}

// builtinIPMIGetFRU reports board vendor fields over IPMI LAN. Starlark call
// is ipmi_get_fru(host, port, username, password).
func builtinIPMIGetFRU(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	host, port, user, pass, err := ipmiConn("ipmi_get_fru", args)
	if err != nil {
		return starlark.None, err
	}

	var fru *goipmi.FRU
	err = ipmiWithClient(contextFromThread(thread), host, port, user, pass, func(ctx context.Context, c *goipmi.Client) error {
		f, ferr := c.GetFRU(ctx, 0, "")
		fru = f
		return ferr
	})
	if err != nil {
		return starlark.None, fmt.Errorf("ipmi_get_fru: %w", err)
	}

	manufacturer, product, serial := ipmiFRUVendor(fru)

	out := starlark.NewDict(0)
	_ = out.SetKey(starlark.String("manufacturer"), starlark.String(manufacturer))
	_ = out.SetKey(starlark.String("productName"), starlark.String(product))
	_ = out.SetKey(starlark.String("serialNumber"), starlark.String(serial))

	return out, nil
}

// builtinIPMIIsHealthy reports whether all BMC sensors are within safe thresholds.
// Starlark call is ipmi_is_healthy(host, port, username, password).
func builtinIPMIIsHealthy(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	host, port, user, pass, err := ipmiConn("ipmi_is_healthy", args)
	if err != nil {
		return starlark.None, err
	}

	var sensors []*goipmi.Sensor
	err = ipmiWithClient(contextFromThread(thread), host, port, user, pass, func(ctx context.Context, c *goipmi.Client) error {
		s, serr := c.GetSensors(ctx)
		sensors = s
		return serr
	})
	if err != nil {
		return starlark.None, fmt.Errorf("ipmi_is_healthy: %w", err)
	}

	// No sensors means no data to judge health, so surface an error to the script.
	if len(sensors) == 0 {
		return starlark.None, errors.New("ipmi_is_healthy: no sensor data")
	}

	healthy := true
	for _, s := range sensors {
		if ipmiSensorCritical(s.Threshold.ThresholdStatus) {
			healthy = false
			break
		}
	}

	return starlark.Bool(healthy), nil
}

// builtinIPMIGetDeviceID reports BMC firmware version and GUID over IPMI LAN.
// Starlark call is ipmi_get_device_id(host, port, username, password).
func builtinIPMIGetDeviceID(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	host, port, user, pass, err := ipmiConn("ipmi_get_device_id", args)
	if err != nil {
		return starlark.None, err
	}

	var firmware, guid string
	err = ipmiWithClient(contextFromThread(thread), host, port, user, pass, func(ctx context.Context, c *goipmi.Client) error {
		id, ierr := c.GetDeviceID(ctx)
		if ierr != nil {
			return ierr
		}
		firmware = id.FirmwareVersionStr()

		g, gerr := c.GetDeviceGUID(ctx)
		if gerr != nil {
			return gerr
		}
		guid = g.Format()

		return nil
	})
	if err != nil {
		return starlark.None, fmt.Errorf("ipmi_get_device_id: %w", err)
	}

	out := starlark.NewDict(0)
	_ = out.SetKey(starlark.String("firmware_version"), starlark.String(firmware))
	_ = out.SetKey(starlark.String("guid"), starlark.String(guid))

	return out, nil
}

// ipmiBuiltins exposes the IPMI power, boot, and info helpers to scripts.
func ipmiBuiltins() starlark.StringDict {
	return starlark.StringDict{
		"ipmi_power_on":        ipmiPowerBuiltin("ipmi_power_on", goipmi.ChassisControlPowerUp),
		"ipmi_power_off":       ipmiPowerBuiltin("ipmi_power_off", goipmi.ChassisControlPowerDown),
		"ipmi_power_cycle":     ipmiPowerBuiltin("ipmi_power_cycle", goipmi.ChassisControlPowerCycle),
		"ipmi_power_reset":     ipmiPowerBuiltin("ipmi_power_reset", goipmi.ChassisControlHardReset),
		"ipmi_power_soft":      ipmiPowerBuiltin("ipmi_power_soft", goipmi.ChassisControlSoftShutdown),
		"ipmi_power_status":    starlark.NewBuiltin("ipmi_power_status", builtinIPMIPowerStatus),
		"ipmi_set_boot_device": starlark.NewBuiltin("ipmi_set_boot_device", builtinIPMISetBootDevice),
		"ipmi_get_boot_device": starlark.NewBuiltin("ipmi_get_boot_device", builtinIPMIGetBootDevice),
		"ipmi_get_fru":         starlark.NewBuiltin("ipmi_get_fru", builtinIPMIGetFRU),
		"ipmi_is_healthy":      starlark.NewBuiltin("ipmi_is_healthy", builtinIPMIIsHealthy),
		"ipmi_get_device_id":   starlark.NewBuiltin("ipmi_get_device_id", builtinIPMIGetDeviceID),
	}
}
