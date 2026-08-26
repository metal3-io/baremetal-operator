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

// Cover for the Kubernetes and host read builtins against a recording stub.

package starlib

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.starlark.net/starlark"
)

// stubKube records what the builtins asked for and returns canned answers.
type stubKube struct {
	obj       map[string]any
	list      []map[string]any
	err       error
	gotNS     string
	gotKind   string
	gotName   string
	uncached  bool
	patchType string
	patchBody string
	deleted   bool
	fieldMgr  string
	force     bool
	// host read side
	secret string
	spec   map[string]any
	status map[string]any
	field  string
}

func (s *stubKube) GetObject(_ context.Context, ns, _, kind, name string, uncached bool) (map[string]any, error) {
	s.gotNS, s.gotKind, s.gotName, s.uncached = ns, kind, name, uncached
	return s.obj, s.err
}

func (s *stubKube) ListObjects(_ context.Context, ns, _, kind, _ string) ([]map[string]any, error) {
	s.gotNS, s.gotKind = ns, kind
	return s.list, s.err
}

func (s *stubKube) ApplyObject(_ context.Context, ns string, obj map[string]any, fm string, force bool) (map[string]any, error) {
	s.gotNS, s.fieldMgr, s.force = ns, fm, force
	return obj, s.err
}

func (s *stubKube) PatchObject(_ context.Context, ns, _, kind, name, pt string, data []byte, _ bool) (map[string]any, error) {
	s.gotNS, s.gotKind, s.gotName, s.patchType, s.patchBody = ns, kind, name, pt, string(data)
	return s.obj, s.err
}

func (s *stubKube) DeleteObject(_ context.Context, ns, _, kind, name string) error {
	s.gotNS, s.gotKind, s.gotName, s.deleted = ns, kind, name, true
	return s.err
}

func (s *stubKube) ReadHostSecret(_ context.Context, _, _, field string) (string, error) {
	s.field = field
	return s.secret, s.err
}

func (s *stubKube) ReadHostSpec(_ context.Context, _, _ string) (map[string]any, error) {
	return s.spec, s.err
}

func (s *stubKube) ReadHostStatus(_ context.Context, _, _ string) (map[string]any, error) {
	return s.status, s.err
}

// kubeThread wires a stub in as both the Kube and Host resolver.
func kubeThread(s *stubKube) *starlark.Thread {
	thread := &starlark.Thread{Name: "t"}
	thread.SetLocal(HostResolverThreadLocal, s)
	thread.SetLocal(HostNamespaceThreadLocal, "metal3-system")
	thread.SetLocal(HostNameThreadLocal, "node-1")

	return thread
}

func kw(pairs ...string) []starlark.Tuple {
	out := make([]starlark.Tuple, 0, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		out = append(out, starlark.Tuple{starlark.String(pairs[i]), starlark.String(pairs[i+1])})
	}

	return out
}

// Every builtin scopes to the host namespace from the thread, never one a
// script supplies, so a script cannot reach into another namespace.
func TestKubeBuiltinsUseTheThreadNamespace(t *testing.T) {
	s := &stubKube{obj: map[string]any{"kind": "ConfigMap"}}

	if _, err := BuiltinK8sGet(kubeThread(s), nil, nil, kw("api_version", "v1", "kind", "ConfigMap", "name", "cm")); err != nil {
		t.Fatalf("k8s_get: %v", err)
	}

	if s.gotNS != "metal3-system" || s.gotKind != "ConfigMap" || s.gotName != "cm" {
		t.Errorf("k8s_get asked for %s/%s in %q", s.gotKind, s.gotName, s.gotNS)
	}

	if s.uncached {
		t.Error("k8s_get defaulted to an uncached read")
	}
}

func TestK8sGetSectionAndAbsent(t *testing.T) {
	s := &stubKube{obj: map[string]any{"spec": map[string]any{"online": true}, "kind": "BareMetalHost"}}

	got, err := BuiltinK8sGet(kubeThread(s), nil, nil,
		kw("api_version", "metal3.io/v1alpha1", "kind", "BareMetalHost", "name", "n", "section", "spec"))
	if err != nil {
		t.Fatalf("k8s_get with section: %v", err)
	}

	m, ok := ToGo(got).(map[string]any)
	if !ok || m["online"] != true {
		t.Errorf("section = %v, want just the spec", ToGo(got))
	}

	// A section the object does not carry is None rather than an error.
	got, err = BuiltinK8sGet(kubeThread(s), nil, nil,
		kw("api_version", "v1", "kind", "BareMetalHost", "name", "n", "section", "absent"))
	if err != nil || got != starlark.None {
		t.Errorf("missing section = (%v, %v), want None", got, err)
	}

	// An absent object is None, which is how scripts test for existence.
	empty := &stubKube{}

	got, err = BuiltinK8sGet(kubeThread(empty), nil, nil, kw("api_version", "v1", "kind", "ConfigMap", "name", "gone"))
	if err != nil || got != starlark.None {
		t.Errorf("absent object = (%v, %v), want None", got, err)
	}
}

func TestK8sListApplyPatchDelete(t *testing.T) {
	s := &stubKube{list: []map[string]any{{"metadata": map[string]any{"name": "a"}}}}

	got, err := BuiltinK8sList(kubeThread(s), nil, nil, kw("api_version", "v1", "kind", "ConfigMap"))
	if err != nil {
		t.Fatalf("k8s_list: %v", err)
	}

	if l, ok := got.(*starlark.List); !ok || l.Len() != 1 {
		t.Errorf("k8s_list = %v, want one item", got)
	}

	obj := starlark.NewDict(2)
	_ = obj.SetKey(starlark.String("apiVersion"), starlark.String("v1"))
	_ = obj.SetKey(starlark.String("kind"), starlark.String("ConfigMap"))

	if _, err = BuiltinK8sApply(kubeThread(s), nil, nil, []starlark.Tuple{{starlark.String("object"), obj}}); err != nil {
		t.Fatalf("k8s_apply: %v", err)
	}

	// Force defaults off so a script must opt in to seizing another owner's fields.
	if s.force {
		t.Error("k8s_apply defaulted to force")
	}

	if s.fieldMgr != DefaultFieldManager {
		t.Errorf("field manager = %q, want %q", s.fieldMgr, DefaultFieldManager)
	}

	patch := starlark.NewDict(1)
	_ = patch.SetKey(starlark.String("data"), starlark.String("x"))

	if _, err = BuiltinK8sPatch(kubeThread(s), nil, nil, append(
		kw("api_version", "v1", "kind", "ConfigMap", "name", "cm"),
		starlark.Tuple{starlark.String("patch"), patch}),
	); err != nil {
		t.Fatalf("k8s_patch: %v", err)
	}

	if s.patchType != "merge" || !strings.Contains(s.patchBody, `"data"`) {
		t.Errorf("patch = %q type %q, want a merge patch", s.patchBody, s.patchType)
	}

	if _, err = BuiltinK8sDelete(kubeThread(s), nil, nil, kw("api_version", "v1", "kind", "ConfigMap", "name", "cm")); err != nil {
		t.Fatalf("k8s_delete: %v", err)
	}

	if !s.deleted {
		t.Error("k8s_delete did not reach the resolver")
	}
}

func TestKubeBuiltinsRequireAResolver(t *testing.T) {
	bare := &starlark.Thread{Name: "t"}

	if _, err := BuiltinK8sGet(bare, nil, nil, kw("api_version", "v1", "kind", "ConfigMap", "name", "cm")); err == nil {
		t.Error("k8s_get worked with no resolver configured")
	}

	// A resolver with no namespace on the thread must not fall back to all namespaces.
	noNS := &starlark.Thread{Name: "t"}
	noNS.SetLocal(HostResolverThreadLocal, &stubKube{})

	if _, err := BuiltinK8sGet(noNS, nil, nil, kw("api_version", "v1", "kind", "ConfigMap", "name", "cm")); err == nil {
		t.Error("k8s_get worked with no namespace on the thread")
	}
}

func TestKubeBuiltinsPropagateErrors(t *testing.T) {
	s := &stubKube{err: errors.New("api down")}

	if _, err := BuiltinK8sList(kubeThread(s), nil, nil, kw("api_version", "v1", "kind", "ConfigMap")); err == nil {
		t.Error("k8s_list swallowed a resolver error")
	}
}

func TestHostReadBuiltins(t *testing.T) {
	s := &stubKube{
		secret: "#cloud-config",
		spec:   map[string]any{"online": true},
		status: map[string]any{"poweredOn": true},
	}

	got, err := BuiltinReadHostSecret(kubeThread(s), nil, starlark.Tuple{starlark.String("userData")}, nil)
	if err != nil {
		t.Fatalf("read_host_secret: %v", err)
	}

	if v, _ := starlark.AsString(got); v != "#cloud-config" {
		t.Errorf("read_host_secret = %q", v)
	}

	if s.field != "userData" {
		t.Errorf("resolver saw field %q", s.field)
	}

	// An unknown field is rejected before any lookup happens.
	if _, err = BuiltinReadHostSecret(kubeThread(s), nil, starlark.Tuple{starlark.String("nonsense")}, nil); err == nil {
		t.Error("read_host_secret accepted an unknown field")
	}

	got, err = BuiltinReadHostSpec(kubeThread(s), nil, nil, nil)
	if err != nil {
		t.Fatalf("read_host_spec: %v", err)
	}

	if m, ok := ToGo(got).(map[string]any); !ok || m["online"] != true {
		t.Errorf("read_host_spec = %v", ToGo(got))
	}

	got, err = BuiltinReadHostStatus(kubeThread(s), nil, nil, nil)
	if err != nil {
		t.Fatalf("read_host_status: %v", err)
	}

	if m, ok := ToGo(got).(map[string]any); !ok || m["poweredOn"] != true {
		t.Errorf("read_host_status = %v", ToGo(got))
	}

	// Without a resolver each one names itself, so the script author knows which failed.
	bare := &starlark.Thread{Name: "t"}
	for name, fn := range map[string]func() error{
		"read_host_spec":   func() error { _, e := BuiltinReadHostSpec(bare, nil, nil, nil); return e },
		"read_host_status": func() error { _, e := BuiltinReadHostStatus(bare, nil, nil, nil); return e },
	} {
		err := fn()
		if err == nil || !strings.Contains(err.Error(), name) {
			t.Errorf("%s error = %v, want it to name itself", name, err)
		}
	}
}
