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
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-logr/logr"
)

func TestSynthesizeToken(t *testing.T) {
	base := SynthesizeToken("ns", "n1", "user", "pass")

	if len(base) != 64 {
		t.Errorf("token length = %d, want 64 hex chars", len(base))
	}

	if base != SynthesizeToken("ns", "n1", "user", "pass") {
		t.Error("token is not deterministic")
	}

	if base == SynthesizeToken("ns", "n1", "user", "other") {
		t.Error("a different password must change the token")
	}

	if base == SynthesizeToken("ns", "n2", "user", "pass") {
		t.Error("a different host must change the token")
	}
}

func TestBearerToken(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/callback/ns/n", nil)
	req.Header.Set("Authorization", "Bearer abc")

	if got := BearerToken(req); got != "abc" {
		t.Errorf("BearerToken = %q, want abc", got)
	}

	bare := httptest.NewRequest(http.MethodPost, "/callback/ns/n", nil)
	if got := BearerToken(bare); got != "" {
		t.Errorf("missing header should be empty, got %q", got)
	}
}

// stubResolver stands in for KubeHostResolver so the handler test needs no client.
type stubResolver struct {
	user    string
	pass    string
	err     error
	written []byte
}

func (s *stubResolver) BMCCredentials(_ context.Context, _, _ string) (string, string, error) {
	return s.user, s.pass, s.err
}

func (s *stubResolver) WriteCallback(_ context.Context, _, _ string, body []byte, _ string) error {
	s.written = body

	return nil
}

func TestCallbackHandler(t *testing.T) {
	stub := &stubResolver{user: "user", pass: "pass"}
	srv := &PluginServer{Resolver: stub, Log: logr.Discard()}
	token := SynthesizeToken("ns", "n1", "user", "pass")

	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodPost, "/callback/ns/n1", strings.NewReader(`{"done":true}`))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("valid token: got %d, body %s", rec.Code, rec.Body.String())
	}

	if string(stub.written) != `{"done":true}` {
		t.Errorf("persisted body = %q", stub.written)
	}

	stub.written = nil
	bad := httptest.NewRequest(http.MethodPost, "/callback/ns/n1", strings.NewReader("x"))
	bad.Header.Set("Authorization", "Bearer wrong")
	badRec := httptest.NewRecorder()
	handler.ServeHTTP(badRec, bad)

	if badRec.Code != http.StatusUnauthorized {
		t.Errorf("invalid token: got %d, want 401", badRec.Code)
	}

	if stub.written != nil {
		t.Error("invalid token must not persist a body")
	}
}

func TestCallbackHandlerUnknownHost(t *testing.T) {
	stub := &stubResolver{err: errors.New("no such host")}
	srv := &PluginServer{Resolver: stub, Log: logr.Discard()}

	req := httptest.NewRequest(http.MethodPost, "/callback/ns/n1", strings.NewReader("x"))
	req.Header.Set("Authorization", "Bearer whatever")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	// An unknown host answers as unauthorized so it is not an existence oracle.
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("unknown host: got %d, want 401", rec.Code)
	}

	if stub.written != nil {
		t.Error("unknown host must not persist a body")
	}
}

func TestCallbackStartBindError(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	// Binding the already held address must fail synchronously so callbacks stay off.
	srv := &PluginServer{Config: CallbackConfig{Addr: ln.Addr().String()}, Log: logr.Discard()}
	if err := srv.Start(); err == nil {
		t.Error("start on an in use address should return an error")
	}
}

func TestPluginServerStartTLSHalfConfig(t *testing.T) {
	certOnly := &PluginServer{Config: CallbackConfig{Addr: "127.0.0.1:0", TLSCert: "/tls/cert"}, Log: logr.Discard()}
	if err := certOnly.Start(); err == nil {
		t.Error("start with only a TLS cert should fail closed")
	}

	keyOnly := &PluginServer{Config: CallbackConfig{Addr: "127.0.0.1:0", TLSKey: "/tls/key"}, Log: logr.Discard()}
	if err := keyOnly.Start(); err == nil {
		t.Error("start with only a TLS key should fail closed")
	}
}

func TestCallbackHandlerMethodNotAllowed(t *testing.T) {
	srv := &PluginServer{Resolver: &stubResolver{user: "user", pass: "pass"}, Log: logr.Discard()}

	req := httptest.NewRequest(http.MethodGet, "/callback/ns/n1", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	// The POST only route rejects other methods before any credential lookup.
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET: got %d, want 405", rec.Code)
	}
}

func TestCallbackHandlerOversizedBody(t *testing.T) {
	stub := &stubResolver{user: "user", pass: "pass"}
	srv := &PluginServer{Resolver: stub, Log: logr.Discard()}
	token := SynthesizeToken("ns", "n1", "user", "pass")

	big := strings.Repeat("a", CallbackMaxBodyBytes+1)
	req := httptest.NewRequest(http.MethodPost, "/callback/ns/n1", strings.NewReader(big))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("oversized body: got %d, want 400", rec.Code)
	}

	if stub.written != nil {
		t.Error("oversized body must not persist")
	}
}

func TestLoadCallbackConfig(t *testing.T) {
	t.Setenv("STARLARK_CALLBACK_ADDR", ":9080")
	t.Setenv("STARLARK_CALLBACK_BASE_URL", "http://cb:9080")
	t.Setenv("STARLARK_CALLBACK_TLS_CERT", "/tls/cert")
	t.Setenv("STARLARK_CALLBACK_TLS_KEY", "/tls/key")

	cfg := LoadCallbackConfig()
	if cfg.Addr != ":9080" || cfg.BaseURL != "http://cb:9080" || cfg.TLSCert != "/tls/cert" || cfg.TLSKey != "/tls/key" {
		t.Errorf("LoadCallbackConfig = %+v", cfg)
	}

	if !cfg.Enabled() {
		t.Error("a config with an address should be enabled")
	}
}

func TestCallbackConfigDisabledWithoutAddr(t *testing.T) {
	t.Setenv("STARLARK_CALLBACK_ADDR", "")

	if LoadCallbackConfig().Enabled() {
		t.Error("a config without an address must be disabled")
	}
}
