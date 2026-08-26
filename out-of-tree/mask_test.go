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

package starlark

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/go-logr/logr"
)

func TestMaskSecret(t *testing.T) {
	if got := MaskSecret("user pass secret", "secret"); got != "user pass ***" {
		t.Errorf("MaskSecret = %q", got)
	}

	if got := MaskSecret("nothing", ""); got != "nothing" {
		t.Errorf("empty secret should be a noop, got %q", got)
	}
}

func TestMaskingPublisher(t *testing.T) {
	var gotReason, gotMsg string

	pub := MaskingPublisher(func(reason, message string) {
		gotReason, gotMsg = reason, message
	}, "s3cr3t")

	pub("Auth s3cr3t", "failed for s3cr3t")

	if gotReason != "Auth ***" || gotMsg != "failed for ***" {
		t.Errorf("publisher not masked, reason=%q msg=%q", gotReason, gotMsg)
	}

	if MaskingPublisher(nil, "x") != nil {
		t.Error("nil publisher should stay nil")
	}
}

// captureSink records the last message and values passed to the sink.
type captureSink struct {
	msg string
	kv  []any
	err error
}

func (c *captureSink) Init(logr.RuntimeInfo) {}

func (c *captureSink) Enabled(int) bool { return true }

func (c *captureSink) Info(_ int, msg string, kv ...any) { c.msg, c.kv = msg, kv }

func (c *captureSink) Error(err error, msg string, kv ...any) { c.msg, c.kv, c.err = msg, kv, err }

func (c *captureSink) WithValues(...any) logr.LogSink { return c }

func (c *captureSink) WithName(string) logr.LogSink { return c }

func TestMaskingLogger(t *testing.T) {
	sink := &captureSink{}
	log := MaskingLogger(logr.New(sink), "p@ss")

	log.Info("connect p@ss", "detail", "used p@ss", "count", 3,
		"nested", map[string]any{"pw": "x p@ss y", "list": []any{"a p@ss", 7}})

	if sink.msg != "connect ***" {
		t.Errorf("message not masked, got %q", sink.msg)
	}

	if sink.kv[1] != "used ***" {
		t.Errorf("string value not masked, got %v", sink.kv[1])
	}

	if sink.kv[3] != 3 {
		t.Errorf("non string value changed, got %v", sink.kv[3])
	}

	nested, ok := sink.kv[5].(map[string]any)
	if !ok {
		t.Fatalf("nested value is not a map, got %T", sink.kv[5])
	}

	if nested["pw"] != "x *** y" {
		t.Errorf("nested map value not masked, got %v", nested["pw"])
	}

	list, ok := nested["list"].([]any)
	if !ok || len(list) != 2 || list[0] != "a ***" || list[1] != 7 {
		t.Errorf("nested slice not masked correctly, got %v", nested["list"])
	}

	log.Error(errors.New("x"), "boom p@ss")

	if sink.msg != "boom ***" {
		t.Errorf("error message not masked, got %q", sink.msg)
	}
}

func TestMaskingLoggerKeysNotMasked(t *testing.T) {
	sink := &captureSink{}
	log := MaskingLogger(logr.New(sink), "count")

	log.Info("m", "count", "count is 5")

	if sink.kv[0] != "count" {
		t.Errorf("key should not be masked, got %v", sink.kv[0])
	}

	if sink.kv[1] != "*** is 5" {
		t.Errorf("value should be masked, got %v", sink.kv[1])
	}
}

func TestMaskValueCollections(t *testing.T) {
	sink := &captureSink{}
	log := MaskingLogger(logr.New(sink), "p@ss")

	log.Info("m",
		"strmap", map[string]string{"k": "a p@ss b"},
		"strs", []string{"p@ss", "ok"})

	sm, ok := sink.kv[1].(map[string]string)
	if !ok || sm["k"] != "a *** b" {
		t.Errorf("map[string]string value not masked, got %v", sink.kv[1])
	}

	ss, ok := sink.kv[3].([]string)
	if !ok || len(ss) != 2 || ss[0] != "***" || ss[1] != "ok" {
		t.Errorf("[]string value not masked, got %v", sink.kv[3])
	}
}

// The sink renders err.Error() into the record, so an error built from a BMC
// response would otherwise carry the password straight into the operator log.
func TestMaskingLoggerMasksTheError(t *testing.T) {
	sink := &captureSink{}
	log := MaskingLogger(logr.New(sink), "p@ss")

	wrapped := errors.New("sentinel")
	log.Error(fmt.Errorf("redfish rejected p@ss: %w", wrapped), "boom")

	if sink.err == nil {
		t.Fatal("sink saw no error")
	}

	if strings.Contains(sink.err.Error(), "p@ss") {
		t.Errorf("error text leaks the secret, got %q", sink.err.Error())
	}

	if !strings.Contains(sink.err.Error(), "***") {
		t.Errorf("error text was not masked, got %q", sink.err.Error())
	}

	// Masking must not break the chain callers match on.
	if !errors.Is(sink.err, wrapped) {
		t.Error("errors.Is no longer sees through the masked error")
	}
}

// A nil error must stay nil rather than becoming a wrapper around nothing.
func TestMaskingLoggerNilErrorStaysNil(t *testing.T) {
	sink := &captureSink{}
	MaskingLogger(logr.New(sink), "p@ss").Error(nil, "no error here")

	if sink.err != nil {
		t.Errorf("nil error became %v", sink.err)
	}
}

// WithValues and WithName must keep masking, since a logger derived once at
// factory time is the one every later call goes through.
func TestMaskingSinkDerivedLoggersStillMask(t *testing.T) {
	sink := &captureSink{}
	log := MaskingLogger(logr.New(sink), "p@ss").
		WithValues("bound", "carries p@ss").
		WithName("sub")

	log.Info("msg p@ss", "later", "also p@ss")

	if sink.msg != "msg ***" {
		t.Errorf("derived logger message = %q", sink.msg)
	}

	if sink.kv[1] != "also ***" {
		t.Errorf("derived logger value = %v", sink.kv[1])
	}
}

// An empty secret means nothing to hide, so the logger is passed through
// untouched rather than wrapped.
func TestMaskingLoggerEmptySecretIsPassthrough(t *testing.T) {
	sink := &captureSink{}
	base := logr.New(sink)

	if got := MaskingLogger(base, ""); got != base {
		t.Error("an empty secret should return the original logger")
	}
}
