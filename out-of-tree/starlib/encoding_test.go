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

// Cover for the encoding and environment builtins scripts lean on.

package starlib

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	goipmi "github.com/bougou/go-ipmi"
	"github.com/go-logr/logr"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	"go.starlark.net/starlark"
)

// callBuiltin invokes a builtin with positional args and returns the value.
func callBuiltin(t *testing.T, fn func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error), args ...starlark.Value) starlark.Value {
	t.Helper()

	got, err := fn(&starlark.Thread{Name: "t"}, nil, starlark.Tuple(args), nil)
	if err != nil {
		t.Fatalf("builtin returned %v", err)
	}

	return got
}

func TestBase64Builtins(t *testing.T) {
	enc := callBuiltin(t, BuiltinBase64Encode, starlark.String("hello"))
	if s, _ := starlark.AsString(enc); s != "aGVsbG8=" {
		t.Errorf("base64_encode = %q", s)
	}

	dec := callBuiltin(t, BuiltinBase64Decode, starlark.String("aGVsbG8="))
	if s, _ := starlark.AsString(dec); s != "hello" {
		t.Errorf("base64_decode = %q", s)
	}

	// Invalid input must error rather than yield garbage a script would then use.
	if _, err := BuiltinBase64Decode(&starlark.Thread{}, nil, starlark.Tuple{starlark.String("!!!")}, nil); err == nil {
		t.Error("base64_decode accepted invalid input")
	}
}

func TestJSONBuiltinsKeepIntegersExact(t *testing.T) {
	// A disk size larger than float64 can represent exactly must survive the
	// round trip, which is why the decoder uses UseNumber.
	dec := callBuiltin(t, BuiltinJSONDecode, starlark.String(`{"sizeBytes": 960197124096, "name": "sda"}`))

	m, ok := ToGo(dec).(map[string]any)
	if !ok {
		t.Fatalf("json_decode gave %T, want a map", ToGo(dec))
	}

	if m["sizeBytes"] != int64(960197124096) {
		t.Errorf("sizeBytes = %v (%T), want an exact int64", m["sizeBytes"], m["sizeBytes"])
	}

	enc := callBuiltin(t, BuiltinJSONEncode, dec)
	if s, _ := starlark.AsString(enc); s != `{"name":"sda","sizeBytes":960197124096}` {
		t.Errorf("json_encode = %q", s)
	}

	if _, err := BuiltinJSONDecode(&starlark.Thread{}, nil, starlark.Tuple{starlark.String("{oops")}, nil); err == nil {
		t.Error("json_decode accepted malformed JSON")
	}
}

func TestYAMLBuiltins(t *testing.T) {
	dec := callBuiltin(t, BuiltinYAMLDecode, starlark.String("links:\n  - id: eth0\n    mtu: 1500\n"))

	m, ok := ToGo(dec).(map[string]any)
	if !ok {
		t.Fatalf("yaml_decode gave %T, want a map", ToGo(dec))
	}

	links, ok := m["links"].([]any)
	if !ok || len(links) != 1 {
		t.Fatalf("links = %v", m["links"])
	}

	first, ok := links[0].(map[string]any)
	if !ok || first["id"] != "eth0" || first["mtu"] != int64(1500) {
		t.Errorf("first link = %v, want id eth0 and an exact mtu", links[0])
	}

	enc := callBuiltin(t, BuiltinYAMLEncode, dec)
	if s, _ := starlark.AsString(enc); s == "" {
		t.Error("yaml_encode produced nothing")
	}

	if _, err := BuiltinYAMLDecode(&starlark.Thread{}, nil, starlark.Tuple{starlark.String("\tbad: [")}, nil); err == nil {
		t.Error("yaml_decode accepted malformed YAML")
	}
}

func TestGetenvAndReadFile(t *testing.T) {
	t.Setenv("STARLARK_TEST_VAR", "present")

	if s, _ := starlark.AsString(callBuiltin(t, BuiltinGetenv, starlark.String("STARLARK_TEST_VAR"))); s != "present" {
		t.Errorf("getenv = %q", s)
	}

	// An unset variable reads as empty so scripts can use `or` defaults.
	if s, _ := starlark.AsString(callBuiltin(t, BuiltinGetenv, starlark.String("STARLARK_TEST_ABSENT"))); s != "" {
		t.Errorf("getenv of an unset name = %q, want empty", s)
	}

	path := filepath.Join(t.TempDir(), "creds")
	if err := os.WriteFile(path, []byte("  secret\n"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	// Contents are trimmed, since these files hold credentials written with a newline.
	if s, _ := starlark.AsString(callBuiltin(t, BuiltinReadFile, starlark.String(path))); s != "secret" {
		t.Errorf("read_file = %q, want the trimmed contents", s)
	}

	// A missing file reads as empty rather than failing the whole call.
	if s, _ := starlark.AsString(callBuiltin(t, BuiltinReadFile, starlark.String(path+".absent"))); s != "" {
		t.Errorf("read_file of a missing path = %q, want empty", s)
	}
}

func TestHeadersToStarlarkAndBack(t *testing.T) {
	h := http.Header{}
	h.Add("X-One", "a")
	h.Add("X-One", "b")
	h.Set("Content-Type", "application/json")

	d := HeadersToStarlark(h)
	if d.Len() != 2 {
		t.Fatalf("dict has %d keys, want 2", d.Len())
	}

	got, ok := ToGo(d).(map[string]any)
	if !ok {
		t.Fatalf("headers converted to %T", ToGo(d))
	}

	vals, ok := got["X-One"].([]any)
	if !ok || len(vals) != 2 || vals[0] != "a" || vals[1] != "b" {
		t.Errorf("X-One = %v, want both values in order", got["X-One"])
	}

	// A string or a list of strings both round trip back into a header.
	in := starlark.NewDict(2)
	_ = in.SetKey(starlark.String("X-Single"), starlark.String("one"))
	_ = in.SetKey(starlark.String("X-Multi"), starlark.NewList([]starlark.Value{starlark.String("m1"), starlark.String("m2")}))

	back, err := DictToHTTPHeader(in)
	if err != nil {
		t.Fatalf("DictToHTTPHeader: %v", err)
	}

	if back.Get("X-Single") != "one" || len(back.Values("X-Multi")) != 2 {
		t.Errorf("header = %v", back)
	}

	// A non string value is a script bug and must be reported.
	bad := starlark.NewDict(1)
	_ = bad.SetKey(starlark.String("X-Bad"), starlark.MakeInt(1))

	if _, err = DictToHTTPHeader(bad); err == nil {
		t.Error("DictToHTTPHeader accepted a non string value")
	}

	// A nil dict is an empty header, not a crash.
	if got, err := DictToHTTPHeader(nil); err != nil || len(got) != 0 {
		t.Errorf("DictToHTTPHeader(nil) = (%v, %v)", got, err)
	}
}

func TestKwargsToLogrValues(t *testing.T) {
	kv := KwargsToLogrValues([]starlark.Tuple{
		{starlark.String("count"), starlark.MakeInt(3)},
		{starlark.String("name"), starlark.String("node-1")},
	})

	if len(kv) != 4 {
		t.Fatalf("got %d values, want 4", len(kv))
	}

	if kv[0] != "count" || kv[1] != int64(3) || kv[2] != "name" || kv[3] != "node-1" {
		t.Errorf("kv = %v", kv)
	}
}

func TestIPMIPureHelpers(t *testing.T) {
	// Every selector maps back to the name a script passed in.
	for name, sel := range IPMIBootDevices {
		if got := IPMIBootDeviceName(sel); got != name {
			t.Errorf("IPMIBootDeviceName(%v) = %q, want %q", sel, got, name)
		}
	}

	if _, err := IPMIPort("t", starlark.MakeInt(623)); err != nil {
		t.Errorf("IPMIPort(623): %v", err)
	}

	host, port, user, pass, err := IPMIConn("t", starlark.Tuple{
		starlark.String("bmc"), starlark.MakeInt(623), starlark.String("u"), starlark.String("p"),
	})
	if err != nil || host != "bmc" || port != 623 || user != "u" || pass != "p" {
		t.Errorf("IPMIConn = (%q, %d, %q, %q, %v)", host, port, user, pass, err)
	}

	// Too few arguments is a script bug, not a connection attempt.
	if _, _, _, _, err = IPMIConn("t", starlark.Tuple{starlark.String("bmc")}); err == nil {
		t.Error("IPMIConn accepted a short argument list")
	}

	crit := []goipmi.SensorThresholdStatus{
		goipmi.SensorThresholdStatus_LCR, goipmi.SensorThresholdStatus_LNR,
		goipmi.SensorThresholdStatus_UCR, goipmi.SensorThresholdStatus_UNR,
	}
	for _, s := range crit {
		if !IPMISensorCritical(s) {
			t.Errorf("IPMISensorCritical(%v) = false, want true", s)
		}
	}

	if IPMISensorCritical(goipmi.SensorThresholdStatus_OK) {
		t.Error("an OK sensor was reported critical")
	}

	// A nil FRU is what a BMC that serves none looks like.
	if m, p, s := IPMIFRUVendor(nil); m != "" || p != "" || s != "" {
		t.Errorf("IPMIFRUVendor(nil) = (%q, %q, %q)", m, p, s)
	}
}

// captureLogSink records what the log builtins emitted.
type captureLogSink struct {
	msg string
	kv  []any
	lvl int
	err error
}

func (c *captureLogSink) Init(logr.RuntimeInfo)                {}
func (c *captureLogSink) Enabled(int) bool                     { return true }
func (c *captureLogSink) Info(l int, msg string, kv ...any)    { c.lvl, c.msg, c.kv = l, msg, kv }
func (c *captureLogSink) Error(e error, msg string, kv ...any) { c.err, c.msg, c.kv = e, msg, kv }
func (c *captureLogSink) WithValues(...any) logr.LogSink       { return c }
func (c *captureLogSink) WithName(string) logr.LogSink         { return c }

func TestLogBuiltins(t *testing.T) {
	sink := &captureLogSink{}
	thread := &starlark.Thread{Name: "t"}
	thread.SetLocal(LoggerThreadLocal, logr.New(sink))

	if _, err := BuiltinLogInfo(thread, nil, starlark.Tuple{starlark.String("hello")},
		[]starlark.Tuple{{starlark.String("host"), starlark.String("node-1")}}); err != nil {
		t.Fatalf("log_info: %v", err)
	}

	if sink.msg != "hello" || len(sink.kv) != 2 || sink.kv[1] != "node-1" {
		t.Errorf("log_info recorded msg=%q kv=%v", sink.msg, sink.kv)
	}

	// log_debug goes to V(1) so it can be filtered separately from log_info.
	if _, err := BuiltinLogDebug(thread, nil, starlark.Tuple{starlark.String("quiet")}, nil); err != nil {
		t.Fatalf("log_debug: %v", err)
	}

	if sink.lvl != 1 {
		t.Errorf("log_debug level = %d, want 1", sink.lvl)
	}

	if _, err := BuiltinLogError(thread, nil, starlark.Tuple{starlark.String("boom")}, nil); err != nil {
		t.Fatalf("log_error: %v", err)
	}

	if sink.msg != "boom" {
		t.Errorf("log_error msg = %q", sink.msg)
	}

	// With no logger on the thread the builtins fall back rather than panic.
	if _, err := BuiltinLogInfo(&starlark.Thread{}, nil, starlark.Tuple{starlark.String("x")}, nil); err != nil {
		t.Errorf("log_info without a thread logger: %v", err)
	}
}

func TestPublishEventBuiltin(t *testing.T) {
	var reason, message string

	thread := &starlark.Thread{Name: "t"}
	thread.SetLocal(PublisherThreadLocal, provisioner.EventPublisher(func(r, m string) {
		reason, message = r, m
	}))

	if _, err := BuiltinPublishEvent(thread, nil,
		starlark.Tuple{starlark.String("Registered"), starlark.String("all good")}, nil); err != nil {
		t.Fatalf("publish_event: %v", err)
	}

	if reason != "Registered" || message != "all good" {
		t.Errorf("published (%q, %q)", reason, message)
	}

	// Outside a provisioner call there is no publisher, and that is a no op
	// rather than an error, so scripts can call it unconditionally.
	if _, err := BuiltinPublishEvent(&starlark.Thread{}, nil,
		starlark.Tuple{starlark.String("r"), starlark.String("m")}, nil); err != nil {
		t.Errorf("publish_event with no publisher: %v", err)
	}
}

func TestIronicCACertFile(t *testing.T) {
	// A deployment sets this, so pin it or the default assertion is not testable.
	t.Setenv("IRONIC_CACERT_FILE", "")

	if got := IronicCACertFile(); got != "/opt/metal3/certs/ca/tls.crt" {
		t.Errorf("default CA path = %q", got)
	}
}
