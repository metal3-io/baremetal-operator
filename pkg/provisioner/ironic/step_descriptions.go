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

	// Clean steps (common defaults)
	"deploy.erase_devices_metadata": "Erasing device metadata",
	"deploy.erase_devices":          "Securely erasing devices",
	"raid.delete_configuration":     "Deleting RAID configuration",
	"raid.create_configuration":     "Creating RAID configuration",
	"bios.apply_configuration":      "Applying BIOS configuration",
	"bios.factory_reset":            "Resetting BIOS to factory defaults",
	"management.clear_job_queue":    "Clearing management controller job queue",
	"management.reset_idrac":        "Resetting iDRAC",
}

// describeStep returns a human-readable description for the given
// "<interface>.<step>" identifier, or an empty string if no description is
// known.
func describeStep(name string) string {
	return stepDescriptions[name]
}
