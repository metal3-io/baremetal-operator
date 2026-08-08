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

// Secret masking shared by the error, event, and log output channels.

package starlark

import (
	"strings"

	"github.com/go-logr/logr"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	"github.com/s3rj1k/starlark-provisioner/starlib"
)

// maskMarker replaces a masked secret in output.
const maskMarker = "***"

// maskSecret replaces every occurrence of secret in s with the marker.
// An empty secret is a noop so callers need not guard.
func maskSecret(s, secret string) string {
	if secret == "" {
		return s
	}

	return strings.ReplaceAll(s, secret, maskMarker)
}

// maskingSink is a logr.LogSink that masks the secret in the message and string values.
type maskingSink struct {
	sink   logr.LogSink
	secret string
}

// maskValue masks the secret in v, recursing into the maps and slices scripts produce.
func (m *maskingSink) maskValue(v any) any {
	switch val := v.(type) {
	case string:
		return maskSecret(val, m.secret)
	case map[string]any:
		out := make(map[string]any, len(val))
		for k, e := range val {
			out[k] = m.maskValue(e)
		}

		return out
	case []any:
		return starlib.MapSlice(val, m.maskValue)
	case map[string]string:
		out := make(map[string]string, len(val))
		for k, e := range val {
			out[k] = maskSecret(e, m.secret)
		}

		return out
	case []string:
		return starlib.MapSlice(val, func(s string) string { return maskSecret(s, m.secret) })
	default:
		return v
	}
}

// maskValues returns a copy of kv with the secret masked in every value position.
// logr pairs are key then value, so keys at even indices stay intact.
func (m *maskingSink) maskValues(kv []any) []any {
	out := make([]any, len(kv))
	for i, v := range kv {
		if i%2 == 1 {
			out[i] = m.maskValue(v)
		} else {
			out[i] = v
		}
	}

	return out
}

func (m *maskingSink) Init(info logr.RuntimeInfo) { m.sink.Init(info) }

func (m *maskingSink) Enabled(level int) bool { return m.sink.Enabled(level) }

func (m *maskingSink) Info(level int, msg string, kv ...any) {
	m.sink.Info(level, maskSecret(msg, m.secret), m.maskValues(kv)...)
}

func (m *maskingSink) Error(err error, msg string, kv ...any) {
	m.sink.Error(err, maskSecret(msg, m.secret), m.maskValues(kv)...)
}

func (m *maskingSink) WithValues(kv ...any) logr.LogSink {
	return &maskingSink{sink: m.sink.WithValues(m.maskValues(kv)...), secret: m.secret}
}

func (m *maskingSink) WithName(name string) logr.LogSink {
	return &maskingSink{sink: m.sink.WithName(name), secret: m.secret}
}

// WithCallDepth forwards depth adjustments so the wrapped sink still reports the true caller.
func (m *maskingSink) WithCallDepth(depth int) logr.LogSink {
	if cd, ok := m.sink.(logr.CallDepthLogSink); ok {
		return &maskingSink{sink: cd.WithCallDepth(depth), secret: m.secret}
	}

	return m
}

// newMaskingSink wraps sink, bumping its call depth by one to hide the maskingSink frame.
func newMaskingSink(sink logr.LogSink, secret string) logr.LogSink {
	if cd, ok := sink.(logr.CallDepthLogSink); ok {
		sink = cd.WithCallDepth(1)
	}

	return &maskingSink{sink: sink, secret: secret}
}

// maskingPublisher wraps an EventPublisher so the reason and message never carry the secret.
func maskingPublisher(pub provisioner.EventPublisher, secret string) provisioner.EventPublisher {
	if pub == nil || secret == "" {
		return pub
	}

	return func(reason, message string) {
		pub(maskSecret(reason, secret), maskSecret(message, secret))
	}
}

// maskingLogger wraps a logger so its message and string values never carry the secret.
func maskingLogger(l logr.Logger, secret string) logr.Logger {
	if secret == "" {
		return l
	}

	return logr.New(newMaskingSink(l.GetSink(), secret))
}
