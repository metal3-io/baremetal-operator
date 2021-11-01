package testserver

import (
	"net/http"
	"testing"

	"github.com/gophercloud/gophercloud/openstack/baremetalintrospection/v1/introspection"
)

// InspectorMock is a test server that implements Ironic Inspector's semantics
type InspectorMock struct {
	*MockServer
}

// NewInspector builds a new inspector mock server
func NewInspector(t *testing.T) *InspectorMock {
	return &InspectorMock{
		New(t, "inspector"),
	}
}

// Endpoint returns the URL to the server
func (m *InspectorMock) Endpoint() string {
	if m == nil {
		return "https://inspector.test/v1/"
	}
	return m.MockServer.Endpoint()
}

// Ready configures the server with a valid response for /v1
func (m *InspectorMock) Ready() *InspectorMock {
	m.ResponseWithCode("/v1", "{}", http.StatusOK)
	return m
}

// NotReady configures the server with an error response for /v1
func (m *InspectorMock) NotReady(errorCode int) *InspectorMock {
	m.ErrorResponse("/v1", errorCode)
	return m
}

// WithIntrospection configures the server with a valid response for /v1/introspection/<node>
func (m *InspectorMock) WithIntrospection(nodeUUID string, status introspection.Introspection) *InspectorMock {
	m.ResponseJSON("/v1/introspection/"+nodeUUID, status)
	return m
}

// WithIntrospectionFailed configures the server with an error response for /v1/introspection/<node>
func (m *InspectorMock) WithIntrospectionFailed(nodeUUID string, errorCode int) *InspectorMock {
	m.ErrorResponse("/v1/introspection/"+nodeUUID, errorCode)
	return m
}

// WithIntrospectionData configures the server with a valid response for /v1/introspection/<node>/data
func (m *InspectorMock) WithIntrospectionData(nodeUUID string, data introspection.Data) *InspectorMock {
	m.ResponseJSON("/v1/introspection/"+nodeUUID+"/data", data)
	return m
}

// WithIntrospectionDataFailed configures the server with an error response for /v1/introspection/<node>/data
func (m *InspectorMock) WithIntrospectionDataFailed(nodeUUID string, errorCode int) *InspectorMock {
	m.ErrorResponse("/v1/introspection/"+nodeUUID+"/data", errorCode)
	return m
}
