//go:build vbmctl
// +build vbmctl

package libvirt

import (
	"errors"
	"slices"
	"testing"

	"libvirt.org/go/libvirt"
)

// TestMain initializes libvirt before the tests run.
//
// The network and VM tests free the empty *libvirt.Network and
// *libvirt.Domain handles that fakeConnection returns. libvirt rejects an
// empty handle with an error, and it can only record that error once the
// library is initialized. Opening a connection initializes it, but the tests
// never open one, so TestMain calls libvirt.GetVersion, which also
// initializes it.
//
// TestMain ignores the error from GetVersion. GetVersion fails when the
// libvirt_dlopen build tag is set and the libvirt library is not installed.
// The tests still work then: the Go bindings cannot load the library, so each
// libvirt call, such as freeing a handle, returns an error without calling
// into libvirt, and the code under test ignores the error from freeing a
// handle.
func TestMain(m *testing.M) {
	_, _ = libvirt.GetVersion()
	m.Run()
}

// errFakeDefine is returned when fakeConnection is asked to define a network
// or a domain. The tests only check whether that step was reached.
var errFakeDefine = errors.New("fake connection cannot define resources")

// fakeConnection is a networkConnection and a vmConnection for unit tests.
// The networks named in networks and the domains named in domains exist in
// the fake connection only; nothing is defined in libvirt. Looking up any
// other network or domain fails with lookupErr, or with a "not found" error
// if lookupErr is nil.
type fakeConnection struct {
	networks  []string
	domains   []string
	lookupErr error

	// defined counts the calls to NetworkDefineXML and DomainDefineXML.
	defined int
}

// LookupNetworkByName returns an empty handle for a network in networks.
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

// LookupDomainByName returns an empty handle for a domain in domains.
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
