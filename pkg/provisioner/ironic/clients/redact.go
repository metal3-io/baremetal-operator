package clients

import (
	"net/url"
	"reflect"
	"regexp"
	"strings"
	"unicode"
)

// Stand-in for a value we never want to show, such as a password.
const redactedValue = "<redacted>"

// Stand-in for a URL query parameter value. No angle brackets here, or
// url.Values.Encode would percent-escape them.
const redactedURLValue = "REDACTED"

// Query parameters that tend to carry credentials or signed access tokens.
// Names are compared case-insensitively, component by component, so prefixed
// and suffixed variants are covered without an entry each.
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

// Schemes are case-insensitive (RFC 3986 section 3.1), so an Ironic error
// about HTTPS://user:pass@host has to match too. Apostrophes are legal in
// userinfo, so the match must not end on one - otherwise a password like
// 'hunter2' survives. A quote that merely wraps the URL in prose is trimmed
// later as trailing punctuation.
var urlInTextRegexp = regexp.MustCompile(`(?i)(?:https?|file|oci)://[^\s"<>]+`)

// A quoted value, escaped and unterminated quotes included, so redaction
// fails closed on both.
const quotedValuePattern = `"(?:\\.|[^"\\\n])*"?|'(?:\\.|[^'\\\n])*'?`

// Matches any "name=value" pair in free text and leaves the verdict to
// redactParamMatch. Ironic likes to quote a request without its scheme ("Max
// retries exceeded with url: /image.qcow2?X-Amz-Signature=..."), which
// urlInTextRegexp cannot match. Every pair is matched rather than the known
// names only, so the decision rests on the decoded name here as well and no
// spelling of a name can slip past. The value runs up to the next "&" or
// space, so it keeps commas, semicolons and nested "?" instead of being cut
// short, and it survives its own closing quote ("token='first'tail"). A pair
// hiding inside a value is found by recursing from redactParamMatch.
var paramPairRegexp = regexp.MustCompile(
	`[^\s=&?,;"<>]+=(?:` + quotedValuePattern + `)?[^&\s"<>]*`)

// isParamNameSeparator tells the parts of a parameter name apart. Everything
// that is not a letter or a digit counts, so "x-amz-signature",
// "temp_url_sig", and a name glued to the prose around it ("(secret") all
// break down into components we can look up.
func isParamNameSeparator(r rune) bool {
	return !unicode.IsLetter(r) && !unicode.IsDigit(r)
}

// isSensitiveParamName says whether a query parameter name denotes a
// credential or a signed access token, for both a parsed URL and a pair found
// in free text. The name is percent-decoded first, then matched as a whole and
// component by component, which covers vendor-prefixed and suffixed variants
// (X-Amz-Signature, temp_url_sig, secret_key) without an entry for every one.
// When in doubt, redact: losing a bit of context beats leaking a credential.
func isSensitiveParamName(name string) bool {
	decoded, err := url.QueryUnescape(name)
	if err != nil {
		// A broken escape sequence cannot decode to a denylisted name, so
		// check the literal name instead of giving up.
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

// Trimmed from a URL found in prose: Ironic ends sentences and wraps URLs in
// brackets or quotes, and none of that belongs to the URL.
const urlTrailingPunctuation = `.,;:!?)]}'`

// RedactSensitiveURL removes embedded credentials (user:pass@host) and the
// values of known sensitive query parameters from a URL, so that a pre-signed
// or credentialed image URL does not show up in status, events or logs.
// Input that does not parse as a URL may still hold a credential, so it is
// redacted as a whole instead of being passed through.
func RedactSensitiveURL(raw string) string {
	if raw == "" {
		return raw
	}

	u, err := url.Parse(raw)
	if err != nil {
		// No idea what this is, so don't risk printing it.
		return redactedValue
	}

	// Drop user:pass@host.
	u.User = nil

	// Then the query parameters that carry tokens or signatures.
	if u.RawQuery != "" {
		q, perr := url.ParseQuery(u.RawQuery)
		if perr != nil {
			// ParseQuery silently drops what it cannot read, so redact the
			// whole query rather than hope the secret was not in there.
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

// RedactSensitiveText redacts the URLs inside an arbitrary string, such as
// Ironic's LastError or a log value; RedactSensitiveURL only deals with a
// string that is a URL by itself. Credential-named "name=value" pairs are
// redacted even outside a full URL, because Ironic wraps urllib3 errors that
// repeat the request path without its scheme.
//
// This is defence in depth over text another project wrote, not a complete
// sanitiser: a message is prose, so exotic quoting or nesting can always
// defeat a pattern. The guarantee covers the URLs and node data BMO itself
// hands to Ironic, which RedactSensitiveURL and RedactSensitiveMap redact by
// parsing rather than by matching.
func RedactSensitiveText(s string) string {
	if s == "" {
		return s
	}

	// Pairs go first: a quoted value has to be consumed whole before the URL
	// pass rewrites the prefix in front of it. Otherwise the quote ends the
	// URL match, the empty value before it gets redacted, and the real value
	// is left sitting in the text.
	s = paramPairRegexp.ReplaceAllStringFunc(s, redactParamMatch)

	return urlInTextRegexp.ReplaceAllStringFunc(s, redactURLMatch)
}

// redactURLMatch redacts one URL found inside a larger string, putting back
// the trailing punctuation that the match greedily swallowed.
func redactURLMatch(match string) string {
	raw := strings.TrimRight(match, urlTrailingPunctuation)

	return RedactSensitiveURL(raw) + match[len(raw):]
}

// redactParamMatch redacts the value of a "name=value" pair when the decoded
// name denotes a credential, keeping only the name.
func redactParamMatch(match string) string {
	name, value, found := strings.Cut(match, "=")
	if !found {
		return match
	}
	if !isSensitiveParamName(name) {
		// A benign name can still carry a whole request path, quoted or not,
		// with a sensitive pair inside it ("url='/image?X-Amz-Signature=...'"),
		// and the value swallowed that pair before anyone looked at it. Every
		// nested value is shorter than the match, so this terminates.
		return name + "=" + paramPairRegexp.ReplaceAllStringFunc(value, redactParamMatch)
	}

	// The value goes whole, trailing punctuation included. "password=hunter2!"
	// and "password=!!!" are both legal query values, so putting the "!" back
	// as if it belonged to the sentence would put the credential back with it.
	return name + "=" + redactedURLValue
}

// RedactSensitiveMap copies a map of Ironic node data (InstanceInfo,
// DriverInfo, ...) into something safe to log: values whose key names a
// credential are replaced outright, and URLs lose their embedded credentials
// and signed query parameters. The input map is left untouched.
func RedactSensitiveMap(data map[string]any) map[string]any {
	if data == nil {
		return nil
	}

	// sanitisedValue keeps the type of the map it is given, so this assertion
	// always holds.
	safe, _ := sanitisedValue(data).(map[string]any)

	return safe
}

// redactLogValue redacts string log values and leaves everything else alone.
func redactLogValue(v any) any {
	if s, ok := v.(string); ok {
		return RedactSensitiveText(s)
	}
	return v
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
		datum := safeDatumValue.Interface()
		if isSensitiveOption(strings.ToLower(k.String())) {
			safeDatumValue = reflect.ValueOf(redactedValue)
		} else if s, ok := datum.(string); ok {
			// Not being named like a credential does not mean it isn't one:
			// Register logs the whole DriverInfo map when the BMC credentials
			// or address change.
			safeDatumValue = reflect.ValueOf(RedactSensitiveText(s))
		} else if datum != nil && reflect.TypeOf(datum).Kind() == reflect.Map {
			// Ironic nests maps in DriverInfo and InstanceInfo
			// (deploy_kernel_by_arch, root_device), and the strings in there
			// are URLs just as often.
			safeDatumValue = reflect.ValueOf(sanitisedValue(datum))
		}
		safeValue.SetMapIndex(k, safeDatumValue)
	}

	return safeValue.Interface()
}

func isSensitiveOption(name string) bool {
	return strings.Contains(name, "password") || strings.Contains(name, "secret")
}
