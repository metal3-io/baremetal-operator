package testserver

import "testing"

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
	m.Response("/v1", "{}")
	return m
}

// NotReady configures the server with an error response for /v1
func (m *InspectorMock) NotReady(errorCode int) *InspectorMock {
	m.ErrorResponse("/v1", errorCode)
	return m
}
