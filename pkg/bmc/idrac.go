package bmc

import (
	"fmt"
	"net/url"
	"strings"
)

type iDracAccessDetails struct {
	bmcType  string
	portNum  string
	hostname string
	path     string
}

func (a *iDracAccessDetails) Type() string {
	return a.bmcType
}

// NeedsMAC returns true when the host is going to need a separate
// port created rather than having it discovered.
func (a *iDracAccessDetails) NeedsMAC() bool {
	return false
}

func (a *iDracAccessDetails) Driver() string {
	return "idrac"
}

// DriverInfo returns a data structure to pass as the DriverInfo
// parameter when creating a node in Ironic. The structure is
// pre-populated with the access information, and the caller is
// expected to add any other information that might be needed (such as
// the kernel and ramdisk locations).
func (a *iDracAccessDetails) DriverInfo(bmcCreds Credentials) map[string]interface{} {
	host := a.hostname
	if strings.ContainsRune(host, ':') {
		// Hostname is an IPv6 address
		host = fmt.Sprintf("[%s]", host)
	}
	if a.portNum != "" {
		host = fmt.Sprintf("%s:%s", host, a.portNum)
	}

	address := url.URL{
		Scheme: "http",
		Host:   host,
		Path:   a.path,
	}

	return map[string]interface{}{
		"drac_username": bmcCreds.Username,
		"drac_password": bmcCreds.Password,
		"drac_address":  address.String(),
	}
}
