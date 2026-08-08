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
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"go.starlark.net/starlark"
)

// DefaultFieldManager owns fields written by k8s_apply server side applies.
const DefaultFieldManager = "starlark-provisioner"

// KubeResolver reads and writes Kubernetes objects in the host namespace.
type KubeResolver interface {
	GetObject(ctx context.Context, namespace, apiVersion, kind, name string, uncached bool) (map[string]any, error)
	ListObjects(ctx context.Context, namespace, apiVersion, kind, labelSelector string) ([]map[string]any, error)
	ApplyObject(ctx context.Context, namespace string, obj map[string]any, fieldManager string, force bool) (map[string]any, error)
	PatchObject(ctx context.Context, namespace, apiVersion, kind, name, patchType string, data []byte, status bool) (map[string]any, error)
	DeleteObject(ctx context.Context, namespace, apiVersion, kind, name string) error
}

// kubeResolverFromThread resolves the KubeResolver, host namespace, and context.
func kubeResolverFromThread(thread *starlark.Thread, name string) (KubeResolver, string, context.Context, error) {
	resolver, ok := thread.Local(HostResolverThreadLocal).(KubeResolver)
	if !ok || resolver == nil {
		return nil, "", nil, fmt.Errorf("%s: no Kubernetes resolver configured", name)
	}

	ns, _ := thread.Local(HostNamespaceThreadLocal).(string)
	if ns == "" {
		return nil, "", nil, fmt.Errorf("%s: BMH namespace not set on thread", name)
	}

	return resolver, ns, contextFromThread(thread), nil
}

// builtinK8sGet returns a Kubernetes object as a dict, or None when absent.
// An optional section (spec, status, metadata) selects one top level field.
func builtinK8sGet(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var apiVersion, kind, name, section starlark.String
	var uncached starlark.Bool
	if err := starlark.UnpackArgs("k8s_get", args, kwargs,
		"api_version", &apiVersion, "kind", &kind, "name", &name, "section?", &section, "uncached?", &uncached); err != nil {
		return starlark.None, err
	}

	resolver, ns, ctx, err := kubeResolverFromThread(thread, "k8s_get")
	if err != nil {
		return starlark.None, err
	}

	obj, err := resolver.GetObject(ctx, ns, string(apiVersion), string(kind), string(name), bool(uncached))
	if err != nil {
		return starlark.None, fmt.Errorf("k8s_get: %w", err)
	}

	if obj == nil {
		return starlark.None, nil
	}

	if s := string(section); s != "" {
		v, ok := obj[s]
		if !ok {
			return starlark.None, nil
		}

		return GoToStarlark(v), nil
	}

	return GoToStarlark(obj), nil
}

// builtinK8sList lists objects of a kind in the host namespace as a list of dicts.
// Starlark call is k8s_list(api_version, kind, label_selector).
func builtinK8sList(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var apiVersion, kind, labelSelector starlark.String
	if err := starlark.UnpackArgs("k8s_list", args, kwargs,
		"api_version", &apiVersion, "kind", &kind, "label_selector?", &labelSelector); err != nil {
		return starlark.None, err
	}

	resolver, ns, ctx, err := kubeResolverFromThread(thread, "k8s_list")
	if err != nil {
		return starlark.None, err
	}

	items, err := resolver.ListObjects(ctx, ns, string(apiVersion), string(kind), string(labelSelector))
	if err != nil {
		return starlark.None, fmt.Errorf("k8s_list: %w", err)
	}

	elems := MapSlice(items, func(o map[string]any) starlark.Value { return GoToStarlark(o) })

	return starlark.NewList(elems), nil
}

// builtinK8sApply server side applies an object into the host namespace.
// Starlark call is k8s_apply(object, field_manager, force).
func builtinK8sApply(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var object *starlark.Dict
	var fieldManager starlark.String
	// Default force false so a caller must opt in to seizing fields owned by other managers.
	force := starlark.Bool(false)

	if err := starlark.UnpackArgs("k8s_apply", args, kwargs,
		"object", &object, "field_manager?", &fieldManager, "force?", &force); err != nil {
		return starlark.None, err
	}

	if object == nil {
		return starlark.None, errors.New("k8s_apply: object is required")
	}

	obj, ok := ToGo(object).(map[string]any)
	if !ok {
		return starlark.None, errors.New("k8s_apply: object must be a dict")
	}

	resolver, ns, ctx, err := kubeResolverFromThread(thread, "k8s_apply")
	if err != nil {
		return starlark.None, err
	}

	fm := cmp.Or(string(fieldManager), DefaultFieldManager)

	applied, err := resolver.ApplyObject(ctx, ns, obj, fm, bool(force))
	if err != nil {
		return starlark.None, fmt.Errorf("k8s_apply: %w", err)
	}

	return GoToStarlark(applied), nil
}

// builtinK8sPatch patches an object by name in the host namespace. Starlark call
// is k8s_patch(api_version, kind, name, patch, patch_type, status).
func builtinK8sPatch(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var apiVersion, kind, name, patchType starlark.String
	var patch starlark.Value
	patchType = starlark.String("merge")
	status := starlark.Bool(false)

	if err := starlark.UnpackArgs("k8s_patch", args, kwargs,
		"api_version", &apiVersion, "kind", &kind, "name", &name,
		"patch", &patch, "patch_type?", &patchType, "status?", &status); err != nil {
		return starlark.None, err
	}

	if patch == nil {
		return starlark.None, errors.New("k8s_patch: patch is required")
	}

	data, err := json.Marshal(ToGo(patch))
	if err != nil {
		return starlark.None, fmt.Errorf("k8s_patch: marshal patch: %w", err)
	}

	resolver, ns, ctx, err := kubeResolverFromThread(thread, "k8s_patch")
	if err != nil {
		return starlark.None, err
	}

	patched, err := resolver.PatchObject(ctx, ns, string(apiVersion), string(kind), string(name), string(patchType), data, bool(status))
	if err != nil {
		return starlark.None, fmt.Errorf("k8s_patch: %w", err)
	}

	return GoToStarlark(patched), nil
}

// builtinK8sDelete deletes an object by name in the host namespace. Starlark call
// is k8s_delete(api_version, kind, name). Missing objects are treated as deleted.
func builtinK8sDelete(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var apiVersion, kind, name starlark.String
	if err := starlark.UnpackArgs("k8s_delete", args, kwargs,
		"api_version", &apiVersion, "kind", &kind, "name", &name); err != nil {
		return starlark.None, err
	}

	resolver, ns, ctx, err := kubeResolverFromThread(thread, "k8s_delete")
	if err != nil {
		return starlark.None, err
	}

	if err := resolver.DeleteObject(ctx, ns, string(apiVersion), string(kind), string(name)); err != nil {
		return starlark.None, fmt.Errorf("k8s_delete: %w", err)
	}

	return starlark.None, nil
}

// kubeBuiltins exposes the generic Kubernetes read and write helpers.
func kubeBuiltins() starlark.StringDict {
	return starlark.StringDict{
		"k8s_get":    starlark.NewBuiltin("k8s_get", builtinK8sGet),
		"k8s_list":   starlark.NewBuiltin("k8s_list", builtinK8sList),
		"k8s_apply":  starlark.NewBuiltin("k8s_apply", builtinK8sApply),
		"k8s_patch":  starlark.NewBuiltin("k8s_patch", builtinK8sPatch),
		"k8s_delete": starlark.NewBuiltin("k8s_delete", builtinK8sDelete),
	}
}
