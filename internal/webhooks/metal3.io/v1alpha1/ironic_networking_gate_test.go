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

package webhooks

import (
	"context"
	"testing"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/features"
	"github.com/stretchr/testify/require"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
)

func TestIronicNetworkingGateOnCreate(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		name := "disabled"
		if enabled {
			name = "enabled"
		}
		t.Run(name, func(t *testing.T) {
			featuregatetesting.SetFeatureGateDuringTest(t, features.CurrentFeatureGate, features.FeatureIronicNetworking, enabled)

			_, err := (&BareMetalSwitch{}).ValidateCreate(context.Background(), &metal3api.BareMetalSwitch{})
			if enabled {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, "IronicNetworking feature gate must be enabled to create BareMetalSwitch resources")
			}

			attachment := &metal3api.HostNetworkAttachment{
				Spec: metal3api.HostNetworkAttachmentSpec{
					Mode:       metal3api.SwitchportModeAccess,
					NativeVLAN: 1,
				},
			}
			_, err = (&HostNetworkAttachment{}).ValidateCreate(context.Background(), attachment)
			if enabled {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, "IronicNetworking feature gate must be enabled to create HostNetworkAttachment resources")
			}
		})
	}
}
