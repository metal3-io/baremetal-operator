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
	"strings"

	"go.starlark.net/starlark"
)

// HostResolver resolves BMH data on demand. Named spec secret fields
// (ReadHostSecret), the full spec (ReadHostSpec), and the full status (ReadHostStatus).
type HostResolver interface {
	ReadHostSecret(ctx context.Context, namespace, name, field string) (string, error)
	ReadHostSpec(ctx context.Context, namespace, name string) (map[string]any, error)
	ReadHostStatus(ctx context.Context, namespace, name string) (map[string]any, error)
}

// Starlark read_host_secret(field) resolves a BMH secret ref to its string content.
func BuiltinReadHostSecret(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	var field starlark.String
	if err := starlark.UnpackPositionalArgs("read_host_secret", args, nil, 1, &field); err != nil {
		return starlark.None, err
	}

	switch strings.ToLower(string(field)) {
	case "userdata", "networkdata", "metadata", "preprovisioningnetworkdata":
	default:
		return starlark.None, fmt.Errorf("read_host_secret: unknown field %q (want userData/networkData/metaData/preprovisioningNetworkData)", string(field))
	}

	resolver, ok := thread.Local(HostResolverThreadLocal).(HostResolver)
	if !ok || resolver == nil {
		return starlark.None, errors.New("read_host_secret: no HostResolver configured (factory constructed without one?)")
	}

	ns, _ := thread.Local(HostNamespaceThreadLocal).(string)
	name, _ := thread.Local(HostNameThreadLocal).(string)
	if ns == "" || name == "" {
		return starlark.None, errors.New("read_host_secret: BMH coordinates not set on thread")
	}

	ctx := ContextFromThread(thread)

	s, err := resolver.ReadHostSecret(ctx, ns, name, string(field))
	if err != nil {
		return starlark.None, fmt.Errorf("read_host_secret(%s): %w", string(field), err)
	}

	return starlark.String(s), nil
}

// ReadHostMap runs a zero arg host lookup (spec or status) returning a readonly
// dict, sharing the coordinate lookup and resolver plumbing.
func ReadHostMap(
	thread *starlark.Thread,
	name string,
	args starlark.Tuple,
	resolve func(ctx context.Context, resolver HostResolver, ns, host string) (map[string]any, error),
) (starlark.Value, error) {
	if err := starlark.UnpackPositionalArgs(name, args, nil, 0); err != nil {
		return starlark.None, err
	}

	resolver, ok := thread.Local(HostResolverThreadLocal).(HostResolver)
	if !ok || resolver == nil {
		return starlark.None, fmt.Errorf("%s: no HostResolver configured (factory constructed without one?)", name)
	}

	ns, _ := thread.Local(HostNamespaceThreadLocal).(string)
	host, _ := thread.Local(HostNameThreadLocal).(string)
	if ns == "" || host == "" {
		return starlark.None, fmt.Errorf("%s: BMH coordinates not set on thread", name)
	}

	m, err := resolve(ContextFromThread(thread), resolver, ns, host)
	if err != nil {
		return starlark.None, fmt.Errorf("%s: %w", name, err)
	}

	return GoToStarlark(m), nil
}

// Starlark read_host_spec() returns BareMetalHost.Spec as a readonly dict.
func BuiltinReadHostSpec(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	return ReadHostMap(thread, "read_host_spec", args, func(ctx context.Context, resolver HostResolver, ns, host string) (map[string]any, error) {
		return resolver.ReadHostSpec(ctx, ns, host)
	})
}

// Starlark read_host_status() returns BareMetalHost.Status as a readonly dict.
func BuiltinReadHostStatus(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	return ReadHostMap(thread, "read_host_status", args, func(ctx context.Context, resolver HostResolver, ns, host string) (map[string]any, error) {
		return resolver.ReadHostStatus(ctx, ns, host)
	})
}
