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

func TestMatchConditionsPatch(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to locate test file")
	}

	data, err := os.ReadFile(filepath.Join(filepath.Dir(filename), "matchconditions_patch.yaml"))
	if err != nil {
		t.Fatalf("reading match conditions patch: %v", err)
	}

	var patch struct {
		APIVersion string `yaml:"apiVersion"`
		Kind       string `yaml:"kind"`
		Metadata   struct {
			Name string `yaml:"name"`
		} `yaml:"metadata"`
		Webhooks []struct {
			Name            string `yaml:"name"`
			MatchConditions []struct {
				Name       string `yaml:"name"`
				Expression string `yaml:"expression"`
			} `yaml:"matchConditions"`
		} `yaml:"webhooks"`
	}
	if err := yaml.Unmarshal(data, &patch); err != nil {
		t.Fatalf("parsing match conditions patch: %v", err)
	}

	if patch.APIVersion != "admissionregistration.k8s.io/v1" {
		t.Errorf("apiVersion = %q, want admissionregistration.k8s.io/v1", patch.APIVersion)
	}
	if patch.Kind != "ValidatingWebhookConfiguration" {
		t.Errorf("kind = %q, want ValidatingWebhookConfiguration", patch.Kind)
	}
	if patch.Metadata.Name != "validating-webhook-configuration" {
		t.Errorf("metadata.name = %q, want validating-webhook-configuration", patch.Metadata.Name)
	}
	if len(patch.Webhooks) != 1 {
		t.Fatalf("webhooks has %d entries, want 1", len(patch.Webhooks))
	}

	webhook := patch.Webhooks[0]
	if webhook.Name != "baremetalhost.metal3.io" {
		t.Errorf("webhook name = %q, want baremetalhost.metal3.io", webhook.Name)
	}
	if len(webhook.MatchConditions) != 1 {
		t.Fatalf("matchConditions has %d entries, want 1", len(webhook.MatchConditions))
	}

	condition := webhook.MatchConditions[0]
	if condition.Name != "spec-or-annotations-changed" {
		t.Errorf("condition name = %q, want spec-or-annotations-changed", condition.Name)
	}
	wantExpression := "request.operation == 'CREATE' || object.spec != oldObject.spec || object.metadata.?annotations.orValue({}) != oldObject.metadata.?annotations.orValue({})"
	if condition.Expression != wantExpression {
		t.Errorf("condition expression = %q, want %q", condition.Expression, wantExpression)
	}
}
