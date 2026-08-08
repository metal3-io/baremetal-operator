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
	"math"
	"testing"

	"go.starlark.net/starlark"
)

func TestMapFieldStrict(t *testing.T) {
	m := map[string]any{"ok": true, "wrong": "nope"}

	if v, err := MapFieldStrict[bool](m, "ok"); err != nil || !v {
		t.Errorf("present bool, v=%v err=%v", v, err)
	}

	if _, err := MapFieldStrict[bool](m, "missing"); err == nil {
		t.Error("missing key should error")
	}

	if _, err := MapFieldStrict[bool](m, "wrong"); err == nil {
		t.Error("wrong type should error")
	}

	if _, err := MapFieldStrict[bool](nil, "any"); err == nil {
		t.Error("nil map should error")
	}
}

func TestMapFieldDurationNaN(t *testing.T) {
	if d := MapFieldDuration(map[string]any{"k": math.NaN()}, "k"); d != 0 {
		t.Errorf("NaN should clamp to 0, got %v", d)
	}
}

func TestToGoNonStringKeys(t *testing.T) {
	d := starlark.NewDict(2)
	_ = d.SetKey(starlark.MakeInt(1), starlark.String("a"))
	_ = d.SetKey(starlark.MakeInt(2), starlark.String("b"))

	m, ok := ToGo(d).(map[string]any)
	if !ok {
		t.Fatalf("ToGo dict is not a map, got %T", ToGo(d))
	}

	if m["1"] != "a" || m["2"] != "b" {
		t.Errorf("non string keys collapsed, got %v", m)
	}
}

func TestDerefOr(t *testing.T) {
	if got := DerefOr(nil, 7); got != 7 {
		t.Errorf("nil should return default, got %d", got)
	}

	if got := DerefOr(new(3), 7); got != 3 {
		t.Errorf("non nil should deref, got %d", got)
	}
}

func TestPositive(t *testing.T) {
	if _, ok := Positive[int](nil); ok {
		t.Error("nil pointer should not be positive")
	}

	if v, ok := Positive(new(5)); !ok || v != 5 {
		t.Errorf("positive int, v=%d ok=%v", v, ok)
	}

	if _, ok := Positive(new(0)); ok {
		t.Error("zero should not be positive")
	}

	if _, ok := Positive(new(-2)); ok {
		t.Error("negative should not be positive")
	}

	if v, ok := Positive(new(1.5)); !ok || v != 1.5 {
		t.Errorf("positive float, v=%v ok=%v", v, ok)
	}
}

func TestPutNonZero(t *testing.T) {
	m := map[string]any{}
	PutNonZero(m, "empty", "")
	PutNonZero(m, "name", "n1")
	PutNonZero(m, "zero", 0)
	PutNonZero(m, "count", 5)

	if _, ok := m["empty"]; ok {
		t.Error("empty string should be skipped")
	}

	if _, ok := m["zero"]; ok {
		t.Error("zero int should be skipped")
	}

	if m["name"] != "n1" || m["count"] != 5 {
		t.Errorf("non zero values missing, got %v", m)
	}
}

func TestAsMap(t *testing.T) {
	d := starlark.NewDict(1)
	_ = d.SetKey(starlark.String("k"), starlark.String("v"))

	m, err := AsMap("t", d)
	if err != nil || m["k"] != "v" {
		t.Errorf("dict to map failed, m=%v err=%v", m, err)
	}

	if _, err := AsMap("t", starlark.String("nope")); err == nil {
		t.Error("non dict should error")
	}
}
