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
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

// Cache key shared across calls with matching (timeout, TLS-verify) settings.
type httpClientKey struct {
	TimeoutSec    int
	ValidateCerts bool
}

// Cache of *http.Client keyed by httpClientKey.
var httpClientCache sync.Map

// Builtins returns the predeclared Starlark functions available to provisioner scripts.
func Builtins() starlark.StringDict {
	return starlark.StringDict{
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
	}
}

// Starlark yaml_decode(string): parse YAML/JSON into a Starlark value.
func builtinYAMLDecode(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	var s starlark.String

	err := starlark.UnpackPositionalArgs("yaml_decode", args, nil, 1, &s)
	if err != nil {
		return starlark.None, err
	}

	var v any
	if err := yaml.Unmarshal([]byte(string(s)), &v); err != nil {
		return starlark.None, fmt.Errorf("yaml_decode: %w", err)
	}

	return GoToStarlark(v), nil
}

// Starlark yaml_encode(value): serialize a Starlark value to YAML.
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

// Starlark getenv(name): return the env var or empty string.
func builtinGetenv(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	var name starlark.String

	err := starlark.UnpackPositionalArgs("getenv", args, nil, 1, &name)
	if err != nil {
		return starlark.None, err
	}

	return starlark.String(os.Getenv(string(name))), nil
}

// Starlark read_file(path): return trimmed file contents or empty string if missing.
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

// Return the per-host logger from the thread-local, falling back to the package logger.
func loggerFromThread(thread *starlark.Thread) logr.Logger {
	if l, ok := thread.Local(LoggerThreadLocal).(logr.Logger); ok {
		return l
	}
	return log
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

// Starlark log_info(msg, **kwargs): emit a structured Info log entry.
func builtinLogInfo(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var msg starlark.String
	if err := starlark.UnpackPositionalArgs("log_info", args, nil, 1, &msg); err != nil {
		return starlark.None, err
	}

	loggerFromThread(thread).Info(string(msg), kwargsToLogrValues(kwargs)...)

	return starlark.None, nil
}

// Starlark log_debug(msg, **kwargs): emit at V(1).
func builtinLogDebug(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var msg starlark.String
	if err := starlark.UnpackPositionalArgs("log_debug", args, nil, 1, &msg); err != nil {
		return starlark.None, err
	}

	loggerFromThread(thread).V(1).Info(string(msg), kwargsToLogrValues(kwargs)...)

	return starlark.None, nil
}

// Starlark log_error(msg, **kwargs): emit via logr.Error with nil error.
func builtinLogError(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var msg starlark.String
	if err := starlark.UnpackPositionalArgs("log_error", args, nil, 1, &msg); err != nil {
		return starlark.None, err
	}

	loggerFromThread(thread).Error(nil, string(msg), kwargsToLogrValues(kwargs)...)

	return starlark.None, nil
}

// Starlark publish_event(reason, message): emit a Kubernetes event via the thread-local publisher.
func builtinPublishEvent(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	var reason, message starlark.String

	err := starlark.UnpackPositionalArgs("publish_event", args, nil, 2, &reason, &message) //nolint:mnd // two fixed args
	if err != nil {
		return starlark.None, err
	}

	pub, ok := thread.Local(PublisherThreadLocal).(provisioner.EventPublisher)
	if !ok || pub == nil {
		// No-op when called outside a provisioner method.
		return starlark.None, nil
	}

	pub(string(reason), string(message))

	return starlark.None, nil
}

// Starlark http_request_raw(method, url, user, pass, validateCerts, timeout, body[, headers]): returns (body, status, headers).
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
	// Zero/negative timeout would cancel immediately; reject with a clear error.
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

	ctx, ok := thread.Local(CtxThreadLocal).(context.Context)
	if !ok || ctx == nil {
		ctx = context.Background()
	}

	result, statusCode, headers, err := doHTTPRequest(ctx,
		string(method), string(url), string(username), string(password),
		bool(validateCerts), int(ts), string(body), extraHeaders,
	)
	if err != nil {
		return starlark.None, err
	}

	return starlark.Tuple{starlark.String(result), starlark.MakeInt(statusCode), headersToStarlark(headers)}, nil
}

// Convert http.Header to a Starlark dict of string->list-of-strings.
func headersToStarlark(h http.Header) *starlark.Dict {
	d := starlark.NewDict(len(h))

	for k, vals := range h {
		items := make([]starlark.Value, len(vals))

		for i, v := range vals {
			items[i] = starlark.String(v)
		}

		_ = d.SetKey(starlark.String(k), starlark.NewList(items))
	}

	return d
}

// Convert a Starlark dict (string or list-of-strings values) to http.Header.
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

// Starlark json_decode(string): parse JSON with UseNumber so ints stay ints.
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

// Starlark json_encode(value): serialize a Starlark value to JSON.
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

// Return (and lazily build) the cached client for the given key.
func httpClientFor(key httpClientKey) *http.Client {
	// Only *http.Client is ever stored, so assertions always succeed.
	if c, ok := httpClientCache.Load(key); ok {
		client, _ := c.(*http.Client)
		return client
	}

	tlsCfg := &tls.Config{
		InsecureSkipVerify: !key.ValidateCerts,
	}

	if key.ValidateCerts {
		caFile := os.Getenv("IRONIC_CACERT_FILE")
		if caFile == "" {
			caFile = "/opt/metal3/certs/ca/tls.crt"
		}

		if pem, err := os.ReadFile(caFile); err == nil { //nolint:gosec // operator-controlled CA path
			pool := x509.NewCertPool()
			if pool.AppendCertsFromPEM(pem) {
				tlsCfg.RootCAs = pool
			}
		}
	}

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsCfg,
		},
		Timeout: time.Duration(key.TimeoutSec) * time.Second,
	}

	actual, _ := httpClientCache.LoadOrStore(key, client)
	cached, _ := actual.(*http.Client)

	return cached
}

// Perform an HTTP request with per-request timeout layered over the caller's ctx.
func doHTTPRequest(
	ctx context.Context,
	method, url, username, password string, validateCerts bool, timeoutSec int, body string, extraHeaders http.Header,
) (string, int, http.Header, error) {
	client := httpClientFor(httpClientKey{TimeoutSec: timeoutSec, ValidateCerts: validateCerts})

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
			// Content-Length must go through req.ContentLength, not req.Header.
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

	resp, err := client.Do(req) //nolint:gosec // target address is caller-supplied by design
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
