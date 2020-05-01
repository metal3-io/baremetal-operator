/*
Copyright 2015 The Kubernetes Authors.

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

package orderedmap

import (
	"fmt"
	"strings"
)

// NestedFieldCopy returns a deep copy of the value of a nested field.
// Returns false if the value is missing.
// No error is returned for a nil field.
//
// Note: fields passed to this function are treated as keys within the passed
// object; no array/slice syntax is supported.
func NestedFieldCopy(obj *OrderedMap, fields ...string) (interface{}, bool, error) {
	val, found, err := NestedFieldNoCopy(obj, fields...)
	if !found || err != nil {
		return nil, found, err
	}
	return DeepCopyJSONValue(val), true, nil
}

// NestedFieldNoCopy returns a reference to a nested field.
// Returns false if value is not found and an error if unable
// to traverse obj.
//
// Note: fields passed to this function are treated as keys within the passed
// object; no array/slice syntax is supported.
func NestedFieldNoCopy(obj *OrderedMap, fields ...string) (interface{}, bool, error) {
	var val interface{} = obj

	for i, field := range fields {
		if val == nil {
			return nil, false, nil
		}
		if m, ok := val.(*OrderedMap); ok {
			val, ok = m.Get(field)
			if !ok {
				return nil, false, nil
			}
		} else {
			return nil, false, fmt.Errorf("%v accessor error: %v is of the type %T, expected OrderedMap", jsonPath(fields[:i+1]), val, val)
		}
	}
	return val, true, nil
}

// NestedString returns the string value of a nested field.
// Returns false if value is not found and an error if not a string.
func NestedString(obj *OrderedMap, fields ...string) (string, bool, error) {
	val, found, err := NestedFieldNoCopy(obj, fields...)
	if !found || err != nil {
		return "", found, err
	}
	s, ok := val.(string)
	if !ok {
		return "", false, fmt.Errorf("%v accessor error: %v is of the type %T, expected string", jsonPath(fields), val, val)
	}
	return s, true, nil
}

// NestedBool returns the bool value of a nested field.
// Returns false if value is not found and an error if not a bool.
func NestedBool(obj *OrderedMap, fields ...string) (bool, bool, error) {
	val, found, err := NestedFieldNoCopy(obj, fields...)
	if !found || err != nil {
		return false, found, err
	}
	b, ok := val.(bool)
	if !ok {
		return false, false, fmt.Errorf("%v accessor error: %v is of the type %T, expected bool", jsonPath(fields), val, val)
	}
	return b, true, nil
}

// NestedFloat64 returns the float64 value of a nested field.
// Returns false if value is not found and an error if not a float64.
func NestedFloat64(obj *OrderedMap, fields ...string) (float64, bool, error) {
	val, found, err := NestedFieldNoCopy(obj, fields...)
	if !found || err != nil {
		return 0, found, err
	}
	f, ok := val.(float64)
	if !ok {
		return 0, false, fmt.Errorf("%v accessor error: %v is of the type %T, expected float64", jsonPath(fields), val, val)
	}
	return f, true, nil
}

// NestedInt64 returns the int64 value of a nested field.
// Returns false if value is not found and an error if not an int64.
func NestedInt64(obj *OrderedMap, fields ...string) (int64, bool, error) {
	val, found, err := NestedFieldNoCopy(obj, fields...)
	if !found || err != nil {
		return 0, found, err
	}
	i, ok := val.(int64)
	if !ok {
		return 0, false, fmt.Errorf("%v accessor error: %v is of the type %T, expected int64", jsonPath(fields), val, val)
	}
	return i, true, nil
}

// NestedStringSlice returns a copy of []string value of a nested field.
// Returns false if value is not found and an error if not a []interface{} or contains non-string items in the slice.
func NestedStringSlice(obj *OrderedMap, fields ...string) ([]string, bool, error) {
	val, found, err := NestedFieldNoCopy(obj, fields...)
	if !found || err != nil {
		return nil, found, err
	}
	m, ok := val.([]interface{})
	if !ok {
		return nil, false, fmt.Errorf("%v accessor error: %v is of the type %T, expected []interface{}", jsonPath(fields), val, val)
	}
	strSlice := make([]string, 0, len(m))
	for _, v := range m {
		if str, ok := v.(string); ok {
			strSlice = append(strSlice, str)
		} else {
			return nil, false, fmt.Errorf("%v accessor error: contains non-string key in the slice: %v is of the type %T, expected string", jsonPath(fields), v, v)
		}
	}
	return strSlice, true, nil
}

// NestedSlice returns a deep copy of []interface{} value of a nested field.
// Returns false if value is not found and an error if not a []interface{}.
func NestedSlice(obj *OrderedMap, fields ...string) ([]interface{}, bool, error) {
	val, found, err := NestedFieldNoCopy(obj, fields...)
	if !found || err != nil {
		return nil, found, err
	}
	_, ok := val.([]interface{})
	if !ok {
		return nil, false, fmt.Errorf("%v accessor error: %v is of the type %T, expected []interface{}", jsonPath(fields), val, val)
	}
	return DeepCopyJSONValue(val).([]interface{}), true, nil
}

// NestedStringMap returns a copy of map[string]string value of a nested field.
// Returns false if value is not found and an error if not a OrderedMap or contains non-string values in the map.
func NestedStringMap(obj *OrderedMap, fields ...string) (map[string]string, bool, error) {
	m, found, err := nestedMapNoCopy(obj, fields...)
	if !found || err != nil {
		return nil, found, err
	}
	strMap := make(map[string]string, m.Len())
	for _, k := range m.Keys() {
		v, _ := m.Get(k)
		if str, ok := v.(string); ok {
			strMap[k] = str
		} else {
			return nil, false, fmt.Errorf("%v accessor error: contains non-string key in the map: %v is of the type %T, expected string", jsonPath(fields), v, v)
		}
	}
	return strMap, true, nil
}

// NestedMap returns a deep copy of OrderedMap value of a nested field.
// Returns false if value is not found and an error if not a OrderedMap.
func NestedMap(obj *OrderedMap, fields ...string) (*OrderedMap, bool, error) {
	m, found, err := nestedMapNoCopy(obj, fields...)
	if !found || err != nil {
		return nil, found, err
	}
	return DeepCopyJSON(m), true, nil
}

// nestedMapNoCopy returns a OrderedMap value of a nested field.
// Returns false if value is not found and an error if not a OrderedMap.
func nestedMapNoCopy(obj *OrderedMap, fields ...string) (*OrderedMap, bool, error) {
	val, found, err := NestedFieldNoCopy(obj, fields...)
	if !found || err != nil {
		return nil, found, err
	}
	m, ok := val.(*OrderedMap)
	if !ok {
		return nil, false, fmt.Errorf("%v accessor error: %v is of the type %T, expected OrderedMap", jsonPath(fields), val, val)
	}
	return m, true, nil
}

// SetNestedField sets the value of a nested field to a deep copy of the value provided.
// Returns an error if value cannot be set because one of the nesting levels is not a OrderedMap.
func SetNestedField(obj *OrderedMap, value interface{}, fields ...string) error {
	return setNestedFieldNoCopy(obj, DeepCopyJSONValue(value), fields...)
}

func setNestedFieldNoCopy(obj *OrderedMap, value interface{}, fields ...string) error {
	m := obj

	for i, field := range fields[:len(fields)-1] {
		if val, ok := m.Get(field); ok {
			if valMap, ok := val.(*OrderedMap); ok {
				m = valMap
			} else {
				return fmt.Errorf("value cannot be set because %v is not a OrderedMap", jsonPath(fields[:i+1]))
			}
		} else {
			newVal := New()
			m.Set(field, newVal)
			m = newVal
		}
	}
	m.Set(fields[len(fields)-1], value)
	return nil
}

// SetNestedStringSlice sets the string slice value of a nested field.
// Returns an error if value cannot be set because one of the nesting levels is not a OrderedMap.
func SetNestedStringSlice(obj *OrderedMap, value []string, fields ...string) error {
	m := make([]interface{}, 0, len(value)) // convert []string into []interface{}
	for _, v := range value {
		m = append(m, v)
	}
	return setNestedFieldNoCopy(obj, m, fields...)
}

// SetNestedSlice sets the slice value of a nested field.
// Returns an error if value cannot be set because one of the nesting levels is not a OrderedMap.
func SetNestedSlice(obj *OrderedMap, value []interface{}, fields ...string) error {
	return SetNestedField(obj, value, fields...)
}

// SetNestedStringMap sets the map[string]string value of a nested field.
// Returns an error if value cannot be set because one of the nesting levels is not a OrderedMap.
func SetNestedStringMap(obj *OrderedMap, value map[string]string, fields ...string) error {
	m := New() // convert map[string]string into OrderedMap
	for k, v := range value {
		m.Set(k, v)
	}
	return setNestedFieldNoCopy(obj, m, fields...)
}

// SetNestedMap sets the OrderedMap value of a nested field.
// Returns an error if value cannot be set because one of the nesting levels is not a OrderedMap.
func SetNestedMap(obj *OrderedMap, value OrderedMap, fields ...string) error {
	return SetNestedField(obj, value, fields...)
}

// RemoveNestedField removes the nested field from the obj.
func RemoveNestedField(obj *OrderedMap, fields ...string) {
	m := obj
	for _, field := range fields[:len(fields)-1] {
		if x, ok := m.Get(field); ok {
			m = x.(*OrderedMap)
		} else {
			return
		}
	}
	m.Delete(fields[len(fields)-1])
}

func jsonPath(fields []string) string {
	return "." + strings.Join(fields, ".")
}
