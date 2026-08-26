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

package starlib

import (
	"context"
	"fmt"
	"net/url"

	"go.starlark.net/starlark"
)

// ServeRegistration describes one ConfigMap backed route served over HTTP.
type ServeRegistration struct {
	ConfigMap string         `json:"configmap"`
	Key       string         `json:"key"`
	Render    bool           `json:"render"`
	Vars      map[string]any `json:"vars,omitempty"`
}

// ServeStore persists and removes per host serve registrations.
type ServeStore interface {
	WriteServe(ctx context.Context, namespace, name, id string, reg ServeRegistration) error
	DeleteServe(ctx context.Context, namespace, name, id string) error
}

// ServeContext carries the per host serve base URL and host UID to builtins.
type ServeContext struct {
	// URLPrefix is everything before the route id, built by the listener.
	URLPrefix string
}

// ServeContextFromThread resolves the serve context, erroring when unset.
func ServeContextFromThread(thread *starlark.Thread, name string) (*ServeContext, error) {
	sc, ok := thread.Local(ServeThreadLocal).(*ServeContext)
	if !ok || sc == nil || sc.URLPrefix == "" {
		return nil, fmt.Errorf("%s: serving is not configured", name)
	}

	return sc, nil
}

// serveStoreFromThread resolves the store and host coordinates, gated on serve being
// configured so a script cannot persist a route the listener will never serve.
func serveStoreFromThread(thread *starlark.Thread, name string) (ServeStore, string, string, context.Context, error) {
	if _, err := ServeContextFromThread(thread, name); err != nil {
		return nil, "", "", nil, err
	}

	store, ok := thread.Local(HostResolverThreadLocal).(ServeStore)
	if !ok || store == nil {
		return nil, "", "", nil, fmt.Errorf("%s: no serve store configured", name)
	}

	ns, _ := thread.Local(HostNamespaceThreadLocal).(string)
	host, _ := thread.Local(HostNameThreadLocal).(string)
	if ns == "" || host == "" {
		return nil, "", "", nil, fmt.Errorf("%s: BMH coordinates not set on thread", name)
	}

	return store, ns, host, ContextFromThread(thread), nil
}

// ValidateServeID rejects ids that are empty or not a single URL path safe segment,
// so a registered route always matches the URL that serve_url hands out.
func ValidateServeID(name, id string) error {
	if id == "" || id != url.PathEscape(id) {
		return fmt.Errorf("%s: id %q must be a non empty URL path safe segment", name, id)
	}

	return nil
}

// BuiltinServeEnabled reports whether serving is actually wired up. The env var is
// only a request, the listener may have failed to bind.
func BuiltinServeEnabled(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	if err := starlark.UnpackPositionalArgs("serve_enabled", args, nil, 0); err != nil {
		return starlark.None, err
	}

	_, err := ServeContextFromThread(thread, "serve_enabled")

	return starlark.Bool(err == nil), nil
}

// BuiltinServeRegister persists a ConfigMap backed route for the current host.
func BuiltinServeRegister(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		id        string
		configMap string
		key       string
		render    bool
		vars      *starlark.Dict
	)

	if err := starlark.UnpackArgs("serve_register", args, kwargs,
		"id", &id, "configmap", &configMap, "key", &key, "render?", &render, "vars?", &vars); err != nil {
		return starlark.None, err
	}

	if err := ValidateServeID("serve_register", id); err != nil {
		return starlark.None, err
	}

	store, ns, host, ctx, err := serveStoreFromThread(thread, "serve_register")
	if err != nil {
		return starlark.None, err
	}

	varsMap := map[string]any{}
	if vars != nil {
		m, merr := AsMap("serve_register vars", vars)
		if merr != nil {
			return starlark.None, merr
		}

		varsMap = m
	}

	reg := ServeRegistration{ConfigMap: configMap, Key: key, Render: render, Vars: varsMap}
	if err := store.WriteServe(ctx, ns, host, id, reg); err != nil {
		return starlark.None, fmt.Errorf("serve_register: %w", err)
	}

	return starlark.None, nil
}

// BuiltinServeDeregister removes a previously registered route for the host.
func BuiltinServeDeregister(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	var id string
	if err := starlark.UnpackPositionalArgs("serve_deregister", args, nil, 1, &id); err != nil {
		return starlark.None, err
	}

	store, ns, host, ctx, err := serveStoreFromThread(thread, "serve_deregister")
	if err != nil {
		return starlark.None, err
	}

	if err := store.DeleteServe(ctx, ns, host, id); err != nil {
		return starlark.None, fmt.Errorf("serve_deregister: %w", err)
	}

	return starlark.None, nil
}

// BuiltinServeURL returns the URL an external agent should fetch the route from.
func BuiltinServeURL(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	var id string
	if err := starlark.UnpackPositionalArgs("serve_url", args, nil, 1, &id); err != nil {
		return starlark.None, err
	}

	if err := ValidateServeID("serve_url", id); err != nil {
		return starlark.None, err
	}

	sc, err := ServeContextFromThread(thread, "serve_url")
	if err != nil {
		return starlark.None, err
	}

	return starlark.String(sc.URLPrefix + id), nil
}

// ServeBuiltins exposes the serve registration builtins to scripts.
func ServeBuiltins() starlark.StringDict {
	return starlark.StringDict{
		"serve_register":   starlark.NewBuiltin("serve_register", BuiltinServeRegister),
		"serve_deregister": starlark.NewBuiltin("serve_deregister", BuiltinServeDeregister),
		"serve_url":        starlark.NewBuiltin("serve_url", BuiltinServeURL),
		"serve_enabled":    starlark.NewBuiltin("serve_enabled", BuiltinServeEnabled),
	}
}
