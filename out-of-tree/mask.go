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

// MaskMarker replaces a masked secret in output.
const MaskMarker = "***"

// MaskSecret replaces every occurrence of secret in s with the marker.
// An empty secret is a noop so callers need not guard.
func MaskSecret(s, secret string) string {
	if secret == "" {
		return s
	}

	return strings.ReplaceAll(s, secret, MaskMarker)
}

// MaskingSink is a logr.LogSink that masks the secret in the message and string values.
type MaskingSink struct {
	sink   logr.LogSink
	secret string
}

// MaskValue masks the secret in v, recursing into the maps and slices scripts produce.
func (m *MaskingSink) MaskValue(v any) any {
	switch val := v.(type) {
	case string:
		return MaskSecret(val, m.secret)
	case map[string]any:
		out := make(map[string]any, len(val))
		for k, e := range val {
			out[k] = m.MaskValue(e)
		}

		return out
	case []any:
		return starlib.MapSlice(val, m.MaskValue)
	case map[string]string:
		out := make(map[string]string, len(val))
		for k, e := range val {
			out[k] = MaskSecret(e, m.secret)
		}

		return out
	case []string:
		return starlib.MapSlice(val, func(s string) string { return MaskSecret(s, m.secret) })
	default:
		return v
	}
}

// MaskValues returns a copy of kv with the secret masked in every value position.
// logr pairs are key then value, so keys at even indices stay intact.
func (m *MaskingSink) MaskValues(kv []any) []any {
	out := make([]any, len(kv))
	for i, v := range kv {
		if i%2 == 1 {
			out[i] = m.MaskValue(v)
		} else {
			out[i] = v
		}
	}

	return out
}

func (m *MaskingSink) Init(info logr.RuntimeInfo) { m.sink.Init(info) }

func (m *MaskingSink) Enabled(level int) bool { return m.sink.Enabled(level) }

func (m *MaskingSink) Info(level int, msg string, kv ...any) {
	m.sink.Info(level, MaskSecret(msg, m.secret), m.MaskValues(kv)...)
}

func (m *MaskingSink) Error(err error, msg string, kv ...any) {
	// The sink renders err.Error() into the record, so mask it too or an error
	// built from a BMC response leaks the password straight into the log.
	if err != nil {
		err = &RedactedError{err: err, password: m.secret}
	}

	m.sink.Error(err, MaskSecret(msg, m.secret), m.MaskValues(kv)...)
}

func (m *MaskingSink) WithValues(kv ...any) logr.LogSink {
	return &MaskingSink{sink: m.sink.WithValues(m.MaskValues(kv)...), secret: m.secret}
}

func (m *MaskingSink) WithName(name string) logr.LogSink {
	return &MaskingSink{sink: m.sink.WithName(name), secret: m.secret}
}

// WithCallDepth forwards depth adjustments so the wrapped sink still reports the true caller.
func (m *MaskingSink) WithCallDepth(depth int) logr.LogSink {
	if cd, ok := m.sink.(logr.CallDepthLogSink); ok {
		return &MaskingSink{sink: cd.WithCallDepth(depth), secret: m.secret}
	}

	return m
}

// NewMaskingSink wraps sink, bumping its call depth by one to hide the MaskingSink frame.
func NewMaskingSink(sink logr.LogSink, secret string) logr.LogSink {
	if cd, ok := sink.(logr.CallDepthLogSink); ok {
		sink = cd.WithCallDepth(1)
	}

	return &MaskingSink{sink: sink, secret: secret}
}

// MaskingPublisher wraps an EventPublisher so the reason and message never carry the secret.
func MaskingPublisher(pub provisioner.EventPublisher, secret string) provisioner.EventPublisher {
	if pub == nil || secret == "" {
		return pub
	}

	return func(reason, message string) {
		pub(MaskSecret(reason, secret), MaskSecret(message, secret))
	}
}

// MaskingLogger wraps a logger so its message and string values never carry the secret.
func MaskingLogger(l logr.Logger, secret string) logr.Logger {
	if secret == "" {
		return l
	}

	return logr.New(NewMaskingSink(l.GetSink(), secret))
}
