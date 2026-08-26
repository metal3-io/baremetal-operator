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
	"net/http"
	"slices"
	"strings"
	"time"

	gofish "github.com/stmcginnis/gofish"
	"github.com/stmcginnis/gofish/schemas"
	"go.starlark.net/starlark"
)

// RedfishTimeout bounds a single Redfish request over the network.
const RedfishTimeout = 30 * time.Second

// RedfishTaskTimeout bounds waiting for an async Redfish task to finish.
const RedfishTaskTimeout = 5 * time.Minute

// RedfishPollRate is how often an async Redfish task is polled.
const RedfishPollRate = 2 * time.Second

// RedfishBootTargets maps script boot target names to Redfish boot sources.
var RedfishBootTargets = map[string]schemas.BootSource{
	"none":  schemas.NoneBootSource,
	"pxe":   schemas.PxeBootSource,
	"disk":  schemas.HddBootSource,
	"cd":    schemas.CdBootSource,
	"cdrom": schemas.CdBootSource,
	"bios":  schemas.BiosSetupBootSource,
}

// RedfishConnArgs holds the connection arguments every Redfish builtin accepts.
type RedfishConnArgs struct {
	Endpoint starlark.String
	Username starlark.String
	Password starlark.String
	Insecure starlark.Bool
}

// ConnPairs returns the three required connection pairs.
func (c *RedfishConnArgs) ConnPairs() []any {
	return []any{"endpoint", &c.Endpoint, "username", &c.Username, "password", &c.Password}
}

// Unpack reads endpoint, username, password, then extra, then the insecure flag.
// This is the argument order of every builtin that takes its own arguments.
func (c *RedfishConnArgs) Unpack(name string, args starlark.Tuple, kwargs []starlark.Tuple, extra ...any) error {
	pairs := append(c.ConnPairs(), extra...)
	pairs = append(pairs, "insecure?", &c.Insecure)

	return starlark.UnpackArgs(name, args, kwargs, pairs...)
}

// UnpackInsecureFirst reads endpoint, username, password, the insecure flag, then extra.
// Only redfish_inventory uses this order because all of its own arguments are optional.
func (c *RedfishConnArgs) UnpackInsecureFirst(name string, args starlark.Tuple, kwargs []starlark.Tuple, extra ...any) error {
	pairs := append(c.ConnPairs(), "insecure?", &c.Insecure)
	pairs = append(pairs, extra...)

	return starlark.UnpackArgs(name, args, kwargs, pairs...)
}

// Strings returns the connection arguments as plain Go values.
func (c *RedfishConnArgs) Strings() (endpoint, user, pass string, insecure bool) {
	return string(c.Endpoint), string(c.Username), string(c.Password), bool(c.Insecure)
}

// RedfishConn unpacks endpoint, username, password, and the insecure flag.
func RedfishConn(name string, args starlark.Tuple, kwargs []starlark.Tuple) (endpoint, user, pass string, insecure bool, err error) {
	var conn RedfishConnArgs
	if uerr := conn.Unpack(name, args, kwargs); uerr != nil {
		return "", "", "", false, uerr
	}

	endpoint, user, pass, insecure = conn.Strings()

	return endpoint, user, pass, insecure, nil
}

// RedfishWithClient opens a Redfish session and runs fn, logging out afterward.
func RedfishWithClient(ctx context.Context, endpoint, user, pass string, insecure bool, fn func(*gofish.APIClient) error) error {
	// ConnectContext threads the caller ctx into every request so a canceled
	// reconcile aborts the call. The transport carries the insecure flag.
	client, err := gofish.ConnectContext(ctx, gofish.ClientConfig{
		Endpoint:  endpoint,
		Username:  user,
		Password:  pass,
		Insecure:  insecure,
		BasicAuth: true,
		HTTPClient: &http.Client{
			Timeout: RedfishTimeout,
			Transport: &http.Transport{
				Proxy:           http.ProxyFromEnvironment,
				TLSClientConfig: TLSConfig(insecure),
			},
		},
	})
	if err != nil {
		return err
	}
	defer client.Logout()

	return fn(client)
}

// RedfishWaitTask blocks until an async Redfish task finishes. A nil info means
// the action already completed synchronously.
func RedfishWaitTask(ctx context.Context, client *gofish.APIClient, info *schemas.TaskMonitorInfo) error {
	if info == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, RedfishTaskTimeout)
	defer cancel()

	resp, err := schemas.WaitForTaskMonitor(ctx, client, RedfishPollRate, info, nil)
	if resp != nil {
		_ = resp.Body.Close()
	}

	return err
}

// RedfishFirstSystem returns the first computer system exposed by the service.
func RedfishFirstSystem(client *gofish.APIClient) (*schemas.ComputerSystem, error) {
	systems, err := client.Service.Systems()
	if err != nil {
		return nil, err
	}

	if len(systems) == 0 {
		return nil, errors.New("no computer system found")
	}

	return systems[0], nil
}

// RedfishWithSystem opens a session, resolves the first system, and runs fn.
func RedfishWithSystem(ctx context.Context, name string, args starlark.Tuple, kwargs []starlark.Tuple, fn func(*schemas.ComputerSystem) error) error {
	endpoint, user, pass, insecure, err := RedfishConn(name, args, kwargs)
	if err != nil {
		return err
	}

	return RedfishWithClient(ctx, endpoint, user, pass, insecure, func(c *gofish.APIClient) error {
		sys, serr := RedfishFirstSystem(c)
		if serr != nil {
			return serr
		}

		return fn(sys)
	})
}

// RedfishPowerBuiltin builds a starlark call name(endpoint, username, password, insecure)
// that applies one reset action over Redfish.
func RedfishPowerBuiltin(name string, reset schemas.ResetType) *starlark.Builtin {
	return starlark.NewBuiltin(name, func(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		endpoint, user, pass, insecure, err := RedfishConn(name, args, kwargs)
		if err != nil {
			return starlark.None, err
		}

		ctx := ContextFromThread(thread)
		err = RedfishWithClient(ctx, endpoint, user, pass, insecure, func(c *gofish.APIClient) error {
			sys, serr := RedfishFirstSystem(c)
			if serr != nil {
				return serr
			}

			info, rerr := sys.Reset(reset)
			if rerr != nil {
				return rerr
			}

			return RedfishWaitTask(ctx, c, info)
		})
		if err != nil {
			return starlark.None, fmt.Errorf("%s: %w", name, err)
		}

		return starlark.None, nil
	})
}

// BuiltinRedfishPowerStatus reports system power state over Redfish.
// Starlark call is redfish_power_status(endpoint, username, password, insecure).
func BuiltinRedfishPowerStatus(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var state schemas.PowerState
	err := RedfishWithSystem(ContextFromThread(thread), "redfish_power_status", args, kwargs, func(sys *schemas.ComputerSystem) error {
		state = sys.PowerState
		return nil
	})
	if err != nil {
		return starlark.None, fmt.Errorf("redfish_power_status: %w", err)
	}

	out := starlark.NewDict(0)
	_ = out.SetKey(starlark.String("power_on"), starlark.Bool(state == schemas.OnPowerState))
	_ = out.SetKey(starlark.String("power_state"), starlark.String(string(state)))

	return out, nil
}

// BuiltinRedfishSetBoot sets the boot source override over Redfish. Starlark call
// is redfish_set_boot(endpoint, username, password, target, persistent, efi, insecure).
func BuiltinRedfishSetBoot(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var conn RedfishConnArgs
	var target starlark.String
	var persistent, efi starlark.Bool

	if err := conn.Unpack("redfish_set_boot", args, kwargs,
		"target", &target, "persistent?", &persistent, "efi?", &efi); err != nil {
		return starlark.None, err
	}

	src, ok := RedfishBootTargets[strings.ToLower(string(target))]
	if !ok {
		return starlark.None, fmt.Errorf("redfish_set_boot: unknown target %q", string(target))
	}

	enabled := schemas.OnceBootSourceOverrideEnabled
	if bool(persistent) {
		enabled = schemas.ContinuousBootSourceOverrideEnabled
	}

	mode := schemas.LegacyBootSourceOverrideMode
	if bool(efi) {
		mode = schemas.UEFIBootSourceOverrideMode
	}

	endpoint, user, pass, insecure := conn.Strings()

	err := RedfishWithClient(ContextFromThread(thread), endpoint, user, pass, insecure, func(c *gofish.APIClient) error {
		sys, serr := RedfishFirstSystem(c)
		if serr != nil {
			return serr
		}

		return sys.SetBoot(&schemas.Boot{
			BootSourceOverrideTarget:  src,
			BootSourceOverrideEnabled: enabled,
			BootSourceOverrideMode:    mode,
		})
	})
	if err != nil {
		return starlark.None, fmt.Errorf("redfish_set_boot: %w", err)
	}

	return starlark.None, nil
}

// BuiltinRedfishGetBoot reports the boot source override over Redfish.
// Starlark call is redfish_get_boot(endpoint, username, password, insecure).
func BuiltinRedfishGetBoot(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var boot schemas.Boot
	err := RedfishWithSystem(ContextFromThread(thread), "redfish_get_boot", args, kwargs, func(sys *schemas.ComputerSystem) error {
		boot = sys.Boot
		return nil
	})
	if err != nil {
		return starlark.None, fmt.Errorf("redfish_get_boot: %w", err)
	}

	out := starlark.NewDict(0)
	_ = out.SetKey(starlark.String("target"), starlark.String(string(boot.BootSourceOverrideTarget)))
	_ = out.SetKey(starlark.String("enabled"), starlark.String(string(boot.BootSourceOverrideEnabled)))
	_ = out.SetKey(starlark.String("mode"), starlark.String(string(boot.BootSourceOverrideMode)))

	return out, nil
}

// RedfishVirtualMedia collects virtual media from the managers and the system.
// When nothing is found the underlying lookup errors are surfaced together.
func RedfishVirtualMedia(client *gofish.APIClient) ([]*schemas.VirtualMedia, error) {
	var media []*schemas.VirtualMedia
	var errs []error

	if managers, err := client.Service.Managers(); err != nil {
		errs = append(errs, err)
	} else {
		for _, m := range managers {
			if vm, verr := m.VirtualMedia(); verr != nil {
				errs = append(errs, verr)
			} else {
				media = append(media, vm...)
			}
		}
	}

	if len(media) == 0 {
		if sys, serr := RedfishFirstSystem(client); serr != nil {
			errs = append(errs, serr)
		} else if vm, verr := sys.VirtualMedia(); verr != nil {
			errs = append(errs, verr)
		} else {
			media = append(media, vm...)
		}
	}

	if len(media) == 0 {
		if len(errs) > 0 {
			return nil, fmt.Errorf("no virtual media found: %w", errors.Join(errs...))
		}

		return nil, errors.New("no virtual media found")
	}

	return media, nil
}

// RedfishMediaIsCD reports whether a media slot accepts CD or DVD images.
func RedfishMediaIsCD(m *schemas.VirtualMedia) bool {
	return slices.ContainsFunc(m.MediaTypes, func(t schemas.VirtualMediaType) bool {
		return t == schemas.CDVirtualMediaType || t == schemas.DVDVirtualMediaType
	})
}

// RedfishInsertableMedia prefers a free CD or DVD slot, then any free slot, then
// the first slot.
func RedfishInsertableMedia(media []*schemas.VirtualMedia) *schemas.VirtualMedia {
	free := func(m *schemas.VirtualMedia) bool { return !DerefOr(m.Inserted, false) }

	for _, m := range media {
		if free(m) && RedfishMediaIsCD(m) {
			return m
		}
	}

	for _, m := range media {
		if free(m) {
			return m
		}
	}

	return media[0]
}

// RedfishInsertedMedia returns the media slot that currently holds an image, or
// nil when nothing is inserted.
func RedfishInsertedMedia(media []*schemas.VirtualMedia) *schemas.VirtualMedia {
	for _, m := range media {
		if DerefOr(m.Inserted, false) {
			return m
		}
	}

	return nil
}

// BuiltinRedfishInsertMedia attaches an image to virtual media over Redfish. Call
// is redfish_insert_media(endpoint, username, password, image, insecure).
func BuiltinRedfishInsertMedia(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var conn RedfishConnArgs
	var image starlark.String

	if err := conn.Unpack("redfish_insert_media", args, kwargs, "image", &image); err != nil {
		return starlark.None, err
	}

	ctx := ContextFromThread(thread)
	endpoint, user, pass, insecure := conn.Strings()

	err := RedfishWithClient(ctx, endpoint, user, pass, insecure, func(c *gofish.APIClient) error {
		media, merr := RedfishVirtualMedia(c)
		if merr != nil {
			return merr
		}

		info, ierr := RedfishInsertableMedia(media).InsertMedia(&schemas.VirtualMediaInsertMediaParameters{
			Image:    string(image),
			Inserted: new(true),
		})
		if ierr != nil {
			return ierr
		}

		return RedfishWaitTask(ctx, c, info)
	})
	if err != nil {
		return starlark.None, fmt.Errorf("redfish_insert_media: %w", err)
	}

	return starlark.None, nil
}

// BuiltinRedfishEjectMedia detaches the mounted virtual media over Redfish.
// Starlark call is redfish_eject_media(endpoint, username, password, insecure).
func BuiltinRedfishEjectMedia(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	endpoint, user, pass, insecure, err := RedfishConn("redfish_eject_media", args, kwargs)
	if err != nil {
		return starlark.None, err
	}

	ctx := ContextFromThread(thread)
	err = RedfishWithClient(ctx, endpoint, user, pass, insecure, func(c *gofish.APIClient) error {
		media, merr := RedfishVirtualMedia(c)
		if merr != nil {
			return merr
		}

		m := RedfishInsertedMedia(media)
		if m == nil {
			return nil
		}

		info, eerr := m.EjectMedia()
		if eerr != nil {
			return eerr
		}

		return RedfishWaitTask(ctx, c, info)
	})
	if err != nil {
		return starlark.None, fmt.Errorf("redfish_eject_media: %w", err)
	}

	return starlark.None, nil
}

// BuiltinRedfishMediaStatus reports virtual media state over Redfish. Starlark
// call is redfish_media_status(endpoint, username, password, insecure).
func BuiltinRedfishMediaStatus(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	endpoint, user, pass, insecure, err := RedfishConn("redfish_media_status", args, kwargs)
	if err != nil {
		return starlark.None, err
	}

	var inserted bool
	var image string
	err = RedfishWithClient(ContextFromThread(thread), endpoint, user, pass, insecure, func(c *gofish.APIClient) error {
		media, merr := RedfishVirtualMedia(c)
		if merr != nil {
			return merr
		}

		if m := RedfishInsertedMedia(media); m != nil {
			inserted = true
			image = m.Image
		}

		return nil
	})
	if err != nil {
		return starlark.None, fmt.Errorf("redfish_media_status: %w", err)
	}

	out := starlark.NewDict(0)
	_ = out.SetKey(starlark.String("inserted"), starlark.Bool(inserted))
	_ = out.SetKey(starlark.String("image"), starlark.String(image))

	return out, nil
}

// BuiltinRedfishGetSystem reports system vendor fields over Redfish. Starlark
// call is redfish_get_system(endpoint, username, password, insecure).
func BuiltinRedfishGetSystem(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var manufacturer, model, serial string
	err := RedfishWithSystem(ContextFromThread(thread), "redfish_get_system", args, kwargs, func(sys *schemas.ComputerSystem) error {
		manufacturer = sys.Manufacturer
		model = sys.Model
		serial = sys.SerialNumber
		return nil
	})
	if err != nil {
		return starlark.None, fmt.Errorf("redfish_get_system: %w", err)
	}

	out := starlark.NewDict(0)
	_ = out.SetKey(starlark.String("manufacturer"), starlark.String(manufacturer))
	_ = out.SetKey(starlark.String("model"), starlark.String(model))
	_ = out.SetKey(starlark.String("serialNumber"), starlark.String(serial))

	return out, nil
}

// BuiltinRedfishIsHealthy reports whether the system health rolls up to OK.
// Starlark call is redfish_is_healthy(endpoint, username, password, insecure).
func BuiltinRedfishIsHealthy(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var health schemas.Health
	err := RedfishWithSystem(ContextFromThread(thread), "redfish_is_healthy", args, kwargs, func(sys *schemas.ComputerSystem) error {
		health = sys.Status.Health
		return nil
	})
	if err != nil {
		return starlark.None, fmt.Errorf("redfish_is_healthy: %w", err)
	}

	// An empty health rollup means the BMC reported no data, so surface an error.
	if health == "" {
		return starlark.None, errors.New("redfish_is_healthy: no health data")
	}

	return starlark.Bool(health == schemas.OKHealth), nil
}

// BuiltinRedfishGetFirmware reports the system BIOS version over Redfish.
// Starlark call is redfish_get_firmware(endpoint, username, password, insecure).
func BuiltinRedfishGetFirmware(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var bios string
	err := RedfishWithSystem(ContextFromThread(thread), "redfish_get_firmware", args, kwargs, func(sys *schemas.ComputerSystem) error {
		bios = sys.BiosVersion
		return nil
	})
	if err != nil {
		return starlark.None, fmt.Errorf("redfish_get_firmware: %w", err)
	}

	out := starlark.NewDict(0)
	_ = out.SetKey(starlark.String("bios_version"), starlark.String(bios))

	return out, nil
}

// RedfishBuiltins exposes the Redfish power, boot, media, and info helpers.
func RedfishBuiltins() starlark.StringDict {
	return starlark.StringDict{
		"redfish_power_on":     RedfishPowerBuiltin("redfish_power_on", schemas.OnResetType),
		"redfish_power_off":    RedfishPowerBuiltin("redfish_power_off", schemas.ForceOffResetType),
		"redfish_power_soft":   RedfishPowerBuiltin("redfish_power_soft", schemas.GracefulShutdownResetType),
		"redfish_power_cycle":  RedfishPowerBuiltin("redfish_power_cycle", schemas.PowerCycleResetType),
		"redfish_power_reset":  RedfishPowerBuiltin("redfish_power_reset", schemas.ForceRestartResetType),
		"redfish_power_status": starlark.NewBuiltin("redfish_power_status", BuiltinRedfishPowerStatus),
		"redfish_set_boot":     starlark.NewBuiltin("redfish_set_boot", BuiltinRedfishSetBoot),
		"redfish_get_boot":     starlark.NewBuiltin("redfish_get_boot", BuiltinRedfishGetBoot),
		"redfish_insert_media": starlark.NewBuiltin("redfish_insert_media", BuiltinRedfishInsertMedia),
		"redfish_eject_media":  starlark.NewBuiltin("redfish_eject_media", BuiltinRedfishEjectMedia),
		"redfish_media_status": starlark.NewBuiltin("redfish_media_status", BuiltinRedfishMediaStatus),
		"redfish_get_system":   starlark.NewBuiltin("redfish_get_system", BuiltinRedfishGetSystem),
		"redfish_is_healthy":   starlark.NewBuiltin("redfish_is_healthy", BuiltinRedfishIsHealthy),
		"redfish_get_firmware": starlark.NewBuiltin("redfish_get_firmware", BuiltinRedfishGetFirmware),
		"redfish_inventory":    starlark.NewBuiltin("redfish_inventory", BuiltinRedfishInventory),
	}
}
