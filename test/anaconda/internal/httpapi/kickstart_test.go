// SPDX-License-Identifier: Apache-2.0

package httpapi_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	"metal3.local/anaconda/internal/core"
	"metal3.local/anaconda/internal/httpapi"
)

// ksRequest asks for a kickstart the way anaconda does, with the MAC headers and
// nothing else to go on.
func ksRequest(t *testing.T, srv *httpapi.PluginServer, macs ...string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, httpapi.KickstartPathPrefix+"kickstart", http.NoBody)
	for i, mac := range macs {
		req.Header.Set(fmt.Sprintf("X-RHN-Provisioning-MAC-%d", i), fmt.Sprintf("eth%d %s", i, mac))
	}

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	return rec
}

// The host's own preprovisioning Secret is the kickstart, resolved at request
// time so a host is servable the moment it exists.
func TestKickstartServedFromThePreprovisioningSecret(t *testing.T) {
	stub := &stubResolver{
		uid: testUID,
		hosts: []core.HostRef{{
			Name:            testHost,
			Namespace:       "ns",
			UID:             testUID,
			BootMAC:         testMAC,
			KickstartSecret: testSecret,
		}},
		kickstart: map[string]string{testSecret: "text\nnetwork --hostname={{ .Name }}\n# {{ .CallbackURL }}\n"},
	}

	srv := &httpapi.PluginServer{
		Config:   core.Config{BaseURL: "http://bmo:8080"},
		Resolver: stub,
		Log:      logr.Discard(),
	}

	rec := ksRequest(t, srv, testMAC)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "--hostname=node-1") {
		t.Errorf("body = %q, want the host's rendered kickstart", body)
	}

	// The callback URL has to be rendered in, it is what ends the provision, and
	// the uid in the path is what keeps it unguessable.
	if !strings.Contains(body, "http://bmo:8080"+httpapi.CallbackPathPrefix+"bmh-uid/ns/node-1") {
		t.Errorf("body = %q, want the callback URL rendered in", body)
	}
}

// Anything unresolvable answers 200 with the inert fallback, never a wipe and
// never a 404, which would drop anaconda to a prompt on a live machine.
func TestKickstartServedForEveryUnresolvableRequest(t *testing.T) {
	const named = "ignoredisk --only-use=sdb\n"

	ref := func(secret string) []core.HostRef {
		return []core.HostRef{{Name: testHost, Namespace: "ns", KickstartSecret: secret}}
	}

	cases := map[string]struct {
		stub     *stubResolver
		want     string
		withMACs bool
	}{
		"host names no secret": {
			stub: &stubResolver{hosts: ref("")}, withMACs: true, want: httpapi.DefaultFallbackKickstart,
		},
		"secret is absent": {
			stub:     &stubResolver{hosts: ref("gone"), kickstart: map[string]string{}},
			withMACs: true, want: httpapi.DefaultFallbackKickstart,
		},
		"no host declares the mac": {
			stub: &stubResolver{hosts: nil}, withMACs: true, want: httpapi.DefaultFallbackKickstart,
		},
		// Rendering it would emit a storage command with nothing after it.
		"template names a disk nothing resolves": {
			stub: &stubResolver{
				hosts:     ref(testSecret),
				kickstart: map[string]string{testSecret: "clearpart --all --drives={{ .InstallDisk }}\n"},
			},
			withMACs: true, want: httpapi.DefaultFallbackKickstart,
		},
		// Without inst.ks.sendmac there is nothing to match on, and guessing a
		// host would install the wrong kickstart on a machine.
		"no MAC headers reported": {
			stub: &stubResolver{hosts: ref(testSecret)}, want: httpapi.DefaultFallbackKickstart,
		},
		// A kickstart naming its own disk predates the variable and has to keep
		// working with no default configured.
		"template names its own disk": {
			stub: &stubResolver{
				hosts:     ref(testSecret),
				kickstart: map[string]string{testSecret: named},
			},
			withMACs: true, want: named,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			srv := &httpapi.PluginServer{Config: core.Config{}, Resolver: tc.stub, Log: logr.Discard()}

			var macs []string
			if tc.withMACs {
				macs = []string{testMAC}
			}

			rec := ksRequest(t, srv, macs...)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 so anaconda does not sit at a prompt", rec.Code)
			}

			if rec.Body.String() != tc.want {
				t.Errorf("body = %q, want %q", rec.Body.String(), tc.want)
			}
		})
	}
}

// The host's own hint beats the fleet default, so one odd machine is fixed on
// its BareMetalHost rather than in the operator's config.
func TestKickstartRendersTheHostInstallDisk(t *testing.T) {
	const tmpl = "ignoredisk --only-use={{ .InstallDisk }}\n"

	cases := map[string]struct {
		hostDisk string
		want     string
	}{
		"device name": {hostDisk: testDisk, want: testDisk},
		"by-id link":  {hostDisk: "disk/by-id/wwn-0x5000c500", want: "disk/by-id/wwn-0x5000c500"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			stub := &stubResolver{
				hosts: []core.HostRef{{
					Name: testHost, Namespace: "ns", BootMAC: testMAC,
					KickstartSecret: testSecret, InstallDisk: tc.hostDisk,
				}},
				kickstart: map[string]string{testSecret: tmpl},
			}

			srv := &httpapi.PluginServer{
				Config:   core.Config{},
				Resolver: stub,
				Log:      logr.Discard(),
			}

			body := ksRequest(t, srv, testMAC).Body.String()
			if body != "ignoredisk --only-use="+tc.want+"\n" {
				t.Errorf("body = %q, want the disk resolved to %q", body, tc.want)
			}
		})
	}
}
