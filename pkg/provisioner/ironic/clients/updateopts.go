package clients

import (
	"encoding/hex"
	"fmt"
	"net/url"
	"reflect"
	"regexp"
	"slices"
	"strings"

	"github.com/go-logr/logr"
	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/nodes"
)

type UpdateOptsData map[string]any

// redactedValue replaces the value of a field whose name marks it as a
// credential, so the value never reaches status, events or logs.
const redactedValue = "<redacted>"

// redactedURLValue replaces the value of a sensitive URL query parameter. It
// avoids angle brackets so url.Values.Encode does not percent-escape it.
const redactedURLValue = "REDACTED"

// sensitiveURLQueryParams are query parameter names (compared
// case-insensitively) whose values may carry credential or signed access
// tokens and must not be leaked in status, events or logs.
var sensitiveURLQueryParams = map[string]struct{}{
	"token":                {},
	"access_token":         {},
	"accesstoken":          {},
	"password":             {},
	"secret":               {},
	"signature":            {},
	"sig":                  {},
	"x-amz-signature":      {},
	"x-amz-credential":     {},
	"x-amz-security-token": {},
	"x-goog-signature":     {}, // GCS V4 signed URL
	"x-goog-credential":    {}, // GCS V4 signed URL
	"temp_url_sig":         {}, // OpenStack Swift TempURL
}

// URL schemes are case-insensitive (RFC 3986 section 3.1), so match them that
// way: an Ironic error mentioning HTTPS://user:pass@host must be redacted too.
// An apostrophe is a sub-delim and therefore valid in userinfo and in query
// values, so it must not end the match or a password like 'hunter2' would stay
// in the text; a quote that merely surrounds the URL in prose is stripped
// afterwards as trailing punctuation.
var urlInTextRegexp = regexp.MustCompile(`(?i)(?:https?|file|oci)://[^\s"<>]+`)

// paramNameBoundary must precede a denylisted name, so that
// "x-custom-signature" counts while "design" does not contain a "sig"
// parameter. The separators of isParamNameSeparator are accepted
// percent-encoded too, because "my%5Ftoken" decodes to the "my_token" that the
// parsed-URL path redacts.
const paramNameBoundary = `(?:^|[^a-z0-9]|%2d|%5f|%2e)`

// sensitiveParamRegexp matches a "name=value" pair whose name denotes a
// credential, for Ironic errors that repeat a request without its scheme
// ("Max retries exceeded with url: /image.qcow2?X-Amz-Signature=..."), which
// urlInTextRegexp cannot match. It must stay anchored on the denylisted name:
// a generic "name=value" pattern consumes a benign pair along with everything
// after it, so "os=" would hide the signature in
// "/images/os=linux/image.qcow2?sig=...". A quoted value is consumed whole,
// escaped quotes and unterminated ones included, so redaction fails closed.
var sensitiveParamRegexp = func() *regexp.Regexp {
	names := make([]string, 0, len(sensitiveURLQueryParams))
	for name := range sensitiveURLQueryParams {
		names = append(names, percentEncodedPattern(name))
	}
	slices.Sort(names) // keep the generated pattern deterministic

	return regexp.MustCompile(`(?i)` + paramNameBoundary + `(?:` + strings.Join(names, "|") +
		`)=(?:"(?:\\.|[^"\\\n])*"?|'(?:\\.|[^'\\\n])*'?|[^&\s"<>]*)`)
}()

// percentEncodedPattern returns a pattern matching s with any of its
// characters optionally percent-encoded, so that an encoded parameter name
// cannot bypass the denylist.
func percentEncodedPattern(s string) string {
	parts := make([]string, 0, len(s))
	for i := range len(s) {
		char := s[i : i+1]
		// A letter can be encoded in either case, and "%74" and "%54" are
		// different bytes that (?i) cannot equate, so both are listed.
		lower, upper := strings.ToLower(char), strings.ToUpper(char)
		alternatives := regexp.QuoteMeta(char) + `|%` + hex.EncodeToString([]byte(lower))
		if upper != lower {
			alternatives += `|%` + hex.EncodeToString([]byte(upper))
		}
		parts = append(parts, `(?:`+alternatives+`)`)
	}

	return strings.Join(parts, "")
}

// isParamNameSeparator reports whether a rune separates the components of a
// query parameter name.
func isParamNameSeparator(r rune) bool {
	return r == '-' || r == '_' || r == '.'
}

// isSensitiveParamName reports whether a parsed query parameter name
// designates a credential or a signed access token. It backs the parsed-URL
// path, where url.ParseQuery has already decoded the name; the scheme-less
// path uses sensitiveParamRegexp, generated from the same denylist. Matching is
// component-wise, so vendor-prefixed variants (X-Amz-Signature, temp_url_sig)
// are covered without an entry per prefix. Over-redaction is preferable to
// leaking a credential into status, events or logs.
func isSensitiveParamName(name string) bool {
	decoded, err := url.QueryUnescape(name)
	if err != nil {
		// An invalid escape sequence cannot decode to a denylisted name, so
		// keep checking the literal name rather than giving up on it.
		decoded = name
	}
	decoded = strings.ToLower(decoded)

	if _, ok := sensitiveURLQueryParams[decoded]; ok {
		return true
	}
	for _, part := range strings.FieldsFunc(decoded, isParamNameSeparator) {
		if _, ok := sensitiveURLQueryParams[part]; ok {
			return true
		}
	}

	return false
}

// urlTrailingPunctuation is stripped from a URL matched inside prose: Ironic
// ends sentences and wraps URLs in brackets or quotes, and those characters
// are not part of the URL.
const urlTrailingPunctuation = `.,;:!?)]}'`

// RedactSensitiveURL redacts embedded credentials (user:pass@host) and the
// values of known-sensitive query parameters from a bare URL, so pre-signed or
// credentialed image URLs are not disclosed in status, events or logs. Input
// that cannot be parsed as a URL may still hold credentials, so it is redacted
// wholesale rather than returned as-is.
func RedactSensitiveURL(raw string) string {
	if raw == "" {
		return raw
	}

	u, err := url.Parse(raw)
	if err != nil {
		// Unparsable input may still contain credentials, so fail closed.
		return redactedValue
	}

	// Strip embedded credentials (user:pass@host).
	u.User = nil

	// Redact sensitive query parameters.
	if u.RawQuery != "" {
		q, perr := url.ParseQuery(u.RawQuery)
		if perr != nil {
			// Malformed query: fail safe rather than risk leaking a value
			// that ParseQuery would silently drop.
			u.RawQuery = redactedURLValue
		} else {
			changed := false
			for key := range q {
				if isSensitiveParamName(key) {
					q.Set(key, redactedURLValue)
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
// string that is itself a bare URL). Sensitive "name=value" pairs are redacted
// even outside a full URL, because Ironic wraps urllib3 errors that repeat the
// request path without its scheme.
func RedactSensitiveText(s string) string {
	if s == "" {
		return s
	}

	s = urlInTextRegexp.ReplaceAllStringFunc(s, redactURLMatch)

	return sensitiveParamRegexp.ReplaceAllStringFunc(s, redactParamMatch)
}

// redactURLMatch redacts a URL matched inside a larger string, preserving any
// trailing punctuation that the match greedily consumed.
func redactURLMatch(match string) string {
	raw := strings.TrimRight(match, urlTrailingPunctuation)

	return RedactSensitiveURL(raw) + match[len(raw):]
}

// redactParamMatch redacts the value of a sensitive "name=value" pair, keeping
// the name, the boundary character the match consumed before it, and any
// trailing sentence punctuation. Trimming the value is safe because it is
// discarded anyway.
func redactParamMatch(match string) string {
	name, value, found := strings.Cut(match, "=")
	if !found {
		return match
	}
	// A quoted value was consumed as a unit, quotes included, so there is no
	// punctuation of the surrounding prose to put back.
	if strings.HasPrefix(value, `"`) || strings.HasPrefix(value, "'") {
		return name + "=" + redactedURLValue
	}
	trimmed := strings.TrimRight(match, urlTrailingPunctuation)

	return name + "=" + redactedURLValue + match[len(trimmed):]
}

// RedactSensitiveMap returns a copy of a map of Ironic node data (InstanceInfo,
// DriverInfo, ...) that is safe to log: values of keys that name a credential
// are replaced wholesale, and URLs in string values have their embedded
// credentials and signed query parameters redacted. The input is not modified.
func RedactSensitiveMap(data map[string]any) map[string]any {
	if data == nil {
		return nil
	}

	// sanitisedValue applies the same rules to any string-keyed map and
	// preserves the map type, so the assertion cannot fail.
	safe, _ := sanitisedValue(data).(map[string]any)

	return safe
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
		if isSensitiveOption(strings.ToLower(k.String())) {
			safeDatumValue = reflect.ValueOf(redactedValue)
		} else if s, ok := safeDatumValue.Interface().(string); ok {
			// A value that is not named as a credential can still be a URL
			// carrying one: Register logs the whole DriverInfo map when the
			// BMC credentials or address change.
			safeDatumValue = reflect.ValueOf(RedactSensitiveText(s))
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
				logValue = redactedValue
			}
			if present {
				oldLogValue := redactLogValue(sanitisedValue(current))
				if isSensitiveOption(name) {
					oldLogValue = redactedValue
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
