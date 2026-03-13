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
	"errors"
	"testing"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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
				assert.NotNil(t, cond)
				assert.Equal(t, metav1.ConditionTrue, cond.Status)
				assert.Equal(t, tc.expectedReason, cond.Reason)
			} else if tc.expectedReason != "" {
				assert.NotNil(t, cond)
				assert.Equal(t, metav1.ConditionFalse, cond.Status)
				assert.Equal(t, tc.expectedReason, cond.Reason)
			}
		})
	}
}

func TestSwitchPortConfigurationNeedsUpdate(t *testing.T) {
	testCases := []struct {
		name           string
		host           *metal3api.BareMetalHost
		portConfigs    map[string]*provisioner.PortConfig
		expectedUpdate bool
	}{
		{
			name: "no-network-interfaces-with-existing-applied",
			host: &metal3api.BareMetalHost{
				Spec: metal3api.BareMetalHostSpec{},
				Status: metal3api.BareMetalHostStatus{
					AppliedPortConfigs: []metal3api.AppliedPortConfig{
						{Name: "eth0", SwitchPortConfig: metal3api.SwitchPortConfig{Mode: "access", NativeVLAN: 100}},
					},
				},
			},
			portConfigs:    nil,
			expectedUpdate: true,
		},
		{
			name: "validation-not-passed",
			host: &metal3api.BareMetalHost{
				Spec: metal3api.BareMetalHostSpec{
					NetworkInterfaces: []metal3api.NetworkInterface{
						{Name: "eth0"},
					},
				},
				Status: metal3api.BareMetalHostStatus{
					Conditions: []metav1.Condition{
						{Type: metal3api.NetworkInterfacesValidCondition, Status: metav1.ConditionFalse, Reason: "InvalidInterfaceNames"},
					},
				},
			},
			portConfigs:    nil,
			expectedUpdate: false,
		},
		{
			name: "initial-configuration",
			host: &metal3api.BareMetalHost{
				Spec: metal3api.BareMetalHostSpec{
					NetworkInterfaces: []metal3api.NetworkInterface{
						{Name: "eth0"},
					},
				},
				Status: metal3api.BareMetalHostStatus{
					HardwareDetails: &metal3api.HardwareDetails{
						NIC: []metal3api.NIC{
							{Name: "eth0", MAC: "00:11:22:33:44:55"},
						},
					},
					Conditions: []metav1.Condition{
						{Type: metal3api.NetworkInterfacesValidCondition, Status: metav1.ConditionTrue, Reason: "AllInterfacesValid"},
					},
				},
			},
			portConfigs: map[string]*provisioner.PortConfig{
				"00:11:22:33:44:55": {SwitchPortConfig: metal3api.SwitchPortConfig{Mode: "access", NativeVLAN: 100}},
			},
			expectedUpdate: true,
		},
		{
			name: "configuration-changed",
			host: &metal3api.BareMetalHost{
				Spec: metal3api.BareMetalHostSpec{
					NetworkInterfaces: []metal3api.NetworkInterface{
						{Name: "eth0"},
					},
				},
				Status: metal3api.BareMetalHostStatus{
					HardwareDetails: &metal3api.HardwareDetails{
						NIC: []metal3api.NIC{
							{Name: "eth0", MAC: "00:11:22:33:44:55"},
						},
					},
					Conditions: []metav1.Condition{
						{Type: metal3api.NetworkInterfacesValidCondition, Status: metav1.ConditionTrue, Reason: "AllInterfacesValid"},
					},
					AppliedPortConfigs: []metal3api.AppliedPortConfig{
						{Name: "eth0", SwitchPortConfig: metal3api.SwitchPortConfig{Mode: "access", NativeVLAN: 100}},
					},
				},
			},
			portConfigs: map[string]*provisioner.PortConfig{
				"00:11:22:33:44:55": {SwitchPortConfig: metal3api.SwitchPortConfig{Mode: "access", NativeVLAN: 200}},
			},
			expectedUpdate: true,
		},
		{
			name: "no-changes",
			host: &metal3api.BareMetalHost{
				Spec: metal3api.BareMetalHostSpec{
					NetworkInterfaces: []metal3api.NetworkInterface{
						{Name: "eth0"},
					},
				},
				Status: metal3api.BareMetalHostStatus{
					HardwareDetails: &metal3api.HardwareDetails{
						NIC: []metal3api.NIC{
							{Name: "eth0", MAC: "00:11:22:33:44:55"},
						},
					},
					Conditions: []metav1.Condition{
						{Type: metal3api.NetworkInterfacesValidCondition, Status: metav1.ConditionTrue, Reason: "AllInterfacesValid"},
					},
					AppliedPortConfigs: []metal3api.AppliedPortConfig{
						{Name: "eth0", SwitchPortConfig: metal3api.SwitchPortConfig{Mode: "access", NativeVLAN: 100}},
					},
				},
			},
			portConfigs: map[string]*provisioner.PortConfig{
				"00:11:22:33:44:55": {SwitchPortConfig: metal3api.SwitchPortConfig{Mode: "access", NativeVLAN: 100}},
			},
			expectedUpdate: false,
		},
		{
			name: "hna-deleted-triggers-drift",
			host: &metal3api.BareMetalHost{
				Spec: metal3api.BareMetalHostSpec{
					NetworkInterfaces: []metal3api.NetworkInterface{
						{Name: "eth0"},
					},
				},
				Status: metal3api.BareMetalHostStatus{
					HardwareDetails: &metal3api.HardwareDetails{
						NIC: []metal3api.NIC{
							{Name: "eth0", MAC: "00:11:22:33:44:55"},
						},
					},
					Conditions: []metav1.Condition{
						{Type: metal3api.NetworkInterfacesValidCondition, Status: metav1.ConditionTrue, Reason: "AllInterfacesValid"},
					},
					AppliedPortConfigs: []metal3api.AppliedPortConfig{
						{Name: "eth0", SwitchPortConfig: metal3api.SwitchPortConfig{Mode: "access", NativeVLAN: 100}},
					},
				},
			},
			portConfigs:    map[string]*provisioner.PortConfig{},
			expectedUpdate: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r := &BareMetalHostReconciler{}
			info := &reconcileInfo{
				host:        tc.host,
				portConfigs: tc.portConfigs,
				hardwareData: &metal3api.HardwareData{
					Spec: metal3api.HardwareDataSpec{
						HardwareDetails: tc.host.Status.HardwareDetails,
					},
				},
			}
			needsUpdate := r.portConfigsNeedUpdate(tc.host, info)
			assert.Equal(t, tc.expectedUpdate, needsUpdate)
		})
	}
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
	assert.Equal(t, "aa:bb:cc:dd:ee:ff", cfg.LocalLinkConnection.SwitchID)
	assert.Equal(t, "Ethernet1/1", cfg.LocalLinkConnection.PortID)
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

func TestPortConfigsNeedUpdateWithLLC(t *testing.T) {
	makeHostWithLLC := func() *metal3api.BareMetalHost {
		return &metal3api.BareMetalHost{
			Spec: metal3api.BareMetalHostSpec{
				NetworkInterfaces: []metal3api.NetworkInterface{{Name: "eth0"}},
			},
			Status: metal3api.BareMetalHostStatus{
				HardwareDetails: &metal3api.HardwareDetails{
					NIC: []metal3api.NIC{{Name: "eth0", MAC: "00:11:22:33:44:55"}},
				},
				Conditions: []metav1.Condition{
					{Type: metal3api.NetworkInterfacesValidCondition, Status: metav1.ConditionTrue, Reason: "AllInterfacesValid"},
				},
				AppliedPortConfigs: []metal3api.AppliedPortConfig{
					{
						Name:                "eth0",
						SwitchPortConfig:    metal3api.SwitchPortConfig{Mode: "access", NativeVLAN: 100},
						LocalLinkConnection: &metal3api.SwitchPortIdentifier{SwitchID: "aa:bb:cc:dd:ee:ff", PortID: "Ethernet1/1"},
					},
				},
			},
		}
	}

	r := &BareMetalHostReconciler{}

	t.Run("llc-no-changes", func(t *testing.T) {
		host := makeHostWithLLC()
		info := &reconcileInfo{
			host: host,
			hardwareData: &metal3api.HardwareData{
				Spec: metal3api.HardwareDataSpec{HardwareDetails: host.Status.HardwareDetails},
			},
			portConfigs: map[string]*provisioner.PortConfig{
				"00:11:22:33:44:55": {
					SwitchPortConfig:    metal3api.SwitchPortConfig{Mode: "access", NativeVLAN: 100},
					LocalLinkConnection: &metal3api.SwitchPortIdentifier{SwitchID: "aa:bb:cc:dd:ee:ff", PortID: "Ethernet1/1"},
				},
			},
		}
		assert.False(t, r.portConfigsNeedUpdate(host, info))
	})

	t.Run("llc-changed", func(t *testing.T) {
		host := makeHostWithLLC()
		info := &reconcileInfo{
			host: host,
			hardwareData: &metal3api.HardwareData{
				Spec: metal3api.HardwareDataSpec{HardwareDetails: host.Status.HardwareDetails},
			},
			portConfigs: map[string]*provisioner.PortConfig{
				"00:11:22:33:44:55": {
					SwitchPortConfig:    metal3api.SwitchPortConfig{Mode: "access", NativeVLAN: 100},
					LocalLinkConnection: &metal3api.SwitchPortIdentifier{SwitchID: "aa:bb:cc:dd:ee:ff", PortID: "Ethernet1/2"},
				},
			},
		}
		assert.True(t, r.portConfigsNeedUpdate(host, info))
	})
}

func TestBuildAppliedPortConfigs(t *testing.T) {
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
				SwitchPortConfig:    metal3api.SwitchPortConfig{Mode: "trunk", NativeVLAN: 1, AllowedVLANs: []int{10, 20}},
				LocalLinkConnection: &metal3api.SwitchPortIdentifier{SwitchID: "switch1", PortID: "Eth1/1"},
			},
		},
	}

	applied := buildAppliedPortConfigs(info)
	assert.Len(t, applied, 2)

	configByName := make(map[string]metal3api.AppliedPortConfig)
	for _, a := range applied {
		configByName[a.Name] = a
	}

	eth0 := configByName["eth0"]
	assert.Equal(t, metal3api.SwitchPortMode("access"), eth0.SwitchPortConfig.Mode)
	assert.Equal(t, 100, eth0.SwitchPortConfig.NativeVLAN)
	assert.Equal(t, ptr.To(9000), eth0.SwitchPortConfig.MTU)
	assert.Nil(t, eth0.LocalLinkConnection)

	eth1 := configByName["eth1"]
	assert.Equal(t, metal3api.SwitchPortMode("trunk"), eth1.SwitchPortConfig.Mode)
	assert.Equal(t, 1, eth1.SwitchPortConfig.NativeVLAN)
	assert.Equal(t, []int{10, 20}, eth1.SwitchPortConfig.AllowedVLANs)
	require.NotNil(t, eth1.LocalLinkConnection)
	assert.Equal(t, "switch1", eth1.LocalLinkConnection.SwitchID)
	assert.Equal(t, "Eth1/1", eth1.LocalLinkConnection.PortID)
}

type networkTestProvisioner struct {
	mockProvisioner
	ensurePortsErr   error
	ensurePortsCalls int
}

func (p *networkTestProvisioner) EnsurePorts(_ context.Context) error {
	p.ensurePortsCalls++
	return p.ensurePortsErr
}

func TestApplyPortConfigs(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = metal3api.AddToScheme(scheme)

	host := &metal3api.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{Name: "test-host", Namespace: "test-ns"},
		Spec: metal3api.BareMetalHostSpec{
			NetworkInterfaces: []metal3api.NetworkInterface{
				{Name: "eth0"},
			},
		},
		Status: metal3api.BareMetalHostStatus{
			HardwareDetails: &metal3api.HardwareDetails{
				NIC: []metal3api.NIC{
					{Name: "eth0", MAC: "00:11:22:33:44:55"},
				},
			},
			Conditions: []metav1.Condition{
				{Type: metal3api.NetworkInterfacesValidCondition, Status: metav1.ConditionTrue, Reason: "AllInterfacesValid"},
			},
		},
	}

	info := &reconcileInfo{
		host: host,
		hardwareData: &metal3api.HardwareData{
			Spec: metal3api.HardwareDataSpec{HardwareDetails: host.Status.HardwareDetails},
		},
		portConfigs: map[string]*provisioner.PortConfig{
			"00:11:22:33:44:55": {SwitchPortConfig: metal3api.SwitchPortConfig{Mode: "access", NativeVLAN: 100}},
		},
	}

	prov := &networkTestProvisioner{}
	r := &BareMetalHostReconciler{
		Client: fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(host).Build(),
	}

	dirty, err := r.applyPortConfigs(context.Background(), prov, host, info)
	require.NoError(t, err)
	assert.True(t, dirty)
	assert.Equal(t, 1, prov.ensurePortsCalls)
	require.Len(t, host.Status.AppliedPortConfigs, 1)
	assert.Equal(t, "eth0", host.Status.AppliedPortConfigs[0].Name)
	assert.Equal(t, metal3api.SwitchPortMode("access"), host.Status.AppliedPortConfigs[0].SwitchPortConfig.Mode)
}

func TestApplyPortConfigsEnsurePortsError(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = metal3api.AddToScheme(scheme)

	host := &metal3api.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{Name: "test-host", Namespace: "test-ns"},
		Spec: metal3api.BareMetalHostSpec{
			NetworkInterfaces: []metal3api.NetworkInterface{
				{Name: "eth0"},
			},
		},
		Status: metal3api.BareMetalHostStatus{
			HardwareDetails: &metal3api.HardwareDetails{
				NIC: []metal3api.NIC{
					{Name: "eth0", MAC: "00:11:22:33:44:55"},
				},
			},
			Conditions: []metav1.Condition{
				{Type: metal3api.NetworkInterfacesValidCondition, Status: metav1.ConditionTrue, Reason: "AllInterfacesValid"},
			},
		},
	}

	info := &reconcileInfo{
		host: host,
		portConfigs: map[string]*provisioner.PortConfig{
			"00:11:22:33:44:55": {SwitchPortConfig: metal3api.SwitchPortConfig{Mode: "access", NativeVLAN: 100}},
		},
	}

	prov := &networkTestProvisioner{ensurePortsErr: errors.New("ironic connection refused")}
	r := &BareMetalHostReconciler{
		Client: fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(host).Build(),
	}

	dirty, err := r.applyPortConfigs(context.Background(), prov, host, info)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to apply switch port configuration")
	assert.Contains(t, err.Error(), "ironic connection refused")
	assert.False(t, dirty)
	assert.Nil(t, host.Status.AppliedPortConfigs)
}

func TestManagePortConfigsValidationFails(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = metal3api.AddToScheme(scheme)

	host := &metal3api.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{Name: "test-host", Namespace: "test-ns"},
		Spec: metal3api.BareMetalHostSpec{
			NetworkInterfaces: []metal3api.NetworkInterface{
				{Name: "invalid-nic"},
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

	info := &reconcileInfo{
		host: host,
		log:  ctrl.Log.WithName("test"),
	}

	prov := &networkTestProvisioner{}
	r := &BareMetalHostReconciler{
		Client: fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(host).Build(),
	}

	result := r.managePortConfigs(context.Background(), prov, info)
	_, isUpdate := result.(actionUpdate)
	assert.True(t, isUpdate, "expected actionUpdate when validation sets condition")
	assert.Equal(t, 0, prov.ensurePortsCalls, "provisioner should not be called when validation fails")
}

func TestManagePortConfigsAppliesConfigs(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = metal3api.AddToScheme(scheme)

	host := &metal3api.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{Name: "test-host", Namespace: "test-ns"},
		Spec: metal3api.BareMetalHostSpec{
			NetworkInterfaces: []metal3api.NetworkInterface{
				{
					Name:                  "eth0",
					HostNetworkAttachment: metal3api.HostNetworkAttachmentRef{Name: "my-hna"},
				},
			},
		},
		Status: metal3api.BareMetalHostStatus{
			HardwareDetails: &metal3api.HardwareDetails{
				NIC: []metal3api.NIC{
					{Name: "eth0", MAC: "00:11:22:33:44:55"},
				},
			},
			Conditions: []metav1.Condition{
				{Type: metal3api.NetworkInterfacesValidCondition, Status: metav1.ConditionTrue, Reason: "AllInterfacesValid"},
			},
		},
	}

	hna := &metal3api.HostNetworkAttachment{
		ObjectMeta: metav1.ObjectMeta{Name: "my-hna", Namespace: "test-ns"},
		Spec: metal3api.HostNetworkAttachmentSpec{
			Mode:       metal3api.SwitchportModeAccess,
			NativeVLAN: 200,
		},
	}

	info := &reconcileInfo{
		host: host,
		log:  ctrl.Log.WithName("test"),
		hardwareData: &metal3api.HardwareData{
			Spec: metal3api.HardwareDataSpec{HardwareDetails: host.Status.HardwareDetails},
		},
		portConfigs: map[string]*provisioner.PortConfig{
			"00:11:22:33:44:55": {SwitchPortConfig: metal3api.SwitchPortConfig{Mode: "access", NativeVLAN: 200}},
		},
	}

	prov := &networkTestProvisioner{}
	r := &BareMetalHostReconciler{
		Client: fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(host, hna).Build(),
	}

	result := r.managePortConfigs(context.Background(), prov, info)
	_, isUpdate := result.(actionUpdate)
	assert.True(t, isUpdate, "expected actionUpdate after applying configs")
	assert.Equal(t, 1, prov.ensurePortsCalls)
	require.Len(t, host.Status.AppliedPortConfigs, 1)
}

func TestManagePortConfigsNoChanges(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = metal3api.AddToScheme(scheme)

	host := &metal3api.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{Name: "test-host", Namespace: "test-ns"},
		Spec: metal3api.BareMetalHostSpec{
			NetworkInterfaces: []metal3api.NetworkInterface{
				{
					Name:                  "eth0",
					HostNetworkAttachment: metal3api.HostNetworkAttachmentRef{Name: "my-hna"},
				},
			},
		},
		Status: metal3api.BareMetalHostStatus{
			HardwareDetails: &metal3api.HardwareDetails{
				NIC: []metal3api.NIC{
					{Name: "eth0", MAC: "00:11:22:33:44:55"},
				},
			},
			Conditions: []metav1.Condition{
				{Type: metal3api.NetworkInterfacesValidCondition, Status: metav1.ConditionTrue, Reason: "AllInterfacesValid", Message: "All network interfaces and attachments are valid"},
			},
			AppliedPortConfigs: []metal3api.AppliedPortConfig{
				{Name: "eth0", SwitchPortConfig: metal3api.SwitchPortConfig{Mode: "access", NativeVLAN: 200}},
			},
		},
	}

	hna := &metal3api.HostNetworkAttachment{
		ObjectMeta: metav1.ObjectMeta{Name: "my-hna", Namespace: "test-ns"},
		Spec: metal3api.HostNetworkAttachmentSpec{
			Mode:       metal3api.SwitchportModeAccess,
			NativeVLAN: 200,
		},
	}

	info := &reconcileInfo{
		host: host,
		log:  ctrl.Log.WithName("test"),
		hardwareData: &metal3api.HardwareData{
			Spec: metal3api.HardwareDataSpec{HardwareDetails: host.Status.HardwareDetails},
		},
		portConfigs: map[string]*provisioner.PortConfig{
			"00:11:22:33:44:55": {SwitchPortConfig: metal3api.SwitchPortConfig{Mode: "access", NativeVLAN: 200}},
		},
	}

	prov := &networkTestProvisioner{}
	r := &BareMetalHostReconciler{
		Client: fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(host, hna).Build(),
	}

	result := r.managePortConfigs(context.Background(), prov, info)
	_, isContinue := result.(actionContinue)
	assert.True(t, isContinue, "expected actionContinue when nothing changed")
	assert.Equal(t, 0, prov.ensurePortsCalls)
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
