package testserver

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/gophercloud/gophercloud/openstack/baremetal/v1/nodes"
)

// IronicMock is a test server that implements Ironic's semantics
type IronicMock struct {
	*MockServer
}

// NewIronic builds an ironic mock server
func NewIronic(t *testing.T) *IronicMock {
	return &IronicMock{
		New(t, "ironic"),
	}
}

// Endpoint returns the URL for accessing the server
func (m *IronicMock) Endpoint() string {
	if m == nil {
		return "https://ironic.test/v1/"
	}
	return m.MockServer.Endpoint()
}

// Ready configures the server with a valid response for /v1
func (m *IronicMock) Ready() *IronicMock {
	m.Response("/v1", "{}")
	return m
}

// NotReady configures the server with an error response for /v1
func (m *IronicMock) NotReady(errorCode int) *IronicMock {
	m.ErrorResponse("/v1", errorCode)
	return m
}

// WithDrivers configures the server so /v1/drivers returns a valid value
func (m *IronicMock) WithDrivers() *IronicMock {
	m.Response("/v1/drivers", `
	{
		"drivers": [{
			"hosts": [
			  "master-2.ostest.test.metalkube.org"
			],
			"links": [
			  {
				"href": "http://[fd00:1101::3]:6385/v1/drivers/fake-hardware",
				"rel": "self"
			  },
			  {
				"href": "http://[fd00:1101::3]:6385/drivers/fake-hardware",
				"rel": "bookmark"
			  }
			],
			"name": "fake-hardware"
		}]
	}
	`)
	return m
}

// WithNode configures the server with a valid response for /v1/nodes
func (m *IronicMock) WithNode(node nodes.Node) *IronicMock {
	m.ResponseJSON("/v1/nodes/"+node.UUID, node)
	return m
}

// WithNodeStatesProvision configures the server with a valid response for /v1/nodes/<node>/states/provision
func (m *IronicMock) WithNodeStatesProvision(nodeUUID string) *IronicMock {
	m.ResponseWithCode("/v1/nodes/"+nodeUUID+"/states/provision", "{}", http.StatusAccepted)
	return m
}

// NoNode configures the server so /v1/nodes/name returns a 404
func (m *IronicMock) NoNode(name string) *IronicMock {
	m.NotFound(fmt.Sprintf("/v1/nodes/%s", name))
	return m
}
