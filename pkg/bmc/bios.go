package bmc

import (
	"reflect"
	"strconv"
)

var (
	trueToEnabled = map[string]string{
		"true":  "Enabled",
		"false": "Disabled",
	}
)

func contains(val string, strList []string) bool {
	for _, cmp := range strList {
		if val == cmp {
			return true
		}
	}
	return false
}

// A private method to build bios settings from different driver
func buildBIOSSettings(biosSettings interface{}, nameMap map[string]string, valueMap map[string]string) []map[string]string {
	settings := []map[string]string{}
	var value string
	var name string
	v := reflect.ValueOf(biosSettings)
	t := reflect.TypeOf(biosSettings)
	for i := 0; v.NumField() > i; i++ {
		name = t.Field(i).Name
		if newName, ok := nameMap[name]; ok {
			if newName != "" {
				name = newName
			}
		} else {
			continue
		}
		switch v.Field(i).Kind() {
		case reflect.String:
			value = v.Field(i).String()
		case reflect.Bool:
			value = strconv.FormatBool(v.Field(i).Bool())
		case reflect.Float32, reflect.Float64:
			value = strconv.FormatFloat(v.Field(i).Float(), 'f', -1, 64)
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			value = strconv.FormatInt(v.Field(i).Int(), 10)
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			value = strconv.FormatUint(v.Field(i).Uint(), 10)
		default:
			value = ""
		}
		if value == "" {
			continue
		}
		if len(valueMap) != 0 && valueMap[value] != "" {
			value = valueMap[value]
		}
		settings = append(
			settings,
			map[string]string{
				"name":  name,
				"value": value,
			},
		)
	}
	return settings
}
