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

// Serves registered ConfigMap keys over HTTP, optionally rendered as a Go template.

package starlark

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"text/template"

	"github.com/s3rj1k/starlark-provisioner/starlib"
)

// ServePathPrefix is the fixed route prefix for served ConfigMap routes.
const ServePathPrefix = "/serve/"

// KickstartPathPrefix is the fixed route prefix anaconda fetches a kickstart from.
const KickstartPathPrefix = "/ks/"

// ServeResolver resolves a route by BMH UID or MAC and reads the backing ConfigMap key.
type ServeResolver interface {
	ReadServe(ctx context.Context, uid, id string) (reg starlib.ServeRegistration, found bool, err error)
	ConfigMapValue(ctx context.Context, namespace, name, key string) (value string, found bool, err error)
	FindHostsByMAC(ctx context.Context, macs []string) ([]HostRef, error)
}

// ServeCleaner drops every serve route a host owns.
type ServeCleaner interface {
	DeleteServeHost(ctx context.Context, namespace, name string) error
}

// clearServeRoutes deletes this host's serve Secret on teardown, so the open
// endpoint stops serving stale content once provisioning is undone.
func (p *starlarkProvisioner) clearServeRoutes(ctx context.Context) {
	if !p.serveEnabled {
		return
	}

	// Matched on the capability, not a concrete type, so a wrapping resolver
	// does not silently skip the cleanup.
	cleaner, ok := p.secretResolver.(ServeCleaner)
	if !ok {
		return
	}

	if err := cleaner.DeleteServeHost(ctx, p.hostData.ObjectMeta.Namespace, p.hostData.ObjectMeta.Name); err != nil {
		p.log.Error(err, "serve routes cleanup failed")
	}
}

// RenderTemplate executes content as a text/template with vars as the root context.
// A missing key is an error so template typos surface instead of silently emitting nothing.
func RenderTemplate(name, content string, vars map[string]any) (string, error) {
	tmpl, err := template.New(name).Option("missingkey=error").Parse(content)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, vars); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// writeKickstart emits a kickstart body.
func writeKickstart(w http.ResponseWriter, content string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.WriteString(w, content)
}

// serveFallbackKickstart emits the configured no-op kickstart, or the compiled in one.
// It answers 200 on purpose, a 404 would drop anaconda to an interactive installer.
func (s *PluginServer) serveFallbackKickstart(w http.ResponseWriter, r *http.Request, reason string, macs []string) {
	s.Log.Info("kickstart: serving fallback", "reason", reason, "macs", macs)

	cfg := s.Config
	if cfg.KSFallbackConfigMap != "" {
		content, found, err := s.Serve.ConfigMapValue(r.Context(), s.Namespace, cfg.KSFallbackConfigMap, cfg.KSFallbackKey)
		if err != nil {
			s.Log.Error(err, "kickstart: fallback configmap read failed", "configmap", cfg.KSFallbackConfigMap)
		} else if found {
			writeKickstart(w, content)

			return
		}
	}

	writeKickstart(w, DefaultFallbackKickstart)
}

// HandleKickstart resolves the caller from the MACs anaconda reported and serves that
// host's registered kickstart. Anything unresolved gets the fallback, never a wipe.
func (s *PluginServer) HandleKickstart(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	macs := AnacondaMACs(r)

	if len(macs) == 0 {
		// Without inst.ks.sendmac on the kernel cmdline there is nothing to match on.
		s.serveFallbackKickstart(w, r, "request carried no MAC headers", macs)

		return
	}

	hosts, err := s.Serve.FindHostsByMAC(r.Context(), macs)
	if err != nil {
		s.Log.Error(err, "kickstart: host lookup failed", "macs", macs)
		http.Error(w, "host lookup failed", http.StatusInternalServerError)

		return
	}

	if len(hosts) == 0 {
		s.serveFallbackKickstart(w, r, "no host claims these MACs", macs)

		return
	}

	if len(hosts) > 1 {
		s.Log.Error(nil, "kickstart: multiple hosts claim these MACs, using the first",
			"macs", macs, "hosts", hosts)
	}

	host := hosts[0]

	reg, found, err := s.Serve.ReadServe(r.Context(), host.UID, id)
	if err != nil {
		s.Log.Error(err, "kickstart: registration read failed", "host", host.Name, "id", id)
		http.Error(w, "registration read failed", http.StatusInternalServerError)

		return
	}

	if !found {
		s.serveFallbackKickstart(w, r, "host has no "+id+" registration", macs)

		return
	}

	content, found, err := s.Serve.ConfigMapValue(r.Context(), s.Namespace, reg.ConfigMap, reg.Key)
	if err != nil {
		s.Log.Error(err, "kickstart: configmap read failed", "host", host.Name, "configmap", reg.ConfigMap)
		http.Error(w, "configmap read failed", http.StatusInternalServerError)

		return
	}

	if !found {
		s.serveFallbackKickstart(w, r, "registration points at a missing ConfigMap key", macs)

		return
	}

	if reg.Render {
		rendered, rerr := RenderTemplate(id, content, reg.Vars)
		if rerr != nil {
			s.Log.Error(rerr, "kickstart: template render failed", "host", host.Name, "id", id)
			http.Error(w, "template render failed", http.StatusInternalServerError)

			return
		}

		content = rendered
	}

	s.Log.Info("kickstart: served", "host", host.Name, "id", id)
	writeKickstart(w, content)
}

// HandleServe serves a registered ConfigMap key, rendering it when configured.
// It performs no auth, reachability is a provisioning network isolation concern.
func (s *PluginServer) HandleServe(w http.ResponseWriter, r *http.Request) {
	uid := r.PathValue("uid")
	id := r.PathValue("id")

	reg, found, err := s.Serve.ReadServe(r.Context(), uid, id)
	if err != nil {
		s.Log.Error(err, "serve registration read failed", "uid", uid, "id", id)
		http.Error(w, "registration read failed", http.StatusInternalServerError)

		return
	}

	if !found {
		http.Error(w, "not found", http.StatusNotFound)

		return
	}

	content, found, err := s.Serve.ConfigMapValue(r.Context(), s.Namespace, reg.ConfigMap, reg.Key)
	if err != nil {
		s.Log.Error(err, "serve configmap read failed", "uid", uid, "id", id)
		http.Error(w, "configmap read failed", http.StatusInternalServerError)

		return
	}

	if !found {
		http.Error(w, "not found", http.StatusNotFound)

		return
	}

	if reg.Render {
		rendered, rerr := RenderTemplate(id, content, reg.Vars)
		if rerr != nil {
			s.Log.Error(rerr, "serve template render failed", "uid", uid, "id", id)
			http.Error(w, "template render failed", http.StatusInternalServerError)

			return
		}

		content = rendered
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.WriteString(w, content)
}
