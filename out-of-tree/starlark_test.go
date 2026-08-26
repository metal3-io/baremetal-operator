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

package starlark

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	"github.com/s3rj1k/starlark-provisioner/starlib"
	"github.com/s3rj1k/starlark-provisioner/starscript"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// provFromScript builds a provisioner backed by the given Starlark source.
func provFromScript(t *testing.T, src string) *starlarkProvisioner {
	t.Helper()

	path := filepath.Join(t.TempDir(), "s.star")
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatalf("write script: %v", err)
	}

	globals, err := starscript.LoadScript(path, starlib.Builtins(), starlib.ThreadPrint(logr.Discard()), starlib.MaxExecutionSteps)
	if err != nil {
		t.Fatalf("load script: %v", err)
	}

	return &starlarkProvisioner{globals: globals, log: logr.Discard()}
}

// The URLs handed to hosts must match the routes Handler mounts, since the two
// are built from the same constants but in different places.
func TestProvisionerURLBuilders(t *testing.T) {
	p := &starlarkProvisioner{
		baseURL: "http://172.17.1.10:8080/",
		hostData: provisioner.HostData{
			ObjectMeta: metav1.ObjectMeta{Namespace: "metal3-system", Name: "node-1", UID: "uid-1"},
		},
	}

	if got := p.CallbackURL(); got != "http://172.17.1.10:8080/callback/metal3-system/node-1" {
		t.Errorf("CallbackURL = %q", got)
	}

	if got := p.ServeURLPrefix(); got != "http://172.17.1.10:8080/serve/uid-1/" {
		t.Errorf("ServeURLPrefix = %q", got)
	}

	// A base URL without the trailing slash must produce the same result.
	p.baseURL = "http://172.17.1.10:8080"

	if got := p.CallbackURL(); got != "http://172.17.1.10:8080/callback/metal3-system/node-1" {
		t.Errorf("CallbackURL without trailing slash = %q", got)
	}

	if got := p.ServeURLPrefix(); got != "http://172.17.1.10:8080/serve/uid-1/" {
		t.Errorf("ServeURLPrefix without trailing slash = %q", got)
	}
}

// has_capacity gates every host, so a missing key must fail loudly rather than
// read as false and silently stall provisioning.
func TestHasCapacity(t *testing.T) {
	ok, err := provFromScript(t, "def has_capacity(host):\n    return {\"has_capacity\": True}\n").
		HasCapacity(context.Background())
	if err != nil || !ok {
		t.Errorf("HasCapacity = (%v, %v), want (true, nil)", ok, err)
	}

	ok, err = provFromScript(t, "def has_capacity(host):\n    return {}\n").
		HasCapacity(context.Background())
	if err == nil {
		t.Errorf("HasCapacity = (%v, nil), want an error for the missing key", ok)
	}
}

func TestPreprovisioningImageFormats(t *testing.T) {
	cases := []struct {
		name string
		body string
		want []string
		err  bool
	}{
		{"none means no image required", "    return None\n", nil, false},
		{"list passes through", "    return [\"iso\", \"initrd\"]\n", []string{"iso", "initrd"}, false},
		{"empty list is not an error", "    return []\n", []string{}, false},
		{"a dict is a shape error", "    return {}\n", nil, true},
		{"a non string element is an error", "    return [1]\n", nil, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := provFromScript(t, "def preprovisioning_image_formats(host):\n"+tc.body)

			got, err := p.PreprovisioningImageFormats(context.Background())
			if tc.err {
				if err == nil {
					t.Fatalf("got %v, want an error", got)
				}

				return
			}

			if err != nil {
				t.Fatalf("PreprovisioningImageFormats: %v", err)
			}

			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}

			for i, w := range tc.want {
				if string(got[i]) != w {
					t.Errorf("format %d = %q, want %q", i, got[i], w)
				}
			}
		})
	}
}

// The needs registration sentinel has to become the typed error the controller
// matches on, not an error message.
func TestPowerOffNeedsRegistrationSentinel(t *testing.T) {
	p := provFromScript(t, "def power_off(host, mode, force, acm):\n"+
		"    return {\"dirty\": True, \"error\": \""+SentinelNeedsRegistration+"\"}\n")

	result, err := p.PowerOff(context.Background(), "hard", false, "")
	if !errors.Is(err, provisioner.ErrNeedsRegistration) {
		t.Fatalf("err = %v, want ErrNeedsRegistration", err)
	}

	if result.ErrorMessage != "" {
		t.Errorf("ErrorMessage = %q, want the sentinel stripped", result.ErrorMessage)
	}
}

func TestGetDataImageStatus(t *testing.T) {
	attached, err := provFromScript(t, "def get_data_image_status(host):\n    return {\"attached\": True}\n").
		GetDataImageStatus(context.Background())
	if err != nil || !attached {
		t.Errorf("GetDataImageStatus = (%v, %v), want (true, nil)", attached, err)
	}

	// A node reserved by another task is a typed retry, not a failure.
	_, err = provFromScript(t, "def get_data_image_status(host):\n    return {\"error\": \""+SentinelNodeBusy+"\"}\n").
		GetDataImageStatus(context.Background())
	if !errors.Is(err, provisioner.ErrNodeIsBusy) {
		t.Errorf("err = %v, want ErrNodeIsBusy", err)
	}

	// A missing key would read as false and leak the attached media.
	if _, err = provFromScript(t, "def get_data_image_status(host):\n    return {}\n").
		GetDataImageStatus(context.Background()); err == nil {
		t.Error("a missing attached key must be an error")
	}
}

func TestAttachAndDetachDataImage(t *testing.T) {
	p := provFromScript(t, "def attach_data_image(host, url):\n    return None\n"+
		"def detach_data_image(host):\n    return None\n")

	if err := p.AttachDataImage(context.Background(), "http://x/img.iso"); err != nil {
		t.Errorf("AttachDataImage: %v", err)
	}

	if err := p.DetachDataImage(context.Background()); err != nil {
		t.Errorf("DetachDataImage: %v", err)
	}

	// A script level failure has to propagate rather than be swallowed.
	bad := provFromScript(t, "def attach_data_image(host, url):\n    fail(\"no media slot\")\n")
	if err := bad.AttachDataImage(context.Background(), "http://x/img.iso"); err == nil {
		t.Error("a failing attach_data_image must return an error")
	}
}

func TestGetFirmwareSettings(t *testing.T) {
	// None means the script does not implement firmware settings.
	settings, schema, err := provFromScript(t, "def get_firmware_settings(host, include_schema):\n    return None\n").
		GetFirmwareSettings(context.Background(), true)
	if err != nil || settings != nil || schema != nil {
		t.Errorf("None = (%v, %v, %v), want all nil", settings, schema, err)
	}

	src := "def get_firmware_settings(host, include_schema):\n" +
		"    return {\"settings\": {\"boot\": \"uefi\"}, \"schema\": {\"boot\": {\"attribute_type\": \"Enumeration\"}}}\n"

	// Without the schema requested the settings still come back, schema does not.
	settings, schema, err = provFromScript(t, src).GetFirmwareSettings(context.Background(), false)
	if err != nil {
		t.Fatalf("GetFirmwareSettings: %v", err)
	}

	if settings["boot"] != "uefi" {
		t.Errorf("settings = %v, want boot uefi", settings)
	}

	if schema != nil {
		t.Errorf("schema = %v, want nil when not requested", schema)
	}

	settings, schema, err = provFromScript(t, src).GetFirmwareSettings(context.Background(), true)
	if err != nil {
		t.Fatalf("GetFirmwareSettings with schema: %v", err)
	}

	if len(schema) != 1 || schema["boot"].AttributeType != "Enumeration" {
		t.Errorf("schema = %v, want the boot attribute", schema)
	}

	// Requesting the schema must not cost the settings.
	if settings["boot"] != "uefi" {
		t.Errorf("settings = %v, want them alongside the schema", settings)
	}

	// Success with nothing set still returns a non nil map so callers can tell
	// an empty result from an unrequested one.
	settings, _, err = provFromScript(t, "def get_firmware_settings(host, include_schema):\n    return {}\n").
		GetFirmwareSettings(context.Background(), false)
	if err != nil || settings == nil {
		t.Errorf("empty result = (%v, %v), want a non nil map", settings, err)
	}
}

// actionDeleting reads only the error and ignores the Result, so a script failure
// must become an error or the finalizer is dropped while the node still exists.
func TestDeleteAndDetachSurfaceScriptErrors(t *testing.T) {
	cases := map[string]func(*starlarkProvisioner) error{
		"delete": func(p *starlarkProvisioner) error {
			_, err := p.Delete(context.Background())
			return err
		},
		"detach": func(p *starlarkProvisioner) error {
			_, err := p.Detach(context.Background(), false)
			return err
		},
	}

	for name, call := range cases {
		t.Run(name, func(t *testing.T) {
			p := provFromScript(t, "def "+name+"(host, force=None):\n"+
				"    return {\"error\": \"BMC refused, node still registered\"}\n")

			err := call(p)
			if err == nil {
				t.Fatal("a failed teardown reported success")
			}

			if !strings.Contains(err.Error(), "still registered") {
				t.Errorf("err = %v, want the script message", err)
			}
		})
	}

	// The needs registration sentinel still means the node is already gone.
	p := provFromScript(t, "def delete(host):\n    return {\"error\": \""+SentinelNeedsRegistration+"\"}\n")
	if _, err := p.Delete(context.Background()); err != nil {
		t.Errorf("the needs registration sentinel became an error: %v", err)
	}
}

// The subscription controller discards the Result entirely, so the same applies.
func TestBMCEventSubscriptionSurfacesScriptErrors(t *testing.T) {
	p := provFromScript(t, "def remove_bmc_event_subscription(host, sub):\n"+
		"    return {\"error\": \"redfish DELETE failed 500\"}\n")

	if _, err := p.RemoveBMCEventSubscriptionForNode(context.Background(), metal3api.BMCEventSubscription{}); err == nil {
		t.Error("a failed unsubscribe reported success, the CR would be deleted with the subscription live")
	}

	add := provFromScript(t, "def add_bmc_event_subscription(host, sub):\n"+
		"    return {\"error\": \"not supported\"}\n")

	if _, err := add.AddBMCEventSubscriptionForNode(context.Background(), &metal3api.BMCEventSubscription{}, nil); err == nil {
		t.Error("a failed subscribe reported success")
	}
}

// print() goes to stderr by default, the one channel the masking layer does not
// wrap, and the host dict carries the BMC password in plaintext.
func TestScriptPrintIsMaskedAndLogged(t *testing.T) {
	sink := &captureSink{}

	p := provFromScript(t, "def get_health(host):\n    print(\"creds \" + host[\"BMCCredentials\"][\"Password\"])\n    return \"OK\"\n")
	p.log = MaskingLogger(logr.New(sink), "s3cret")
	p.hostData.BMCCredentials.Password = "s3cret"

	if got := p.GetHealth(context.Background()); got != "OK" {
		t.Fatalf("GetHealth = %q", got)
	}

	if sink.msg == "" {
		t.Fatal("print produced no log record, it went to stderr")
	}

	if strings.Contains(sink.msg, "s3cret") {
		t.Errorf("print leaked the password, got %q", sink.msg)
	}

	if !strings.Contains(sink.msg, "***") {
		t.Errorf("print was not masked, got %q", sink.msg)
	}
}

// A runaway script must be stopped rather than pinning a reconcile worker.
func TestRunawayScriptIsBounded(t *testing.T) {
	p := provFromScript(t, "def get_data_image_status(host):\n"+
		"    total = 0\n"+
		"    for i in range(1000000000):\n"+
		"        total += i\n"+
		"    return {\"attached\": False}\n")
	// A small budget keeps the test quick, the default would take about a minute.
	p.maxSteps = 1_000_000

	done := make(chan error, 1)

	go func() {
		_, err := p.GetDataImageStatus(context.Background())
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("the runaway script completed, the budget did not apply")
		}

		// Assert it is the budget, not some unrelated failure that would keep
		// this test green while the budget silently stopped working.
		if !strings.Contains(err.Error(), "steps") {
			t.Errorf("err = %v, want the execution step budget", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the runaway script was never stopped")
	}
}

// A wrong type on a load bearing key would read as the zero value, so the
// controller would advance the host while the work is still in flight.
func TestWrongTypedResultKeysAreRejected(t *testing.T) {
	cases := map[string]func(*starlarkProvisioner) error{
		"dirty": func(p *starlarkProvisioner) error {
			_, err := p.PowerOn(context.Background(), false)
			return err
		},
		"started": func(p *starlarkProvisioner) error {
			_, _, err := p.Prepare(context.Background(), provisioner.PrepareData{}, false, false)
			return err
		},
	}

	srcs := map[string]string{
		"dirty":   "def power_on(host, force):\n    return {\"dirty\": 1}\n",
		"started": "def prepare(host, data, u, r):\n    return {\"started\": \"yes\"}\n",
	}

	for key, call := range cases {
		t.Run(key, func(t *testing.T) {
			err := call(provFromScript(t, srcs[key]))
			if err == nil {
				t.Fatalf("a non bool %q was accepted", key)
			}

			if !strings.Contains(err.Error(), key) {
				t.Errorf("err = %v, want it to name %q", err, key)
			}
		})
	}
}

// attach_data_image had no error channel at all, so a failure read as success and
// the controller recorded the image as attached.
func TestAttachDataImageSurfacesScriptError(t *testing.T) {
	p := provFromScript(t, "def attach_data_image(host, url):\n"+
		"    return {\"error\": \"no free virtual media slot\"}\n")

	err := p.AttachDataImage(context.Background(), "http://x/img.iso")
	if err == nil || !strings.Contains(err.Error(), "no free virtual media slot") {
		t.Errorf("err = %v, want the script failure", err)
	}

	// None is still a valid success, the method is void shaped.
	ok := provFromScript(t, "def attach_data_image(host, url):\n    return None\n")
	if err := ok.AttachDataImage(context.Background(), "http://x/img.iso"); err != nil {
		t.Errorf("a None return became an error: %v", err)
	}
}

// A route the script registers during teardown must survive the same pass, and a
// pass that is still dirty must not have its routes cleared out from under it.
func TestTeardownClearsRoutesOnlyWhenComplete(t *testing.T) {
	cases := map[string]struct {
		body      string
		wantClear bool
	}{
		"still working keeps routes": {"    return {\"dirty\": True, \"requeue_after_seconds\": 5}\n", false},
		"complete clears routes":     {"    return {}\n", true},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cleaner := &recordingCleaner{}
			p := provFromScript(t, "def deprovision(host, restart, acm):\n"+tc.body)
			p.serveEnabled = true
			p.secretResolver = cleaner

			if _, err := p.Deprovision(context.Background(), false, ""); err != nil {
				t.Fatalf("Deprovision: %v", err)
			}

			if cleaner.cleared != tc.wantClear {
				t.Errorf("routes cleared = %v, want %v", cleaner.cleared, tc.wantClear)
			}
		})
	}
}

// recordingCleaner satisfies the serve cleanup capability and records the call.
type recordingCleaner struct {
	cleared bool
}

func (r *recordingCleaner) ReadHostSecret(_ context.Context, _, _, _ string) (string, error) {
	return "", nil
}

func (r *recordingCleaner) ReadHostSpec(_ context.Context, _, _ string) (map[string]any, error) {
	return map[string]any{}, nil
}

func (r *recordingCleaner) ReadHostStatus(_ context.Context, _, _ string) (map[string]any, error) {
	return map[string]any{}, nil
}

func (r *recordingCleaner) DeleteServeHost(_ context.Context, _, _ string) error {
	r.cleared = true

	return nil
}

// Serving is scoped to the operator namespace, so a host outside it must see
// serve_enabled() as false rather than fail inside serve_register.
func TestServeDisabledForForeignNamespaceHost(t *testing.T) {
	t.Setenv("POD_NAMESPACE", "metal3-system")

	f := &starlarkProvisionerFactory{log: logr.Discard(), serveEnabled: true}

	local, err := f.NewProvisioner(context.Background(), provisioner.HostData{
		ObjectMeta: metav1.ObjectMeta{Namespace: "metal3-system", Name: "local"},
	}, nil)
	if err != nil {
		t.Fatalf("NewProvisioner: %v", err)
	}

	localProv, ok := local.(*starlarkProvisioner)
	if !ok {
		t.Fatalf("NewProvisioner returned %T", local)
	}

	if !localProv.serveEnabled {
		t.Error("serving disabled for a host in the operator namespace")
	}

	foreign, err := f.NewProvisioner(context.Background(), provisioner.HostData{
		ObjectMeta: metav1.ObjectMeta{Namespace: "tenant-a", Name: "remote"},
	}, nil)
	if err != nil {
		t.Fatalf("NewProvisioner: %v", err)
	}

	foreignProv, ok := foreign.(*starlarkProvisioner)
	if !ok {
		t.Fatalf("NewProvisioner returned %T", foreign)
	}

	if foreignProv.serveEnabled {
		t.Error("serving stayed enabled outside the operator namespace, so the script would hard error")
	}
}
