// SPDX-License-Identifier: Apache-2.0

// What crosses layer boundaries, a resolved host, the verdict a host reports
// about its own install, and the MAC normalizer both sides need.

package core

import (
	"net"
	"strings"
)

// HostRef identifies a BareMetalHost resolved from a request. It carries the
// preprovisioning Secret name because that is where the kickstart lives.
type HostRef struct {
	Name      string
	Namespace string
	UID       string
	BootMAC   string
	// KickstartSecret is spec.preprovisioningNetworkDataName, empty when the
	// host names none and can only be served the fallback.
	KickstartSecret string
	// InstallDisk is spec.rootDeviceHints rendered as a kickstart device spec,
	// empty when the host hints nothing kickstart can name.
	InstallDisk string
}

// InstallReport is the verdict a host posted, which is all that is kept. The
// body itself is read and thrown away, only the outcome drives anything.
type InstallReport struct {
	Message   string
	Succeeded bool
}

// KickstartSecretKey has to be a key BMO accepts, it reads networkData falling
// back to value and takes a finalizer on the Secret either way.
const KickstartSecretKey = "value"

// NormalizeMAC returns the canonical lowercase form of a MAC address, or empty when
// it does not parse, so colon, hyphen and dotted notations all compare equal.
func NormalizeMAC(s string) string {
	hw, err := net.ParseMAC(strings.TrimSpace(s))
	if err != nil {
		return ""
	}

	return strings.ToLower(hw.String())
}
