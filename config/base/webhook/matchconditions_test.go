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

	"github.com/google/cel-go/cel"
	"sigs.k8s.io/yaml"
)

func TestBareMetalHostMatchCondition(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to locate test file")
	}

	data, err := os.ReadFile(filepath.Join(filepath.Dir(filename), "manifests.yaml"))
	if err != nil {
		t.Fatalf("reading generated webhook manifest: %v", err)
	}

	var manifest struct {
		Webhooks []struct {
			Name            string `yaml:"name"`
			MatchConditions []struct {
				Name       string `yaml:"name"`
				Expression string `yaml:"expression"`
			} `yaml:"matchConditions"`
		} `yaml:"webhooks"`
	}
	if unmarshalErr := yaml.Unmarshal(data, &manifest); unmarshalErr != nil {
		t.Fatalf("parsing generated webhook manifest: %v", unmarshalErr)
	}

	var expression string
	for _, webhook := range manifest.Webhooks {
		if webhook.Name != "baremetalhost.metal3.io" {
			continue
		}
		if len(webhook.MatchConditions) != 1 {
			t.Fatalf("BareMetalHost matchConditions has %d entries, want 1", len(webhook.MatchConditions))
		}
		condition := webhook.MatchConditions[0]
		if condition.Name != "spec-or-annotations-changed" {
			t.Fatalf("condition name = %q, want spec-or-annotations-changed", condition.Name)
		}
		expression = condition.Expression
		break
	}
	if expression == "" {
		t.Fatal("BareMetalHost webhook or match condition not found")
	}

	env, err := cel.NewEnv(
		cel.OptionalTypes(),
		cel.Variable("request", cel.DynType),
		cel.Variable("object", cel.DynType),
		cel.Variable("oldObject", cel.DynType),
	)
	if err != nil {
		t.Fatalf("creating CEL environment: %v", err)
	}
	ast, issues := env.Compile(expression)
	if issues != nil && issues.Err() != nil {
		t.Fatalf("compiling match condition: %v", issues.Err())
	}
	program, err := env.Program(ast)
	if err != nil {
		t.Fatalf("creating CEL program: %v", err)
	}

	metadata := func(labels map[string]interface{}, annotations map[string]interface{}) map[string]interface{} {
		result := map[string]interface{}{"labels": labels}
		if annotations != nil {
			result["annotations"] = annotations
		}
		return result
	}

	tests := []struct {
		name    string
		request map[string]interface{}
		object  map[string]interface{}
		old     map[string]interface{}
		want    bool
		setOld  bool
	}{
		{
			name:    "create always matches",
			request: map[string]interface{}{"operation": "CREATE"},
			object:  map[string]interface{}{"metadata": metadata(nil, nil)},
			want:    true,
		},
		{
			name:    "metadata update with missing spec is skipped",
			request: map[string]interface{}{"operation": "UPDATE"},
			object:  map[string]interface{}{"metadata": metadata(map[string]interface{}{"pool": "new"}, nil)},
			old:     map[string]interface{}{"metadata": metadata(map[string]interface{}{"pool": "old"}, nil)},
			want:    false,
			setOld:  true,
		},
		{
			name:    "spec update matches",
			request: map[string]interface{}{"operation": "UPDATE"},
			object:  map[string]interface{}{"spec": map[string]interface{}{"online": true}, "metadata": metadata(nil, nil)},
			old:     map[string]interface{}{"spec": map[string]interface{}{"online": false}, "metadata": metadata(nil, nil)},
			want:    true,
			setOld:  true,
		},
		{
			name:    "annotation update matches",
			request: map[string]interface{}{"operation": "UPDATE"},
			object:  map[string]interface{}{"metadata": metadata(nil, map[string]interface{}{"example.com/key": "new"})},
			old:     map[string]interface{}{"metadata": metadata(nil, map[string]interface{}{"example.com/key": "old"})},
			want:    true,
			setOld:  true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			activation := map[string]interface{}{
				"request": test.request,
				"object":  test.object,
			}
			if test.setOld {
				activation["oldObject"] = test.old
			}
			result, _, err := program.Eval(activation)
			if err != nil {
				t.Fatalf("evaluating match condition: %v", err)
			}
			got, ok := result.Value().(bool)
			if !ok {
				t.Fatalf("match condition returned %T, want bool", result.Value())
			}
			if got != test.want {
				t.Errorf("match condition = %t, want %t", got, test.want)
			}
		})
	}
}
