// SPDX-License-Identifier: Apache-2.0

// End to end cover for the provisioner against a stub BMC.

package anaconda_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/hardwareutils/bmc"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"metal3.local/anaconda/internal/anaconda"
	"metal3.local/anaconda/internal/core"
)

type fakeStore struct {
	report       *core.InstallReport
	hardwareData *metal3api.HardwareDetails
	started      time.Time
	marks        int
	cleared      int
	noKickstart  bool
	noRootHints  bool
}

// The disk is only ever the host's own hint, so a store that reports none stands
// for a BareMetalHost that left spec.rootDeviceHints unset.
func (f *fakeStore) HostInstallDisk(_ context.Context, _, _ string) (string, error) {
	if f.noRootHints {
		return "", nil
	}

	return testDisk, nil
}

func (f *fakeStore) HostHardwareData(_ context.Context, _, _ string) (*metal3api.HardwareDetails, error) {
	return f.hardwareData, nil
}

func (f *fakeStore) HasKickstart(_ context.Context, _, _ string) (bool, error) {
	return !f.noKickstart, nil
}

func (f *fakeStore) ReadInstallReport(_ context.Context, _, _ string) (*core.InstallReport, error) {
	return f.report, nil
}

func (f *fakeStore) ClearInstallReport(_ context.Context, _, _ string) error {
	f.cleared++
	f.report = nil
	f.started = time.Time{}

	return nil
}

func (f *fakeStore) InstallStartedAt(_ context.Context, _, _ string) (time.Time, error) {
	return f.started, nil
}

func (f *fakeStore) MarkInstallStarted(_ context.Context, _, _ string, at time.Time) error {
	f.marks++
	f.started = at

	return nil
}

// testProvisioner points a provisioner at srv over redfish-virtualmedia+http,
// the form a BareMetalHost carries for a plaintext BMC.
func testProvisioner(t *testing.T, srv *httptest.Server, store anaconda.HostStore, opts ...func(*anaconda.Provisioner)) *anaconda.Provisioner {
	t.Helper()

	authority := strings.TrimPrefix(srv.URL, "http://")

	p := &anaconda.Provisioner{
		Cfg:   core.Config{InstallTimeout: time.Hour},
		Store: store,
		Log:   logr.Discard(),
		HostData: provisioner.HostData{
			ObjectMeta:     metav1.ObjectMeta{Namespace: "ns", Name: testHost, UID: testUID},
			BMCAddress:     "redfish-virtualmedia+http://" + authority + systemPath,
			BMCCredentials: bmc.Credentials{Username: stubUser, Password: stubPass},
			BootMACAddress: testMAC,
			ProvisionerID:  testUID,
		},
		CallbackEnabled: true,
	}

	for _, opt := range opts {
		opt(p)
	}

	return p
}

// The two Redfish reset types the provisioner ever issues.
const (
	resetGraceful = "GracefulShutdown"
	resetForced   = "ForceOff"
)

// noBMC fails the test if anything reaches it, since nothing on the inspection
// path may cost a BMC round trip now.
func noBMC(t *testing.T) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		t.Errorf("the BMC was contacted at %s, want no round trips", r.URL.Path)
	}))
	t.Cleanup(srv.Close)

	return srv
}

// liveISOData builds the ProvisionData the BMH controller passes for a live ISO,
// which carries no checksum because BMO exempts the format from that check.
func liveISOData(url string) provisioner.ProvisionData {
	format := anaconda.LiveISO

	return provisioner.ProvisionData{
		Image:    metal3api.Image{URL: url, DiskFormat: &format},
		BootMode: metal3api.UEFI,
	}
}

func TestProvisionBootsLiveISOAndWaitsForCallback(t *testing.T) {
	const testISO = "http://boot.example/rocky-10.2-x86_64-ks1.testISO"

	state := &liveISOState{}
	store := &fakeStore{}
	p := testProvisioner(t, liveISOService(t, state), store)
	ctx := t.Context()

	// First pass, the host is off, so the media goes in and the machine starts.
	result, err := p.Provision(ctx, liveISOData(testISO), false)
	if err != nil {
		t.Fatalf("Provision: %v", err)
	}

	if !result.Dirty || result.ErrorMessage != "" {
		t.Fatalf("result = %+v, want dirty while the install is in flight", result)
	}

	if !state.Inserted || state.Image != testISO {
		t.Errorf("media = (inserted %v, image %q), want the ISO inserted", state.Inserted, state.Image)
	}

	// A once only override, so the reboot ending the install lands on disk.
	want := map[string]any{
		"BootSourceOverrideTarget":  "Cd",
		"BootSourceOverrideEnabled": "Once",
		"BootSourceOverrideMode":    "UEFI",
	}

	for key, wantValue := range want {
		if got := state.Boot[key]; got != wantValue {
			t.Errorf("boot[%s] = %v, want %v", key, got, wantValue)
		}
	}

	if len(state.Resets) != 1 || state.Resets[0] != "On" {
		t.Errorf("resets = %v, want a single power on", state.Resets)
	}

	if store.cleared != 1 {
		t.Errorf("report clears = %d, want the previous verdict dropped once", store.cleared)
	}

	// Second pass, booted but nothing reported yet, so keep waiting.
	result, err = p.Provision(ctx, liveISOData(testISO), false)
	if err != nil {
		t.Fatalf("Provision (waiting): %v", err)
	}

	if !result.Dirty || result.RequeueAfter != core.InstallPollInterval {
		t.Errorf("result = %+v, want a requeue at the poll interval", result)
	}

	if state.Ejects != 0 {
		t.Error("media was ejected before the install reported in")
	}

	// Third pass, the callback landed. The kickstart issues no power command, so
	// the host is asked to go down and the drive stays put while it does.
	store.report = &core.InstallReport{Succeeded: true}

	result, err = p.Provision(ctx, liveISOData(testISO), false)
	if err != nil {
		t.Fatalf("Provision (shutting down): %v", err)
	}

	if !result.Dirty {
		t.Errorf("result = %+v, want a requeue while the host shuts down", result)
	}

	if state.Ejects != 0 {
		t.Error("media was ejected while the host was still running, which aborts anaconda's shutdown")
	}

	if last := state.Resets[len(state.Resets)-1]; last != resetGraceful {
		t.Errorf("resets = %v, want a graceful shutdown so the target is unmounted", state.Resets)
	}

	// Fourth pass, the host is down, so the media comes out and it boots to disk.
	result, err = p.Provision(ctx, liveISOData(testISO), false)
	if err != nil {
		t.Fatalf("Provision (complete): %v", err)
	}

	if result.Dirty || result.ErrorMessage != "" {
		t.Errorf("result = %+v, want a clean finish once the host is down", result)
	}

	if state.Inserted || state.Ejects != 1 {
		t.Errorf("media = (inserted %v, ejects %d), want exactly one eject", state.Inserted, state.Ejects)
	}

	if !state.PowerOn {
		t.Error("the host was left off, want it booted onto its disk")
	}

	// A BMC that ignored the one time override would send the rebooted host back
	// to an empty drive, so the machine is pointed at disk explicitly.
	done := map[string]any{
		"BootSourceOverrideTarget":  "Hdd",
		"BootSourceOverrideEnabled": "Continuous",
		"BootSourceOverrideMode":    "UEFI",
	}

	for key, wantValue := range done {
		if got := state.Boot[key]; got != wantValue {
			t.Errorf("boot[%s] = %v, want %v after the install finished", key, got, wantValue)
		}
	}
}

// Swapping media under a running machine leaves it on the old boot source, so
// the host goes down before the drive is touched.
func TestProvisionPowersOffBeforeInsertingMedia(t *testing.T) {
	state := &liveISOState{PowerOn: true}
	p := testProvisioner(t, liveISOService(t, state), &fakeStore{})

	result, err := p.Provision(t.Context(), liveISOData(testISO), false)
	if err != nil {
		t.Fatalf("Provision: %v", err)
	}

	if !result.Dirty {
		t.Errorf("result = %+v, want dirty while the host powers down", result)
	}

	if state.Inserted {
		t.Error("media was inserted while the host was still running")
	}

	if len(state.Resets) != 1 || state.Resets[0] != resetForced {
		t.Errorf("resets = %v, want a single forced power off", state.Resets)
	}
}

// An emulator that never reaches its hypervisor still answers the insert, and
// the host then boots its disk and waits out the whole install timeout.
func TestProvisionRefusesWhenTheDriveStaysEmpty(t *testing.T) {
	state := &liveISOState{DropInsert: true}
	p := testProvisioner(t, liveISOService(t, state), &fakeStore{})

	result, err := p.Provision(t.Context(), liveISOData(testISO), false)
	if err != nil {
		t.Fatalf("Provision: %v", err)
	}

	if !strings.Contains(result.ErrorMessage, "drive empty") {
		t.Errorf("result = %+v, want a refusal naming the empty drive", result)
	}

	if len(state.Resets) != 0 {
		t.Errorf("resets = %v, want the host left alone rather than booted to no media", state.Resets)
	}
}

// Some BMCs fetch the image themselves and report their own URL, which is not a
// failure and must not stop the install.
func TestProvisionAcceptsARewrittenImageURL(t *testing.T) {
	state := &liveISOState{RewriteImage: "file:///var/lib/bmc/cached.testISO"}
	p := testProvisioner(t, liveISOService(t, state), &fakeStore{})

	result, err := p.Provision(t.Context(), liveISOData(testISO), false)
	if err != nil {
		t.Fatalf("Provision: %v", err)
	}

	if result.ErrorMessage != "" {
		t.Errorf("result = %+v, want the install to proceed despite the rewritten URL", result)
	}

	if !state.PowerOn {
		t.Error("the host was never powered on")
	}
}

// The override is what points the host at the drive, so a BMC that accepts the
// write and keeps booting the disk has to be caught before the host comes up.
func TestProvisionRefusesWhenTheBootOverrideIsIgnored(t *testing.T) {
	state := &liveISOState{DropBoot: true}
	p := testProvisioner(t, liveISOService(t, state), &fakeStore{})

	result, err := p.Provision(t.Context(), liveISOData(testISO), false)
	if err != nil {
		t.Fatalf("Provision: %v", err)
	}

	if !strings.Contains(result.ErrorMessage, "boot override") {
		t.Errorf("result = %+v, want a refusal naming the ignored override", result)
	}

	if state.PowerOn {
		t.Error("the host was powered on despite the override never taking")
	}
}

// A reinstall must not read the previous run's report and declare victory
// before anaconda has even booted.
func TestProvisionClearsAStaleCallback(t *testing.T) {
	state := &liveISOState{}
	store := &fakeStore{report: &core.InstallReport{Succeeded: true}}
	p := testProvisioner(t, liveISOService(t, state), store)

	result, err := p.Provision(t.Context(), liveISOData(testISO), false)
	if err != nil {
		t.Fatalf("Provision: %v", err)
	}

	if !result.Dirty {
		t.Fatalf("result = %+v, want dirty, the install has only just started", result)
	}

	if store.report != nil {
		t.Error("the previous install's callback survived the start of a new one")
	}

	if state.Ejects != 0 {
		t.Error("media was ejected on the strength of a stale callback")
	}
}

// An install that never reports has to fail rather than requeue forever, or the
// host sits in provisioning with nothing to show for it.
func TestProvisionTimesOutWaitingForCallback(t *testing.T) {
	state := &liveISOState{PowerOn: true, Inserted: true, Image: testISO}
	store := &fakeStore{started: time.Now().Add(-2 * time.Hour)}
	p := testProvisioner(t, liveISOService(t, state), store)

	result, err := p.Provision(t.Context(), liveISOData(testISO), false)
	if err != nil {
		t.Fatalf("Provision: %v", err)
	}

	if result.ErrorMessage == "" {
		t.Fatalf("result = %+v, want an error once the install timed out", result)
	}

	if !strings.Contains(result.ErrorMessage, "did not report completion") {
		t.Errorf("ErrorMessage = %q, want it to name the timeout", result.ErrorMessage)
	}
}

// With no listener nothing can ever report in, so waiting would strand the host
// until the timeout for no reason.
func TestProvisionWithoutCallbackListenerFinishesAtBoot(t *testing.T) {
	// Inserted with a start stamp, so this is an install under way rather than a
	// stale mount left behind by a failed teardown.
	state := &liveISOState{PowerOn: true, Inserted: true, Image: testISO}
	p := testProvisioner(t, liveISOService(t, state), &fakeStore{started: time.Now()}, func(p *anaconda.Provisioner) {
		p.CallbackEnabled = false
	})

	result, err := p.Provision(t.Context(), liveISOData(testISO), false)
	if err != nil {
		t.Fatalf("Provision: %v", err)
	}

	if result.Dirty || result.ErrorMessage != "" {
		t.Errorf("result = %+v, want a clean finish when nothing can report in", result)
	}
}

// Writing a disk image needs an agent on the host, which out of band Redfish
// cannot run, so it has to be refused rather than silently reported done.
func TestProvisionRefusesNonLiveISO(t *testing.T) {
	qcow2 := "qcow2"

	cases := map[string]provisioner.ProvisionData{
		"no image":   {},
		"disk image": {Image: metal3api.Image{URL: "http://images.example/debian.qcow2", DiskFormat: &qcow2}},
		"no format":  {Image: metal3api.Image{URL: "http://images.example/debian.img"}},
	}

	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			state := &liveISOState{}
			p := testProvisioner(t, liveISOService(t, state), &fakeStore{})

			result, err := p.Provision(t.Context(), data, false)
			if err != nil {
				t.Fatalf("Provision: %v", err)
			}

			if !strings.Contains(result.ErrorMessage, "not supported") {
				t.Errorf("ErrorMessage = %q, want an unsupported report", result.ErrorMessage)
			}

			if state.Inserted {
				t.Error("media was inserted for an image that cannot be deployed")
			}
		})
	}
}

// A deprovisioned host must stop booting the installer, otherwise it reinstalls
// itself on the next power cycle.
func TestDeprovisionEjectsMediaAndForgetsTheInstall(t *testing.T) {
	state := &liveISOState{Inserted: true, Image: testISO}
	store := &fakeStore{report: &core.InstallReport{Succeeded: true}, started: time.Now()}
	p := testProvisioner(t, liveISOService(t, state), store)

	result, err := p.Deprovision(t.Context(), false, "")
	if err != nil {
		t.Fatalf("Deprovision: %v", err)
	}

	if result.Dirty || result.ErrorMessage != "" {
		t.Errorf("result = %+v, want a clean deprovision", result)
	}

	if state.Inserted || state.Ejects != 1 {
		t.Errorf("media = (inserted %v, ejects %d), want exactly one eject", state.Inserted, state.Ejects)
	}

	if store.cleared != 1 || store.report != nil {
		t.Errorf("install state was not forgotten, clears = %d report = %+v", store.cleared, store.report)
	}
}

// Teardown must not depend on the BMC, otherwise a host with a broken address
// keeps its finalizer and cannot be deleted.
func TestTeardownIgnoresBrokenBMC(t *testing.T) {
	state := &liveISOState{}
	p := testProvisioner(t, liveISOService(t, state), &fakeStore{})
	p.HostData.BMCAddress = "not-a-url"

	ctx := t.Context()

	cases := map[string]func() (provisioner.Result, error){
		"deprovision": func() (provisioner.Result, error) { return p.Deprovision(ctx, false, "") },
		"delete":      func() (provisioner.Result, error) { return p.Delete(ctx) },
		"detach":      func() (provisioner.Result, error) { return p.Detach(ctx, false) },
		"power off":   func() (provisioner.Result, error) { return p.PowerOff(ctx, metal3api.RebootModeSoft, false, "") },
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

// Register claims the host by UID and probes the BMC once. A steady state
// reconcile must cost no BMC call at all, which is the whole point of the skip.
func TestRegisterProbesOnceThenGoesQuiet(t *testing.T) {
	var events []string

	state := &liveISOState{}
	p := testProvisioner(t, liveISOService(t, state), &fakeStore{}, func(p *anaconda.Provisioner) {
		p.Publisher = func(reason, message string) {
			events = append(events, reason+" "+message)
		}
	})
	p.HostData.ProvisionerID = ""

	result, provID, err := p.Register(t.Context(), provisioner.ManagementAccessData{}, false, false)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	if result.Dirty || provID != testUID {
		t.Errorf("result = %+v, provID = %q, want a clean claim of %q", result, provID, testUID)
	}

	if len(events) != 1 || !strings.HasPrefix(events[0], "Registered") {
		t.Errorf("events = %v, want exactly one Registered", events)
	}

	// Claimed now, so a later pass must not reach the BMC. noBMC fails on any
	// request, so this is the assertion.
	quiet := testProvisioner(t, noBMC(t), &fakeStore{})
	quiet.HostData.ProvisionerID = testUID

	if _, id, rerr := quiet.Register(t.Context(), provisioner.ManagementAccessData{}, false, false); rerr != nil || id != testUID {
		t.Errorf("steady state Register = (%q, %v), want the same id and no BMC call", id, rerr)
	}
}

// Rotated credentials have to be proven against the BMC before the host is
// trusted again, so the probe runs even though the host is already claimed.
func TestRegisterReprobesOnChangedCredentials(t *testing.T) {
	state := &liveISOState{}
	p := testProvisioner(t, liveISOService(t, state), &fakeStore{})
	p.HostData.ProvisionerID = testUID

	if _, _, err := p.Register(t.Context(), provisioner.ManagementAccessData{}, true, false); err != nil {
		t.Fatalf("Register with changed credentials: %v", err)
	}
}

// Without a UID there is nothing to claim the host by, and inventing one would
// let two provisioners believe they own the same machine.
func TestRegisterRefusesAHostWithNoUID(t *testing.T) {
	p := testProvisioner(t, noBMC(t), &fakeStore{})
	p.HostData.ProvisionerID = ""
	p.HostData.ObjectMeta.UID = ""

	_, _, err := p.Register(t.Context(), provisioner.ManagementAccessData{}, false, false)
	if err == nil {
		t.Fatal("Register accepted a host with no UID")
	}

	if !strings.Contains(err.Error(), "no UID") {
		t.Errorf("error = %v, want it to name the missing UID", err)
	}
}

// A host that ignores the graceful request would hold provisioning open for
// good, so the install budget is also the deadline for it to go down.
func TestShutDownAfterInstallForcesAHostThatWillNotGo(t *testing.T) {
	state := &liveISOState{PowerOn: true, Inserted: true, Image: testISO}
	store := &fakeStore{
		report:  &core.InstallReport{Succeeded: true},
		started: time.Now().Add(-2 * time.Hour),
	}

	p := testProvisioner(t, liveISOService(t, state), store)

	if _, err := p.Provision(t.Context(), liveISOData(testISO), false); err != nil {
		t.Fatalf("Provision: %v", err)
	}

	if len(state.Resets) != 1 || state.Resets[0] != resetForced {
		t.Errorf("resets = %v, want the host cut off once its budget is spent", state.Resets)
	}

	// Cutting power is not finishing. The drive stays until the host reads Off.
	if state.Ejects != 0 {
		t.Errorf("ejects = %d, want the media left until the host is actually down", state.Ejects)
	}
}

// Inside the budget the host is asked politely, so anaconda can unmount.
func TestShutDownAfterInstallAsksFirst(t *testing.T) {
	state := &liveISOState{PowerOn: true, Inserted: true, Image: testISO}
	store := &fakeStore{
		report:  &core.InstallReport{Succeeded: true},
		started: time.Now(),
	}

	p := testProvisioner(t, liveISOService(t, state), store)

	if _, err := p.Provision(t.Context(), liveISOData(testISO), false); err != nil {
		t.Fatalf("Provision: %v", err)
	}

	if len(state.Resets) != 1 || state.Resets[0] != resetGraceful {
		t.Errorf("resets = %v, want a graceful shutdown while the budget holds", state.Resets)
	}
}

// The soft versus forced decision is the difference between asking a machine to
// shut down and cutting its power, so every arm is pinned.
func TestPowerOffChoosesTheResetType(t *testing.T) {
	cases := map[string]struct {
		mode      metal3api.RebootMode
		wantReset string
		powerOn   bool
		force     bool
		wantDirty bool
	}{
		"soft asks the OS":         {mode: metal3api.RebootModeSoft, powerOn: true, wantReset: resetGraceful, wantDirty: true},
		"soft with force cuts it":  {mode: metal3api.RebootModeSoft, powerOn: true, force: true, wantReset: resetForced, wantDirty: true},
		"hard cuts it":             {mode: metal3api.RebootModeHard, powerOn: true, wantReset: resetForced, wantDirty: true},
		"already off does nothing": {mode: metal3api.RebootModeSoft, powerOn: false, wantReset: ""},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			state := &liveISOState{PowerOn: tc.powerOn}
			p := testProvisioner(t, liveISOService(t, state), &fakeStore{})

			result, err := p.PowerOff(t.Context(), tc.mode, tc.force, "")
			if err != nil {
				t.Fatalf("PowerOff: %v", err)
			}

			if result.Dirty != tc.wantDirty {
				t.Errorf("dirty = %v, want %v", result.Dirty, tc.wantDirty)
			}

			if tc.wantReset == "" {
				if len(state.Resets) != 0 {
					t.Errorf("resets = %v, want an off host left alone", state.Resets)
				}

				return
			}

			if len(state.Resets) != 1 || state.Resets[0] != tc.wantReset {
				t.Errorf("resets = %v, want exactly one %s", state.Resets, tc.wantReset)
			}
		})
	}
}

// A host already on its way down must not be hit with a second reset, which on
// some BMCs cancels the shutdown it is in the middle of.
func TestPowerOffWaitsOutAHostAlreadyPoweringOff(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case rootPath:
			_, _ = fmt.Fprint(w, `{"@odata.id":"/redfish/v1/","Systems":{"@odata.id":"/redfish/v1/Systems"}}`)
		case systemPath:
			_, _ = fmt.Fprintf(w, `{"@odata.id":%q,"Id":"1","PowerState":"PoweringOff"}`, systemPath)
		default:
			t.Errorf("unexpected request to %s, want no reset while the host is already going down", r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)

	p := testProvisioner(t, noBMC(t), &fakeStore{}, func(p *anaconda.Provisioner) {
		p.HostData.BMCAddress = "redfish-virtualmedia+http://" + strings.TrimPrefix(srv.URL, "http://") + systemPath
	})

	result, err := p.PowerOff(t.Context(), metal3api.RebootModeSoft, false, "")
	if err != nil {
		t.Fatalf("PowerOff: %v", err)
	}

	if !result.Dirty {
		t.Errorf("result = %+v, want a requeue while the host finishes going down", result)
	}
}

// BMO hands over empty credentials in StateNone, StateUnmanaged and during
// deletion, then calls GetHealth anyway. The probe must not need them.
func TestGetHealthWithoutCredentials(t *testing.T) {
	var events []string

	state := &liveISOState{}
	p := testProvisioner(t, liveISOService(t, state), &fakeStore{}, func(p *anaconda.Provisioner) {
		p.HostData.BMCCredentials = bmc.Credentials{}
		p.Publisher = func(reason, message string) {
			events = append(events, reason+" "+message)
		}
	})

	if got := p.GetHealth(t.Context()); got != provisioner.HealthOK {
		t.Errorf("GetHealth = %q, want a reachable BMC to report healthy without credentials", got)
	}

	// One event per reconcile is what made this unusable in the first place.
	for _, e := range events {
		if strings.HasPrefix(e, "HealthCheckError") {
			t.Errorf("published %q, want the condition to carry this rather than an event", e)
		}
	}
}

// A BMC that cannot be reached at all is the one case worth reporting.
func TestGetHealthReportsAnUnreachableBMC(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	srv.Close()

	p := testProvisioner(t, noBMC(t), &fakeStore{}, func(p *anaconda.Provisioner) {
		p.HostData.BMCAddress = "redfish-virtualmedia+http://" + strings.TrimPrefix(srv.URL, "http://") + systemPath
	})

	if got := p.GetHealth(t.Context()); got != "" {
		t.Errorf("GetHealth = %q, want unknown when the BMC never answered", got)
	}
}

// A teardown that could not reach the BMC leaves the ISO mounted. The next
// provision has to eject and start over, not wait out a callback for no install.
func TestProvisionEjectsAStaleMountAndStartsOver(t *testing.T) {
	state := &liveISOState{Inserted: true, Image: "http://boot.example/previous.iso"}
	store := &fakeStore{}
	p := testProvisioner(t, liveISOService(t, state), store)

	result, err := p.Provision(t.Context(), liveISOData(testISO), false)
	if err != nil {
		t.Fatalf("Provision: %v", err)
	}

	if !result.Dirty {
		t.Errorf("result = %+v, want the install to have started rather than waited", result)
	}

	if state.Ejects != 1 {
		t.Errorf("ejects = %d, want the stale image pulled out exactly once", state.Ejects)
	}

	if state.Image != testISO {
		t.Errorf("image = %q, want the requested ISO in the drive", state.Image)
	}

	if store.marks != 1 {
		t.Errorf("install start marks = %d, want the clock started once", store.marks)
	}
}

// A BMC that reports back its own rewritten URL must not look like somebody
// else's image, or every pass power cycles the machine anaconda is installing on.
func TestProvisionDoesNotRestartOnARewrittenImageURL(t *testing.T) {
	state := &liveISOState{RewriteImage: "http://bmc-cache.local/copy.iso"}
	store := &fakeStore{}
	p := testProvisioner(t, liveISOService(t, state), store)
	ctx := t.Context()

	if _, err := p.Provision(ctx, liveISOData(testISO), false); err != nil {
		t.Fatalf("Provision: %v", err)
	}

	// Three more passes with the install still in flight.
	for range 3 {
		if _, err := p.Provision(ctx, liveISOData(testISO), false); err != nil {
			t.Fatalf("Provision (waiting): %v", err)
		}
	}

	if len(state.Resets) != 1 || state.Resets[0] != "On" {
		t.Errorf("resets = %v, want a single power on rather than a reinstall loop", state.Resets)
	}

	if state.Ejects != 0 {
		t.Errorf("ejects = %d, want the drive left alone while the install runs", state.Ejects)
	}

	if store.cleared != 1 {
		t.Errorf("report clears = %d, want the verdict cleared once, not once per pass", store.cleared)
	}
}

// BeginInstall refuses before the machine is touched when a field it needs is
// missing, since a wipe is not recoverable and each refusal has to name it.
func TestProvisionRefusesWithoutItsPreconditions(t *testing.T) {
	cases := map[string]struct {
		store   *fakeStore
		wantErr string
		noMAC   bool
	}{
		"no boot MAC":          {store: &fakeStore{}, noMAC: true, wantErr: "bootMACAddress"},
		"no kickstart":         {store: &fakeStore{noKickstart: true}, wantErr: "no kickstart"},
		"no root device hints": {store: &fakeStore{noRootHints: true}, wantErr: "no root device hints"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			// Running, so a guard that fires late would show up as a power action.
			state := &liveISOState{PowerOn: true}
			p := testProvisioner(t, liveISOService(t, state), tc.store)

			if tc.noMAC {
				p.HostData.BootMACAddress = ""
			}

			result, err := p.Provision(t.Context(), liveISOData(testISO), false)
			if err != nil {
				t.Fatalf("Provision: %v", err)
			}

			if result.ErrorMessage == "" || result.Dirty {
				t.Fatalf("result = %+v, want a non dirty provisioning error", result)
			}

			if !strings.Contains(result.ErrorMessage, tc.wantErr) {
				t.Errorf("ErrorMessage = %q, want it to name %q", result.ErrorMessage, tc.wantErr)
			}

			if len(state.Resets) != 0 || state.Inserted {
				t.Errorf("resets = %v, inserted = %v, want the host left alone", state.Resets, state.Inserted)
			}
		})
	}
}

// A reported failure has to reach the BareMetalHost as a provisioning error.
// Reporting success instead would leave an unusable machine marked Provisioned.
func TestProvisionSurfacesAReportedFailure(t *testing.T) {
	state := &liveISOState{PowerOn: true, Inserted: true, Image: testISO}
	store := &fakeStore{
		started: time.Now(),
		report:  &core.InstallReport{Message: testFailure},
	}
	p := testProvisioner(t, liveISOService(t, state), store)

	result, err := p.Provision(t.Context(), liveISOData(testISO), false)
	if err != nil {
		t.Fatalf("Provision: %v", err)
	}

	if result.ErrorMessage == "" || result.Dirty {
		t.Fatalf("result = %+v, want a non dirty provisioning error", result)
	}

	if !strings.Contains(result.ErrorMessage, testFailure) {
		t.Errorf("ErrorMessage = %q, want the installer's own reason", result.ErrorMessage)
	}

	// The drive stays as anaconda left it so the failure can be looked at.
	if state.Ejects != 0 || !state.Inserted {
		t.Errorf("media = (inserted %v, ejects %d), want it left in place on failure", state.Inserted, state.Ejects)
	}
}

// An operator written HardwareData CR is handed straight back. Returning
// anything else would have BMO rewrite the CR from it and lose that inventory.
func TestInspectIngestsHardwareData(t *testing.T) {
	recorded := &metal3api.HardwareDetails{
		Hostname: "ingested",
		NIC:      []metal3api.NIC{{Name: "eth0", MAC: "aa:bb:cc:dd:ee:09"}},
	}

	p := testProvisioner(t, noBMC(t), &fakeStore{hardwareData: recorded})

	_, started, details, err := p.InspectHardware(t.Context(), provisioner.InspectData{}, false, false, false)
	if err != nil {
		t.Fatalf("InspectHardware: %v", err)
	}

	if details != recorded || started {
		t.Errorf("details = %+v, started = %v, want the recorded data handed straight back", details, started)
	}
}

// BMO deletes the inspect annotation only when a provisioner reports inspection
// started, and moves the host back into Inspecting forever while it is there.
func TestInspectAcknowledgesARefresh(t *testing.T) {
	p := testProvisioner(t, noBMC(t), &fakeStore{})

	_, started, details, err := p.InspectHardware(t.Context(), provisioner.InspectData{}, false, true, false)
	if err != nil {
		t.Fatalf("InspectHardware: %v", err)
	}

	if !started {
		t.Error("started = false, the refresh annotation would never be cleared")
	}

	if details != nil {
		t.Errorf("details = %+v, want the pass to only clear the annotation", details)
	}
}

// Provisioning waits for a boot MAC, so the recorded hardware data has to say
// which ones the machine has, otherwise there is nothing to choose from.
func TestInspectNamesBootMACCandidates(t *testing.T) {
	var events []string

	recorded := &metal3api.HardwareDetails{
		NIC: []metal3api.NIC{{Name: "eth0", MAC: testMAC}},
	}

	p := testProvisioner(t, noBMC(t), &fakeStore{hardwareData: recorded}, func(p *anaconda.Provisioner) {
		p.Publisher = func(reason, message string) {
			events = append(events, reason+" "+message)
		}
	})
	p.HostData.BootMACAddress = ""

	if _, _, _, err := p.InspectHardware(t.Context(), provisioner.InspectData{}, false, false, false); err != nil {
		t.Fatalf("InspectHardware: %v", err)
	}

	found := false

	for _, e := range events {
		if strings.HasPrefix(e, "BootMACRequired") && strings.Contains(e, testMAC) {
			found = true
		}
	}

	if !found {
		t.Errorf("events = %v, want one naming the recorded MAC", events)
	}
}

// Nothing discovers hardware, so a host with no HardwareData still has to leave
// Inspecting. Nil details would requeue it there forever.
func TestInspectPassesWithoutAnyHardwareData(t *testing.T) {
	p := testProvisioner(t, noBMC(t), &fakeStore{})

	result, started, details, err := p.InspectHardware(t.Context(), provisioner.InspectData{}, false, false, false)
	if err != nil {
		t.Fatalf("InspectHardware: %v", err)
	}

	if details == nil {
		t.Fatal("details = nil, which loops the host in Inspecting forever")
	}

	if details.Hostname != testHost {
		t.Errorf("hostname = %q, want the host's own name", details.Hostname)
	}

	if result.Dirty || started {
		t.Errorf("result = %+v, started = %v, want inspection to complete in one pass", result, started)
	}
}
