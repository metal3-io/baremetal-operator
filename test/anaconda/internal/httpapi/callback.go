// SPDX-License-Identifier: Apache-2.0

// Package httpapi is the plugin's listener, serving the kickstart anaconda
// fetches and the token gated callback it reports completion on.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"metal3.local/anaconda/internal/core"
)

// MaxCallbackDetailBytes bounds what a host supplied report puts into the BMH
// status, since the body may be 256 KiB and all of it would reach etcd.
const MaxCallbackDetailBytes = 512

// CallbackMaxBodyBytes caps the size of a callback body the listener will read,
// so an unbounded POST cannot make the listener buffer it.
const CallbackMaxBodyBytes = 256 << 10

type ServerResolver interface {
	FindHostsByMAC(ctx context.Context, macs []string) ([]core.HostRef, error)
	ReadKickstart(ctx context.Context, namespace, secretName string) (string, bool, error)
	HostUID(ctx context.Context, namespace, name string) (string, error)
	WriteInstallReport(ctx context.Context, namespace, name string, report core.InstallReport) error
}

type PluginServer struct {
	Resolver ServerResolver
	Log      logr.Logger
	Config   core.Config
}

func Truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}

	return s[:n] + "... (truncated)"
}

// Statuses a kickstart may report as success. Anything else is a failure, so a
// typo fails the host loudly instead of provisioning it silently.
var CallbackSuccessStatuses = []string{"installed", "ok", "success", "complete"}

const CallbackPathPrefix = "/callback/"

// Listener timeouts, bounding the open kickstart route against slow or idle
// clients rather than letting an installer hold a connection forever.
const (
	ListenerReadHeaderTimeout = 10 * time.Second
	ListenerReadTimeout       = 30 * time.Second
	ListenerWriteTimeout      = 60 * time.Second
	ListenerIdleTimeout       = 120 * time.Second
)

// ParseInstallReport turns a posted body into a verdict. Only an empty body or a
// known good status passes, so a typo cannot quietly provision a host.
func ParseInstallReport(raw []byte) core.InstallReport {
	body := strings.TrimSpace(string(raw))
	if body == "" {
		return core.InstallReport{Succeeded: true}
	}

	var report struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}

	if err := json.Unmarshal([]byte(body), &report); err != nil {
		return core.InstallReport{Message: "install report is not JSON: " + Truncate(body, MaxCallbackDetailBytes)}
	}

	if slices.ContainsFunc(CallbackSuccessStatuses, func(ok string) bool {
		return strings.EqualFold(report.Status, ok)
	}) {
		return core.InstallReport{Succeeded: true}
	}

	if report.Message != "" {
		return core.InstallReport{Message: Truncate(report.Message, MaxCallbackDetailBytes)}
	}

	// A bare POST is the documented way to say finished, but a body that parsed
	// and named no status misspelled the key, so it cannot mean success.
	if report.Status == "" {
		return core.InstallReport{Message: "install report names no status, body " + Truncate(body, MaxCallbackDetailBytes)}
	}

	return core.InstallReport{Message: "status " + strconv.Quote(report.Status)}
}

// HandleCallback validates the UID then records the verdict as host annotations.
// It makes no provisioning decision, Provision reads the result.
func (s *PluginServer) HandleCallback(w http.ResponseWriter, r *http.Request) {
	ns := r.PathValue("namespace")
	name := r.PathValue("name")

	// Nothing authenticates the poster, the unguessable UID in the path is the
	// whole check. Answer the same either way so this is not a host oracle.
	uid, err := s.Resolver.HostUID(r.Context(), ns, name)
	if err != nil || uid == "" || uid != r.PathValue("uid") {
		http.Error(w, "unauthorized", http.StatusUnauthorized)

		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, CallbackMaxBodyBytes))
	if err != nil {
		http.Error(w, "bad request body", http.StatusBadRequest)

		return
	}

	report := ParseInstallReport(body)

	if err := s.Resolver.WriteInstallReport(r.Context(), ns, name, report); err != nil {
		s.Log.Error(err, "recording the install report failed", "namespace", ns, "name", name)
		http.Error(w, "persist failed", http.StatusInternalServerError)

		return
	}

	s.Log.Info("install reported", "namespace", ns, "name", name, "succeeded", report.Succeeded)

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

// CallbackURL is the endpoint a host posts its install report to. The UID makes
// it unguessable, which is all there is, since nothing authenticates the poster.
func CallbackURL(baseURL, uid, namespace, name string) string {
	return strings.TrimRight(baseURL, "/") + CallbackPathPrefix + uid + "/" + namespace + "/" + name
}

func (s *PluginServer) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST "+CallbackPathPrefix+"{uid}/{namespace}/{name}", s.HandleCallback)
	mux.HandleFunc("GET "+KickstartPathPrefix+"{id}", s.HandleKickstart)

	return mux
}

// Start binds under ctx then serves in the background, returning a bind error so
// the caller can leave the facility off rather than advertise it.
func (s *PluginServer) Start(ctx context.Context) error {
	var lc net.ListenConfig

	ln, err := lc.Listen(ctx, "tcp", s.Config.ListenAddr)
	if err != nil {
		return err
	}

	srv := &http.Server{
		Handler:           s.Handler(),
		ReadHeaderTimeout: ListenerReadHeaderTimeout,
		ReadTimeout:       ListenerReadTimeout,
		WriteTimeout:      ListenerWriteTimeout,
		IdleTimeout:       ListenerIdleTimeout,
	}

	go func() {
		serveErr := srv.Serve(ln)
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			s.Log.Error(serveErr, "plugin listener stopped")
		}
	}()

	s.Log.Info("plugin listener started", "addr", s.Config.ListenAddr)

	return nil
}
