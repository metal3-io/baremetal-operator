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

// ServeResolver resolves a route by BMH UID and reads the backing ConfigMap key.
type ServeResolver interface {
	ReadServe(ctx context.Context, uid, id string) (reg starlib.ServeRegistration, namespace string, found bool, err error)
	ConfigMapValue(ctx context.Context, namespace, name, key string) (value string, found bool, err error)
}

// clearServeRoutes deletes this host's serve Secret on teardown, so the open
// endpoint stops serving stale content once provisioning is undone.
func (p *starlarkProvisioner) clearServeRoutes(ctx context.Context) {
	if !p.serveEnabled {
		return
	}

	kr, ok := p.secretResolver.(*KubeHostResolver)
	if !ok {
		return
	}

	if err := kr.DeleteServeHost(ctx, p.hostData.ObjectMeta.Namespace, p.hostData.ObjectMeta.Name); err != nil {
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

// HandleServe serves a registered ConfigMap key, rendering it when configured.
// It performs no auth, reachability is a provisioning network isolation concern.
func (s *PluginServer) HandleServe(w http.ResponseWriter, r *http.Request) {
	uid := r.PathValue("uid")
	id := r.PathValue("id")

	reg, namespace, found, err := s.Serve.ReadServe(r.Context(), uid, id)
	if err != nil {
		s.Log.Error(err, "serve registration read failed", "uid", uid, "id", id)
		http.Error(w, "registration read failed", http.StatusInternalServerError)

		return
	}

	if !found {
		http.Error(w, "not found", http.StatusNotFound)

		return
	}

	content, found, err := s.Serve.ConfigMapValue(r.Context(), namespace, reg.ConfigMap, reg.Key)
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
