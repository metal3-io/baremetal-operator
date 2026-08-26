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

// Cover for the pure conversion helpers every provisioner method funnels through.

package starlib

import (
	"testing"

	"go.starlark.net/starlark"
)

func TestStructToMap(t *testing.T) {
	type inner struct {
		WWN string `json:"wwn"`
	}

	type outer struct {
		Name  string `json:"name"`
		Size  int64  `json:"sizeBytes"`
		Inner inner  `json:"inner"`
		Skip  string `json:"-"`
	}

	m, err := StructToMap(outer{Name: "disk0", Size: 960197124096, Inner: inner{WWN: "naa.5"}, Skip: "hidden"})
	if err != nil {
		t.Fatalf("StructToMap: %v", err)
	}

	if m["name"] != "disk0" {
		t.Errorf("name = %v", m["name"])
	}

	// UseNumber keeps whole numbers exact instead of routing them through float64,
	// which would round a large disk size.
	if got, ok := m["sizeBytes"].(interface{ Int64() (int64, error) }); !ok {
		t.Errorf("sizeBytes is %T, want a json.Number", m["sizeBytes"])
	} else if v, _ := got.Int64(); v != 960197124096 {
		t.Errorf("sizeBytes = %d, want the exact value", v)
	}

	if _, present := m["Skip"]; present {
		t.Error("a json:\"-\" field leaked into the map")
	}

	// A nil input must yield a map callers can mutate rather than a nil map.
	m, err = StructToMap(nil)
	if err != nil || m == nil {
		t.Fatalf("StructToMap(nil) = (%v, %v), want an empty map", m, err)
	}

	m["k"] = "v"
}

func TestMapToStructAndSliceToStruct(t *testing.T) {
	type drive struct {
		Name string `json:"name"`
		Size int64  `json:"sizeBytes"`
	}

	got, err := MapToStruct[drive](map[string]any{"name": "sda", "sizeBytes": 42})
	if err != nil || got.Name != "sda" || got.Size != 42 {
		t.Errorf("MapToStruct = (%+v, %v)", got, err)
	}

	// A type mismatch must be reported, not silently zeroed.
	if _, err = MapToStruct[drive](map[string]any{"sizeBytes": "not a number"}); err == nil {
		t.Error("MapToStruct accepted a string for an int field")
	}

	list, err := SliceToStruct[drive]([]any{
		map[string]any{"name": "sda"},
		map[string]any{"name": "sdb"},
	})
	if err != nil || len(list) != 2 || list[1].Name != "sdb" {
		t.Errorf("SliceToStruct = (%+v, %v)", list, err)
	}

	if _, err = SliceToStruct[drive]([]any{"not a drive"}); err == nil {
		t.Error("SliceToStruct accepted a bare string")
	}
}

func TestMapField(t *testing.T) {
	m := map[string]any{"s": "text", "b": true, "n": int64(7)}

	if got := MapField[string](m, "s"); got != "text" {
		t.Errorf("string field = %q", got)
	}

	if got := MapField[bool](m, "b"); !got {
		t.Errorf("bool field = %v", got)
	}

	// An absent key reads as the zero value rather than panicking.
	if got := MapField[string](m, "absent"); got != "" {
		t.Errorf("absent key = %q, want empty", got)
	}

	// A type mismatch also reads as zero, the caller decides whether that matters.
	if got := MapField[string](m, "b"); got != "" {
		t.Errorf("mismatched type = %q, want empty", got)
	}
}

// A script can build a self referential value, and an unguarded recursion would
// be a fatal stack overflow that no recover can catch, killing the operator.
func TestToGoSurvivesCycles(t *testing.T) {
	d := starlark.NewDict(2)
	if err := d.SetKey(starlark.String("name"), starlark.String("node-1")); err != nil {
		t.Fatalf("set name: %v", err)
	}

	if err := d.SetKey(starlark.String("self"), d); err != nil {
		t.Fatalf("set self: %v", err)
	}

	got, ok := ToGo(d).(map[string]any)
	if !ok {
		t.Fatalf("ToGo gave %T, want a map", ToGo(d))
	}

	// The rest of the value still converts, only the cycle is dropped.
	if got["name"] != "node-1" {
		t.Errorf("name = %v, want the sibling key to survive", got["name"])
	}

	if got["self"] != nil {
		t.Errorf("self = %v, want nil for the cycle", got["self"])
	}

	// A list that contains itself is the same hazard.
	l := starlark.NewList([]starlark.Value{starlark.String("a")})
	if err := l.Append(l); err != nil {
		t.Fatalf("append self: %v", err)
	}

	items, ok := ToGo(l).([]any)
	if !ok || len(items) != 2 || items[0] != "a" || items[1] != nil {
		t.Errorf("ToGo(list) = %v, want the cycle dropped", ToGo(l))
	}
}

// Sibling references are not cycles and must both convert, since the guard is
// released as each container finishes.
func TestToGoKeepsRepeatedSiblings(t *testing.T) {
	shared := starlark.NewList([]starlark.Value{starlark.String("x")})

	outer := starlark.NewDict(2)
	if err := outer.SetKey(starlark.String("a"), shared); err != nil {
		t.Fatalf("set a: %v", err)
	}

	if err := outer.SetKey(starlark.String("b"), shared); err != nil {
		t.Fatalf("set b: %v", err)
	}

	m, ok := ToGo(outer).(map[string]any)
	if !ok {
		t.Fatalf("ToGo gave %T", ToGo(outer))
	}

	for _, k := range []string{"a", "b"} {
		got, ok := m[k].([]any)
		if !ok || len(got) != 1 || got[0] != "x" {
			t.Errorf("%s = %v, want the shared list converted", k, m[k])
		}
	}
}

// Deep but acyclic nesting is bounded too, so a script cannot overflow by depth.
func TestToGoBoundsDepth(t *testing.T) {
	var v starlark.Value = starlark.String("bottom")
	for range MaxConvertDepth + 50 {
		l := starlark.NewList([]starlark.Value{v})
		v = l
	}

	// The point is that it returns at all rather than crashing.
	if got := ToGo(v); got == nil {
		t.Error("deep value converted to nil at the top level")
	}
}

// None converts to a nil value, and None means "no value" in every other result
// field, so it must read as absent rather than as a type error.
func TestMapFieldTypedTreatsNoneAsAbsent(t *testing.T) {
	got, err := MapFieldTyped[bool](map[string]any{"dirty": nil}, "dirty")
	if err != nil || got {
		t.Errorf("explicit None = (%v, %v), want (false, nil)", got, err)
	}

	// An absent key and a nil map are both still fine.
	if _, err = MapFieldTyped[bool](map[string]any{}, "dirty"); err != nil {
		t.Errorf("absent key: %v", err)
	}

	if _, err = MapFieldTyped[bool](nil, "dirty"); err != nil {
		t.Errorf("nil map: %v", err)
	}

	// A genuinely wrong type is still reported.
	if _, err = MapFieldTyped[bool](map[string]any{"dirty": int64(1)}, "dirty"); err == nil {
		t.Error("an int was accepted for a bool field")
	}
}

// The call deadline bounds a whole script call, so it must not sit below the
// per request HTTP clamp or it would silently cap that tunable.
func TestCallDeadlineExceedsHTTPTimeout(t *testing.T) {
	if MaxCallSeconds <= MaxHTTPTimeoutSec {
		t.Errorf("MaxCallSeconds %d must exceed MaxHTTPTimeoutSec %d, or a permitted long request can never complete",
			MaxCallSeconds, MaxHTTPTimeoutSec)
	}
}
