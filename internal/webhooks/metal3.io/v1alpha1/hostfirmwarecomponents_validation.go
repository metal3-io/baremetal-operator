/*
Copyright 2025 The Metal3 Authors.

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

package webhooks

import (
	"fmt"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
)

// ValidateHostFirmwareComponents validates HostFirmwareComponents resource for creation and updating.
func (webhook *HostFirmwareComponents) validateHostFirmwareComponents(hfc *metal3api.HostFirmwareComponents) []error {
	var errs []error

	for _, update := range hfc.Spec.Updates {
		if err := validateURL(update.URL); err != nil {
			errs = append(errs, fmt.Errorf("invalid URL \"%s\" in FirmwareUpdate in component \"%s\": %w", update.URL, update.Component, err))
		}
	}

	return errs
}
