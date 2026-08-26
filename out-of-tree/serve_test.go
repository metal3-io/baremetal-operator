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
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/go-logr/logr"
	"github.com/s3rj1k/starlark-provisioner/starlib"
)

// stubServeResolver returns a canned registration and ConfigMap value, no client.
type stubServeResolver struct {
	reg      starlib.ServeRegistration
	regFound bool
	readErr  error
	content  string
	cmFound  bool
	cmErr    error
	hosts    []HostRef
	hostsErr error
}

func (s stubServeResolver) ReadServe(_ context.Context, _, _ string) (starlib.ServeRegistration, bool, error) {
	return s.reg, s.regFound, s.readErr
}

func (s stubServeResolver) ConfigMapValue(_ context.Context, _, _, _ string) (string, bool, error) {
	return s.content, s.cmFound, s.cmErr
}

func (s stubServeResolver) FindHostsByMAC(_ context.Context, _ []string) ([]HostRef, error) {
	return s.hosts, s.hostsErr
}

func newServeServer(res ServeResolver) *PluginServer {
	return &PluginServer{Serve: res, Log: logr.Discard()}
}

func serveGet(t *testing.T, srv *PluginServer, path string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	return rec
}

func TestServeHandlerRaw(t *testing.T) {
	srv := newServeServer(stubServeResolver{
		reg:      starlib.ServeRegistration{ConfigMap: "cm", Key: "k", Render: false},
		regFound: true, content: "hello {{ .name }}", cmFound: true,
	})

	rec := serveGet(t, srv, "/serve/uid-1/cfg")
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, body %s", rec.Code, rec.Body.String())
	}

	if rec.Body.String() != "hello {{ .name }}" {
		t.Errorf("raw body = %q, want the template verbatim", rec.Body.String())
	}
}

func TestServeHandlerRender(t *testing.T) {
	srv := newServeServer(stubServeResolver{
		reg: starlib.ServeRegistration{
			ConfigMap: "cm", Key: "k", Render: true,
			Vars: map[string]any{"disk": "/dev/sda", "token": "abc"},
		},
		regFound: true, content: "disk={{ .disk }} tok={{ .token }}", cmFound: true,
	})

	rec := serveGet(t, srv, "/serve/uid-1/cfg")
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, body %s", rec.Code, rec.Body.String())
	}

	if rec.Body.String() != "disk=/dev/sda tok=abc" {
		t.Errorf("rendered body = %q", rec.Body.String())
	}
}

func TestServeHandlerUnknownRoute(t *testing.T) {
	srv := newServeServer(stubServeResolver{regFound: false})

	if rec := serveGet(t, srv, "/serve/uid-1/missing"); rec.Code != http.StatusNotFound {
		t.Errorf("unregistered route: got %d, want 404", rec.Code)
	}
}

func TestServeHandlerConfigMapMissing(t *testing.T) {
	srv := newServeServer(stubServeResolver{
		reg:      starlib.ServeRegistration{ConfigMap: "cm", Key: "k"},
		regFound: true, cmFound: false,
	})

	if rec := serveGet(t, srv, "/serve/uid-1/cfg"); rec.Code != http.StatusNotFound {
		t.Errorf("missing configmap: got %d, want 404", rec.Code)
	}
}

func TestServeHandlerRenderError(t *testing.T) {
	srv := newServeServer(stubServeResolver{
		reg:      starlib.ServeRegistration{ConfigMap: "cm", Key: "k", Render: true, Vars: map[string]any{}},
		regFound: true, content: "{{ .nope }}", cmFound: true,
	})

	// missingkey=error turns an absent template var into a 500.
	if rec := serveGet(t, srv, "/serve/uid-1/cfg"); rec.Code != http.StatusInternalServerError {
		t.Errorf("render error: got %d, want 500", rec.Code)
	}
}

func TestRenderTemplate(t *testing.T) {
	out, err := RenderTemplate("t", "a={{ .a }} nested={{ .d.wwn }}", map[string]any{
		"a": "x",
		"d": map[string]any{"wwn": "0x123"},
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	if out != "a=x nested=0x123" {
		t.Errorf("out = %q", out)
	}

	if _, err := RenderTemplate("t", "{{ .oops ", map[string]any{}); err == nil {
		t.Error("a malformed template should return a parse error")
	}
}

func TestServeHandlerMethodNotAllowed(t *testing.T) {
	srv := newServeServer(stubServeResolver{regFound: true, content: "x", cmFound: true})

	req := httptest.NewRequest(http.MethodPost, "/serve/uid-1/cfg", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST on serve route: got %d, want 405", rec.Code)
	}
}

// ksGet issues a kickstart request carrying the MAC headers anaconda would send.
func ksGet(t *testing.T, srv *PluginServer, macs ...string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/ks/kickstart", http.NoBody)
	for i, mac := range macs {
		req.Header.Set(fmt.Sprintf("X-RHN-Provisioning-MAC-%d", i), "eth"+strconv.Itoa(i)+" "+mac)
	}

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	return rec
}

func TestKickstartServesMatchedHost(t *testing.T) {
	srv := newServeServer(stubServeResolver{
		hosts:    []HostRef{{Name: "node-1", UID: "uid-1"}},
		reg:      starlib.ServeRegistration{ConfigMap: "cm", Key: "ks.cfg", Render: true, Vars: map[string]any{"Name": "node-1"}},
		regFound: true,
		content:  "# host {{ .Name }}",
		cmFound:  true,
	})

	rec := ksGet(t, srv, "aa:bb:cc:dd:ee:01")
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, body %s", rec.Code, rec.Body.String())
	}

	if rec.Body.String() != "# host node-1" {
		t.Errorf("body = %q, want the rendered per host kickstart", rec.Body.String())
	}
}

// Every unresolved case has to answer 200 with an inert kickstart. A 404 would drop
// anaconda to an interactive installer, and a wrong body could wipe the machine.
func TestKickstartFallbackPaths(t *testing.T) {
	cases := []struct {
		name string
		res  stubServeResolver
		macs []string
	}{
		{"no MAC headers", stubServeResolver{}, nil},
		{"no host claims the MAC", stubServeResolver{}, []string{"aa:bb:cc:dd:ee:01"}},
		{
			"host has no registration",
			stubServeResolver{hosts: []HostRef{{Name: "node-1", UID: "uid-1"}}},
			[]string{"aa:bb:cc:dd:ee:01"},
		},
		{
			"registration points at a missing key",
			stubServeResolver{
				hosts:    []HostRef{{Name: "node-1", UID: "uid-1"}},
				reg:      starlib.ServeRegistration{ConfigMap: "cm", Key: "gone"},
				regFound: true,
			},
			[]string{"aa:bb:cc:dd:ee:01"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := ksGet(t, newServeServer(tc.res), tc.macs...)
			if rec.Code != http.StatusOK {
				t.Fatalf("got %d, want 200 so anaconda does not prompt", rec.Code)
			}

			if rec.Body.String() != DefaultFallbackKickstart {
				t.Errorf("body = %q, want the inert fallback", rec.Body.String())
			}
		})
	}
}

func TestKickstartLookupErrorIsNotAFallback(t *testing.T) {
	srv := newServeServer(stubServeResolver{hostsErr: errors.New("api down")})

	rec := ksGet(t, srv, "aa:bb:cc:dd:ee:01")
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("got %d, want 500 rather than silently serving a fallback", rec.Code)
	}
}
