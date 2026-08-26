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
	"cmp"
	"os"
	"strconv"
)

const (
	SecondsPerMinute = 60
	SecondsPerHour   = 60 * SecondsPerMinute
	SecondsPerDay    = 24 * SecondsPerHour

	// Compiled in defaults for the tuneables below.
	defaultHTTPResponseBytes = 16 << 20
	defaultExecutionSteps    = 1 << 32
	// The call budget must leave room for more than one maximum length request.
	callSecondsPerHTTPTimeout = 2
)

// Parse os.Getenv(name) as a signed integer, returning 0 (the zero value that cmp.Or skips) on empty or invalid input.
func EnvInt[T int | int64 | uint64](name string) T {
	n, _ := strconv.ParseInt(os.Getenv(name), 10, 64)
	return T(n)
}

// Package level tuneables. Each has a compiled in default with an optional env override.
var (
	// User Agent sent with all HTTP requests from Starlark scripts.
	UserAgent = cmp.Or(os.Getenv("BMO_STARLARK_USER_AGENT"), "BMO-Starlark/1.0")
	// Upper bound on http_request_raw timeout that keeps the client cache bounded.
	MaxHTTPTimeoutSec = cmp.Or(EnvInt[int64]("BMO_STARLARK_MAX_HTTP_TIMEOUT_SEC"), int64(SecondsPerHour))
	// Ceiling for requeue_after_seconds that guards against garbage returned by scripts.
	MaxRequeueSeconds = cmp.Or(EnvInt[int64]("BMO_STARLARK_MAX_REQUEUE_SECONDS"), int64(SecondsPerDay))
	// Ceiling on an http_request_raw response body held in memory as a Starlark value.
	MaxHTTPResponseBytes = cmp.Or(EnvInt[int64]("BMO_STARLARK_MAX_HTTP_RESPONSE_BYTES"), int64(defaultHTTPResponseBytes))
	// Ceiling on interpreter steps per call, so a runaway script cannot pin a
	// reconcile worker. Generous, since a slow script is not the failure mode.
	MaxExecutionSteps = cmp.Or(EnvInt[uint64]("BMO_STARLARK_MAX_EXECUTION_STEPS"), uint64(defaultExecutionSteps))
	// Wall clock ceiling on one script call, since the reconcile context has none. It
	// must stay above MaxHTTPTimeoutSec or it would silently cap that tunable.
	MaxCallSeconds = cmp.Or(EnvInt[int64]("BMO_STARLARK_MAX_CALL_SECONDS"), callSecondsPerHTTPTimeout*MaxHTTPTimeoutSec)
)
