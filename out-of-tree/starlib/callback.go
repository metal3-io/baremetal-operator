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
	"errors"
	"fmt"

	"go.starlark.net/starlark"
)

// HostSecretName is the single per host Secret holding all Starlark state, keyed
// by use case, so callback data and serve registrations share one owned object.
func HostSecretName(bmhName string) string {
	return bmhName + "-starlark"
}

// CallbackContext carries the per host callback base URL and token to builtins.
type CallbackContext struct {
	// URL is the finished callback endpoint, built by the listener that mounts it.
	URL   string
	Token string
}

// CallbackStore reads and clears the persisted per host callback data.
type CallbackStore interface {
	ReadCallback(ctx context.Context, namespace, name string) (map[string]any, error)
	DeleteCallback(ctx context.Context, namespace, name string) error
}

// CallbackContextFromThread returns the callback context, erroring when it is unset.
func CallbackContextFromThread(thread *starlark.Thread, name string) (*CallbackContext, error) {
	cb, ok := thread.Local(CallbackThreadLocal).(*CallbackContext)
	if !ok || cb == nil {
		return nil, fmt.Errorf("%s: callbacks are not configured", name)
	}

	return cb, nil
}

// CallbackStoreFromThread resolves the CallbackStore, host coordinates, and context.
func CallbackStoreFromThread(thread *starlark.Thread, name string) (CallbackStore, string, string, context.Context, error) {
	store, ok := thread.Local(HostResolverThreadLocal).(CallbackStore)
	if !ok || store == nil {
		return nil, "", "", nil, fmt.Errorf("%s: no callback store configured", name)
	}

	ns, _ := thread.Local(HostNamespaceThreadLocal).(string)
	host, _ := thread.Local(HostNameThreadLocal).(string)
	if ns == "" || host == "" {
		return nil, "", "", nil, fmt.Errorf("%s: BMH coordinates not set on thread", name)
	}

	return store, ns, host, ContextFromThread(thread), nil
}

// BuiltinCallbackURL returns the callback URL the external agent should post to.
func BuiltinCallbackURL(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	if err := starlark.UnpackPositionalArgs("callback_url", args, nil, 0); err != nil {
		return starlark.None, err
	}

	cb, err := CallbackContextFromThread(thread, "callback_url")
	if err != nil {
		return starlark.None, err
	}

	if cb.URL == "" {
		return starlark.None, errors.New("callback_url: no callback URL configured")
	}

	return starlark.String(cb.URL), nil
}

// BuiltinCallbackToken returns the synthesized per host callback token.
func BuiltinCallbackToken(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	if err := starlark.UnpackPositionalArgs("callback_token", args, nil, 0); err != nil {
		return starlark.None, err
	}

	cb, err := CallbackContextFromThread(thread, "callback_token")
	if err != nil {
		return starlark.None, err
	}

	return starlark.String(cb.Token), nil
}

// BuiltinCallbackGet returns the persisted callback data as a dict, or None when absent.
func BuiltinCallbackGet(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	if err := starlark.UnpackPositionalArgs("callback_get", args, nil, 0); err != nil {
		return starlark.None, err
	}

	store, ns, host, ctx, err := CallbackStoreFromThread(thread, "callback_get")
	if err != nil {
		return starlark.None, err
	}

	data, err := store.ReadCallback(ctx, ns, host)
	if err != nil {
		return starlark.None, fmt.Errorf("callback_get: %w", err)
	}

	if data == nil {
		return starlark.None, nil //nolint:nilnil // absent callback is a valid None result
	}

	return GoToStarlark(data), nil
}

// BuiltinCallbackClear deletes the persisted callback data for the current host.
func BuiltinCallbackClear(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	if err := starlark.UnpackPositionalArgs("callback_clear", args, nil, 0); err != nil {
		return starlark.None, err
	}

	store, ns, host, ctx, err := CallbackStoreFromThread(thread, "callback_clear")
	if err != nil {
		return starlark.None, err
	}

	if err := store.DeleteCallback(ctx, ns, host); err != nil {
		return starlark.None, fmt.Errorf("callback_clear: %w", err)
	}

	return starlark.None, nil
}

// CallbackBuiltins exposes the callback URL, token, and stored data to scripts.
func CallbackBuiltins() starlark.StringDict {
	return starlark.StringDict{
		"callback_url":   starlark.NewBuiltin("callback_url", BuiltinCallbackURL),
		"callback_token": starlark.NewBuiltin("callback_token", BuiltinCallbackToken),
		"callback_get":   starlark.NewBuiltin("callback_get", BuiltinCallbackGet),
		"callback_clear": starlark.NewBuiltin("callback_clear", BuiltinCallbackClear),
	}
}
