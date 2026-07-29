package ironic

import (
	"fmt"
	"reflect"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
)

// buildLocalLinkFromConfig builds a local_link_connection map from manual
// switch port identity. Ironic requires both switch_id and port_id when
// local_link_connection is set, so this returns nil if either is missing.
func buildLocalLinkFromConfig(config *metal3api.SwitchPortIdentifier) map[string]any {
	if config.SwitchID == "" || config.PortID == "" {
		return nil
	}
	llc := map[string]any{
		"switch_id": config.SwitchID,
		"port_id":   config.PortID,
	}
	if config.SwitchSystemName != "" {
		llc["switch_info"] = config.SwitchSystemName
	}
	return llc
}

// buildLocalLinkFromNIC creates a local_link_connection map from stored LLDP
// data. Ironic requires both switch_id and port_id when local_link_connection
// is set, so this returns nil if either is missing.
func buildLocalLinkFromNIC(nic metal3api.NIC) map[string]interface{} {
	if nic.LLDP == nil || nic.LLDP.SwitchID == "" || nic.LLDP.PortID == "" {
		return nil
	}

	connection := map[string]interface{}{
		"switch_id": nic.LLDP.SwitchID,
		"port_id":   nic.LLDP.PortID,
	}

	if nic.LLDP.SwitchSystemName != "" {
		connection["switch_info"] = nic.LLDP.SwitchSystemName
	}

	return connection
}

// buildSwitchPortFromConfig builds a map with snake_case keys suitable for
// storing in Ironic's port extra field, independent of the struct's JSON tags.
func buildSwitchPortFromConfig(config *metal3api.SwitchPortConfig) map[string]any {
	m := map[string]any{
		"mode":        string(config.Mode),
		"native_vlan": config.NativeVLAN,
	}
	if len(config.AllowedVLANs) > 0 {
		m["allowed_vlans"] = config.AllowedVLANs
	}
	if config.MTU != nil {
		m["mtu"] = *config.MTU
	}
	return m
}

// parseSwitchPortConfig converts a switchport config from Ironic (map[string]any)
// into a SwitchPortConfig struct. Fields are extracted using the snake_case keys
// that Ironic stores, rather than depending on the struct's JSON tags.
func parseSwitchPortConfig(raw any) (*metal3api.SwitchPortConfig, error) {
	m, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("expected map[string]any, got %T", raw)
	}

	config := &metal3api.SwitchPortConfig{}

	if v, ok := m["mode"]; ok {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("expected string for mode, got %T", v)
		}
		config.Mode = metal3api.SwitchPortMode(s)
	}

	if v, ok := m["native_vlan"]; ok {
		n, ok := v.(float64)
		if !ok {
			return nil, fmt.Errorf("expected number for native_vlan, got %T", v)
		}
		config.NativeVLAN = int(n)
	}

	if v, ok := m["allowed_vlans"]; ok {
		vals, ok := v.([]any)
		if !ok {
			return nil, fmt.Errorf("expected []any for allowed_vlans, got %T", v)
		}
		for i, item := range vals {
			n, ok := item.(float64)
			if !ok {
				return nil, fmt.Errorf("expected number for allowed_vlans[%d], got %T", i, item)
			}
			config.AllowedVLANs = append(config.AllowedVLANs, int(n))
		}
	}

	if v, ok := m["mtu"]; ok {
		n, ok := v.(float64)
		if !ok {
			return nil, fmt.Errorf("expected number for mtu, got %T", v)
		}
		mtu := int(n)
		config.MTU = &mtu
	}

	return config, nil
}

// switchPortConfigsEqual compares a switchport config from Ironic (map[string]any)
// with a new SwitchPortConfig struct, handling JSON type conversions.
func switchPortConfigsEqual(existing any, desired *metal3api.SwitchPortConfig) (bool, error) {
	parsed, err := parseSwitchPortConfig(existing)
	if err != nil {
		return false, err
	}

	// Normalize nil vs empty slice for comparison without mutating inputs
	normalizedParsed := *parsed
	if len(normalizedParsed.AllowedVLANs) == 0 {
		normalizedParsed.AllowedVLANs = nil
	}
	normalizedDesired := *desired
	if len(normalizedDesired.AllowedVLANs) == 0 {
		normalizedDesired.AllowedVLANs = nil
	}

	return reflect.DeepEqual(normalizedParsed, normalizedDesired), nil
}
