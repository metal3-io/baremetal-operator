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
	"time"

	"go.starlark.net/starlark"
)

// MapToStruct decodes a script-returned map into a typed Go struct via JSON roundtrip.
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

	// Normalize null/non-object inputs to an empty map so callers can freely mutate.
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
		if val == float64(int64(val)) {
			return starlark.MakeInt64(int64(val))
		}

		return starlark.Float(val)
	case json.Number:
		// Prefer int to preserve int64 precision; fall back to float, then string.
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

		elems := make([]starlark.Value, len(val))

		for i, e := range val {
			elems[i] = GoToStarlark(e)
		}

		return starlark.NewList(elems)
	case []string:
		if val == nil {
			return starlark.None
		}

		elems := make([]starlark.Value, len(val))

		for i, e := range val {
			elems[i] = starlark.String(e)
		}

		return starlark.NewList(elems)
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
		data, err := json.Marshal(val)
		if err == nil {
			var decoded any
			if err := json.Unmarshal(data, &decoded); err == nil {
				return GoToStarlark(decoded)
			}
		}

		return starlark.String(fmt.Sprint(val))
	}
}

// ToGo converts a Starlark value to a Go value.
func ToGo(v starlark.Value) any {
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
			result[i] = ToGo(val.Index(i))
		}

		return result
	case starlark.Tuple:
		result := make([]any, len(val))
		for i, e := range val {
			result[i] = ToGo(e)
		}

		return result
	case *starlark.Set:
		result := make([]any, 0, val.Len())

		iter := val.Iterate()
		defer iter.Done()

		var elem starlark.Value

		for iter.Next(&elem) {
			result = append(result, ToGo(elem))
		}

		return result
	case *starlark.Dict:
		result := make(map[string]any, val.Len())
		for _, item := range val.Items() {
			k, _ := starlark.AsString(item[0])
			result[k] = ToGo(item[1])
		}

		return result
	default:
		return v.String()
	}
}

// MapField returns m[key] as T; zero value if absent or type-mismatched (V(1) log on mismatch).
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

// MapFieldDuration reads a seconds-valued key (int/int64/float64) and returns it as time.Duration.
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
		// Preserve sub-second precision via float, then clamp.
		if val < 0 {
			return 0
		}

		if val > float64(maxRequeueSeconds) {
			return time.Duration(maxRequeueSeconds) * time.Second
		}

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

	if seconds > maxRequeueSeconds {
		seconds = maxRequeueSeconds
	}

	return time.Duration(seconds) * time.Second
}
