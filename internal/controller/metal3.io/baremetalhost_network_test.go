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

package controllers

import (
	"context"
	"testing"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/features"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func nicsFromHost(host *metal3api.BareMetalHost) []metal3api.NIC {
	if host.Status.HardwareDetails != nil {
		return host.Status.HardwareDetails.NIC
	}
	return nil
}

func TestValidateNetworkInterfaces(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = metal3api.AddToScheme(scheme)

	testCases := []struct {
		name                   string
		host                   *metal3api.BareMetalHost
		expectedDirty          bool
		expectedValidationPass bool
		expectedReason         string
	}{
		{
			name: "no-network-interfaces-specified",
			host: &metal3api.BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-host",
					Namespace: "test-ns",
				},
				Spec: metal3api.BareMetalHostSpec{},
			},
			expectedDirty:          false,
			expectedValidationPass: false,
		},
		{
			name: "hardware-inspection-not-complete",
			host: &metal3api.BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-host",
					Namespace: "test-ns",
				},
				Spec: metal3api.BareMetalHostSpec{
					NetworkInterfaces: []metal3api.NetworkInterface{
						{Name: "eth0"},
					},
				},
				Status: metal3api.BareMetalHostStatus{
					Provisioning: metal3api.ProvisionStatus{
						State: metal3api.StateRegistering,
					},
				},
			},
			expectedDirty:          true,
			expectedValidationPass: false,
			expectedReason:         "HardwareInspectionIncomplete",
		},
		{
			name: "all-interfaces-valid-by-name",
			host: &metal3api.BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-host",
					Namespace: "test-ns",
				},
				Spec: metal3api.BareMetalHostSpec{
					NetworkInterfaces: []metal3api.NetworkInterface{
						{Name: "eth0"},
						{Name: "eth1"},
					},
				},
				Status: metal3api.BareMetalHostStatus{
					Provisioning: metal3api.ProvisionStatus{
						State: metal3api.StateAvailable,
					},
					HardwareDetails: &metal3api.HardwareDetails{
						NIC: []metal3api.NIC{
							{Name: "eth0", MAC: "00:11:22:33:44:55"},
							{Name: "eth1", MAC: "00:11:22:33:44:66"},
						},
					},
				},
			},
			expectedDirty:          true,
			expectedValidationPass: true,
			expectedReason:         "AllInterfacesValid",
		},
		{
			name: "all-interfaces-valid-by-mac",
			host: &metal3api.BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-host",
					Namespace: "test-ns",
				},
				Spec: metal3api.BareMetalHostSpec{
					NetworkInterfaces: []metal3api.NetworkInterface{
						{MACAddress: "00:11:22:33:44:55"},
					},
				},
				Status: metal3api.BareMetalHostStatus{
					Provisioning: metal3api.ProvisionStatus{
						State: metal3api.StateAvailable,
					},
					HardwareDetails: &metal3api.HardwareDetails{
						NIC: []metal3api.NIC{
							{Name: "eth0", MAC: "00:11:22:33:44:55"},
						},
					},
				},
			},
			expectedDirty:          true,
			expectedValidationPass: true,
			expectedReason:         "AllInterfacesValid",
		},
		{
			name: "invalid-interface-name",
			host: &metal3api.BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-host",
					Namespace: "test-ns",
				},
				Spec: metal3api.BareMetalHostSpec{
					NetworkInterfaces: []metal3api.NetworkInterface{
						{Name: "invalid-interface"},
					},
				},
				Status: metal3api.BareMetalHostStatus{
					Provisioning: metal3api.ProvisionStatus{
						State: metal3api.StateAvailable,
					},
					HardwareDetails: &metal3api.HardwareDetails{
						NIC: []metal3api.NIC{
							{Name: "eth0", MAC: "00:11:22:33:44:55"},
						},
					},
				},
			},
			expectedDirty:          true,
			expectedValidationPass: false,
			expectedReason:         "InvalidInterfaceNames",
		},
		{
			name: "mixed-valid-and-invalid",
			host: &metal3api.BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-host",
					Namespace: "test-ns",
				},
				Spec: metal3api.BareMetalHostSpec{
					NetworkInterfaces: []metal3api.NetworkInterface{
						{Name: "eth0"},
						{Name: "invalid"},
					},
				},
				Status: metal3api.BareMetalHostStatus{
					Provisioning: metal3api.ProvisionStatus{
						State: metal3api.StateAvailable,
					},
					HardwareDetails: &metal3api.HardwareDetails{
						NIC: []metal3api.NIC{
							{Name: "eth0", MAC: "00:11:22:33:44:55"},
						},
					},
				},
			},
			expectedDirty:          true,
			expectedValidationPass: false,
			expectedReason:         "InvalidInterfaceNames",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r := &BareMetalHostReconciler{
				Client: fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(tc.host).Build(),
			}

			dirty, err := r.validateNetworkInterfaces(context.Background(), tc.host, tc.host.Status.HardwareDetails)
			require.NoError(t, err)
			assert.Equal(t, tc.expectedDirty, dirty)

			cond := meta.FindStatusCondition(tc.host.Status.Conditions, metal3api.NetworkInterfacesValidCondition)
			if tc.expectedValidationPass {
				require.NotNil(t, cond)
				assert.Equal(t, metav1.ConditionTrue, cond.Status)
				assert.Equal(t, tc.expectedReason, cond.Reason)
			} else if tc.expectedReason != "" {
				require.NotNil(t, cond)
				assert.Equal(t, metav1.ConditionFalse, cond.Status)
				assert.Equal(t, tc.expectedReason, cond.Reason)
			}
		})
	}
}

func TestPreparePortConfigs(t *testing.T) {
	featuregatetesting.SetFeatureGateDuringTest(t, features.CurrentFeatureGate, features.FeatureIronicNetworking, true)

	scheme := runtime.NewScheme()
	require.NoError(t, metal3api.AddToScheme(scheme))

	t.Run("clears validation and prepares empty configs when interfaces are removed", func(t *testing.T) {
		host := &metal3api.BareMetalHost{
			ObjectMeta: metav1.ObjectMeta{Name: "test-host", Namespace: "test-ns"},
			Status: metal3api.BareMetalHostStatus{
				Conditions: []metav1.Condition{{
					Type:   metal3api.NetworkInterfacesValidCondition,
					Status: metav1.ConditionTrue,
				}},
				AppliedNetworkAttachmentConfigs: []metal3api.AppliedNetworkAttachmentConfig{{Name: "eth0", MACAddress: "00:11:22:33:44:55"}},
			},
		}
		info := &reconcileInfo{host: host}
		r := &BareMetalHostReconciler{}

		dirty, err := r.preparePortConfigs(context.Background(), info)

		require.NoError(t, err)
		assert.True(t, dirty)
		assert.NotNil(t, info.portConfigs)
		assert.Empty(t, info.portConfigs)
		assert.Nil(t, meta.FindStatusCondition(host.Status.Conditions, metal3api.NetworkInterfacesValidCondition))
	})

	t.Run("preserves nil configs when validation fails", func(t *testing.T) {
		host := &metal3api.BareMetalHost{
			ObjectMeta: metav1.ObjectMeta{Name: "test-host", Namespace: "test-ns"},
			Spec: metal3api.BareMetalHostSpec{
				NetworkInterfaces: []metal3api.NetworkInterface{{Name: "eth0"}},
			},
		}
		info := &reconcileInfo{host: host, hardwareData: &metal3api.HardwareData{}}
		r := &BareMetalHostReconciler{}

		dirty, err := r.preparePortConfigs(context.Background(), info)

		require.NoError(t, err)
		assert.True(t, dirty)
		assert.Nil(t, info.portConfigs)
		cond := meta.FindStatusCondition(host.Status.Conditions, metal3api.NetworkInterfacesValidCondition)
		require.NotNil(t, cond)
		assert.Equal(t, metav1.ConditionFalse, cond.Status)
		assert.Equal(t, "HardwareInspectionIncomplete", cond.Reason)
	})

	t.Run("stores configs after successful validation", func(t *testing.T) {
		attachment := &metal3api.HostNetworkAttachment{
			ObjectMeta: metav1.ObjectMeta{Name: "test-attachment", Namespace: "test-ns"},
			Spec: metal3api.HostNetworkAttachmentSpec{
				Mode:       metal3api.SwitchportModeAccess,
				NativeVLAN: 100,
			},
		}
		host := &metal3api.BareMetalHost{
			ObjectMeta: metav1.ObjectMeta{Name: "test-host", Namespace: "test-ns"},
			Spec: metal3api.BareMetalHostSpec{
				NetworkInterfaces: []metal3api.NetworkInterface{{
					Name: "eth0",
					HostNetworkAttachment: metal3api.HostNetworkAttachmentRef{
						Name: "test-attachment",
					},
				}},
			},
		}
		info := &reconcileInfo{
			host: host,
			hardwareData: &metal3api.HardwareData{Spec: metal3api.HardwareDataSpec{
				HardwareDetails: &metal3api.HardwareDetails{
					NIC: []metal3api.NIC{{Name: "eth0", MAC: "00:11:22:33:44:55"}},
				},
			}},
		}
		r := &BareMetalHostReconciler{
			Client: fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(attachment).Build(),
		}

		dirty, err := r.preparePortConfigs(context.Background(), info)

		require.NoError(t, err)
		assert.True(t, dirty)
		config, ok := info.portConfigs["00:11:22:33:44:55"]
		require.True(t, ok)
		assert.Equal(t, metal3api.SwitchportModeAccess, config.SwitchPortConfig.Mode)
		assert.Equal(t, 100, config.SwitchPortConfig.NativeVLAN)
		cond := meta.FindStatusCondition(host.Status.Conditions, metal3api.NetworkInterfacesValidCondition)
		require.NotNil(t, cond)
		assert.Equal(t, metav1.ConditionTrue, cond.Status)
	})
}

func TestPreparePortConfigsWithIronicNetworkingDisabled(t *testing.T) {
	featuregatetesting.SetFeatureGateDuringTest(t, features.CurrentFeatureGate, features.FeatureIronicNetworking, false)

	host := &metal3api.BareMetalHost{
		Spec: metal3api.BareMetalHostSpec{
			NetworkInterfaces: []metal3api.NetworkInterface{{Name: "eth0"}},
		},
		Status: metal3api.BareMetalHostStatus{
			AppliedNetworkAttachmentConfigs: []metal3api.AppliedNetworkAttachmentConfig{{Name: "eth0", MACAddress: "00:11:22:33:44:55"}},
		},
	}
	info := &reconcileInfo{host: host}
	r := &BareMetalHostReconciler{}

	dirty, err := r.preparePortConfigs(t.Context(), info)

	require.NoError(t, err)
	assert.False(t, dirty)
	assert.Nil(t, info.portConfigs)
	assert.Nil(t, meta.FindStatusCondition(host.Status.Conditions, metal3api.NetworkInterfacesValidCondition))
	assert.Len(t, host.Status.AppliedNetworkAttachmentConfigs, 1)
}

func TestGetAvailableNICNames(t *testing.T) {
	testCases := []struct {
		name     string
		nics     []metal3api.NIC
		expected []string
	}{
		{
			name: "multiple-nics",
			nics: []metal3api.NIC{
				{Name: "eth2", MAC: "00:11:22:33:44:77"},
				{Name: "eth0", MAC: "00:11:22:33:44:55"},
				{Name: "eth1", MAC: "00:11:22:33:44:66"},
			},
			expected: []string{"eth0", "eth1", "eth2"},
		},
		{
			name: "duplicate-names-removed",
			nics: []metal3api.NIC{
				{Name: "eth1", MAC: "00:11:22:33:44:66"},
				{Name: "eth0", MAC: "00:11:22:33:44:55"},
				{Name: "eth1", MAC: "00:11:22:33:44:77"},
			},
			expected: []string{"eth0", "eth1"},
		},
		{
			name: "empty-names-filtered",
			nics: []metal3api.NIC{
				{Name: "", MAC: "00:11:22:33:44:55"},
				{Name: "eth0", MAC: "00:11:22:33:44:66"},
			},
			expected: []string{"eth0"},
		},
		{
			name:     "empty-list",
			nics:     []metal3api.NIC{},
			expected: []string{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r := &BareMetalHostReconciler{}
			result := r.getAvailableNICNames(tc.nics)
			if len(tc.expected) == 0 {
				assert.Empty(t, result)
				return
			}
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestResolveSwitchPortConfigs(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = metal3api.AddToScheme(scheme)

	attachment := &metal3api.HostNetworkAttachment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-attachment",
			Namespace: "test-ns",
		},
		Spec: metal3api.HostNetworkAttachmentSpec{
			Mode:         metal3api.SwitchportModeAccess,
			NativeVLAN:   100,
			AllowedVLANs: nil,
			MTU:          ptr.To(9000),
		},
	}

	host := &metal3api.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-host",
			Namespace: "test-ns",
		},
		Spec: metal3api.BareMetalHostSpec{
			NetworkInterfaces: []metal3api.NetworkInterface{
				{
					Name: "eth0",
					HostNetworkAttachment: metal3api.HostNetworkAttachmentRef{
						Name: "test-attachment",
					},
				},
			},
		},
		Status: metal3api.BareMetalHostStatus{
			HardwareDetails: &metal3api.HardwareDetails{
				NIC: []metal3api.NIC{
					{Name: "eth0", MAC: "AA:BB:CC:DD:EE:F0"},
				},
			},
		},
	}

	r := &BareMetalHostReconciler{
		Client: fakeclient.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(host, attachment).
			Build(),
	}

	configs, err := r.resolvePortConfigs(context.TODO(), host, nicsFromHost(host))
	require.NoError(t, err)
	assert.Len(t, configs, 1)
	assert.Contains(t, configs, "aa:bb:cc:dd:ee:f0")
	assert.Equal(t, metal3api.SwitchPortMode("access"), configs["aa:bb:cc:dd:ee:f0"].SwitchPortConfig.Mode)
	assert.Equal(t, 100, configs["aa:bb:cc:dd:ee:f0"].SwitchPortConfig.NativeVLAN)
	assert.Equal(t, ptr.To(9000), configs["aa:bb:cc:dd:ee:f0"].SwitchPortConfig.MTU)
}

func TestResolveSwitchPortConfigsCrossNamespace(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = metal3api.AddToScheme(scheme)

	attachment := &metal3api.HostNetworkAttachment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-attachment",
			Namespace: "other-ns",
		},
		Spec: metal3api.HostNetworkAttachmentSpec{
			Mode:         metal3api.SwitchportModeTrunk,
			NativeVLAN:   1,
			AllowedVLANs: []string{"10", "20", "30"},
		},
	}

	host := &metal3api.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-host",
			Namespace: "test-ns",
		},
		Spec: metal3api.BareMetalHostSpec{
			NetworkInterfaces: []metal3api.NetworkInterface{
				{
					Name: "eth0",
					HostNetworkAttachment: metal3api.HostNetworkAttachmentRef{
						Name:      "test-attachment",
						Namespace: "other-ns",
					},
				},
			},
		},
		Status: metal3api.BareMetalHostStatus{
			HardwareDetails: &metal3api.HardwareDetails{
				NIC: []metal3api.NIC{
					{Name: "eth0", MAC: "AA:BB:CC:DD:EE:F0"},
				},
			},
		},
	}

	r := &BareMetalHostReconciler{
		Client: fakeclient.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(host, attachment).
			Build(),
	}

	configs, err := r.resolvePortConfigs(context.TODO(), host, nicsFromHost(host))
	require.NoError(t, err)
	assert.Len(t, configs, 1)
	assert.Contains(t, configs, "aa:bb:cc:dd:ee:f0")
	assert.Equal(t, metal3api.SwitchPortMode("trunk"), configs["aa:bb:cc:dd:ee:f0"].SwitchPortConfig.Mode)
	assert.Equal(t, []int{10, 20, 30}, configs["aa:bb:cc:dd:ee:f0"].SwitchPortConfig.AllowedVLANs)
}

func TestResolveSwitchPortConfigsNotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = metal3api.AddToScheme(scheme)

	host := &metal3api.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-host",
			Namespace: "test-ns",
		},
		Spec: metal3api.BareMetalHostSpec{
			NetworkInterfaces: []metal3api.NetworkInterface{
				{
					MACAddress: "00:11:22:33:44:55",
					HostNetworkAttachment: metal3api.HostNetworkAttachmentRef{
						Name: "non-existent",
					},
				},
			},
		},
	}

	r := &BareMetalHostReconciler{
		Client: fakeclient.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(host).
			Build(),
		Log: ctrl.Log.WithName("controllers").WithName("BareMetalHost"),
	}

	// With the graceful handling of missing attachments, this should not error.
	// Instead, it should return an empty config map (the interface is skipped).
	configs, err := r.resolvePortConfigs(context.TODO(), host, nicsFromHost(host))
	require.NoError(t, err)
	assert.NotNil(t, configs)
	assert.Empty(t, configs)
}

func TestResolveSwitchPortConfigsPartialSuccess(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = metal3api.AddToScheme(scheme)

	// Create one valid attachment
	attachment1 := &metal3api.HostNetworkAttachment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "valid-attachment",
			Namespace: "test-ns",
		},
		Spec: metal3api.HostNetworkAttachmentSpec{
			Mode:       metal3api.SwitchportModeAccess,
			NativeVLAN: 100,
			MTU:        ptr.To(1500),
		},
	}

	host := &metal3api.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-host",
			Namespace: "test-ns",
		},
		Spec: metal3api.BareMetalHostSpec{
			NetworkInterfaces: []metal3api.NetworkInterface{
				{
					Name: "eth0",
					HostNetworkAttachment: metal3api.HostNetworkAttachmentRef{
						Name: "valid-attachment",
					},
				},
				{
					Name: "eth1",
					HostNetworkAttachment: metal3api.HostNetworkAttachmentRef{
						Name: "missing-attachment",
					},
				},
			},
		},
		Status: metal3api.BareMetalHostStatus{
			HardwareDetails: &metal3api.HardwareDetails{
				NIC: []metal3api.NIC{
					{Name: "eth0", MAC: "AA:BB:CC:DD:EE:F0"},
					{Name: "eth1", MAC: "AA:BB:CC:DD:EE:F1"},
				},
			},
		},
	}

	r := &BareMetalHostReconciler{
		Client: fakeclient.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(host, attachment1).
			Build(),
		Log: ctrl.Log.WithName("controllers").WithName("BareMetalHost"),
	}

	// Should succeed and return config for eth0, skip eth1 gracefully
	configs, err := r.resolvePortConfigs(context.TODO(), host, nicsFromHost(host))
	require.NoError(t, err)
	assert.NotNil(t, configs)
	assert.Len(t, configs, 1)
	assert.Contains(t, configs, "aa:bb:cc:dd:ee:f0")
	assert.NotContains(t, configs, "aa:bb:cc:dd:ee:f1")
	assert.Equal(t, metal3api.SwitchPortMode("access"), configs["aa:bb:cc:dd:ee:f0"].SwitchPortConfig.Mode)
	assert.Equal(t, 100, configs["aa:bb:cc:dd:ee:f0"].SwitchPortConfig.NativeVLAN)
}

func TestResolveSwitchPortConfigsWithManualSwitchPort(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = metal3api.AddToScheme(scheme)

	host := &metal3api.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-host",
			Namespace: "test-ns",
		},
		Spec: metal3api.BareMetalHostSpec{
			NetworkInterfaces: []metal3api.NetworkInterface{
				{
					MACAddress: "00:11:22:33:44:55",
					SwitchPort: &metal3api.SwitchPortIdentifier{
						SwitchID: "aa:bb:cc:dd:ee:ff",
						PortID:   "Ethernet1/1",
					},
				},
			},
		},
	}

	r := &BareMetalHostReconciler{
		Client: fakeclient.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(host).
			Build(),
		Log: ctrl.Log.WithName("controllers").WithName("BareMetalHost"),
	}

	// SwitchPort without an HNA is skipped by the code (no switch port config to apply)
	configs, err := r.resolvePortConfigs(context.TODO(), host, nicsFromHost(host))
	require.NoError(t, err)
	assert.Empty(t, configs)
}

func TestResolveSwitchPortConfigsWithHNAAndSwitchPort(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = metal3api.AddToScheme(scheme)

	attachment := &metal3api.HostNetworkAttachment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-attachment",
			Namespace: "test-ns",
		},
		Spec: metal3api.HostNetworkAttachmentSpec{
			Mode:       metal3api.SwitchportModeAccess,
			NativeVLAN: 100,
		},
	}

	host := &metal3api.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-host",
			Namespace: "test-ns",
		},
		Spec: metal3api.BareMetalHostSpec{
			NetworkInterfaces: []metal3api.NetworkInterface{
				{
					MACAddress: "00:11:22:33:44:55",
					HostNetworkAttachment: metal3api.HostNetworkAttachmentRef{
						Name: "test-attachment",
					},
					SwitchPort: &metal3api.SwitchPortIdentifier{
						SwitchID: "aa:bb:cc:dd:ee:ff",
						PortID:   "Ethernet1/1",
					},
				},
			},
		},
	}

	r := &BareMetalHostReconciler{
		Client: fakeclient.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(host, attachment).
			Build(),
	}

	configs, err := r.resolvePortConfigs(context.TODO(), host, nicsFromHost(host))
	require.NoError(t, err)
	assert.Len(t, configs, 1)
	cfg := configs["00:11:22:33:44:55"]
	assert.Equal(t, metal3api.SwitchPortMode("access"), cfg.SwitchPortConfig.Mode)
	assert.Equal(t, 100, cfg.SwitchPortConfig.NativeVLAN)
	assert.Equal(t, "aa:bb:cc:dd:ee:ff", cfg.SwitchPortIdentifier.SwitchID)
	assert.Equal(t, "Ethernet1/1", cfg.SwitchPortIdentifier.PortID)
}

func TestValidateNetworkInterfacesMissingAttachment(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = metal3api.AddToScheme(scheme)

	host := &metal3api.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-host",
			Namespace: "test-ns",
		},
		Spec: metal3api.BareMetalHostSpec{
			NetworkInterfaces: []metal3api.NetworkInterface{
				{
					Name: "eth0",
					HostNetworkAttachment: metal3api.HostNetworkAttachmentRef{
						Name: "missing-hna",
					},
				},
			},
		},
		Status: metal3api.BareMetalHostStatus{
			HardwareDetails: &metal3api.HardwareDetails{
				NIC: []metal3api.NIC{
					{Name: "eth0", MAC: "00:11:22:33:44:55"},
				},
			},
		},
	}

	r := &BareMetalHostReconciler{
		Client: fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(host).Build(),
	}

	dirty, err := r.validateNetworkInterfaces(context.Background(), host, host.Status.HardwareDetails)
	require.NoError(t, err)
	assert.True(t, dirty)

	cond := meta.FindStatusCondition(host.Status.Conditions, metal3api.NetworkInterfacesValidCondition)
	require.NotNil(t, cond)
	assert.Equal(t, metav1.ConditionFalse, cond.Status)
	assert.Equal(t, "AttachmentNotFound", cond.Reason)
	assert.Contains(t, cond.Message, "missing-hna")
}

func TestBuildAppliedNetworkAttachmentConfigs(t *testing.T) {
	hwDetails := &metal3api.HardwareDetails{
		NIC: []metal3api.NIC{
			{Name: "eth0", MAC: "00:11:22:33:44:55"},
			{Name: "eth1", MAC: "aa:bb:cc:dd:ee:ff"},
		},
	}

	info := &reconcileInfo{
		hardwareData: &metal3api.HardwareData{
			Spec: metal3api.HardwareDataSpec{
				HardwareDetails: hwDetails,
			},
		},
		portConfigs: map[string]*provisioner.PortConfig{
			"00:11:22:33:44:55": {
				SwitchPortConfig: metal3api.SwitchPortConfig{Mode: "access", NativeVLAN: 100, MTU: ptr.To(9000)},
			},
			"aa:bb:cc:dd:ee:ff": {
				SwitchPortConfig:     metal3api.SwitchPortConfig{Mode: "trunk", NativeVLAN: 1, AllowedVLANs: []int{10, 20}},
				SwitchPortIdentifier: &metal3api.SwitchPortIdentifier{SwitchID: "switch1", PortID: "Eth1/1"},
			},
		},
	}

	applied := buildAppliedNetworkAttachmentConfigs(info)
	assert.Len(t, applied, 2)

	configByName := make(map[string]metal3api.AppliedNetworkAttachmentConfig)
	for _, a := range applied {
		configByName[a.Name] = a
	}

	eth0 := configByName["eth0"]
	assert.Equal(t, "00:11:22:33:44:55", eth0.MACAddress)
	assert.Equal(t, metal3api.SwitchPortMode("access"), eth0.SwitchPortConfig.Mode)
	assert.Equal(t, 100, eth0.SwitchPortConfig.NativeVLAN)
	assert.Equal(t, ptr.To(9000), eth0.SwitchPortConfig.MTU)
	assert.Nil(t, eth0.SwitchPort)

	eth1 := configByName["eth1"]
	assert.Equal(t, "aa:bb:cc:dd:ee:ff", eth1.MACAddress)
	assert.Equal(t, metal3api.SwitchPortMode("trunk"), eth1.SwitchPortConfig.Mode)
	assert.Equal(t, 1, eth1.SwitchPortConfig.NativeVLAN)
	assert.Equal(t, []int{10, 20}, eth1.SwitchPortConfig.AllowedVLANs)
	require.NotNil(t, eth1.SwitchPort)
	assert.Equal(t, "switch1", eth1.SwitchPort.SwitchID)
	assert.Equal(t, "Eth1/1", eth1.SwitchPort.PortID)
}

func TestBuildRecoveryPortConfigsEmptySnapshot(t *testing.T) {
	t.Run("initial registration", func(t *testing.T) {
		info := &reconcileInfo{host: &metal3api.BareMetalHost{}}
		assert.Nil(t, buildRecoveryPortConfigs(info))
	})

	t.Run("registered host", func(t *testing.T) {
		host := &metal3api.BareMetalHost{}
		host.Status.Provisioning.ID = "old-node-uuid"
		host.Status.Provisioning.State = metal3api.StateProvisioned
		configs := buildRecoveryPortConfigs(&reconcileInfo{host: host})
		require.NotNil(t, configs)
		assert.Empty(t, configs)
	})
}

func TestBuildRecoveryPortConfigs(t *testing.T) {
	mtu := 9000
	host := &metal3api.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{Name: "test-host", Namespace: "test-ns"},
		Spec: metal3api.BareMetalHostSpec{
			NetworkInterfaces: []metal3api.NetworkInterface{
				{
					Name: "eth0",
					HostNetworkAttachment: metal3api.HostNetworkAttachmentRef{
						Name: "deleted-attachment",
					},
				},
				{
					Name: "eth1",
				},
				{
					Name:       "eth2",
					MACAddress: "AA:BB:CC:DD:EE:F2",
					HostNetworkAttachment: metal3api.HostNetworkAttachmentRef{
						Name: "deleted-attachment",
					},
				},
			},
		},
		Status: metal3api.BareMetalHostStatus{
			HardwareDetails: &metal3api.HardwareDetails{
				NIC: []metal3api.NIC{
					{Name: "eth0", MAC: "AA:BB:CC:DD:EE:F0"},
					{Name: "eth1", MAC: "AA:BB:CC:DD:EE:F1"},
					{MAC: "AA:BB:CC:DD:EE:F2"},
				},
			},
			AppliedNetworkAttachmentConfigs: []metal3api.AppliedNetworkAttachmentConfig{
				{
					Name:       "eth0",
					MACAddress: "AA:BB:CC:DD:EE:F0",
					SwitchPortConfig: metal3api.SwitchPortConfig{
						Mode:         metal3api.SwitchportModeTrunk,
						NativeVLAN:   1,
						AllowedVLANs: []int{10, 20},
						MTU:          &mtu,
					},
					SwitchPort: &metal3api.SwitchPortIdentifier{
						SwitchID: "switch1",
						PortID:   "Ethernet1/1",
					},
				},
				{
					Name:       "aa:bb:cc:dd:ee:f1",
					MACAddress: "AA:BB:CC:DD:EE:F1",
					SwitchPortConfig: metal3api.SwitchPortConfig{
						Mode:       metal3api.SwitchportModeAccess,
						NativeVLAN: 100,
					},
				},
				{
					Name:       "aa:bb:cc:dd:ee:f2",
					MACAddress: "AA:BB:CC:DD:EE:F2",
					SwitchPortConfig: metal3api.SwitchPortConfig{
						Mode:       metal3api.SwitchportModeAccess,
						NativeVLAN: 200,
					},
				},
			},
		},
	}
	info := &reconcileInfo{host: host}

	configs := buildRecoveryPortConfigs(info)

	require.Len(t, configs, 3)
	config := configs["aa:bb:cc:dd:ee:f0"]
	require.NotNil(t, config)
	assert.Equal(t, metal3api.SwitchportModeTrunk, config.SwitchPortConfig.Mode)
	assert.Equal(t, 1, config.SwitchPortConfig.NativeVLAN)
	assert.Equal(t, []int{10, 20}, config.SwitchPortConfig.AllowedVLANs)
	assert.Equal(t, &mtu, config.SwitchPortConfig.MTU)
	require.NotNil(t, config.SwitchPortIdentifier)
	assert.Equal(t, "switch1", config.SwitchPortIdentifier.SwitchID)
	assert.Equal(t, "Ethernet1/1", config.SwitchPortIdentifier.PortID)

	// Recovery restores the full last-applied snapshot, including configs for
	// interfaces that are no longer referenced by an HNA in the current spec.
	staleConfig := configs["aa:bb:cc:dd:ee:f1"]
	require.NotNil(t, staleConfig)
	assert.Equal(t, metal3api.SwitchportModeAccess, staleConfig.SwitchPortConfig.Mode)
	assert.Equal(t, 100, staleConfig.SwitchPortConfig.NativeVLAN)

	// Unnamed NICs still have a persisted MAC address for recovery.
	macNamedConfig := configs["aa:bb:cc:dd:ee:f2"]
	require.NotNil(t, macNamedConfig)
	assert.Equal(t, metal3api.SwitchportModeAccess, macNamedConfig.SwitchPortConfig.Mode)
	assert.Equal(t, 200, macNamedConfig.SwitchPortConfig.NativeVLAN)
}

func TestBuildRecoveryPortConfigsWithChangedNICNames(t *testing.T) {
	const mac = "aa:bb:cc:dd:ee:ff"
	tests := []struct {
		name string
		nics []metal3api.NIC
	}{
		{name: "renamed", nics: []metal3api.NIC{{Name: "eno1", MAC: mac}}},
		{name: "unnamed", nics: []metal3api.NIC{{MAC: mac}}},
		{name: "missing hardware data"},
		{
			name: "name reused for another NIC",
			nics: []metal3api.NIC{
				{Name: "eth0", MAC: "00:11:22:33:44:55"},
				{Name: "eno1", MAC: mac},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host := &metal3api.BareMetalHost{}
			host.Status.HardwareDetails = &metal3api.HardwareDetails{
				NIC: []metal3api.NIC{{Name: "eth0", MAC: mac}},
			}
			info := &reconcileInfo{
				host: host,
				portConfigs: map[string]*provisioner.PortConfig{
					mac: {
						SwitchPortConfig: metal3api.SwitchPortConfig{
							Mode:       metal3api.SwitchportModeAccess,
							NativeVLAN: 100,
						},
					},
				},
			}
			host.Status.AppliedNetworkAttachmentConfigs = buildAppliedNetworkAttachmentConfigs(info)
			host.Status.HardwareDetails.NIC = tt.nics

			configs := buildRecoveryPortConfigs(info)

			require.Len(t, configs, 1)
			require.Contains(t, configs, mac)
			assert.Equal(t, 100, configs[mac].SwitchPortConfig.NativeVLAN)
		})
	}
}

func TestClearNetworkInterfaceValidation(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = metal3api.AddToScheme(scheme)

	t.Run("condition-exists-gets-removed", func(t *testing.T) {
		host := &metal3api.BareMetalHost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-host",
				Namespace: "test-ns",
			},
			Spec: metal3api.BareMetalHostSpec{},
			Status: metal3api.BareMetalHostStatus{
				Conditions: []metav1.Condition{
					{Type: metal3api.NetworkInterfacesValidCondition, Status: metav1.ConditionTrue, Reason: "AllInterfacesValid"},
				},
			},
		}

		r := &BareMetalHostReconciler{
			Client: fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(host).Build(),
		}

		dirty, err := r.validateNetworkInterfaces(context.Background(), host, host.Status.HardwareDetails)
		require.NoError(t, err)
		assert.True(t, dirty, "expected dirty=true when condition is removed")

		cond := meta.FindStatusCondition(host.Status.Conditions, metal3api.NetworkInterfacesValidCondition)
		assert.Nil(t, cond, "expected condition to be removed")
	})

	t.Run("no-condition-exists", func(t *testing.T) {
		host := &metal3api.BareMetalHost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-host",
				Namespace: "test-ns",
			},
			Spec: metal3api.BareMetalHostSpec{},
		}

		r := &BareMetalHostReconciler{
			Client: fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(host).Build(),
		}

		dirty, err := r.validateNetworkInterfaces(context.Background(), host, host.Status.HardwareDetails)
		require.NoError(t, err)
		assert.False(t, dirty, "expected dirty=false when no condition existed")
	})
}

func TestSetNetworkInterfaceValidationIdempotency(t *testing.T) {
	r := &BareMetalHostReconciler{}

	host := &metal3api.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-host",
			Namespace: "test-ns",
		},
	}

	// First call should be dirty
	dirty, err := r.setNetworkInterfaceValidation(host, metav1.ConditionTrue, "AllInterfacesValid", "All network interfaces and attachments are valid")
	require.NoError(t, err)
	assert.True(t, dirty, "expected dirty=true on first call")

	// Second call with same values should not be dirty
	dirty, err = r.setNetworkInterfaceValidation(host, metav1.ConditionTrue, "AllInterfacesValid", "All network interfaces and attachments are valid")
	require.NoError(t, err)
	assert.False(t, dirty, "expected dirty=false on second call with same values")

	// A new BMH generation must refresh an otherwise identical condition.
	host.Generation = 1
	dirty, err = r.setNetworkInterfaceValidation(host, metav1.ConditionTrue, "AllInterfacesValid", "All network interfaces and attachments are valid")
	require.NoError(t, err)
	assert.True(t, dirty, "expected dirty=true when the observed generation changes")
	cond := meta.FindStatusCondition(host.Status.Conditions, metal3api.NetworkInterfacesValidCondition)
	require.NotNil(t, cond)
	assert.Equal(t, int64(1), cond.ObservedGeneration)

	dirty, err = r.setNetworkInterfaceValidation(host, metav1.ConditionTrue, "AllInterfacesValid", "All network interfaces and attachments are valid")
	require.NoError(t, err)
	assert.False(t, dirty, "expected dirty=false after observing the current generation")
}

func TestBuildNICNameToMACMap(t *testing.T) {
	t.Run("normal-nics", func(t *testing.T) {
		host := &metal3api.BareMetalHost{
			Status: metal3api.BareMetalHostStatus{
				HardwareDetails: &metal3api.HardwareDetails{
					NIC: []metal3api.NIC{
						{Name: "eth0", MAC: "AA:BB:CC:DD:EE:F0"},
						{Name: "eth1", MAC: "AA:BB:CC:DD:EE:F1"},
					},
				},
			},
		}
		result := buildNICNameToMACMap(nicsFromHost(host))
		assert.Len(t, result, 2)
		assert.Equal(t, "aa:bb:cc:dd:ee:f0", result["eth0"])
		assert.Equal(t, "aa:bb:cc:dd:ee:f1", result["eth1"])
	})

	t.Run("nil-hardware-details", func(t *testing.T) {
		host := &metal3api.BareMetalHost{}
		result := buildNICNameToMACMap(nicsFromHost(host))
		assert.Empty(t, result)
	})

	t.Run("empty-name-or-mac-skipped", func(t *testing.T) {
		host := &metal3api.BareMetalHost{
			Status: metal3api.BareMetalHostStatus{
				HardwareDetails: &metal3api.HardwareDetails{
					NIC: []metal3api.NIC{
						{Name: "", MAC: "AA:BB:CC:DD:EE:F0"},
						{Name: "eth1", MAC: ""},
						{Name: "eth2", MAC: "AA:BB:CC:DD:EE:F2"},
					},
				},
			},
		}
		result := buildNICNameToMACMap(nicsFromHost(host))
		assert.Len(t, result, 1)
		assert.Equal(t, "aa:bb:cc:dd:ee:f2", result["eth2"])
	})
}

func TestBuildNICMacToNameMap(t *testing.T) {
	nics := []metal3api.NIC{
		{Name: "eth0", MAC: "AA:BB:CC:DD:EE:F0"},
		{Name: "eth1", MAC: "AA:BB:CC:DD:EE:F1"},
	}

	t.Run("normal-nics", func(t *testing.T) {
		host := &metal3api.BareMetalHost{
			Status: metal3api.BareMetalHostStatus{
				HardwareDetails: &metal3api.HardwareDetails{NIC: nics},
			},
		}
		result := buildNICMacToNameMap(nicsFromHost(host))
		require.Len(t, result, 2)
		assert.Equal(t, "eth0", result["aa:bb:cc:dd:ee:f0"])
		assert.Equal(t, "eth1", result["aa:bb:cc:dd:ee:f1"])
	})

	t.Run("nil-hardware-details", func(t *testing.T) {
		host := &metal3api.BareMetalHost{}
		result := buildNICMacToNameMap(nicsFromHost(host))
		require.Empty(t, result)
	})

	t.Run("empty-name-or-mac-skipped", func(t *testing.T) {
		host := &metal3api.BareMetalHost{
			Status: metal3api.BareMetalHostStatus{
				HardwareDetails: &metal3api.HardwareDetails{
					NIC: []metal3api.NIC{
						{Name: "", MAC: "AA:BB:CC:DD:EE:F0"},
						{Name: "eth1", MAC: ""},
						{Name: "eth2", MAC: "AA:BB:CC:DD:EE:F2"},
					},
				},
			},
		}
		result := buildNICMacToNameMap(nicsFromHost(host))
		require.Len(t, result, 1)
		assert.Equal(t, "eth2", result["aa:bb:cc:dd:ee:f2"])
	})
}

func TestResolvePortConfigsNameToMACFails(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = metal3api.AddToScheme(scheme)

	attachment := &metal3api.HostNetworkAttachment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-attachment",
			Namespace: "test-ns",
		},
		Spec: metal3api.HostNetworkAttachmentSpec{
			Mode:       metal3api.SwitchportModeAccess,
			NativeVLAN: 100,
		},
	}

	host := &metal3api.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-host",
			Namespace: "test-ns",
		},
		Spec: metal3api.BareMetalHostSpec{
			NetworkInterfaces: []metal3api.NetworkInterface{
				{
					Name: "unknown",
					HostNetworkAttachment: metal3api.HostNetworkAttachmentRef{
						Name: "test-attachment",
					},
				},
			},
		},
		Status: metal3api.BareMetalHostStatus{
			HardwareDetails: &metal3api.HardwareDetails{
				NIC: []metal3api.NIC{
					{Name: "eth0", MAC: "00:11:22:33:44:55"},
				},
			},
		},
	}

	r := &BareMetalHostReconciler{
		Client: fakeclient.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(host, attachment).
			Build(),
		Log: ctrl.Log.WithName("controllers").WithName("BareMetalHost"),
	}

	configs, err := r.resolvePortConfigs(context.TODO(), host, nicsFromHost(host))
	require.NoError(t, err)
	assert.NotNil(t, configs)
	assert.Empty(t, configs, "expected empty configs when interface name cannot be resolved to MAC")
}

func TestExpandVLANRanges(t *testing.T) {
	tests := []struct {
		name    string
		input   []string
		want    []int
		wantErr bool
	}{
		{
			name:  "nil input",
			input: nil,
			want:  nil,
		},
		{
			name:  "empty input",
			input: []string{},
			want:  nil,
		},
		{
			name:  "single VLANs",
			input: []string{"100", "200", "300"},
			want:  []int{100, 200, 300},
		},
		{
			name:  "single range",
			input: []string{"100-103"},
			want:  []int{100, 101, 102, 103},
		},
		{
			name:  "mixed singles and ranges",
			input: []string{"10", "100-103", "200"},
			want:  []int{10, 100, 101, 102, 103, 200},
		},
		{
			name:  "result is sorted",
			input: []string{"300", "100-102", "50"},
			want:  []int{50, 100, 101, 102, 300},
		},
		{
			name:    "invalid entry",
			input:   []string{"abc"},
			wantErr: true,
		},
		{
			name:    "invalid range end",
			input:   []string{"100-abc"},
			wantErr: true,
		},
		{
			name:    "single VLAN below minimum",
			input:   []string{"0"},
			wantErr: true,
		},
		{
			name:    "single VLAN above maximum",
			input:   []string{"4095"},
			wantErr: true,
		},
		{
			name:    "single negative VLAN",
			input:   []string{"-5"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := expandVLANRanges(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
