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

package webhook

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"sigs.k8s.io/yaml"
)

// The BareMetalHost validation is split over two webhooks, and which requests
// reach which one is decided entirely by the generated configuration:
//
//   - baremetalhost.metal3.io is registered for v1alpha1 and leaves matchPolicy
//     unset, which defaults to Equivalent, so the API server converts v1beta1
//     requests and sends them there as well. That covers the validation the two
//     versions have in common.
//   - baremetalhost.v1beta1.metal3.io is registered for v1beta1 with
//     matchPolicy Exact, so it only sees requests a client actually made in
//     v1beta1, and its rules are not applied to v1alpha1 requests.
//
// This test pins that routing down, since it only exists in kubebuilder markers.

type webhookConfig struct {
	Webhooks []struct {
		Name         string `json:"name"`
		MatchPolicy  string `json:"matchPolicy"`
		ClientConfig struct {
			Service struct {
				Path string `json:"path"`
			} `json:"service"`
		} `json:"clientConfig"`
		Rules []struct {
			APIGroups   []string `json:"apiGroups"`
			APIVersions []string `json:"apiVersions"`
			Operations  []string `json:"operations"`
			Resources   []string `json:"resources"`
		} `json:"rules"`
		MatchConditions []struct {
			Name string `json:"name"`
		} `json:"matchConditions"`
	} `json:"webhooks"`
}

func loadWebhookConfig(t *testing.T) webhookConfig {
	t.Helper()

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to locate test file")
	}

	data, err := os.ReadFile(filepath.Join(filepath.Dir(filename), "manifests.yaml"))
	if err != nil {
		t.Fatalf("reading generated webhook manifest: %v", err)
	}

	var config webhookConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		t.Fatalf("parsing generated webhook manifest: %v", err)
	}
	return config
}

func TestBareMetalHostWebhookRouting(t *testing.T) {
	config := loadWebhookConfig(t)

	for _, tc := range []struct {
		Name            string
		Path            string
		MatchPolicy     string
		APIVersions     []string
		MatchConditions []string
	}{
		{
			Name:        "baremetalhost.metal3.io",
			Path:        "/validate-metal3-io-v1alpha1-baremetalhost",
			MatchPolicy: "", // unset, so the default Equivalent also covers v1beta1
			APIVersions: []string{"v1alpha1"},
			MatchConditions: []string{
				"spec-or-annotations-changed",
			},
		},
		{
			Name:        "baremetalhost.v1beta1.metal3.io",
			Path:        "/validate-metal3-io-v1beta1-baremetalhost",
			MatchPolicy: "Exact", // v1alpha1 requests must not be converted up
			APIVersions: []string{"v1beta1"},
			MatchConditions: []string{
				"spec-or-annotations-changed",
			},
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			var found bool
			for _, webhook := range config.Webhooks {
				if webhook.Name != tc.Name {
					continue
				}
				found = true

				if webhook.MatchPolicy != tc.MatchPolicy {
					t.Errorf("matchPolicy = %q, want %q", webhook.MatchPolicy, tc.MatchPolicy)
				}
				if webhook.ClientConfig.Service.Path != tc.Path {
					t.Errorf("path = %q, want %q", webhook.ClientConfig.Service.Path, tc.Path)
				}

				if len(webhook.Rules) != 1 {
					t.Fatalf("rules has %d entries, want 1", len(webhook.Rules))
				}
				rule := webhook.Rules[0]
				if got := rule.APIVersions; !equalStrings(got, tc.APIVersions) {
					t.Errorf("apiVersions = %v, want %v", got, tc.APIVersions)
				}
				if got := rule.Resources; !equalStrings(got, []string{"baremetalhosts"}) {
					t.Errorf("resources = %v, want [baremetalhosts]", got)
				}
				if got := rule.Operations; !equalStrings(got, []string{"CREATE", "UPDATE"}) {
					t.Errorf("operations = %v, want [CREATE UPDATE]", got)
				}

				names := make([]string, 0, len(webhook.MatchConditions))
				for _, condition := range webhook.MatchConditions {
					names = append(names, condition.Name)
				}
				if !equalStrings(names, tc.MatchConditions) {
					t.Errorf("matchConditions = %v, want %v", names, tc.MatchConditions)
				}
				break
			}
			if !found {
				t.Fatalf("webhook %q not found in the generated manifest", tc.Name)
			}
		})
	}
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
