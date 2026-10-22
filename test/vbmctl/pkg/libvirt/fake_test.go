//go:build vbmctl
// +build vbmctl

package libvirt

import (
	"errors"
	"slices"
	"testing"

	"libvirt.org/go/libvirt"
)

// TestMain initializes libvirt before the tests run. The fake connection
// returns empty handles, and freeing one makes libvirt record an error, which
// needs an initialized library. Opening a connection would initialize it, but
// the tests never open one. The error is ignored: if libvirt cannot be loaded,
// freeing a handle fails before it reaches libvirt.
func TestMain(m *testing.M) {
	_, _ = libvirt.GetVersion()
	m.Run()
}

// errFakeDefine is returned when the fake connection is asked to define a
// network or a domain. The tests only check whether that step was reached.
var errFakeDefine = errors.New("fake connection cannot define resources")

// fakeConnection is a libvirt connection for unit tests. The networks and
// domains it lists exist. Looking up any other name fails with lookupErr, or
// with a "not found" error if lookupErr is nil.
type fakeConnection struct {
	networks  []string
	domains   []string
	lookupErr error

	// defined counts the calls to NetworkDefineXML and DomainDefineXML.
	defined int
}

// LookupNetworkByName returns an empty handle for an existing network.
// Freeing it returns an error, which the callers ignore.
func (f *fakeConnection) LookupNetworkByName(name string) (*libvirt.Network, error) {
	if slices.Contains(f.networks, name) {
		return &libvirt.Network{}, nil
	}
	if f.lookupErr != nil {
		return nil, f.lookupErr
	}
	return nil, libvirt.Error{Code: libvirt.ERR_NO_NETWORK, Domain: libvirt.FROM_NETWORK}
}

func (f *fakeConnection) NetworkDefineXML(_ string) (*libvirt.Network, error) {
	f.defined++
	return nil, errFakeDefine
}

// LookupDomainByName returns an empty handle for an existing domain.
// Freeing it returns an error, which the callers ignore.
func (f *fakeConnection) LookupDomainByName(name string) (*libvirt.Domain, error) {
	if slices.Contains(f.domains, name) {
		return &libvirt.Domain{}, nil
	}
	if f.lookupErr != nil {
		return nil, f.lookupErr
	}
	return nil, libvirt.Error{Code: libvirt.ERR_NO_DOMAIN, Domain: libvirt.FROM_DOMAIN}
}

func (f *fakeConnection) DomainDefineXML(_ string) (*libvirt.Domain, error) {
	f.defined++
	return nil, errFakeDefine
}

func (f *fakeConnection) ListAllDomains(_ libvirt.ConnectListAllDomainsFlags) ([]libvirt.Domain, error) {
	return nil, nil
}
