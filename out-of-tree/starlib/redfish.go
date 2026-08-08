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
	"crypto/tls"
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

// redfishTimeout bounds a single Redfish request over the network.
const redfishTimeout = 30 * time.Second

// redfishTaskTimeout bounds waiting for an async Redfish task to finish.
const redfishTaskTimeout = 5 * time.Minute

// redfishPollRate is how often an async Redfish task is polled.
const redfishPollRate = 2 * time.Second

// redfishBootTargets maps script boot target names to Redfish boot sources.
var redfishBootTargets = map[string]schemas.BootSource{
	"none":  schemas.NoneBootSource,
	"pxe":   schemas.PxeBootSource,
	"disk":  schemas.HddBootSource,
	"cd":    schemas.CdBootSource,
	"cdrom": schemas.CdBootSource,
	"bios":  schemas.BiosSetupBootSource,
}

// redfishConn unpacks endpoint, username, password, and the insecure flag.
func redfishConn(name string, args starlark.Tuple, kwargs []starlark.Tuple) (endpoint, user, pass string, insecure bool, err error) {
	var e, u, p starlark.String
	var ins starlark.Bool

	if uerr := starlark.UnpackArgs(name, args, kwargs, "endpoint", &e, "username", &u, "password", &p, "insecure?", &ins); uerr != nil {
		return "", "", "", false, uerr
	}

	return string(e), string(u), string(p), bool(ins), nil
}

// redfishWithClient opens a Redfish session and runs fn, logging out afterward.
func redfishWithClient(ctx context.Context, endpoint, user, pass string, insecure bool, fn func(*gofish.APIClient) error) error {
	// ConnectContext threads the caller ctx into every request so a canceled
	// reconcile aborts the call. The transport carries the insecure flag.
	client, err := gofish.ConnectContext(ctx, gofish.ClientConfig{
		Endpoint:  endpoint,
		Username:  user,
		Password:  pass,
		Insecure:  insecure,
		BasicAuth: true,
		HTTPClient: &http.Client{
			Timeout: redfishTimeout,
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment,
				TLSClientConfig: &tls.Config{
					MinVersion:         tls.VersionTLS12,
					InsecureSkipVerify: insecure,
				},
			},
		},
	})
	if err != nil {
		return err
	}
	defer client.Logout()

	return fn(client)
}

// redfishWaitTask blocks until an async Redfish task finishes. A nil info means
// the action already completed synchronously.
func redfishWaitTask(ctx context.Context, client *gofish.APIClient, info *schemas.TaskMonitorInfo) error {
	if info == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, redfishTaskTimeout)
	defer cancel()

	resp, err := schemas.WaitForTaskMonitor(ctx, client, redfishPollRate, info, nil)
	if resp != nil {
		_ = resp.Body.Close()
	}

	return err
}

// redfishFirstSystem returns the first computer system exposed by the service.
func redfishFirstSystem(client *gofish.APIClient) (*schemas.ComputerSystem, error) {
	systems, err := client.Service.Systems()
	if err != nil {
		return nil, err
	}

	if len(systems) == 0 {
		return nil, errors.New("no computer system found")
	}

	return systems[0], nil
}

// redfishWithSystem opens a session, resolves the first system, and runs fn.
func redfishWithSystem(ctx context.Context, name string, args starlark.Tuple, kwargs []starlark.Tuple, fn func(*schemas.ComputerSystem) error) error {
	endpoint, user, pass, insecure, err := redfishConn(name, args, kwargs)
	if err != nil {
		return err
	}

	return redfishWithClient(ctx, endpoint, user, pass, insecure, func(c *gofish.APIClient) error {
		sys, serr := redfishFirstSystem(c)
		if serr != nil {
			return serr
		}

		return fn(sys)
	})
}

// redfishPowerBuiltin builds a starlark call name(endpoint, username, password, insecure)
// that applies one reset action over Redfish.
func redfishPowerBuiltin(name string, reset schemas.ResetType) *starlark.Builtin {
	return starlark.NewBuiltin(name, func(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		endpoint, user, pass, insecure, err := redfishConn(name, args, kwargs)
		if err != nil {
			return starlark.None, err
		}

		ctx := contextFromThread(thread)
		err = redfishWithClient(ctx, endpoint, user, pass, insecure, func(c *gofish.APIClient) error {
			sys, serr := redfishFirstSystem(c)
			if serr != nil {
				return serr
			}

			info, rerr := sys.Reset(reset)
			if rerr != nil {
				return rerr
			}

			return redfishWaitTask(ctx, c, info)
		})
		if err != nil {
			return starlark.None, fmt.Errorf("%s: %w", name, err)
		}

		return starlark.None, nil
	})
}

// builtinRedfishPowerStatus reports system power state over Redfish.
// Starlark call is redfish_power_status(endpoint, username, password, insecure).
func builtinRedfishPowerStatus(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var state schemas.PowerState
	err := redfishWithSystem(contextFromThread(thread), "redfish_power_status", args, kwargs, func(sys *schemas.ComputerSystem) error {
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

// builtinRedfishSetBoot sets the boot source override over Redfish. Starlark call
// is redfish_set_boot(endpoint, username, password, target, persistent, efi, insecure).
func builtinRedfishSetBoot(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var endpoint, user, pass, target starlark.String
	var persistent, efi, insecure starlark.Bool

	if err := starlark.UnpackArgs("redfish_set_boot", args, kwargs,
		"endpoint", &endpoint, "username", &user, "password", &pass,
		"target", &target, "persistent?", &persistent, "efi?", &efi, "insecure?", &insecure); err != nil {
		return starlark.None, err
	}

	src, ok := redfishBootTargets[strings.ToLower(string(target))]
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

	err := redfishWithClient(contextFromThread(thread), string(endpoint), string(user), string(pass), bool(insecure), func(c *gofish.APIClient) error {
		sys, serr := redfishFirstSystem(c)
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

// builtinRedfishGetBoot reports the boot source override over Redfish.
// Starlark call is redfish_get_boot(endpoint, username, password, insecure).
func builtinRedfishGetBoot(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var boot schemas.Boot
	err := redfishWithSystem(contextFromThread(thread), "redfish_get_boot", args, kwargs, func(sys *schemas.ComputerSystem) error {
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

// redfishVirtualMedia collects virtual media from the managers and the system.
// When nothing is found the underlying lookup errors are surfaced together.
func redfishVirtualMedia(client *gofish.APIClient) ([]*schemas.VirtualMedia, error) {
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
		if sys, serr := redfishFirstSystem(client); serr != nil {
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

// redfishMediaIsCD reports whether a media slot accepts CD or DVD images.
func redfishMediaIsCD(m *schemas.VirtualMedia) bool {
	return slices.ContainsFunc(m.MediaTypes, func(t schemas.VirtualMediaType) bool {
		return t == schemas.CDVirtualMediaType || t == schemas.DVDVirtualMediaType
	})
}

// redfishInsertableMedia prefers a free CD or DVD slot, then any free slot, then
// the first slot.
func redfishInsertableMedia(media []*schemas.VirtualMedia) *schemas.VirtualMedia {
	free := func(m *schemas.VirtualMedia) bool { return !DerefOr(m.Inserted, false) }

	for _, m := range media {
		if free(m) && redfishMediaIsCD(m) {
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

// redfishInsertedMedia returns the media slot that currently holds an image, or
// nil when nothing is inserted.
func redfishInsertedMedia(media []*schemas.VirtualMedia) *schemas.VirtualMedia {
	for _, m := range media {
		if DerefOr(m.Inserted, false) {
			return m
		}
	}

	return nil
}

// builtinRedfishInsertMedia attaches an image to virtual media over Redfish. Call
// is redfish_insert_media(endpoint, username, password, image, insecure).
func builtinRedfishInsertMedia(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var endpoint, user, pass, image starlark.String
	var insecure starlark.Bool

	if err := starlark.UnpackArgs("redfish_insert_media", args, kwargs,
		"endpoint", &endpoint, "username", &user, "password", &pass,
		"image", &image, "insecure?", &insecure); err != nil {
		return starlark.None, err
	}

	ctx := contextFromThread(thread)
	err := redfishWithClient(ctx, string(endpoint), string(user), string(pass), bool(insecure), func(c *gofish.APIClient) error {
		media, merr := redfishVirtualMedia(c)
		if merr != nil {
			return merr
		}

		info, ierr := redfishInsertableMedia(media).InsertMedia(&schemas.VirtualMediaInsertMediaParameters{
			Image:    string(image),
			Inserted: new(true),
		})
		if ierr != nil {
			return ierr
		}

		return redfishWaitTask(ctx, c, info)
	})
	if err != nil {
		return starlark.None, fmt.Errorf("redfish_insert_media: %w", err)
	}

	return starlark.None, nil
}

// builtinRedfishEjectMedia detaches the mounted virtual media over Redfish.
// Starlark call is redfish_eject_media(endpoint, username, password, insecure).
func builtinRedfishEjectMedia(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	endpoint, user, pass, insecure, err := redfishConn("redfish_eject_media", args, kwargs)
	if err != nil {
		return starlark.None, err
	}

	ctx := contextFromThread(thread)
	err = redfishWithClient(ctx, endpoint, user, pass, insecure, func(c *gofish.APIClient) error {
		media, merr := redfishVirtualMedia(c)
		if merr != nil {
			return merr
		}

		m := redfishInsertedMedia(media)
		if m == nil {
			return nil
		}

		info, eerr := m.EjectMedia()
		if eerr != nil {
			return eerr
		}

		return redfishWaitTask(ctx, c, info)
	})
	if err != nil {
		return starlark.None, fmt.Errorf("redfish_eject_media: %w", err)
	}

	return starlark.None, nil
}

// builtinRedfishMediaStatus reports virtual media state over Redfish. Starlark
// call is redfish_media_status(endpoint, username, password, insecure).
func builtinRedfishMediaStatus(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	endpoint, user, pass, insecure, err := redfishConn("redfish_media_status", args, kwargs)
	if err != nil {
		return starlark.None, err
	}

	var inserted bool
	var image string
	err = redfishWithClient(contextFromThread(thread), endpoint, user, pass, insecure, func(c *gofish.APIClient) error {
		media, merr := redfishVirtualMedia(c)
		if merr != nil {
			return merr
		}

		if m := redfishInsertedMedia(media); m != nil {
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

// builtinRedfishGetSystem reports system vendor fields over Redfish. Starlark
// call is redfish_get_system(endpoint, username, password, insecure).
func builtinRedfishGetSystem(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var manufacturer, model, serial string
	err := redfishWithSystem(contextFromThread(thread), "redfish_get_system", args, kwargs, func(sys *schemas.ComputerSystem) error {
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

// builtinRedfishIsHealthy reports whether the system health rolls up to OK.
// Starlark call is redfish_is_healthy(endpoint, username, password, insecure).
func builtinRedfishIsHealthy(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var health schemas.Health
	err := redfishWithSystem(contextFromThread(thread), "redfish_is_healthy", args, kwargs, func(sys *schemas.ComputerSystem) error {
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

// builtinRedfishGetFirmware reports the system BIOS version over Redfish.
// Starlark call is redfish_get_firmware(endpoint, username, password, insecure).
func builtinRedfishGetFirmware(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var bios string
	err := redfishWithSystem(contextFromThread(thread), "redfish_get_firmware", args, kwargs, func(sys *schemas.ComputerSystem) error {
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

// redfishBuiltins exposes the Redfish power, boot, media, and info helpers.
func redfishBuiltins() starlark.StringDict {
	return starlark.StringDict{
		"redfish_power_on":     redfishPowerBuiltin("redfish_power_on", schemas.OnResetType),
		"redfish_power_off":    redfishPowerBuiltin("redfish_power_off", schemas.ForceOffResetType),
		"redfish_power_soft":   redfishPowerBuiltin("redfish_power_soft", schemas.GracefulShutdownResetType),
		"redfish_power_cycle":  redfishPowerBuiltin("redfish_power_cycle", schemas.PowerCycleResetType),
		"redfish_power_reset":  redfishPowerBuiltin("redfish_power_reset", schemas.ForceRestartResetType),
		"redfish_power_status": starlark.NewBuiltin("redfish_power_status", builtinRedfishPowerStatus),
		"redfish_set_boot":     starlark.NewBuiltin("redfish_set_boot", builtinRedfishSetBoot),
		"redfish_get_boot":     starlark.NewBuiltin("redfish_get_boot", builtinRedfishGetBoot),
		"redfish_insert_media": starlark.NewBuiltin("redfish_insert_media", builtinRedfishInsertMedia),
		"redfish_eject_media":  starlark.NewBuiltin("redfish_eject_media", builtinRedfishEjectMedia),
		"redfish_media_status": starlark.NewBuiltin("redfish_media_status", builtinRedfishMediaStatus),
		"redfish_get_system":   starlark.NewBuiltin("redfish_get_system", builtinRedfishGetSystem),
		"redfish_is_healthy":   starlark.NewBuiltin("redfish_is_healthy", builtinRedfishIsHealthy),
		"redfish_get_firmware": starlark.NewBuiltin("redfish_get_firmware", builtinRedfishGetFirmware),
		"redfish_inventory":    starlark.NewBuiltin("redfish_inventory", builtinRedfishInventory),
	}
}
