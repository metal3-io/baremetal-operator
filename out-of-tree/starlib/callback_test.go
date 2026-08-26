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

func TestHostSecretName(t *testing.T) {
	if got := HostSecretName("node-1"); got != "node-1-starlark" {
		t.Errorf("HostSecretName = %q", got)
	}
}

func TestBuiltinsIncludesCallback(t *testing.T) {
	b := Builtins()
	for _, name := range []string{"callback_url", "callback_token", "callback_get", "callback_clear"} {
		if _, ok := b[name]; !ok {
			t.Errorf("Builtins is missing %q", name)
		}
	}
}

func TestCallbackURLAndToken(t *testing.T) {
	thread := &starlark.Thread{Name: "t"}
	thread.SetLocal(CallbackThreadLocal, &CallbackContext{URL: "http://cb:9080/callback/ns/n1", Token: "tok123"})
	thread.SetLocal(HostNamespaceThreadLocal, "ns")
	thread.SetLocal(HostNameThreadLocal, "n1")

	urlVal, err := BuiltinCallbackURL(thread, nil, starlark.Tuple{}, nil)
	if err != nil {
		t.Fatalf("callback_url error: %v", err)
	}

	if s, _ := starlark.AsString(urlVal); s != "http://cb:9080/callback/ns/n1" {
		t.Errorf("callback_url = %q", s)
	}

	tokVal, err := BuiltinCallbackToken(thread, nil, starlark.Tuple{}, nil)
	if err != nil {
		t.Fatalf("callback_token error: %v", err)
	}

	if s, _ := starlark.AsString(tokVal); s != "tok123" {
		t.Errorf("callback_token = %q", s)
	}
}

func TestCallbackUnconfigured(t *testing.T) {
	thread := &starlark.Thread{Name: "t"}

	if _, err := BuiltinCallbackURL(thread, nil, starlark.Tuple{}, nil); err == nil {
		t.Error("callback_url should error when callbacks are unconfigured")
	}

	if _, err := BuiltinCallbackToken(thread, nil, starlark.Tuple{}, nil); err == nil {
		t.Error("callback_token should error when callbacks are unconfigured")
	}
}

// stubStore records DeleteCallback calls and returns canned ReadCallback data.
type stubStore struct {
	data    map[string]any
	cleared bool
}

func (s *stubStore) ReadCallback(_ context.Context, _, _ string) (map[string]any, error) {
	return s.data, nil
}

func (s *stubStore) DeleteCallback(_ context.Context, _, _ string) error {
	s.cleared = true

	return nil
}

func callbackStoreThread(store CallbackStore) *starlark.Thread {
	thread := &starlark.Thread{Name: "t"}
	thread.SetLocal(HostResolverThreadLocal, store)
	thread.SetLocal(HostNamespaceThreadLocal, "ns")
	thread.SetLocal(HostNameThreadLocal, "n1")

	return thread
}

func TestCallbackGet(t *testing.T) {
	thread := callbackStoreThread(&stubStore{data: map[string]any{"data": "hello", "receivedAt": "now"}})

	val, err := BuiltinCallbackGet(thread, nil, starlark.Tuple{}, nil)
	if err != nil {
		t.Fatalf("callback_get error: %v", err)
	}

	dict, ok := val.(*starlark.Dict)
	if !ok {
		t.Fatalf("callback_get returned %T, want dict", val)
	}

	got, _, _ := dict.Get(starlark.String("data"))
	if s, _ := starlark.AsString(got); s != "hello" {
		t.Errorf("callback_get data = %q", s)
	}
}

func TestCallbackGetAbsent(t *testing.T) {
	thread := callbackStoreThread(&stubStore{data: nil})

	val, err := BuiltinCallbackGet(thread, nil, starlark.Tuple{}, nil)
	if err != nil {
		t.Fatalf("callback_get error: %v", err)
	}

	if val != starlark.None {
		t.Errorf("callback_get with no data = %v, want None", val)
	}
}

func TestCallbackClear(t *testing.T) {
	store := &stubStore{}
	thread := callbackStoreThread(store)

	if _, err := BuiltinCallbackClear(thread, nil, starlark.Tuple{}, nil); err != nil {
		t.Fatalf("callback_clear error: %v", err)
	}

	if !store.cleared {
		t.Error("callback_clear did not delete the stored data")
	}
}

func TestCallbackGetNoStore(t *testing.T) {
	thread := &starlark.Thread{Name: "t"}

	if _, err := BuiltinCallbackGet(thread, nil, starlark.Tuple{}, nil); err == nil {
		t.Error("callback_get should error without a callback store")
	}
}

// The context can be present with no URL, so callback_url must still refuse it.
func TestCallbackEmptyURLIsRefused(t *testing.T) {
	thread := &starlark.Thread{Name: "t"}
	thread.SetLocal(CallbackThreadLocal, &CallbackContext{Token: "tok123"})

	if v, err := BuiltinCallbackURL(thread, nil, starlark.Tuple{}, nil); err == nil {
		t.Errorf("callback_url = %v, want an error for an empty URL", v)
	}

	// The token is independent of the URL and must still be readable.
	tok, err := BuiltinCallbackToken(thread, nil, starlark.Tuple{}, nil)
	if err != nil {
		t.Fatalf("callback_token: %v", err)
	}

	if s, _ := starlark.AsString(tok); s != "tok123" {
		t.Errorf("callback_token = %q, want tok123", s)
	}
}
