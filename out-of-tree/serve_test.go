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
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-logr/logr"
	"github.com/s3rj1k/starlark-provisioner/starlib"
)

// stubServeResolver returns a canned registration and ConfigMap value, no client.
type stubServeResolver struct {
	reg      starlib.ServeRegistration
	regNS    string
	regFound bool
	readErr  error
	content  string
	cmFound  bool
	cmErr    error
}

func (s stubServeResolver) ReadServe(_ context.Context, _, _ string) (starlib.ServeRegistration, string, bool, error) {
	return s.reg, s.regNS, s.regFound, s.readErr
}

func (s stubServeResolver) ConfigMapValue(_ context.Context, _, _, _ string) (string, bool, error) {
	return s.content, s.cmFound, s.cmErr
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
