// SPDX-License-Identifier: Apache-2.0

// BMC address handling, deferring to BMO's own parser so the addresses this
// accepts cannot drift from the ones a BareMetalHost is allowed to carry.

package redfish

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/metal3-io/baremetal-operator/pkg/hardwareutils/bmc"
)

// DriverInfo keys every Redfish backed driver fills in. The system id is the
// path from the BMC address, set only when the address names a system.
const (
	RedfishAddressKey  = "redfish_address"
	RedfishSystemIDKey = "redfish_system_id"
)

// SupportedBMCSchemes are the only accepted BMC addresses. Deploying means
// inserting virtual media, and vendor BMCs answer redfish-virtualmedia anyway.
var SupportedBMCSchemes = []string{
	"redfish-virtualmedia",
	"redfish-virtualmedia+http",
	"redfish-virtualmedia+https",
}

type Address struct {
	// Endpoint is the Redfish service root, scheme and authority with no path.
	Endpoint string
	// SystemID is the @odata.id of the system the address names, empty when it
	// names none and the service's only system should be used.
	SystemID string
}

// UnsupportedSchemeError explains a rejection in terms of the address the
// operator wrote, not the driver BMO picked for it.
func UnsupportedSchemeError(address, bmcType string) error {
	// A schemeless address is parsed as ipmi by BMO, so naming only ipmi would
	// baffle anyone who wrote a bare host and port.
	if !strings.Contains(address, "://") {
		return fmt.Errorf("BMC address %q has no scheme, use redfish-virtualmedia://%s", address, address)
	}

	return fmt.Errorf("BMC address %q uses %q, which this provisioner does not support, use one of %s",
		address, bmcType, strings.Join(SupportedBMCSchemes, ", "))
}

func ParseRedfishAddress(address string) (Address, error) {
	if address == "" {
		return Address{}, errors.New("BMC address is empty, a redfish-virtualmedia address is required")
	}

	// The flag only sets a redfish_verify_ca key this ignores, BMC certificates
	// are never verified whatever the host asks for.
	access, err := bmc.NewAccessDetails(address, true)
	if err != nil {
		return Address{}, fmt.Errorf("BMC address %q: %w", address, err)
	}

	if !slices.Contains(SupportedBMCSchemes, access.Type()) {
		return Address{}, UnsupportedSchemeError(address, access.Type())
	}

	// Credentials are only needed for the fields this ignores.
	info := access.DriverInfo(bmc.Credentials{})

	endpoint, _ := info[RedfishAddressKey].(string)
	if endpoint == "" {
		return Address{}, fmt.Errorf("BMC address %q resolved to no Redfish endpoint", address)
	}

	systemID, _ := info[RedfishSystemIDKey].(string)

	return Address{Endpoint: endpoint, SystemID: systemID}, nil
}

// UsableBMCAddress reports whether an address resolves, without failing.
// Teardown must not refuse a broken address or the finalizer stays forever.
func UsableBMCAddress(address string) bool {
	_, err := ParseRedfishAddress(address)

	return err == nil
}
