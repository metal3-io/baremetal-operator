package clients

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/go-logr/logr"
	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/nodes"
)

type UpdateOptsData map[string]any

func optionValueEqual(current, value any) bool {
	if reflect.DeepEqual(current, value) {
		return true
	}
	switch curVal := current.(type) {
	case []any:
		// newType could reasonably be either []interface{} or e.g. []string,
		// so we must use reflection.
		newType := reflect.TypeOf(value)
		switch newType.Kind() {
		case reflect.Slice, reflect.Array:
		default:
			return false
		}
		newList := reflect.ValueOf(value)
		if newList.Len() != len(curVal) {
			return false
		}
		for i, v := range curVal {
			if !optionValueEqual(newList.Index(i).Interface(), v) {
				return false
			}
		}
		return true
	case map[string]any:
		// newType could reasonably be either map[string]interface{} or
		// e.g. map[string]string, so we must use reflection.
		newType := reflect.TypeOf(value)
		if newType.Kind() != reflect.Map ||
			newType.Key().Kind() != reflect.String {
			return false
		}
		newMap := reflect.ValueOf(value)
		if newMap.Len() != len(curVal) {
			return false
		}
		for k, v := range curVal {
			newV := newMap.MapIndex(reflect.ValueOf(k))
			if !(newV.IsValid() && optionValueEqual(newV.Interface(), v)) {
				return false
			}
		}
		return true
	}
	return false
}

func deref(v any) any {
	if v == nil {
		return nil
	}
	if reflect.TypeOf(v).Kind() != reflect.Ptr {
		return v
	}
	ptrVal := reflect.ValueOf(v)
	if ptrVal.IsNil() {
		return nil
	}
	return ptrVal.Elem().Interface()
}

func sanitisedValue(data any) any {
	dataType := reflect.TypeOf(data)
	if dataType.Kind() != reflect.Map ||
		dataType.Key().Kind() != reflect.String {
		return data
	}

	value := reflect.ValueOf(data)
	safeValue := reflect.MakeMap(dataType)

	for _, k := range value.MapKeys() {
		safeDatumValue := value.MapIndex(k)
		if strings.Contains(k.String(), "password") {
			safeDatumValue = reflect.ValueOf("<redacted>")
		}
		safeValue.SetMapIndex(k, safeDatumValue)
	}

	return safeValue.Interface()
}

func getUpdateOperation(name string, currentData map[string]any, desiredValue any, path string, log logr.Logger) *nodes.UpdateOperation {
	current, present := currentData[name]

	desiredValue = deref(desiredValue)
	if desiredValue != nil {
		if !(present && optionValueEqual(deref(current), desiredValue)) {
			if present {
				log.Info("updating option data",
					"value", sanitisedValue(desiredValue),
					"oldValue", current)
			} else {
				log.Info("adding option data",
					"value", sanitisedValue(desiredValue))
			}
			return &nodes.UpdateOperation{
				Op:    nodes.AddOp, // Add also does replace
				Path:  path,
				Value: desiredValue,
			}
		}
	} else {
		if present {
			log.Info("removing option data")
			return &nodes.UpdateOperation{
				Op:   nodes.RemoveOp,
				Path: path,
			}
		}
	}
	return nil
}

type NodeUpdater struct {
	Updates nodes.UpdateOpts
	log     logr.Logger
}

func UpdateOptsBuilder(logger logr.Logger) *NodeUpdater {
	return &NodeUpdater{
		log: logger,
	}
}

func (nu *NodeUpdater) logger(basepath, option string) logr.Logger {
	log := nu.log.WithValues("option", option)
	if basepath != "" {
		log = log.WithValues("section", basepath[1:])
	}
	return log
}

func (nu *NodeUpdater) path(basepath, option string) string {
	return fmt.Sprintf("%s/%s", basepath, option)
}

func (nu *NodeUpdater) setSectionUpdateOpts(currentData map[string]any, settings UpdateOptsData, basepath string) {
	for name, desiredValue := range settings {
		updateOp := getUpdateOperation(name, currentData, desiredValue,
			nu.path(basepath, name), nu.logger(basepath, name))
		if updateOp != nil {
			nu.Updates = append(nu.Updates, *updateOp)
		}
	}
}

func (nu *NodeUpdater) SetTopLevelOpt(name string, desiredValue, currentValue any) *NodeUpdater {
	currentData := map[string]any{name: currentValue}
	desiredData := UpdateOptsData{name: desiredValue}

	nu.setSectionUpdateOpts(currentData, desiredData, "")
	return nu
}

func (nu *NodeUpdater) SetPropertiesOpts(settings UpdateOptsData, node *nodes.Node) *NodeUpdater {
	nu.setSectionUpdateOpts(node.Properties, settings, "/properties")
	return nu
}

func (nu *NodeUpdater) SetInstanceInfoOpts(settings UpdateOptsData, node *nodes.Node) *NodeUpdater {
	nu.setSectionUpdateOpts(node.InstanceInfo, settings, "/instance_info")
	return nu
}

func (nu *NodeUpdater) SetDriverInfoOpts(settings UpdateOptsData, node *nodes.Node) *NodeUpdater {
	nu.setSectionUpdateOpts(node.DriverInfo, settings, "/driver_info")
	return nu
}
