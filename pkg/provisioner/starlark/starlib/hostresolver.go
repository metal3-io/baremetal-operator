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

// HostResolver resolves BMH data on demand: named spec secret fields
// (ReadHostSecret) and the full spec as a dict (ReadHostSpec, read-only).
type HostResolver interface {
	ReadHostSecret(ctx context.Context, namespace, name, field string) (string, error)
	ReadHostSpec(ctx context.Context, namespace, name string) (map[string]any, error)
}

// Starlark read_host_secret(field): resolve a BMH secret ref to its string content.
func builtinReadHostSecret(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
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

	ctx, _ := thread.Local(CtxThreadLocal).(context.Context)
	if ctx == nil {
		ctx = context.Background()
	}

	s, err := resolver.ReadHostSecret(ctx, ns, name, string(field))
	if err != nil {
		return starlark.None, fmt.Errorf("read_host_secret(%s): %w", string(field), err)
	}

	return starlark.String(s), nil
}

// Starlark read_host_spec(): return BareMetalHost.Spec as a read-only dict.
func builtinReadHostSpec(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	if err := starlark.UnpackPositionalArgs("read_host_spec", args, nil, 0); err != nil {
		return starlark.None, err
	}

	resolver, ok := thread.Local(HostResolverThreadLocal).(HostResolver)
	if !ok || resolver == nil {
		return starlark.None, errors.New("read_host_spec: no HostResolver configured (factory constructed without one?)")
	}

	ns, _ := thread.Local(HostNamespaceThreadLocal).(string)
	name, _ := thread.Local(HostNameThreadLocal).(string)
	if ns == "" || name == "" {
		return starlark.None, errors.New("read_host_spec: BMH coordinates not set on thread")
	}

	ctx, _ := thread.Local(CtxThreadLocal).(context.Context)
	if ctx == nil {
		ctx = context.Background()
	}

	spec, err := resolver.ReadHostSpec(ctx, ns, name)
	if err != nil {
		return starlark.None, fmt.Errorf("read_host_spec: %w", err)
	}

	return GoToStarlark(spec), nil
}
