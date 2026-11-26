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

package ironic

import (
	"testing"

	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/nodes"
	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/stretchr/testify/assert"
)

func TestParseSubStates(t *testing.T) {
	cleanSteps := []any{
		map[string]any{"interface": "deploy", "step": "erase_devices_metadata"},
		map[string]any{"interface": "deploy", "step": "erase_devices"},
	}

	tests := []struct {
		name     string
		node     *nodes.Node
		activity string
		steps    []metal3api.ProvisioningStep
	}{
		{
			name: "deploying",
			node: &nodes.Node{
				ProvisionState: string(nodes.Deploying),
				DeployStep:     map[string]any{"interface": "deploy", "step": "write_image"},
				DriverInternalInfo: map[string]any{
					"deploy_step_index": float64(1),
					"deploy_steps": []any{
						map[string]any{"interface": "deploy", "step": "deploy"},
						map[string]any{"interface": "deploy", "step": "write_image"},
						map[string]any{"interface": "deploy", "step": "boot_instance"},
					},
				},
			},
			activity: "deploy.write_image",
			steps: []metal3api.ProvisioningStep{
				{Name: "deploy.deploy", State: metal3api.StepStateCompleted, Description: "Initiating deployment"},
				{Name: "deploy.write_image", State: metal3api.StepStateInProgress, Description: "Writing image to disk"},
				{Name: "deploy.boot_instance", State: metal3api.StepStateNotStarted, Description: "Booting instance"},
			},
		},
		{
			name: "servicing",
			node: &nodes.Node{
				ProvisionState: string(nodes.Servicing),
				ServiceStep:    map[string]any{"interface": "firmware", "step": "update"},
				DriverInternalInfo: map[string]any{
					"service_step_index": float64(0),
					"service_steps": []any{
						map[string]any{"interface": "firmware", "step": "update"},
					},
				},
			},
			activity: "firmware.update",
			steps: []metal3api.ProvisioningStep{
				{Name: "firmware.update", State: metal3api.StepStateInProgress, Description: "Updating firmware"},
			},
		},
		{
			name: "missing step index reports unknown states",
			node: &nodes.Node{
				ProvisionState: string(nodes.CleanWait),
				CleanStep:      map[string]any{"interface": "deploy", "step": "erase_devices"},
				DriverInternalInfo: map[string]any{
					"clean_steps": cleanSteps,
				},
			},
			activity: "deploy.erase_devices",
			steps: []metal3api.ProvisioningStep{
				{Name: "deploy.erase_devices_metadata", State: metal3api.StepStateUnknown, Description: "Erasing device metadata"},
				{Name: "deploy.erase_devices", State: metal3api.StepStateUnknown, Description: "Securely erasing devices"},
			},
		},
		{
			name: "negative step index reports unknown states",
			node: &nodes.Node{
				ProvisionState: string(nodes.Cleaning),
				DriverInternalInfo: map[string]any{
					"clean_step_index": float64(-1),
					"clean_steps":      cleanSteps,
				},
			},
			steps: []metal3api.ProvisioningStep{
				{Name: "deploy.erase_devices_metadata", State: metal3api.StepStateUnknown, Description: "Erasing device metadata"},
				{Name: "deploy.erase_devices", State: metal3api.StepStateUnknown, Description: "Securely erasing devices"},
			},
		},
		{
			name: "invalid step index reports unknown states",
			node: &nodes.Node{
				ProvisionState: string(nodes.Cleaning),
				DriverInternalInfo: map[string]any{
					"clean_step_index": "invalid",
					"clean_steps":      cleanSteps,
				},
			},
			steps: []metal3api.ProvisioningStep{
				{Name: "deploy.erase_devices_metadata", State: metal3api.StepStateUnknown, Description: "Erasing device metadata"},
				{Name: "deploy.erase_devices", State: metal3api.StepStateUnknown, Description: "Securely erasing devices"},
			},
		},
		{
			name: "step index past end marks all steps completed",
			node: &nodes.Node{
				ProvisionState: string(nodes.Cleaning),
				DriverInternalInfo: map[string]any{
					"clean_step_index": float64(5),
					"clean_steps":      cleanSteps,
				},
			},
			steps: []metal3api.ProvisioningStep{
				{Name: "deploy.erase_devices_metadata", State: metal3api.StepStateCompleted, Description: "Erasing device metadata"},
				{Name: "deploy.erase_devices", State: metal3api.StepStateCompleted, Description: "Securely erasing devices"},
			},
		},
		{
			name: "invalid step metadata",
			node: &nodes.Node{
				ProvisionState: string(nodes.DeployWait),
				DeployStep:     map[string]any{"interface": "deploy"},
				DriverInternalInfo: map[string]any{
					"deploy_step_index": "invalid",
					"deploy_steps":      "invalid",
				},
			},
		},
		{
			name: "clean failure is not parsed",
			node: &nodes.Node{
				ProvisionState: string(nodes.CleanFail),
				CleanStep:      map[string]any{},
				DriverInternalInfo: map[string]any{
					"clean_steps": cleanSteps,
				},
			},
		},
		{
			name: "deploy failure is not parsed",
			node: &nodes.Node{
				ProvisionState:     string(nodes.DeployFail),
				DeployStep:         map[string]any{},
				DriverInternalInfo: map[string]any{},
			},
		},
		{
			name: "non-operation state",
			node: &nodes.Node{ProvisionState: string(nodes.Active)},
		},
		{
			name: "nil node",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			activity, steps := parseSubStates(tt.node)

			assert.Equal(t, tt.activity, activity)
			assert.Equal(t, tt.steps, steps)
		})
	}
}
