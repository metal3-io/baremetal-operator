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
	"testing"

	"go.starlark.net/starlark"
)

// stubServeStore records serve writes and deletes without a Kubernetes client.
type stubServeStore struct {
	wrote   map[string]ServeRegistration
	deleted []string
}

func (s *stubServeStore) WriteServe(_ context.Context, _, _, id string, reg ServeRegistration) error {
	if s.wrote == nil {
		s.wrote = map[string]ServeRegistration{}
	}

	s.wrote[id] = reg

	return nil
}

func (s *stubServeStore) DeleteServe(_ context.Context, _, _, id string) error {
	s.deleted = append(s.deleted, id)

	return nil
}

func serveThread(store ServeStore, urlPrefix string) *starlark.Thread {
	thread := &starlark.Thread{Name: "t"}
	thread.SetLocal(ServeThreadLocal, &ServeContext{URLPrefix: urlPrefix})
	thread.SetLocal(HostResolverThreadLocal, store)
	thread.SetLocal(HostNamespaceThreadLocal, "ns")
	thread.SetLocal(HostNameThreadLocal, "n1")

	return thread
}

func TestBuiltinsIncludesServe(t *testing.T) {
	b := Builtins()
	for _, name := range []string{"serve_register", "serve_deregister", "serve_url"} {
		if _, ok := b[name]; !ok {
			t.Errorf("Builtins is missing %q", name)
		}
	}
}

func TestBuiltinServeRegisterAndURL(t *testing.T) {
	store := &stubServeStore{}
	thread := serveThread(store, "http://cb:9080/serve/uid-1/")

	kwargs := []starlark.Tuple{
		{starlark.String("id"), starlark.String("ks")},
		{starlark.String("configmap"), starlark.String("ks-cm")},
		{starlark.String("key"), starlark.String("ks.cfg")},
		{starlark.String("render"), starlark.Bool(true)},
	}

	if _, err := BuiltinServeRegister(thread, nil, starlark.Tuple{}, kwargs); err != nil {
		t.Fatalf("serve_register: %v", err)
	}

	got, ok := store.wrote["ks"]
	if !ok || got.ConfigMap != "ks-cm" || got.Key != "ks.cfg" || !got.Render {
		t.Fatalf("written registration = %+v, ok=%v", got, ok)
	}

	urlVal, err := BuiltinServeURL(thread, nil, starlark.Tuple{starlark.String("ks")}, nil)
	if err != nil {
		t.Fatalf("serve_url: %v", err)
	}

	if s, _ := starlark.AsString(urlVal); s != "http://cb:9080/serve/uid-1/ks" {
		t.Errorf("serve_url = %q", s)
	}

	if _, err := BuiltinServeDeregister(thread, nil, starlark.Tuple{starlark.String("ks")}, nil); err != nil {
		t.Fatalf("serve_deregister: %v", err)
	}

	if len(store.deleted) != 1 || store.deleted[0] != "ks" {
		t.Errorf("deleted = %v, want [ks]", store.deleted)
	}
}

func TestBuiltinServeRegisterVars(t *testing.T) {
	store := &stubServeStore{}
	thread := serveThread(store, "http://cb/serve/uid-1/")

	vars := starlark.NewDict(2)
	_ = vars.SetKey(starlark.String("disk"), starlark.String("/dev/sda"))
	_ = vars.SetKey(starlark.String("n"), starlark.MakeInt(3))

	kwargs := []starlark.Tuple{
		{starlark.String("id"), starlark.String("ks")},
		{starlark.String("configmap"), starlark.String("cm")},
		{starlark.String("key"), starlark.String("k")},
		{starlark.String("vars"), vars},
	}

	if _, err := BuiltinServeRegister(thread, nil, starlark.Tuple{}, kwargs); err != nil {
		t.Fatalf("serve_register: %v", err)
	}

	got := store.wrote["ks"]
	if got.Vars["disk"] != "/dev/sda" {
		t.Errorf("vars[disk] = %v, want /dev/sda", got.Vars["disk"])
	}

	if _, present := got.Vars["n"]; !present || len(got.Vars) != 2 {
		t.Errorf("vars = %v, want both disk and n present", got.Vars)
	}
}

func TestServeRegisterRejectsBadID(t *testing.T) {
	store := &stubServeStore{}
	thread := serveThread(store, "http://cb/serve/uid-1/")

	for _, bad := range []string{"", "a/b", "a b", "a%2Fb"} {
		kwargs := []starlark.Tuple{
			{starlark.String("id"), starlark.String(bad)},
			{starlark.String("configmap"), starlark.String("cm")},
			{starlark.String("key"), starlark.String("k")},
		}

		if _, err := BuiltinServeRegister(thread, nil, starlark.Tuple{}, kwargs); err == nil {
			t.Errorf("serve_register(id=%q) should be rejected", bad)
		}
	}

	if len(store.wrote) != 0 {
		t.Errorf("no bad id should have been written, got %v", store.wrote)
	}
}

func TestServeUnconfigured(t *testing.T) {
	thread := &starlark.Thread{Name: "t"}

	if _, err := BuiltinServeURL(thread, nil, starlark.Tuple{starlark.String("ks")}, nil); err == nil {
		t.Error("serve_url should error when serving is unconfigured")
	}
}

// A present but empty context is what the gate keys on. Without it an empty prefix
// hands a script a bare id, and serve_register persists an unserved route.
func TestServeEmptyContextIsUnconfigured(t *testing.T) {
	thread := &starlark.Thread{Name: "t"}
	thread.SetLocal(ServeThreadLocal, &ServeContext{})
	thread.SetLocal(HostResolverThreadLocal, &stubServeStore{})
	thread.SetLocal(HostNamespaceThreadLocal, "ns")
	thread.SetLocal(HostNameThreadLocal, "n1")

	if v, err := BuiltinServeURL(thread, nil, starlark.Tuple{starlark.String("ks")}, nil); err == nil {
		t.Errorf("serve_url = %v, want an error for an empty prefix", v)
	}

	kwargs := []starlark.Tuple{
		{starlark.String("id"), starlark.String("ks")},
		{starlark.String("configmap"), starlark.String("cm")},
		{starlark.String("key"), starlark.String("ks.cfg")},
	}

	if _, err := BuiltinServeRegister(thread, nil, nil, kwargs); err == nil {
		t.Error("serve_register should refuse a route the listener cannot serve")
	}
}
