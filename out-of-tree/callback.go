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

// Shared plugin HTTP listener hosting the token gated callback and serve facilities.

package starlark

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-logr/logr"
)

// CallbackMaxBodyBytes caps the size of a callback body the listener will read.
// It stays well under the Secret size limit since the body is persisted to one.
const CallbackMaxBodyBytes = 256 << 10

// CallbackPathPrefix is the fixed route prefix for host callbacks.
const CallbackPathPrefix = "/callback/"

// SynthesizeToken derives a per host token by keying an HMAC with the BMC password
// over the host coordinates, so it is unguessable without the secret and never stored.
func SynthesizeToken(namespace, name, bmcUser, bmcPass string) string {
	mac := hmac.New(sha256.New, []byte(bmcPass))
	_, _ = mac.Write([]byte(namespace + "/" + name + "/" + bmcUser))

	return hex.EncodeToString(mac.Sum(nil))
}

// CallbackConfig holds the shared listener settings read from the environment.
type CallbackConfig struct {
	Addr    string
	BaseURL string
	TLSCert string
	TLSKey  string
}

// Enabled reports whether a bind address was configured.
func (c CallbackConfig) Enabled() bool { return c.Addr != "" }

// LoadCallbackConfig reads the listener settings from the environment.
func LoadCallbackConfig() CallbackConfig {
	return CallbackConfig{
		// Setting the bind address starts the shared listener that hosts both the
		// callback and serve facilities. Unset leaves the plugin script only.
		Addr:    os.Getenv("STARLARK_CALLBACK_ADDR"),
		BaseURL: os.Getenv("STARLARK_CALLBACK_BASE_URL"),
		TLSCert: os.Getenv("STARLARK_CALLBACK_TLS_CERT"),
		TLSKey:  os.Getenv("STARLARK_CALLBACK_TLS_KEY"),
	}
}

// CallbackResolver is the subset of KubeHostResolver the callback route needs.
type CallbackResolver interface {
	BMCCredentials(ctx context.Context, namespace, name string) (username, password string, err error)
	WriteCallback(ctx context.Context, namespace, name string, body []byte, receivedAt string) error
}

// PluginServer is the shared HTTP listener hosting the callback and serve facilities.
type PluginServer struct {
	Config   CallbackConfig
	Resolver CallbackResolver
	Serve    ServeResolver
	Log      logr.Logger
}

// BearerToken returns the token from the Authorization header, or empty.
func BearerToken(r *http.Request) string {
	if t, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
		return t
	}

	return ""
}

// HandleCallback validates the token, persists the body to the per host Secret, and acks.
// It makes no provisioning decision, that stays in the Starlark script.
func (s *PluginServer) HandleCallback(w http.ResponseWriter, r *http.Request) {
	ns := r.PathValue("namespace")
	name := r.PathValue("name")

	user, pass, err := s.Resolver.BMCCredentials(r.Context(), ns, name)
	if err != nil {
		// Answer as unauthorized so the endpoint is not a host existence oracle.
		http.Error(w, "unauthorized", http.StatusUnauthorized)

		return
	}

	want := SynthesizeToken(ns, name, user, pass)
	if got := BearerToken(r); !hmac.Equal([]byte(got), []byte(want)) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)

		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, CallbackMaxBodyBytes))
	if err != nil {
		http.Error(w, "bad request body", http.StatusBadRequest)

		return
	}

	if err := s.Resolver.WriteCallback(r.Context(), ns, name, body, time.Now().UTC().Format(time.RFC3339)); err != nil {
		s.Log.Error(err, "callback persist failed", "namespace", ns, "name", name)
		http.Error(w, "persist failed", http.StatusInternalServerError)

		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

// Handler builds the routed mux mounting whichever facilities are configured.
func (s *PluginServer) Handler() http.Handler {
	mux := http.NewServeMux()

	if s.Resolver != nil {
		mux.HandleFunc("POST "+CallbackPathPrefix+"{namespace}/{name}", s.HandleCallback)
	}

	if s.Serve != nil {
		mux.HandleFunc("GET "+ServePathPrefix+"{uid}/{id}", s.HandleServe)
	}

	return mux
}

// Start binds the listener synchronously then serves in the background, returning
// a bind error so the caller can leave the facilities off instead of advertising a dead endpoint.
func (s *PluginServer) Start() error {
	// Fail closed when only one of the TLS pair is set rather than serving plaintext.
	if (s.Config.TLSCert == "") != (s.Config.TLSKey == "") {
		return errors.New("TLS requires both STARLARK_CALLBACK_TLS_CERT and STARLARK_CALLBACK_TLS_KEY")
	}

	ln, err := net.Listen("tcp", s.Config.Addr)
	if err != nil {
		return err
	}

	// Timeouts bound the open serve route against slow or idle clients.
	srv := &http.Server{
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		var serveErr error
		if s.Config.TLSCert != "" && s.Config.TLSKey != "" {
			serveErr = srv.ServeTLS(ln, s.Config.TLSCert, s.Config.TLSKey)
		} else {
			serveErr = srv.Serve(ln)
		}

		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			s.Log.Error(serveErr, "plugin listener stopped")
		}
	}()

	s.Log.Info("plugin listener started", "addr", s.Config.Addr)

	return nil
}
