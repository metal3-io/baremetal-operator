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

// Thread-local keys shared by the provisioner call site (writer) and builtins (readers).
const (
	// Carries the provisioner.EventPublisher for publish_event.
	PublisherThreadLocal = "metal3-event-publisher"
	// Carries the caller's context.Context for HTTP cancellation.
	CtxThreadLocal = "metal3-context"
	// Carries the per-host logr.Logger for log_info/log_debug/log_error.
	LoggerThreadLocal = "metal3-logger"
)
