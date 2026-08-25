package clients

import (
	"fmt"
	"net/url"
	"reflect"
	"regexp"
	"strings"

	"github.com/go-logr/logr"
	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/nodes"
)

type UpdateOptsData map[string]any

// sensitiveURLQueryParams are query parameter names (compared
// case-insensitively) whose values may carry credential or signed access
// tokens and must not be leaked in status, events or logs.
var sensitiveURLQueryParams = map[string]struct{}{
	"token":                {},
	"access_token":         {},
	"accesstoken":          {},
	"signature":            {},
	"sig":                  {},
	"x-amz-signature":      {},
	"x-amz-credential":     {},
	"x-amz-security-token": {},
	"x-goog-signature":     {}, // GCS V4 signed URL
	"x-goog-credential":    {}, // GCS V4 signed URL
	"temp_url_sig":         {}, // OpenStack Swift TempURL
}

var urlInTextRegexp = regexp.MustCompile(`(?:https?|file|oci)://[^\s"'<>]+`)

// RedactSensitiveURL redacts embedded credentials (user:pass@host) and the
// values of known-sensitive query parameters from a bare URL, so pre-signed or
// credentialed image URLs are not disclosed in status, events or logs. If the
// input cannot be parsed as a URL it is returned unchanged.
func RedactSensitiveURL(raw string) string {
	if raw == "" {
		return raw
	}

	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}

	// Strip embedded credentials (user:pass@host).
	u.User = nil

	// Redact sensitive query parameters.
	if u.RawQuery != "" {
		q, perr := url.ParseQuery(u.RawQuery)
		if perr != nil {
			// Malformed query: fail safe rather than risk leaking a value
			// that ParseQuery would silently drop.
			u.RawQuery = "REDACTED"
		} else {
			changed := false
			for key := range q {
				if _, ok := sensitiveURLQueryParams[strings.ToLower(key)]; ok {
					q.Set(key, "REDACTED")
					changed = true
				}
			}
			if changed {
				u.RawQuery = q.Encode()
			}
		}
	}

	return u.String()
}

// RedactSensitiveText redacts any URLs embedded inside an arbitrary string,
// such as Ironic's LastError or a log value (RedactSensitiveURL only handles a
// string that is itself a bare URL).
func RedactSensitiveText(s string) string {
	if s == "" {
		return s
	}

	return urlInTextRegexp.ReplaceAllStringFunc(s, RedactSensitiveURL)
}

// redactLogValue redacts string log values that contain a URL, leaving
// non-string values untouched.
func redactLogValue(v any) any {
	if s, ok := v.(string); ok {
		return RedactSensitiveText(s)
	}
	return v
}

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
	if data == nil {
		return nil
	}
	dataType := reflect.TypeOf(data)
	if dataType.Kind() != reflect.Map ||
		dataType.Key().Kind() != reflect.String {
		return data
	}

	value := reflect.ValueOf(data)
	safeValue := reflect.MakeMap(dataType)

	for _, k := range value.MapKeys() {
		safeDatumValue := value.MapIndex(k)
		if strings.Contains(k.String(), "password") || strings.Contains(k.String(), "secret") {
			safeDatumValue = reflect.ValueOf("<redacted>")
		}
		safeValue.SetMapIndex(k, safeDatumValue)
	}

	return safeValue.Interface()
}

func isSensitiveOption(name string) bool {
	return strings.Contains(name, "password") || strings.Contains(name, "secret")
}

func getUpdateOperation(name string, currentData map[string]any, desiredValue any, path string, log logr.Logger) *nodes.UpdateOperation {
	current, present := currentData[name]

	desiredValue = deref(desiredValue)
	if desiredValue != nil {
		if !(present && optionValueEqual(deref(current), desiredValue)) {
			logValue := redactLogValue(sanitisedValue(desiredValue))
			if isSensitiveOption(name) {
				logValue = "<redacted>"
			}
			if present {
				oldLogValue := redactLogValue(sanitisedValue(current))
				if isSensitiveOption(name) {
					oldLogValue = "<redacted>"
				}
				log.Info("updating option data",
					"value", logValue,
					"oldValue", oldLogValue)
			} else {
				log.Info("adding option data",
					"value", logValue)
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
