// SPDX-License-Identifier: Apache-2.0

package httpapi_test

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	"metal3.local/anaconda/internal/core"
	"metal3.local/anaconda/internal/httpapi"
)

const (
	testHost    = "node-1"
	testUID     = "bmh-uid"
	testMAC     = "aa:bb:cc:dd:ee:01"
	testDisk    = "vda"
	testFailure = "disk sda not found"
	testSecret  = "node-1-ks"
)

// stubResolver stands in for kube.HostResolver so the handler tests need no client.
type stubResolver struct {
	err       error
	written   *core.InstallReport
	kickstart map[string]string
	uid       string
	hosts     []core.HostRef
}

func (s *stubResolver) ReadKickstart(_ context.Context, _, secretName string) (string, bool, error) {
	v, ok := s.kickstart[secretName]

	return v, ok, nil
}

func (s *stubResolver) FindHostsByMAC(_ context.Context, _ []string) ([]core.HostRef, error) {
	return s.hosts, nil
}

func (s *stubResolver) HostUID(_ context.Context, _, _ string) (string, error) {
	return s.uid, s.err
}

func (s *stubResolver) WriteInstallReport(_ context.Context, _, _ string, report core.InstallReport) error {
	s.written = &report

	return nil
}

func TestCallbackHandler(t *testing.T) {
	stub := &stubResolver{uid: testUID}
	srv := &httpapi.PluginServer{Resolver: stub, Log: logr.Discard()}
	handler := srv.Handler()

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/callback/bmh-uid/ns/n1", strings.NewReader(`{"status":"installed"}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("matching uid: got %d, body %s", rec.Code, rec.Body.String())
	}

	if stub.written == nil || !stub.written.Succeeded {
		t.Errorf("recorded report = %+v, want a success", stub.written)
	}

	// The unguessable uid is the whole check, so a wrong one has to be refused.
	stub.written = nil
	bad := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/callback/wrong-uid/ns/n1", strings.NewReader("x"))
	badRec := httptest.NewRecorder()
	handler.ServeHTTP(badRec, bad)

	if badRec.Code != http.StatusUnauthorized {
		t.Errorf("wrong uid: got %d, want 401", badRec.Code)
	}

	if stub.written != nil {
		t.Error("a wrong uid must not persist a body")
	}
}

func TestCallbackHandlerUnknownHost(t *testing.T) {
	stub := &stubResolver{err: errors.New("no such host")}
	srv := &httpapi.PluginServer{Resolver: stub, Log: logr.Discard()}

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/callback/bmh-uid/ns/n1", strings.NewReader("x"))
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
	var lc net.ListenConfig

	ln, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	// Binding the already held address must fail synchronously so callbacks stay off.
	srv := &httpapi.PluginServer{Config: core.Config{ListenAddr: ln.Addr().String()}, Log: logr.Discard()}
	if err := srv.Start(t.Context()); err == nil {
		t.Error("start on an in use address should return an error")
	}
}

func TestCallbackHandlerOversizedBody(t *testing.T) {
	stub := &stubResolver{uid: testUID}
	srv := &httpapi.PluginServer{Resolver: stub, Log: logr.Discard()}

	big := strings.Repeat("a", httpapi.CallbackMaxBodyBytes+1)
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/callback/bmh-uid/ns/n1", strings.NewReader(big))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("oversized body: got %d, want 400", rec.Code)
	}

	if stub.written != nil {
		t.Error("oversized body must not be recorded")
	}
}

// A callback means success only when it says so, or says nothing. Anything else
// fails the host, because the alternative is marking it provisioned on a typo.
func TestParseInstallReportContract(t *testing.T) {
	cases := map[string]struct {
		body          string
		wantMessage   string
		wantSucceeded bool
	}{
		"empty body":        {body: "", wantSucceeded: true},
		"whitespace only":   {body: "   \n", wantSucceeded: true},
		"empty object":      {body: `{}`, wantMessage: "names no status"},
		"misspelled key":    {body: `{"state":"failed"}`, wantMessage: "names no status"},
		"installed":         {body: `{"status":"installed"}`, wantSucceeded: true},
		"mixed case ok":     {body: `{"status":"OK"}`, wantSucceeded: true},
		"success":           {body: `{"status":"success","host":"node-1"}`, wantSucceeded: true},
		"failed with why":   {body: `{"status":"failed","message":"disk sda not found"}`, wantMessage: testFailure},
		"failed without":    {body: `{"status":"failed"}`, wantMessage: `status "failed"`},
		"unknown status":    {body: `{"status":"borked"}`, wantMessage: `status "borked"`},
		"not json":          {body: `installation blew up`, wantMessage: "not JSON"},
		"json but a scalar": {body: `"installed"`, wantMessage: "not JSON"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := httpapi.ParseInstallReport([]byte(tc.body))

			if got.Succeeded != tc.wantSucceeded {
				t.Fatalf("Succeeded = %v (message %q), want %v", got.Succeeded, got.Message, tc.wantSucceeded)
			}

			if tc.wantMessage != "" && !strings.Contains(got.Message, tc.wantMessage) {
				t.Errorf("Message = %q, want it to mention %q", got.Message, tc.wantMessage)
			}
		})
	}
}

// A host can post up to CallbackMaxBodyBytes, none of which belongs unbounded
// in the install-message annotation, which shares a 256 KiB budget.
func TestParseInstallReportTruncatesTheMessage(t *testing.T) {
	huge := strings.Repeat("x", 300*1024)

	got := httpapi.ParseInstallReport([]byte(`{"status":"failed","message":"` + huge + `"}`))
	if got.Succeeded {
		t.Fatal("a failed status did not report failure")
	}

	if len(got.Message) > httpapi.MaxCallbackDetailBytes+len("... (truncated)") {
		t.Errorf("message is %d bytes, want it capped near %d", len(got.Message), httpapi.MaxCallbackDetailBytes)
	}
}
