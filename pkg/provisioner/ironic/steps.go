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
	"fmt"

	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/nodes"
	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
)

// stepDescriptions maps the canonical "<interface>.<step>" identifier reported
// by Ironic to a short human-readable description for display in the
// BareMetalHost status. Unknown steps return "" so the description field is
// omitted from the rendered status.
var stepDescriptions = map[string]string{
	// Deploy steps
	"deploy.deploy":                   "Initiating deployment",
	"deploy.write_image":              "Writing image to disk",
	"deploy.prepare_instance_boot":    "Preparing instance boot",
	"deploy.tear_down_agent":          "Tearing down deployment agent",
	"deploy.switch_to_tenant_network": "Switching to tenant network",
	"deploy.boot_instance":            "Booting instance",

	// Clean and service steps (common defaults)
	"deploy.erase_devices_metadata": "Erasing device metadata",
	"deploy.erase_devices":          "Securely erasing devices",
	"raid.delete_configuration":     "Deleting RAID configuration",
	"raid.create_configuration":     "Creating RAID configuration",
	"bios.apply_configuration":      "Applying BIOS configuration",
	"bios.factory_reset":            "Resetting BIOS to factory defaults",
	"firmware.update":               "Updating firmware",
	"management.clear_job_queue":    "Clearing management controller job queue",
	"management.reset_idrac":        "Resetting iDRAC",
}

// parseSubStates reports the current step and the ordered step list of a
// running deploy, clean or service operation. Failed operations are not
// parsed because Ironic clears the structured current-step information.
func parseSubStates(node *nodes.Node) (activity string, allSteps []metal3api.ProvisioningStep) {
	if node == nil {
		return "", nil
	}

	switch node.ProvisionState {
	case string(nodes.Deploying), string(nodes.DeployWait):
		return parseNodeSteps(node, "deploy", node.DeployStep)
	case string(nodes.Cleaning), string(nodes.CleanWait):
		return parseNodeSteps(node, "clean", node.CleanStep)
	case string(nodes.Servicing), string(nodes.ServiceWait):
		return parseNodeSteps(node, "service", node.ServiceStep)
	default:
		return "", nil
	}
}

func parseNodeSteps(node *nodes.Node, operation string, currentStep map[string]any) (activity string, allSteps []metal3api.ProvisioningStep) {
	stepsList, _ := node.DriverInternalInfo[operation+"_steps"].([]any)

	// Without a valid step index Ironic gives no position, so step states
	// remain unknown rather than guessing which step is running.
	currentIndex := -1
	if index, ok := node.DriverInternalInfo[operation+"_step_index"].(float64); ok && index >= 0 {
		currentIndex = min(int(index), len(stepsList))
	}

	for i, step := range stepsList {
		stepMap, ok := step.(map[string]any)
		if !ok {
			continue
		}
		stepName, ok := stepMap["step"].(string)
		if !ok {
			continue
		}
		stepInterface, _ := stepMap["interface"].(string)
		name := stepName
		if stepInterface != "" {
			name = fmt.Sprintf("%s.%s", stepInterface, stepName)
		}

		var stepState metal3api.StepState
		switch {
		case currentIndex < 0:
			stepState = metal3api.StepStateUnknown
		case i < currentIndex:
			stepState = metal3api.StepStateCompleted
		case i == currentIndex:
			stepState = metal3api.StepStateInProgress
		default:
			stepState = metal3api.StepStateNotStarted
		}

		allSteps = append(allSteps, metal3api.ProvisioningStep{
			Name:        name,
			State:       stepState,
			Description: describeStep(name),
		})
	}

	stepName, _ := currentStep["step"].(string)
	stepInterface, _ := currentStep["interface"].(string)
	if stepName != "" {
		activity = stepName
		if stepInterface != "" {
			activity = fmt.Sprintf("%s.%s", stepInterface, stepName)
		}
	}

	return activity, allSteps
}

// describeStep returns a human-readable description for the given
// "<interface>.<step>" identifier, or an empty string if no description is
// known.
func describeStep(name string) string {
	return stepDescriptions[name]
}
