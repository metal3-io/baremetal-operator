// SPDX-License-Identifier: Apache-2.0

// Package redfish is every BMC operation this provisioner needs, power, boot
// source, virtual media, health and the two system fields registration logs.
package redfish

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"time"

	gofish "github.com/stmcginnis/gofish"
	"github.com/stmcginnis/gofish/schemas"
)

type MediaStatus struct {
	Image    string
	Inserted bool
}

// Conn identifies one BMC. Every Redfish call hangs off it, so the credentials
// travel with the request rather than being reassembled at each call site.
type Conn struct {
	Endpoint string
	Username string
	// Never serialised. Nothing marshals a Conn today, and the tag keeps it that
	// way, the same reason String renders only the endpoint.
	Password string `json:"-"`
	// SystemID is the @odata.id the BMC address named, empty when it named none.
	// A BMC fronting several machines lists them all, so ignoring it picks blind.
	SystemID string
}

// String renders a Conn as its endpoint. Nothing scrubs output any more, so
// without this a Conn reaching a log field or a %v would print the password.
func (c Conn) String() string {
	return c.Endpoint
}

func MediaIsCD(m *schemas.VirtualMedia) bool {
	return slices.ContainsFunc(m.MediaTypes, func(t schemas.VirtualMediaType) bool {
		return t == schemas.CDVirtualMediaType || t == schemas.DVDVirtualMediaType
	})
}

func InsertableMedia(media []*schemas.VirtualMedia) *schemas.VirtualMedia {
	free := func(m *schemas.VirtualMedia) bool { return m.Inserted == nil || !*m.Inserted }

	for _, m := range media {
		if free(m) && MediaIsCD(m) {
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

const (
	// RedfishTimeout bounds a single Redfish request over the network.
	RedfishTimeout = 30 * time.Second
	// RedfishTaskTimeout bounds waiting for an async Redfish task to finish.
	RedfishTaskTimeout = 5 * time.Minute
	// RedfishPollRate is how often an async Redfish task is polled.
	RedfishPollRate = 2 * time.Second
)

func (c Conn) WithClient(ctx context.Context, fn func(*gofish.APIClient) error) error {
	// ConnectContext threads the caller ctx into every request, so a canceled
	// reconcile aborts the call instead of holding a worker.
	client, err := gofish.ConnectContext(ctx, gofish.ClientConfig{
		Endpoint:  c.Endpoint,
		Username:  c.Username,
		Password:  c.Password,
		Insecure:  true,
		BasicAuth: true,
		// Insecure makes gofish set InsecureSkipVerify on this transport, which it
		// can only do when the transport is non nil. Timeout bounds a hung BMC.
		HTTPClient: &http.Client{
			Timeout:   RedfishTimeout,
			Transport: &http.Transport{Proxy: http.ProxyFromEnvironment},
		},
	})
	if err != nil {
		return err
	}
	defer client.Logout()

	return fn(client)
}

func (c Conn) System(client *gofish.APIClient) (*schemas.ComputerSystem, error) {
	if c.SystemID != "" {
		sys, err := schemas.GetComputerSystem(client, c.SystemID)
		if err != nil {
			return nil, fmt.Errorf("computer system %s: %w", c.SystemID, err)
		}

		return sys, nil
	}

	systems, err := client.Service.Systems()
	if err != nil {
		return nil, err
	}

	if len(systems) == 0 {
		return nil, errors.New("no computer system found")
	}

	// Picking one of several would be a guess, and the wrong guess powers off
	// somebody else's machine, so make the address say which.
	if len(systems) > 1 {
		return nil, fmt.Errorf("BMC serves %d computer systems, name one in the BMC address path", len(systems))
	}

	return systems[0], nil
}

// VirtualMedia collects the media belonging to this system, its own first and
// then its managers. Service wide managers would hand back another host's drive.
func (c Conn) VirtualMedia(client *gofish.APIClient) ([]*schemas.VirtualMedia, error) {
	var media []*schemas.VirtualMedia

	var errs []error

	sys, err := c.System(client)
	if err != nil {
		return nil, fmt.Errorf("no virtual media found: %w", err)
	}

	if vm, verr := sys.VirtualMedia(); verr != nil {
		errs = append(errs, verr)
	} else {
		media = append(media, vm...)
	}

	// Real BMCs hang virtual media off the manager rather than the system, so
	// fall back to the ones managing this system and never to every manager.
	if len(media) == 0 {
		if managers, merr := sys.ManagedBy(); merr != nil {
			errs = append(errs, merr)
		} else {
			for _, m := range managers {
				if vm, verr := m.VirtualMedia(); verr != nil {
					errs = append(errs, verr)
				} else {
					media = append(media, vm...)
				}
			}
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

// SystemInfo is the pair of system fields registration logs, so an operator can
// tell from the log which machine a claimed host actually is.
type SystemInfo struct {
	Manufacturer string
	Model        string
}

func (c Conn) WithSystem(ctx context.Context, fn func(*schemas.ComputerSystem) error) error {
	return c.WithClient(ctx, func(client *gofish.APIClient) error {
		sys, err := c.System(client)
		if err != nil {
			return err
		}

		return fn(sys)
	})
}

// WaitTask blocks until an async Redfish task finishes. A nil info means the
// action already completed synchronously.
func WaitTask(ctx context.Context, client *gofish.APIClient, info *schemas.TaskMonitorInfo) error {
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

func InsertedMedia(media []*schemas.VirtualMedia) *schemas.VirtualMedia {
	for _, m := range media {
		if m.Inserted != nil && *m.Inserted {
			return m
		}
	}

	return nil
}

func (c Conn) Reset(ctx context.Context, reset schemas.ResetType) error {
	err := c.WithClient(ctx, func(client *gofish.APIClient) error {
		sys, serr := c.System(client)
		if serr != nil {
			return serr
		}

		info, rerr := sys.Reset(reset)
		if rerr != nil {
			return rerr
		}

		return WaitTask(ctx, client, info)
	})
	if err != nil {
		return fmt.Errorf("redfish reset %s: %w", reset, err)
	}

	return nil
}

func (c Conn) PowerOn(ctx context.Context) error {
	return c.Reset(ctx, schemas.OnResetType)
}

func (c Conn) PowerOff(ctx context.Context) error {
	return c.Reset(ctx, schemas.ForceOffResetType)
}

func (c Conn) PowerSoft(ctx context.Context) error {
	return c.Reset(ctx, schemas.GracefulShutdownResetType)
}

func (c Conn) PowerState(ctx context.Context) (schemas.PowerState, error) {
	var state schemas.PowerState

	err := c.WithSystem(ctx, func(sys *schemas.ComputerSystem) error {
		state = sys.PowerState

		return nil
	})
	if err != nil {
		return "", fmt.Errorf("redfish power state: %w", err)
	}

	return state, nil
}

// SetBoot writes a boot source override. Emulators are free to ignore the
// enabled frequency, so nothing may rely on Once actually applying once.
func (c Conn) SetBoot(
	ctx context.Context,
	src schemas.BootSource,
	enabled schemas.BootSourceOverrideEnabled,
	efi bool,
) error {
	mode := schemas.LegacyBootSourceOverrideMode
	if efi {
		mode = schemas.UEFIBootSourceOverrideMode
	}

	err := c.WithSystem(ctx, func(sys *schemas.ComputerSystem) error {
		return sys.SetBoot(&schemas.Boot{
			BootSourceOverrideTarget:  src,
			BootSourceOverrideEnabled: enabled,
			BootSourceOverrideMode:    mode,
		})
	})
	if err != nil {
		return fmt.Errorf("redfish set boot %s %s: %w", src, enabled, err)
	}

	return nil
}

// BootSource reports the override target the BMC currently holds. A BMC can
// accept the write and keep booting whatever it likes, so this reads it back.
func (c Conn) BootSource(ctx context.Context) (schemas.BootSource, error) {
	var src schemas.BootSource

	err := c.WithSystem(ctx, func(sys *schemas.ComputerSystem) error {
		src = sys.Boot.BootSourceOverrideTarget

		return nil
	})
	if err != nil {
		return "", fmt.Errorf("redfish boot source: %w", err)
	}

	return src, nil
}

func (c Conn) InsertMedia(ctx context.Context, image string) error {
	err := c.WithClient(ctx, func(client *gofish.APIClient) error {
		media, merr := c.VirtualMedia(client)
		if merr != nil {
			return merr
		}

		info, ierr := InsertableMedia(media).InsertMedia(&schemas.VirtualMediaInsertMediaParameters{
			Image:    image,
			Inserted: new(true),
		})
		if ierr != nil {
			return ierr
		}

		return WaitTask(ctx, client, info)
	})
	if err != nil {
		return fmt.Errorf("redfish insert media: %w", err)
	}

	return nil
}

// EjectMedia detaches the mounted virtual media, treating an empty drive as
// success so teardown can call it unconditionally.
func (c Conn) EjectMedia(ctx context.Context) error {
	err := c.WithClient(ctx, func(client *gofish.APIClient) error {
		media, merr := c.VirtualMedia(client)
		if merr != nil {
			return merr
		}

		m := InsertedMedia(media)
		if m == nil {
			return nil
		}

		info, eerr := m.EjectMedia()
		if eerr != nil {
			return eerr
		}

		return WaitTask(ctx, client, info)
	})
	if err != nil {
		return fmt.Errorf("redfish eject media: %w", err)
	}

	return nil
}

func (c Conn) MediaStatus(ctx context.Context) (MediaStatus, error) {
	var status MediaStatus

	err := c.WithClient(ctx, func(client *gofish.APIClient) error {
		media, merr := c.VirtualMedia(client)
		if merr != nil {
			return merr
		}

		if m := InsertedMedia(media); m != nil {
			status = MediaStatus{Inserted: true, Image: m.Image}
		}

		return nil
	})
	if err != nil {
		return MediaStatus{}, fmt.Errorf("redfish media status: %w", err)
	}

	return status, nil
}

func (c Conn) SystemInfo(ctx context.Context) (SystemInfo, error) {
	var info SystemInfo

	err := c.WithSystem(ctx, func(sys *schemas.ComputerSystem) error {
		info = SystemInfo{
			Manufacturer: sys.Manufacturer,
			Model:        sys.Model,
		}

		return nil
	})
	if err != nil {
		return SystemInfo{}, fmt.Errorf("redfish system: %w", err)
	}

	return info, nil
}

// ServiceRootPath is the one Redfish resource DSP0266 leaves unauthenticated.
const ServiceRootPath = "/redfish/v1/"

// Healthy reports whether the BMC answers at all, which is the only thing this
// provisioner can tell. The service root needs no credentials, so a rejected
// password cannot make a reachable BMC look unhealthy.
func (c Conn) Healthy(ctx context.Context) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Endpoint+ServiceRootPath, http.NoBody)
	if err != nil {
		return false, fmt.Errorf("redfish health: %w", err)
	}

	client := &http.Client{
		Timeout: RedfishTimeout,
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			//nolint:gosec // BMC certificates are self signed, the whole package skips verification
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		return false, fmt.Errorf("redfish health: %w", err)
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK, nil
}
