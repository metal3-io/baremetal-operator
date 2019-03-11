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
type AccessDetails interface {
	// Type returns the kind of the BMC, indicating the driver that
	// will be used to communicate with it.
	Type() string

	// NeedsMAC returns true when the host is going to need a separate
	// port created rather than having it discovered.
	NeedsMAC() bool

	// DriverInfo returns a data structure to pass as the DriverInfo
	// parameter when creating a node in Ironic. The structure is
	// pre-populated with the access information, and the caller is
	// expected to add any other information that might be needed
	// (such as the kernel and ramdisk locations).
	DriverInfo(bmcCreds Credentials) map[string]interface{}
}

func getTypeHostPort(address string) (bmcType, host, port string, err error) {
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
			var err2 error
			host, port, err2 = net.SplitHostPort(address)
			if err2 != nil {
				return "", "", "", errors.Wrap(err, "failed to parse BMC address information")
			}
			bmcType = "ipmi"
		} else {
			bmcType = "ipmi"
			host = address
		}
	} else {
		// Successfully parsed the URL
		bmcType = parsedURL.Scheme
		port = parsedURL.Port()
		host = parsedURL.Hostname()
		if parsedURL.Scheme == "" {
			bmcType = "ipmi"
			if host == "" {
				// If there was no scheme at all, the hostname was
				// interpreted as a path.
				host = parsedURL.Path
			}
		}
	}
	return bmcType, host, port, nil
}

// NewAccessDetails creates an AccessDetails structure from the URL
// for a BMC.
func NewAccessDetails(address string) (AccessDetails, error) {

	bmcType, host, port, err := getTypeHostPort(address)
	if err != nil {
		return nil, err
	}

	addr := &ipmiAccessDetails{
		bmcType:  bmcType,
		portNum:  port,
		hostname: host,
	}

	return addr, nil
}
