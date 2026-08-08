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
	"cmp"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	"go.starlark.net/starlark"
	"sigs.k8s.io/yaml"
)

// Cache key shared across calls with matching timeout and TLS verify settings.
type httpClientKey struct {
	TimeoutSec    int
	ValidateCerts bool
	// CA file modtime so a rotated CA bundle yields a fresh cached client.
	CAModTimeNano int64
}

// Cache of *http.Client keyed by httpClientKey.
var httpClientCache sync.Map

// Starlark base64_encode(string) returns standard base64 of the input (UTF8 or byte string).
func builtinBase64Encode(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	var s starlark.String

	err := starlark.UnpackPositionalArgs("base64_encode", args, nil, 1, &s)
	if err != nil {
		return starlark.None, err
	}

	return starlark.String(base64.StdEncoding.EncodeToString([]byte(string(s)))), nil
}

// Starlark base64_decode(string) returns the raw bytes as a Starlark string. Errors on invalid input.
func builtinBase64Decode(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	var s starlark.String

	err := starlark.UnpackPositionalArgs("base64_decode", args, nil, 1, &s)
	if err != nil {
		return starlark.None, err
	}

	decoded, err := base64.StdEncoding.DecodeString(string(s))
	if err != nil {
		return starlark.None, fmt.Errorf("base64_decode: %w", err)
	}

	return starlark.String(string(decoded)), nil
}

// Starlark yaml_decode(string) parses YAML/JSON into a Starlark value.
func builtinYAMLDecode(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	var s starlark.String

	err := starlark.UnpackPositionalArgs("yaml_decode", args, nil, 1, &s)
	if err != nil {
		return starlark.None, err
	}

	// Convert to JSON then decode with UseNumber so whole numbers stay ints.
	jsonBytes, err := yaml.YAMLToJSON([]byte(string(s)))
	if err != nil {
		return starlark.None, fmt.Errorf("yaml_decode: %w", err)
	}

	dec := json.NewDecoder(bytes.NewReader(jsonBytes))
	dec.UseNumber()

	var v any
	if err := dec.Decode(&v); err != nil {
		return starlark.None, fmt.Errorf("yaml_decode: %w", err)
	}

	return GoToStarlark(v), nil
}

// Starlark yaml_encode(value) serializes a Starlark value to YAML.
func builtinYAMLEncode(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	var v starlark.Value

	err := starlark.UnpackPositionalArgs("yaml_encode", args, nil, 1, &v)
	if err != nil {
		return starlark.None, err
	}

	out, err := yaml.Marshal(ToGo(v))
	if err != nil {
		return starlark.None, fmt.Errorf("yaml_encode: %w", err)
	}

	return starlark.String(string(out)), nil
}

// Starlark getenv(name) returns the env var or empty string.
func builtinGetenv(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	var name starlark.String

	err := starlark.UnpackPositionalArgs("getenv", args, nil, 1, &name)
	if err != nil {
		return starlark.None, err
	}

	return starlark.String(os.Getenv(string(name))), nil
}

// Starlark read_file(path) returns trimmed file contents or empty string if missing.
func builtinReadFile(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	var path starlark.String

	err := starlark.UnpackPositionalArgs("read_file", args, nil, 1, &path)
	if err != nil {
		return starlark.None, err
	}

	content, err := os.ReadFile(string(path))
	if err != nil {
		if os.IsNotExist(err) {
			return starlark.String(""), nil
		}
		return starlark.None, fmt.Errorf("read_file: %w", err)
	}

	return starlark.String(strings.TrimSpace(string(content))), nil
}

// Return the per host logger from the thread local, falling back to the package logger.
func loggerFromThread(thread *starlark.Thread) logr.Logger {
	if l, ok := thread.Local(LoggerThreadLocal).(logr.Logger); ok {
		return l
	}
	return log
}

// contextFromThread returns the caller's context from the thread, or Background.
// BMC and HTTP builtins use it so a canceled reconcile aborts in flight calls.
func contextFromThread(thread *starlark.Thread) context.Context {
	if ctx, ok := thread.Local(CtxThreadLocal).(context.Context); ok && ctx != nil {
		return ctx
	}

	return context.Background()
}

// Convert Starlark kwargs into logr's alternating key/value slice.
func kwargsToLogrValues(kwargs []starlark.Tuple) []any {
	out := make([]any, 0, 2*len(kwargs)) //nolint:mnd // key+value per kwarg

	for _, kv := range kwargs {
		key, ok := starlark.AsString(kv[0])
		if !ok {
			key = kv[0].String()
		}

		out = append(out, key, ToGo(kv[1]))
	}

	return out
}

// Starlark log_info(msg, **kwargs) emits a structured Info log entry.
func builtinLogInfo(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var msg starlark.String
	if err := starlark.UnpackPositionalArgs("log_info", args, nil, 1, &msg); err != nil {
		return starlark.None, err
	}

	loggerFromThread(thread).Info(string(msg), kwargsToLogrValues(kwargs)...)

	return starlark.None, nil
}

// Starlark log_debug(msg, **kwargs) emits at V(1).
func builtinLogDebug(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var msg starlark.String
	if err := starlark.UnpackPositionalArgs("log_debug", args, nil, 1, &msg); err != nil {
		return starlark.None, err
	}

	loggerFromThread(thread).V(1).Info(string(msg), kwargsToLogrValues(kwargs)...)

	return starlark.None, nil
}

// Starlark log_error(msg, **kwargs) emits via logr.Error with nil error.
func builtinLogError(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var msg starlark.String
	if err := starlark.UnpackPositionalArgs("log_error", args, nil, 1, &msg); err != nil {
		return starlark.None, err
	}

	loggerFromThread(thread).Error(nil, string(msg), kwargsToLogrValues(kwargs)...)

	return starlark.None, nil
}

// Starlark publish_event(reason, message) emits a Kubernetes event via the thread local publisher.
func builtinPublishEvent(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	var reason, message starlark.String

	err := starlark.UnpackPositionalArgs("publish_event", args, nil, 2, &reason, &message) //nolint:mnd // two fixed args
	if err != nil {
		return starlark.None, err
	}

	pub, ok := thread.Local(PublisherThreadLocal).(provisioner.EventPublisher)
	if !ok || pub == nil {
		// Noop when called outside a provisioner method.
		return starlark.None, nil
	}

	pub(string(reason), string(message))

	return starlark.None, nil
}

// Convert http.Header to a Starlark dict mapping strings to lists of strings.
func headersToStarlark(h http.Header) *starlark.Dict {
	d := starlark.NewDict(len(h))

	for k, vals := range h {
		items := MapSlice(vals, func(v string) starlark.Value { return starlark.String(v) })
		_ = d.SetKey(starlark.String(k), starlark.NewList(items))
	}

	return d
}

// Convert a Starlark dict (string or list of strings values) to http.Header.
func dictToHTTPHeader(d *starlark.Dict) (http.Header, error) {
	if d == nil {
		return http.Header{}, nil
	}

	h := make(http.Header, d.Len())

	for _, item := range d.Items() {
		k, keyOK := starlark.AsString(item[0])
		if !keyOK {
			return nil, fmt.Errorf("http_request_raw: header key must be a string, got %s", item[0].Type())
		}

		if s, strOK := starlark.AsString(item[1]); strOK {
			h.Add(k, s)

			continue
		}

		list, listOK := item[1].(*starlark.List)
		if !listOK {
			return nil, fmt.Errorf("http_request_raw: header %q value must be a string or list of strings, got %s", k, item[1].Type())
		}

		for i := range list.Len() {
			s, elemOK := starlark.AsString(list.Index(i))
			if !elemOK {
				return nil, fmt.Errorf("http_request_raw: header %q list element %d must be a string, got %s", k, i, list.Index(i).Type())
			}

			h.Add(k, s)
		}
	}

	return h, nil
}

// Starlark json_decode(string) parses JSON with UseNumber so ints stay ints.
func builtinJSONDecode(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	var s starlark.String

	err := starlark.UnpackPositionalArgs("json_decode", args, nil, 1, &s)
	if err != nil {
		return starlark.None, err
	}

	dec := json.NewDecoder(strings.NewReader(string(s)))
	dec.UseNumber()

	var data any
	if err := dec.Decode(&data); err != nil {
		return starlark.None, fmt.Errorf("json_decode: %w", err)
	}

	return GoToStarlark(data), nil
}

// Starlark json_encode(value) serializes a Starlark value to JSON.
func builtinJSONEncode(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	if len(args) != 1 {
		return starlark.None, fmt.Errorf("json_encode: got %d args, want 1", len(args))
	}

	goVal := ToGo(args[0])

	b, err := json.Marshal(goVal)
	if err != nil {
		return starlark.None, fmt.Errorf("json_encode: %w", err)
	}

	return starlark.String(string(b)), nil
}

// ironicCACertFile returns the CA bundle path used for validated HTTP requests.
func ironicCACertFile() string {
	return cmp.Or(os.Getenv("IRONIC_CACERT_FILE"), "/opt/metal3/certs/ca/tls.crt")
}

// evictStaleCACachedClients removes cached clients that share the timeout and
// verify mode of current but hold a different CA modtime.
func evictStaleCACachedClients(current httpClientKey) {
	httpClientCache.Range(func(k, v any) bool {
		old, ok := k.(httpClientKey)
		if !ok {
			return true
		}

		if old.TimeoutSec == current.TimeoutSec &&
			old.ValidateCerts == current.ValidateCerts &&
			old.CAModTimeNano != current.CAModTimeNano {
			httpClientCache.Delete(old)

			if c, ok := v.(*http.Client); ok {
				c.CloseIdleConnections()
			}
		}

		return true
	})
}

// Return (and lazily build) the cached client for the given key.
func httpClientFor(key httpClientKey) *http.Client {
	// Only *http.Client is ever stored, so assertions always succeed.
	if c, ok := httpClientCache.Load(key); ok {
		client, _ := c.(*http.Client)
		return client
	}

	tlsCfg := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: !key.ValidateCerts,
	}

	if key.ValidateCerts {
		// Seed from the system roots so well known CAs stay trusted, then add the Ironic CA.
		pool, poolErr := x509.SystemCertPool()
		if poolErr != nil || pool == nil {
			pool = x509.NewCertPool()
		}

		if pem, err := os.ReadFile(ironicCACertFile()); err == nil {
			pool.AppendCertsFromPEM(pem)
		}

		tlsCfg.RootCAs = pool
	}

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsCfg,
		},
		Timeout: time.Duration(key.TimeoutSec) * time.Second,
	}

	actual, loaded := httpClientCache.LoadOrStore(key, client)
	cached, _ := actual.(*http.Client)

	// On a fresh store, drop prior clients that differ only by CA modtime so a
	// rotated bundle does not accumulate dead clients.
	if !loaded {
		evictStaleCACachedClients(key)
	}

	return cached
}

// Perform an HTTP request with per request timeout layered over the caller's ctx.
func doHTTPRequest(
	ctx context.Context,
	method, url, username, password string, validateCerts bool, timeoutSec int, body string, extraHeaders http.Header,
) (string, int, http.Header, error) {
	key := httpClientKey{TimeoutSec: timeoutSec, ValidateCerts: validateCerts}
	// Stat the CA so a rotated bundle produces a new key and a fresh client.
	if validateCerts {
		if fi, statErr := os.Stat(ironicCACertFile()); statErr == nil {
			key.CAModTimeNano = fi.ModTime().UnixNano()
		}
	}

	client := httpClientFor(key)

	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}

	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, method, url, bodyReader)
	if err != nil {
		return "", 0, nil, fmt.Errorf("creating request: %w", err)
	}

	for k, vals := range extraHeaders {
		for _, v := range vals {
			// The Content Length header must go through req.ContentLength, not req.Header.
			if strings.EqualFold(k, "Content-Length") {
				cl, parseErr := strconv.ParseInt(v, 10, 64)
				if parseErr != nil {
					return "", 0, nil, fmt.Errorf("parsing Content-Length header: %w", parseErr)
				}

				req.ContentLength = cl

				continue
			}

			req.Header.Add(k, v)
		}
	}

	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", userAgent)
	}

	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "application/json")
	}

	if body != "" && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	if req.Header.Get("X-Auth-Token") == "" && username != "" {
		req.SetBasicAuth(username, password)
	}

	resp, err := client.Do(req) //nolint:gosec // target address is caller supplied by design
	if err != nil {
		return "", 0, nil, fmt.Errorf("HTTP %s %s: %w", method, url, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, nil, fmt.Errorf("reading response: %w", err)
	}

	return string(respBody), resp.StatusCode, resp.Header, nil
}

// Starlark http_request_raw(method, url, user, pass, validateCerts, timeout, body[, headers]) returns (body, status, headers).
func builtinHTTPRequest(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	var (
		method, url, username, password, body starlark.String
		validateCerts                         starlark.Bool
		timeoutSec                            starlark.Int
		headersDict                           *starlark.Dict
	)

	err := starlark.UnpackPositionalArgs("http_request_raw", args, nil, 7, //nolint:mnd // fixed arg count
		&method, &url, &username, &password, &validateCerts, &timeoutSec, &body, &headersDict)
	if err != nil {
		return starlark.None, err
	}

	ts, ok := timeoutSec.Int64()
	if !ok {
		return starlark.None, errors.New("http_request_raw: timeout must be an integer")
	}
	// Zero/negative timeout would cancel immediately, so reject with a clear error.
	if ts <= 0 {
		return starlark.None, fmt.Errorf("http_request_raw: timeout must be > 0, got %d", ts)
	}
	// Clamp to keep the client cache bounded.
	if ts > maxHTTPTimeoutSec {
		ts = maxHTTPTimeoutSec
	}

	extraHeaders, err := dictToHTTPHeader(headersDict)
	if err != nil {
		return starlark.None, err
	}

	ctx := contextFromThread(thread)

	result, statusCode, headers, err := doHTTPRequest(ctx,
		string(method), string(url), string(username), string(password),
		bool(validateCerts), int(ts), string(body), extraHeaders,
	)
	if err != nil {
		return starlark.None, err
	}

	return starlark.Tuple{starlark.String(result), starlark.MakeInt(statusCode), headersToStarlark(headers)}, nil
}

// Builtins returns the predeclared Starlark functions available to provisioner scripts.
func Builtins() starlark.StringDict {
	b := starlark.StringDict{
		"http_request_raw": starlark.NewBuiltin("http_request_raw", builtinHTTPRequest),
		"json_decode":      starlark.NewBuiltin("json_decode", builtinJSONDecode),
		"json_encode":      starlark.NewBuiltin("json_encode", builtinJSONEncode),
		"publish_event":    starlark.NewBuiltin("publish_event", builtinPublishEvent),
		"log_info":         starlark.NewBuiltin("log_info", builtinLogInfo),
		"log_debug":        starlark.NewBuiltin("log_debug", builtinLogDebug),
		"log_error":        starlark.NewBuiltin("log_error", builtinLogError),
		"getenv":           starlark.NewBuiltin("getenv", builtinGetenv),
		"read_file":        starlark.NewBuiltin("read_file", builtinReadFile),
		"yaml_decode":      starlark.NewBuiltin("yaml_decode", builtinYAMLDecode),
		"yaml_encode":      starlark.NewBuiltin("yaml_encode", builtinYAMLEncode),
		"base64_encode":    starlark.NewBuiltin("base64_encode", builtinBase64Encode),
		"base64_decode":    starlark.NewBuiltin("base64_decode", builtinBase64Decode),
		"read_host_secret": starlark.NewBuiltin("read_host_secret", builtinReadHostSecret),
		"read_host_spec":   starlark.NewBuiltin("read_host_spec", builtinReadHostSpec),
		"read_host_status": starlark.NewBuiltin("read_host_status", builtinReadHostStatus),
	}

	maps.Copy(b, ipmiBuiltins())
	maps.Copy(b, redfishBuiltins())
	maps.Copy(b, kubeBuiltins())

	return b
}
