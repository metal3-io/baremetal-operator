package ironic

import (
	"context"
	"testing"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/hardwareutils/bmc"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/clients"
	// We don't use this package directly here, but need it imported
	// so it registers its test fixture with the other BMC access
	// types.
	_ "github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/testbmc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	logz "sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func init() {
	logf.SetLogger(logz.New(logz.UseDevMode(true)))
}

func TestEnsurePortsNetworkingDisabled(t *testing.T) {
	host := makeHost()
	publisher := func(reason, message string) {}
	prov, err := newProvisionerWithSettings(host, bmc.Credentials{}, publisher, "http://ironic.test", clients.AuthConfig{Type: clients.NoAuth})
	require.NoError(t, err)

	// Override settings: networking disabled but nodeID is set
	prov.config.enableNetworking = false
	prov.nodeID = "test-node-id"

	// EnsurePorts should return nil immediately without making any HTTP calls.
	// If it tried to contact an ironic server, it would fail because the test
	// server URL is not real.
	err = prov.EnsurePorts(context.Background())
	assert.NoError(t, err)
}

func TestSwitchPortConfigsEqual(t *testing.T) {
	mtu9000 := 9000

	tests := []struct {
		name     string
		existing any
		new      *metal3api.SwitchPortConfig
		want     bool
		wantErr  bool
	}{
		{
			name:     "nil existing",
			existing: nil,
			new: &metal3api.SwitchPortConfig{
				Mode: "trunk",
			},
			wantErr: true,
		},
		{
			name:     "wrong type",
			existing: "not a map",
			new: &metal3api.SwitchPortConfig{
				Mode: "trunk",
			},
			wantErr: true,
		},
		{
			name: "equal basic config",
			existing: map[string]any{
				"mode": "trunk",
			},
			new: &metal3api.SwitchPortConfig{
				Mode: "trunk",
			},
			want: true,
		},
		{
			name: "different mode",
			existing: map[string]any{
				"mode": "access",
			},
			new: &metal3api.SwitchPortConfig{
				Mode: "trunk",
			},
			want: false,
		},
		{
			name: "equal with native vlan",
			existing: map[string]any{
				"mode":        "trunk",
				"native_vlan": float64(100), // JSON unmarshals to float64
			},
			new: &metal3api.SwitchPortConfig{
				Mode:       "trunk",
				NativeVLAN: 100,
			},
			want: true,
		},
		{
			name: "different native vlan",
			existing: map[string]any{
				"mode":        "trunk",
				"native_vlan": float64(100),
			},
			new: &metal3api.SwitchPortConfig{
				Mode:       "trunk",
				NativeVLAN: 200,
			},
			want: false,
		},
		{
			name: "equal with allowed vlans",
			existing: map[string]any{
				"mode":          "trunk",
				"allowed_vlans": []any{float64(100), float64(200), float64(300)},
			},
			new: &metal3api.SwitchPortConfig{
				Mode:         "trunk",
				AllowedVLANs: []int{100, 200, 300},
			},
			want: true,
		},
		{
			name: "different allowed vlans order",
			existing: map[string]any{
				"mode":          "trunk",
				"allowed_vlans": []any{float64(100), float64(200), float64(300)},
			},
			new: &metal3api.SwitchPortConfig{
				Mode:         "trunk",
				AllowedVLANs: []int{100, 300, 200}, // different order
			},
			want: false,
		},
		{
			name: "equal with mtu",
			existing: map[string]any{
				"mode": "trunk",
				"mtu":  float64(9000),
			},
			new: &metal3api.SwitchPortConfig{
				Mode: "trunk",
				MTU:  &mtu9000,
			},
			want: true,
		},
		{
			name: "different mtu",
			existing: map[string]any{
				"mode": "trunk",
				"mtu":  float64(1500),
			},
			new: &metal3api.SwitchPortConfig{
				Mode: "trunk",
				MTU:  &mtu9000,
			},
			want: false,
		},
		{
			name: "complete config equal",
			existing: map[string]any{
				"mode":          "trunk",
				"native_vlan":   float64(100),
				"allowed_vlans": []any{float64(100), float64(200), float64(300)},
				"mtu":           float64(9000),
			},
			new: &metal3api.SwitchPortConfig{
				Mode:         "trunk",
				NativeVLAN:   100,
				AllowedVLANs: []int{100, 200, 300},
				MTU:          &mtu9000,
			},
			want: true,
		},
		{
			name: "new has mtu nil, existing has no mtu",
			existing: map[string]any{
				"mode": "trunk",
			},
			new: &metal3api.SwitchPortConfig{
				Mode: "trunk",
				MTU:  nil,
			},
			want: true,
		},
		{
			name: "new has mtu nil, existing has mtu",
			existing: map[string]any{
				"mode": "trunk",
				"mtu":  float64(1500),
			},
			new: &metal3api.SwitchPortConfig{
				Mode: "trunk",
				MTU:  nil,
			},
			want: false,
		},
		{
			name: "new has empty allowed_vlans, existing has none",
			existing: map[string]any{
				"mode": "trunk",
			},
			new: &metal3api.SwitchPortConfig{
				Mode:         "trunk",
				AllowedVLANs: []int{},
			},
			want: true,
		},
		{
			name: "new has empty allowed_vlans, existing has some",
			existing: map[string]any{
				"mode":          "trunk",
				"allowed_vlans": []any{float64(100)},
			},
			new: &metal3api.SwitchPortConfig{
				Mode:         "trunk",
				AllowedVLANs: []int{},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := switchPortConfigsEqual(tt.existing, tt.new)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBuildLocalLinkFromNIC(t *testing.T) {
	tests := []struct {
		name string
		nic  metal3api.NIC
		want map[string]interface{}
	}{
		{
			name: "nil LLDP",
			nic:  metal3api.NIC{MAC: "aa:bb:cc:dd:ee:ff"},
			want: nil,
		},
		{
			name: "empty LLDP",
			nic:  metal3api.NIC{MAC: "aa:bb:cc:dd:ee:ff", LLDP: &metal3api.LLDP{}},
			want: nil,
		},
		{
			name: "full LLDP",
			nic: metal3api.NIC{
				MAC: "aa:bb:cc:dd:ee:ff",
				LLDP: &metal3api.LLDP{
					SwitchID:         "00:11:22:33:44:55",
					PortID:           "Eth1/1",
					SwitchSystemName: "switch-01",
				},
			},
			want: map[string]interface{}{
				"switch_id":   "00:11:22:33:44:55",
				"port_id":     "Eth1/1",
				"switch_info": "switch-01",
			},
		},
		{
			name: "partial LLDP - switch_id only",
			nic: metal3api.NIC{
				MAC: "aa:bb:cc:dd:ee:ff",
				LLDP: &metal3api.LLDP{
					SwitchID: "00:11:22:33:44:55",
				},
			},
			want: map[string]interface{}{
				"switch_id": "00:11:22:33:44:55",
			},
		},
		{
			name: "partial LLDP - port_id and switch_info",
			nic: metal3api.NIC{
				MAC: "aa:bb:cc:dd:ee:ff",
				LLDP: &metal3api.LLDP{
					PortID:           "Eth1/1",
					SwitchSystemName: "switch-01",
				},
			},
			want: map[string]interface{}{
				"port_id":     "Eth1/1",
				"switch_info": "switch-01",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildLocalLinkFromNIC(tt.nic)
			assert.Equal(t, tt.want, got)
		})
	}
}
