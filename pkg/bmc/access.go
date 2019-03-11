package bmc

import (
	"net"
	"net/url"
	"strings"

	"github.com/pkg/errors"
)

// AccessDetails contains the information about how to get to a BMC.
//
// NOTE(dhellmann): This structure is very likely to change as we
// adapt it to additional types.
type AccessDetails struct {
	// The type of the BMC, indicating the driver that will be used to
	// communicate with it.
	Type string

	portNum  string
	hostname string
}

const ipmiDefaultPort = "623"

// NewAccessDetails creates an AccessDetails structure from the URL
// for a BMC.
func NewAccessDetails(address string) (*AccessDetails, error) {
	var addr AccessDetails

	// Start by assuming "type://host:port"
	parsedURL, err := url.Parse(address)
	if err != nil {
		// We failed to parse the URL, but it may just be a host or
		// host:port string (which the URL parser rejects because ":"
		// is not allowed in the first segment of a
		// path. Unfortunately there is no error class to represent
		// that specific error, so we have to guess.
		if strings.Contains(address, ":") {
			// If we can parse host:port, carry on with those
			// values. Otherwise, report the original parser error.
			host, port, err2 := net.SplitHostPort(address)
			if err2 != nil {
				return nil, errors.Wrap(err, "failed to parse BMC address information")
			}
			addr.Type = "ipmi"
			addr.hostname = host
			addr.portNum = port
		} else {
			addr.Type = "ipmi"
			addr.hostname = address
		}
	} else {
		// Successfully parsed the URL
		addr.Type = parsedURL.Scheme
		addr.portNum = parsedURL.Port()
		addr.hostname = parsedURL.Hostname()
		if parsedURL.Scheme == "" {
			addr.Type = "ipmi"
			if addr.hostname == "" {
				// If there was no scheme at all, the hostname was
				// interpreted as a path.
				addr.hostname = parsedURL.Path
			}
		}
	}

	if addr.Type == "ipmi" && addr.portNum == "" {
		addr.portNum = ipmiDefaultPort
	}

	return &addr, nil
}

// NeedsMAC returns true when the host is going to need a separate
// port created rather than having it discovered.
//
// libvirt-based hosts used for dev and testing require a MAC address,
// specified as part of the host, but we don't want the provisioner to
// have to know the rules about which drivers require what so we hide
// that detail inside this class and just let the provisioner know
// that "some" drivers require a MAC and it should ask.
func (a *AccessDetails) NeedsMAC() bool {
	return a.Type == "libvirt"
}

// DriverInfo returns a data structure to pass as the DriverInfo
// parameter when creating a node in Ironic. The structure is
// pre-populated with the access information, and the caller is
// expected to add any other information that might be needed (such as
// the kernel and ramdisk locations).
func (a AccessDetails) DriverInfo(bmcCreds Credentials) map[string]interface{} {
	return map[string]interface{}{
		"ipmi_port":     a.portNum,
		"ipmi_username": bmcCreds.Username,
		"ipmi_password": bmcCreds.Password,
		"ipmi_address":  a.hostname,
	}
}
