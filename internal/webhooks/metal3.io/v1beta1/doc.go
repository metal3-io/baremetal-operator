/*
Copyright 2026 The Metal3 Authors.

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

// Package webhooks contains the webhooks for the v1beta1 API types.
//
// The v1alpha1 webhooks already validate v1beta1 requests: their
// ValidatingWebhookConfiguration does not set matchPolicy, which defaults to
// Equivalent, so the API server converts a v1beta1 request to v1alpha1 and
// sends it there. That covers everything the two versions have in common.
//
// This package holds the rules that only make sense for v1beta1 and therefore
// need the object in its original version. Its webhook uses matchPolicy=Exact
// so that v1alpha1 requests are not converted up and checked against rules
// that do not apply to them.
package webhooks
