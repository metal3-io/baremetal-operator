// SPDX-License-Identifier: Apache-2.0

// The Provisioner interface over Redfish only. Everything it cannot do out of
// band lives in unsupported.go.

package anaconda

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	"github.com/stmcginnis/gofish/schemas"
	"metal3.local/anaconda/internal/core"
	"metal3.local/anaconda/internal/redfish"
)

const LiveISO = "live-iso"

func Requeue(d time.Duration) provisioner.Result {
	return provisioner.Result{Dirty: true, RequeueAfter: d}
}

// Requeue delay. Power and media transitions settle in seconds, so polling
// harder than this only adds BMC load.
const PowerRequeueDelay = 15 * time.Second

func (p *Provisioner) ConnAndPowerState(ctx context.Context) (redfish.Conn, schemas.PowerState, error) {
	conn, err := p.Conn()
	if err != nil {
		return redfish.Conn{}, "", err
	}

	state, err := conn.PowerState(ctx)
	if err != nil {
		return redfish.Conn{}, "", err
	}

	return conn, state, nil
}

func (p *Provisioner) PowerOn(ctx context.Context, _ bool) (provisioner.Result, error) {
	conn, state, err := p.ConnAndPowerState(ctx)
	if err != nil {
		return provisioner.Result{}, err
	}

	if state == schemas.OnPowerState {
		return provisioner.Result{}, nil
	}

	if state == schemas.PoweringOnPowerState {
		return Requeue(PowerRequeueDelay), nil
	}

	p.Log.Info("powering on", "powerState", state)

	if err := conn.PowerOn(ctx); err != nil {
		return provisioner.Result{}, err
	}

	p.Publish("PowerOn", "Host powered on over Redfish")

	return Requeue(PowerRequeueDelay), nil
}

func (p *Provisioner) PowerOff(
	ctx context.Context,
	rebootMode metal3api.RebootMode,
	force bool,
	_ metal3api.AutomatedCleaningMode,
) (provisioner.Result, error) {
	// Runs before deletion, so refusing here would wedge the finalizer.
	if !redfish.UsableBMCAddress(p.HostData.BMCAddress) {
		p.Log.Error(nil, "no usable BMC address, treating the host as off")

		return provisioner.Result{}, nil
	}

	conn, state, err := p.ConnAndPowerState(ctx)
	if err != nil {
		return provisioner.Result{}, err
	}

	if state == schemas.OffPowerState {
		return provisioner.Result{}, nil
	}

	if state == schemas.PoweringOffPowerState {
		return Requeue(PowerRequeueDelay), nil
	}

	if rebootMode == metal3api.RebootModeSoft && !force {
		p.Log.Info("graceful shutdown", "powerState", state)

		if err := conn.PowerSoft(ctx); err != nil {
			return provisioner.Result{}, err
		}
	} else {
		p.Log.Info("forced power off", "powerState", state)

		if err := conn.PowerOff(ctx); err != nil {
			return provisioner.Result{}, err
		}
	}

	p.Publish("PowerOff", "Host powered off over Redfish")

	return Requeue(PowerRequeueDelay), nil
}

func (p *Provisioner) BeginInstall(
	ctx context.Context,
	conn redfish.Conn,
	media redfish.MediaStatus,
	data *provisioner.ProvisionData,
) (provisioner.Result, error) {
	url := data.Image.URL

	// The kickstart is served by matching the MACs anaconda reports, so a host
	// with none declared boots only to take the fallback and power itself off.
	if p.HostData.BootMACAddress == "" {
		return provisioner.Result{
			ErrorMessage: "provision: no boot MAC for " + p.Namespace() + "/" + p.Name() +
				", set spec.bootMACAddress to the NIC the machine boots from",
		}, nil
	}

	// A host with no kickstart Secret boots only to be served the fallback, which
	// powers it off from %pre with nothing on the BareMetalHost saying why.
	ks, err := p.Store.HasKickstart(ctx, p.Namespace(), p.Name())
	if err != nil {
		return provisioner.Result{}, err
	}

	if !ks {
		return provisioner.Result{
			ErrorMessage: "provision: no kickstart for " + p.Namespace() + "/" + p.Name() +
				", set spec.preprovisioningNetworkDataName to a Secret carrying a " + core.KickstartSecretKey + " key",
		}, nil
	}

	// The host's own hint and nothing else. BMO's status carries the hardware
	// profile guess of /dev/sda, which would wipe a disk nobody named.
	disk, err := p.Store.HostInstallDisk(ctx, p.Namespace(), p.Name())
	if err != nil {
		return provisioner.Result{}, err
	}

	// clearpart wipes whatever is named, so an unresolved disk refuses the
	// install rather than letting the render pick one.
	if disk == "" {
		return provisioner.Result{
			ErrorMessage: "provision: no root device hints for " + p.Namespace() + "/" + p.Name() +
				", set spec.rootDeviceHints to a deviceName, wwn or serialNumber",
		}, nil
	}

	// Swapping media under a running machine leaves it booted from the old
	// source, so the host goes down first.
	state, err := conn.PowerState(ctx)
	if err != nil {
		return provisioner.Result{}, err
	}

	if state != schemas.OffPowerState {
		if state == schemas.PoweringOffPowerState {
			return Requeue(PowerRequeueDelay), nil
		}

		p.Log.Info("powering off to insert the live ISO", "image", url)

		if perr := conn.PowerOff(ctx); perr != nil {
			return provisioner.Result{}, perr
		}

		return Requeue(PowerRequeueDelay), nil
	}

	if media.Inserted {
		// A BMC that echoes back a rewritten URL never matches the request and
		// would land here every pass, so the swap is logged rather than silent.
		p.Log.Info("ejecting media that is not the requested image", "attached", media.Image, "wanted", url)

		if eerr := conn.EjectMedia(ctx); eerr != nil {
			return provisioner.Result{}, eerr
		}
	}

	p.Log.Info("inserting live ISO", "image", url)

	if ierr := conn.InsertMedia(ctx, url); ierr != nil {
		return provisioner.Result{}, ierr
	}

	// A BMC can accept the insert and attach nothing, which boots the host off
	// its disk and leaves the install waiting on a callback that cannot come.
	inserted, err := conn.MediaStatus(ctx)
	if err != nil {
		return provisioner.Result{}, err
	}

	if !inserted.Inserted {
		return provisioner.Result{
			ErrorMessage: "provision: the BMC accepted " + url +
				" and reports the virtual media drive empty, so the host would boot from disk",
		}, nil
	}

	// Only worth a log, since a rewritten URL is how some BMCs report an image
	// they fetched themselves.
	if inserted.Image != url {
		p.Log.Info("BMC reports a different image than the one inserted", "attached", inserted.Image, "wanted", url)
	}

	// A one time override, so the reboot ending the install lands on disk rather
	// than starting the installer over again.
	uefi := data.BootMode == metal3api.UEFI
	if berr := conn.SetBoot(ctx, schemas.CdBootSource, schemas.OnceBootSourceOverrideEnabled, uefi); berr != nil {
		return provisioner.Result{}, berr
	}

	// Same again for the override. Accepting the write and booting the disk
	// anyway is the failure this whole sequence exists to catch.
	src, err := conn.BootSource(ctx)
	if err != nil {
		return provisioner.Result{}, err
	}

	if src != schemas.CdBootSource {
		return provisioner.Result{
			ErrorMessage: "provision: the BMC accepted the Cd boot override and reports " +
				strconv.Quote(string(src)) + ", so the host would boot from disk",
		}, nil
	}

	// Without this a reinstall reads the previous run's report on its first pass
	// and reports provisioned before anaconda has booted.
	if err := p.Store.ClearInstallReport(ctx, p.Namespace(), p.Name()); err != nil {
		return provisioner.Result{}, err
	}

	// Starts the clock the timeout runs from and marks the drive as this run's,
	// so a stale ISO is not mistaken for an install already under way.
	if err := p.Store.MarkInstallStarted(ctx, p.Namespace(), p.Name(), time.Now()); err != nil {
		return provisioner.Result{}, err
	}

	if err := conn.PowerOn(ctx); err != nil {
		return provisioner.Result{}, err
	}

	p.Publish("ProvisioningStarted", "Booting live ISO "+url)

	return Requeue(PowerRequeueDelay), nil
}

// ReportBootMACCandidates names the MACs the hardware data lists when the host
// declares no boot MAC, since nothing provisions until an operator picks one.
func (p *Provisioner) ReportBootMACCandidates(details *metal3api.HardwareDetails) {
	if p.HostData.BootMACAddress != "" || len(details.NIC) == 0 {
		return
	}

	macs := make([]string, 0, len(details.NIC))
	for i := range details.NIC {
		macs = append(macs, details.NIC[i].MAC)
	}

	found := strings.Join(macs, ", ")

	p.Log.Info("host declares no boot MAC, provisioning waits until one is set", "candidates", found)
	p.Publish("BootMACRequired", "Set spec.bootMACAddress to one of "+found)
}

// InspectHardware picks up the HardwareData an external tool recorded, or lets
// the host through with only its name when there is none. It never reads a BMC.
func (p *Provisioner) InspectHardware(
	ctx context.Context,
	_ provisioner.InspectData,
	_, refresh, _ bool,
) (provisioner.Result, bool, *metal3api.HardwareDetails, error) {
	// BMO drops the refresh annotation only when a provisioner reports inspection
	// started, and re-enters inspecting forever while the annotation is there.
	if refresh {
		p.Log.Info("acknowledging an inspection refresh request")

		return provisioner.Result{}, true, nil, nil
	}

	details, err := p.Store.HostHardwareData(ctx, p.Namespace(), p.Name())
	if err != nil {
		return provisioner.Result{}, false, nil, err
	}

	if details != nil {
		p.Log.Info("ingesting the hardware data recorded for this host", "nics", len(details.NIC))
		p.Publish("InspectionComplete", "Ingested the HardwareData recorded for this host")
		p.ReportBootMACCandidates(details)

		return provisioner.Result{}, false, details, nil
	}

	// Nothing discovers hardware here, and nil details would loop the host in
	// Inspecting forever, so the name alone is what lets it move on.
	p.Log.Info("no hardware data recorded, passing inspection with the host name alone")

	return provisioner.Result{}, false, &metal3api.HardwareDetails{Hostname: p.Name()}, nil
}

// Register validates the BMC address, probes Redfish once, and claims the host
// by UID. The probe is skipped in steady state so a reconcile costs no BMC call.
func (p *Provisioner) Register(
	ctx context.Context,
	_ provisioner.ManagementAccessData,
	credentialsChanged, _ bool,
) (provisioner.Result, string, error) {
	conn, err := p.Conn()
	if err != nil {
		return provisioner.Result{}, "", err
	}

	provID := p.HostData.ProvisionerID
	if provID == "" {
		provID = string(p.HostData.ObjectMeta.UID)
	}

	if provID == "" {
		return Requeue(PowerRequeueDelay), "", errors.New("register: BareMetalHost has no UID to claim")
	}

	if p.HostData.ProvisionerID == "" || credentialsChanged {
		info, serr := conn.SystemInfo(ctx)
		if serr != nil {
			return provisioner.Result{}, "", serr
		}

		p.Log.Info("registered over Redfish",
			"endpoint", conn.Endpoint, "manufacturer", info.Manufacturer, "model", info.Model)
		p.Publish("Registered", "Registered host over Redfish")
	}

	return provisioner.Result{}, provID, nil
}

func (p *Provisioner) UpdateHardwareState(ctx context.Context) (provisioner.HardwareState, error) {
	conn, err := p.Conn()
	if err != nil {
		return provisioner.HardwareState{}, err
	}

	state, err := conn.PowerState(ctx)
	if err != nil {
		return provisioner.HardwareState{}, err
	}

	on := state == schemas.OnPowerState

	return provisioner.HardwareState{PoweredOn: &on}, nil
}

// FinishInstall lands a reported host on its own disk. The kickstart issues no
// power command, so taking the machine down is this provisioner's job.
func (p *Provisioner) FinishInstall(ctx context.Context, conn redfish.Conn, uefi bool) (provisioner.Result, error) {
	state, err := conn.PowerState(ctx)
	if err != nil {
		return provisioner.Result{}, err
	}

	if state != schemas.OffPowerState {
		return p.ShutDownAfterInstall(ctx, conn, state)
	}

	// Only now. Anaconda reads the installer image while it exits, so a drive
	// pulled out from under a running host aborts its shutdown partway.
	p.Log.Info("host is down, ejecting media")

	if err := conn.EjectMedia(ctx); err != nil {
		return provisioner.Result{}, err
	}

	// The one time override is spent by now, so this states the boot order
	// rather than trusting that the BIOS one lists the disk.
	if err := conn.SetBoot(ctx, schemas.HddBootSource, schemas.ContinuousBootSourceOverrideEnabled, uefi); err != nil {
		return provisioner.Result{}, err
	}

	if err := conn.PowerOn(ctx); err != nil {
		return provisioner.Result{}, err
	}

	p.Publish("ProvisioningComplete", "Anaconda reported the install finished")

	return provisioner.Result{}, nil
}

// ShutDownAfterInstall asks the installed host to go down gracefully, so
// systemd unmounts the target rather than losing whatever is still buffered.
func (p *Provisioner) ShutDownAfterInstall(ctx context.Context, conn redfish.Conn, state schemas.PowerState) (provisioner.Result, error) {
	if state == schemas.PoweringOffPowerState {
		return Requeue(PowerRequeueDelay), nil
	}

	started, err := p.Store.InstallStartedAt(ctx, p.Namespace(), p.Name())
	if err != nil {
		return provisioner.Result{}, err
	}

	// A host that ignores the request would hold provisioning open for good, so
	// it is cut once the install has had its whole budget.
	if !started.IsZero() && time.Since(started) > p.Cfg.InstallTimeout {
		p.Log.Info("host never went down after the install, forcing it off", "powerState", state)

		if err := conn.PowerOff(ctx); err != nil {
			return provisioner.Result{}, err
		}

		return Requeue(PowerRequeueDelay), nil
	}

	p.Log.Info("install reported complete, shutting the host down", "powerState", state)

	if err := conn.PowerSoft(ctx); err != nil {
		return provisioner.Result{}, err
	}

	return Requeue(PowerRequeueDelay), nil
}

func (p *Provisioner) AwaitInstall(ctx context.Context, conn redfish.Conn, data *provisioner.ProvisionData) (provisioner.Result, error) {
	url := data.Image.URL

	// Without a listener nothing can ever report completion, so waiting would
	// hang the host until the timeout for no reason.
	if !p.CallbackEnabled {
		p.Log.Info("no callback listener, treating a booted ISO as provisioned", "image", url)
		p.Publish("ProvisioningComplete", "Booted live ISO "+url)

		return provisioner.Result{}, nil
	}

	report, err := p.Store.ReadInstallReport(ctx, p.Namespace(), p.Name())
	if err != nil {
		return provisioner.Result{}, err
	}

	if report != nil {
		if !report.Succeeded {
			// No eject on purpose, the drive stays as anaconda left it and the
			// spent boot override means a power cycle will not reinstall.

			// The verdict is sticky, every later pass reads the same annotation.
			// Recovery is a deprovision or a new image URL, both clear it.
			p.Log.Error(nil, "install reported failure", "detail", report.Message)

			return provisioner.Result{
				ErrorMessage: "provision: anaconda reported the install failed: " + report.Message,
			}, nil
		}

		return p.FinishInstall(ctx, conn, data.BootMode == metal3api.UEFI)
	}

	started, err := p.Store.InstallStartedAt(ctx, p.Namespace(), p.Name())
	if err != nil {
		return provisioner.Result{}, err
	}

	// Zero means BeginInstall has not stamped yet, so there is no install to time
	// out. Wait rather than fail a host on a clock that never started.
	if started.IsZero() {
		return Requeue(core.InstallPollInterval), nil
	}

	if elapsed := time.Since(started); elapsed > p.Cfg.InstallTimeout {
		return provisioner.Result{
			ErrorMessage: fmt.Sprintf("provision: anaconda did not report completion within %s", p.Cfg.InstallTimeout),
		}, nil
	}

	return Requeue(core.InstallPollInterval), nil
}

// Provision boots the installer ISO and waits for anaconda to report finished.
// One step per reconcile, because booted is not the same as installed.
func (p *Provisioner) Provision(
	ctx context.Context,
	data provisioner.ProvisionData,
	_ bool,
) (provisioner.Result, error) {
	// Only one format is ever deployed, so anything else is refused before the
	// machine is touched rather than booted into an installer it cannot run.
	if data.Image.DiskFormat == nil || *data.Image.DiskFormat != LiveISO {
		format := ""
		if data.Image.DiskFormat != nil {
			format = *data.Image.DiskFormat
		}

		p.Log.Error(nil, "unsupported image format", "format", format, "url", data.Image.URL)

		return Unsupported("provision", "deploying a "+strconv.Quote(format)+" image"), nil
	}

	if data.Image.URL == "" {
		return provisioner.Result{ErrorMessage: "provision: live-iso image has no url"}, nil
	}

	conn, err := p.Conn()
	if err != nil {
		return provisioner.Result{}, err
	}

	media, err := conn.MediaStatus(ctx)
	if err != nil {
		return provisioner.Result{}, err
	}

	// The stamp says the drive holds this run's ISO. Comparing URLs power cycles
	// a rewritten one, and trusting the drive waits out a stale mount.
	started, err := p.Store.InstallStartedAt(ctx, p.Namespace(), p.Name())
	if err != nil {
		return provisioner.Result{}, err
	}

	if media.Inserted && !started.IsZero() {
		return p.AwaitInstall(ctx, conn, &data)
	}

	return p.BeginInstall(ctx, conn, media, &data)
}

// Deprovision ejects the media and forgets the install. It tolerates a broken
// BMC address, refusing here would keep the finalizer and block deletion.
func (p *Provisioner) Deprovision(ctx context.Context, _ bool, _ metal3api.AutomatedCleaningMode) (provisioner.Result, error) {
	// Teardown must finish even if the reconcile is cut short, but the parent
	// still carries values worth keeping.
	ctx = context.WithoutCancel(ctx)

	if err := p.Store.ClearInstallReport(ctx, p.Namespace(), p.Name()); err != nil {
		p.Log.Error(err, "clearing install state failed")
	}

	if !redfish.UsableBMCAddress(p.HostData.BMCAddress) {
		p.Log.Error(nil, "BMC address is unusable, leaving virtual media alone")

		return provisioner.Result{}, nil
	}

	conn, err := p.Conn()
	if err != nil {
		return provisioner.Result{}, nil //nolint:nilerr // teardown must not wedge the finalizer
	}

	media, err := conn.MediaStatus(ctx)
	if err != nil || !media.Inserted {
		return provisioner.Result{}, nil //nolint:nilerr // nothing to eject, or the BMC is gone
	}

	p.Log.Info("ejecting virtual media")

	if err := conn.EjectMedia(ctx); err != nil {
		return provisioner.Result{}, err
	}

	p.Publish("DeprovisioningComplete", "Ejected virtual media over Redfish")

	return provisioner.Result{}, nil
}

func (p *Provisioner) GetDataImageStatus(ctx context.Context) (bool, error) {
	// The DataImage controller builds HostData with no BMC, and refusing there
	// would strand the CR forever.
	if !redfish.UsableBMCAddress(p.HostData.BMCAddress) {
		return false, nil
	}

	conn, err := p.Conn()
	if err != nil {
		return false, err
	}

	media, err := conn.MediaStatus(ctx)
	if err != nil {
		return false, err
	}

	return media.Inserted, nil
}

// AttachDataImage inserts a data image into virtual media. It shares the slot
// with a live-iso deploy, so attaching replaces whatever the host booted.
func (p *Provisioner) AttachDataImage(ctx context.Context, url string) error {
	conn, err := p.Conn()
	if err != nil {
		return err
	}

	p.Log.Info("attaching data image", "image", url)

	return conn.InsertMedia(ctx, url)
}

func (p *Provisioner) DetachDataImage(ctx context.Context) error {
	conn, err := p.Conn()
	if err != nil {
		return err
	}

	p.Log.Info("detaching data image")

	return conn.EjectMedia(ctx)
}

// GetHealth maps the Redfish system health rollup to a controller health string.
// It returns no error by contract, so a failure is logged and published instead.
func (p *Provisioner) GetHealth(ctx context.Context) string {
	conn, err := p.Conn()
	if err != nil {
		return ""
	}

	// No event. BMO turns this return into a Healthy condition already, and BMO
	// calls it every reconcile, so publishing would be a duplicate per pass.
	healthy, err := conn.Healthy(ctx)
	if err != nil {
		p.Log.Error(err, "health check failed")

		return ""
	}

	if healthy {
		return provisioner.HealthOK
	}

	return provisioner.HealthCritical
}
