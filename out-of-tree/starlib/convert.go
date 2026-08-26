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
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"go.starlark.net/starlark"
)

// MapToStruct decodes a map returned by a script into a typed Go struct via JSON roundtrip.
func MapToStruct[T any](raw map[string]any) (T, error) {
	var out T

	data, err := json.Marshal(raw)
	if err != nil {
		return out, fmt.Errorf("marshal: %w", err)
	}

	if err := json.Unmarshal(data, &out); err != nil {
		return out, fmt.Errorf("unmarshal: %w", err)
	}

	return out, nil
}

// SliceToStruct decodes a slice returned by a script into a typed Go slice via JSON roundtrip.
func SliceToStruct[T any](items []any) ([]T, error) {
	data, err := json.Marshal(items)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	var out []T
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}

	return out, nil
}

// MapSlice returns a new slice with f applied to every element.
func MapSlice[T, U any](s []T, f func(T) U) []U {
	out := make([]U, len(s))
	for i, e := range s {
		out[i] = f(e)
	}

	return out
}

// DerefOr returns the pointer value when non nil, otherwise the default.
func DerefOr[T any](p *T, def T) T {
	if p != nil {
		return *p
	}

	return def
}

// Numeric constrains the integer and float kinds that Positive accepts.
type Numeric interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~uintptr |
		~float32 | ~float64
}

// Positive returns the pointer value and true when it is non nil and greater than zero.
func Positive[T Numeric](p *T) (T, bool) {
	if p != nil && *p > 0 {
		return *p, true
	}

	var zero T

	return zero, false
}

// PutNonZero sets m[key] to v when v is not the zero value.
func PutNonZero[T comparable](m map[string]any, key string, v T) {
	var zero T
	if v != zero {
		m[key] = v
	}
}

// StructToMap encodes any Go value to a map[string]any via JSON roundtrip (UseNumber preserves int/float).
func StructToMap(v any) (map[string]any, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()

	var m map[string]any

	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}

	// A null input decodes to a nil map, so normalize it to an empty map that
	// callers can freely mutate.
	if m == nil {
		m = map[string]any{}
	}

	return m, nil
}

// GoToStarlark converts a Go value to a Starlark value.
func GoToStarlark(v any) starlark.Value {
	switch val := v.(type) {
	case nil:
		return starlark.None
	case bool:
		return starlark.Bool(val)
	case int:
		return starlark.MakeInt(val)
	case int64:
		return starlark.MakeInt64(val)
	case float32:
		return starlark.Float(float64(val))
	case float64:
		return starlark.Float(val)
	case json.Number:
		// Prefer int to preserve int64 precision. Fall back to float, then string.
		if i, err := val.Int64(); err == nil {
			return starlark.MakeInt64(i)
		}

		if f, err := val.Float64(); err == nil {
			return starlark.Float(f)
		}

		return starlark.String(string(val))
	case string:
		return starlark.String(val)
	case time.Time:
		// Emit RFC3339 so scripts get a stable, parseable representation.
		return starlark.String(val.Format(time.RFC3339Nano))
	case time.Duration:
		// Seconds, matching requeue_after_seconds encoding.
		return starlark.Float(val.Seconds())
	case []byte:
		return starlark.Bytes(string(val))
	case []any:
		if val == nil {
			return starlark.None
		}

		return starlark.NewList(MapSlice(val, GoToStarlark))
	case []string:
		if val == nil {
			return starlark.None
		}

		return starlark.NewList(MapSlice(val, func(s string) starlark.Value { return starlark.String(s) }))
	case map[string]any:
		if val == nil {
			return starlark.None
		}

		d := starlark.NewDict(len(val))

		for k, v := range val {
			_ = d.SetKey(starlark.String(k), GoToStarlark(v))
		}

		return d
	case map[string]string:
		if val == nil {
			return starlark.None
		}

		d := starlark.NewDict(len(val))

		for k, v := range val {
			_ = d.SetKey(starlark.String(k), starlark.String(v))
		}

		return d
	default:
		// Fallback JSON roundtrip so arbitrary structs/slices reach scripts as dicts/lists.
		// UseNumber keeps whole numbers as ints instead of coercing them through float64.
		data, err := json.Marshal(val)
		if err == nil {
			dec := json.NewDecoder(bytes.NewReader(data))
			dec.UseNumber()

			var decoded any
			if err := dec.Decode(&decoded); err == nil {
				return GoToStarlark(decoded)
			}
		}

		return starlark.String(fmt.Sprint(val))
	}
}

// MaxConvertDepth bounds ToGo recursion. A script value can be self referential,
// and a stack overflow is a fatal error that no recover can catch.
const MaxConvertDepth = 100

// ToGo converts a Starlark value to a Go value. A cycle or a structure deeper
// than MaxConvertDepth converts to nil rather than overflowing the stack.
func ToGo(v starlark.Value) any {
	return toGo(v, 0, nil)
}

// toGo carries the recursion depth and the containers already being converted,
// which is how a self referential dict or list is caught.
func toGo(v starlark.Value, depth int, seen map[starlark.Value]bool) any {
	if depth > MaxConvertDepth {
		return nil
	}

	// Only the mutable containers can close a cycle, so only they are tracked.
	switch v.(type) {
	case *starlark.Dict, *starlark.List, *starlark.Set:
		if seen[v] {
			return nil
		}

		if seen == nil {
			seen = map[starlark.Value]bool{}
		}

		seen[v] = true
		defer delete(seen, v)
	}

	switch val := v.(type) {
	case starlark.NoneType:
		return nil
	case starlark.Bool:
		return bool(val)
	case starlark.Int:
		if i, ok := val.Int64(); ok {
			return i
		}

		return val.String()
	case starlark.Float:
		return float64(val)
	case starlark.String:
		return string(val)
	case starlark.Bytes:
		return string(val)
	case *starlark.List:
		result := make([]any, val.Len())
		for i := range val.Len() {
			result[i] = toGo(val.Index(i), depth+1, seen)
		}

		return result
	case starlark.Tuple:
		return MapSlice(val, func(e starlark.Value) any { return toGo(e, depth+1, seen) })
	case *starlark.Set:
		result := make([]any, 0, val.Len())

		iter := val.Iterate()
		defer iter.Done()

		var elem starlark.Value

		for iter.Next(&elem) {
			result = append(result, toGo(elem, depth+1, seen))
		}

		return result
	case *starlark.Dict:
		result := make(map[string]any, val.Len())
		for _, item := range val.Items() {
			// Non string keys fall back to their repr so they do not collapse to one empty key.
			k, ok := starlark.AsString(item[0])
			if !ok {
				k = item[0].String()
			}

			result[k] = toGo(item[1], depth+1, seen)
		}

		return result
	default:
		return v.String()
	}
}

// AsMap asserts a Starlark dict value and converts it to a Go map, using name in errors.
func AsMap(name string, val starlark.Value) (map[string]any, error) {
	d, ok := val.(*starlark.Dict)
	if !ok {
		return nil, fmt.Errorf("%s: expected dict, got %s", name, val.Type())
	}

	m, ok := ToGo(d).(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s: result is not a map", name)
	}

	return m, nil
}

// MapField returns m[key] as T, or the zero value if absent or type mismatched (V(1) log on mismatch).
func MapField[T any](m map[string]any, key string) T {
	v, ok := m[key]
	if !ok {
		var zero T

		return zero
	}

	t, ok := v.(T)
	if !ok {
		var zero T
		log.V(1).Info("starlark type mismatch",
			"key", key,
			"got", fmt.Sprintf("%T", v),
			"want", fmt.Sprintf("%T", zero),
		)

		return zero
	}

	return t
}

// MapFieldTyped returns m[key] as T. An absent key is the zero value, but a key
// present with the wrong type is an error rather than a silently ignored mistake.
func MapFieldTyped[T any](m map[string]any, key string) (T, error) {
	var zero T

	v, ok := m[key]
	// An explicit None converts to a nil value, and None means "no value" in every
	// other result field, so treat it as absent rather than a type error.
	if !ok || v == nil {
		return zero, nil
	}

	t, ok := v.(T)
	if !ok {
		return zero, fmt.Errorf("key %q has type %T, want %T", key, v, zero)
	}

	return t, nil
}

// MapFieldStrict returns m[key] as T, or an error when the key is absent or the wrong type.
func MapFieldStrict[T any](m map[string]any, key string) (T, error) {
	var zero T

	v, ok := m[key]
	if !ok {
		return zero, fmt.Errorf("missing required key %q", key)
	}

	t, ok := v.(T)
	if !ok {
		return zero, fmt.Errorf("key %q has type %T, want %T", key, v, zero)
	}

	return t, nil
}

// MapFieldDuration reads a seconds valued key (int/int64/float64) and returns it as time.Duration.
func MapFieldDuration(m map[string]any, key string) time.Duration {
	v, ok := m[key]
	if !ok {
		return 0
	}

	var seconds int64
	switch val := v.(type) {
	case int:
		seconds = int64(val)
	case int64:
		seconds = val
	case float64:
		// Preserve subsecond precision via float, then clamp.
		// NaN fails every comparison, so reject it alongside negatives.
		if math.IsNaN(val) || val < 0 {
			return 0
		}

		val = min(val, float64(MaxRequeueSeconds))

		return time.Duration(val * float64(time.Second))
	default:
		log.V(1).Info("starlark duration type mismatch",
			"key", key,
			"got", fmt.Sprintf("%T", v),
		)

		return 0
	}

	if seconds < 0 {
		return 0
	}

	seconds = min(seconds, MaxRequeueSeconds)

	return time.Duration(seconds) * time.Second
}
