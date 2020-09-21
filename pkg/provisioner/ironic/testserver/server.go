package testserver

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// New returns a MockServer
func New(t *testing.T, name string) *MockServer {
	mux := http.NewServeMux()
	return &MockServer{
		t:    t,
		name: name,
		mux:  mux,
	}
}

// MockServer is a simple http testing server
type MockServer struct {
	t            *testing.T
	mux          *http.ServeMux
	name         string
	Requests     string
	FullRequests []*http.Request
	server       *httptest.Server
	errorCode    int
}

// Endpoint returns the URL to the server
func (m *MockServer) Endpoint() string {
	if m == nil || m.server == nil {
		// The consumer of this method expects something valid, but
		// won't use it if m is nil.
		return "https://ironic.test/v1/"
	}
	response := m.server.URL + "/v1/"
	m.t.Logf("%s: endpoint: %s", m.name, response)
	return response
}

func (m *MockServer) logRequest(r *http.Request, response string) {
	m.t.Logf("%s: %s %s -> %s", m.name, r.Method, r.URL, response)
	m.Requests += r.RequestURI + ";"
	m.FullRequests = append(m.FullRequests, r)
}

func (m *MockServer) handleNoResponse(w http.ResponseWriter, r *http.Request) {
	if m.errorCode != 0 {
		http.Error(w, "An error", m.errorCode)
		return
	}
}

// Handler attaches a generic handler function to a request URL pattern
func (m *MockServer) Handler(pattern string, handlerFunc http.HandlerFunc) *MockServer {
	m.t.Logf("%s: adding handler for %s", m.name, pattern)
	m.mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		m.logRequest(r, "(custom)")
		handlerFunc(w, r)
	})
	return m
}

// NotFound attaches a 404 error handler to a request URL pattern
func (m *MockServer) NotFound(pattern string) *MockServer {
	m.ErrorResponse(pattern, http.StatusNotFound)
	return m
}

// Response attaches a handler function that returns the given payload
// from requests to the URL pattern
func (m *MockServer) Response(pattern string, payload string) *MockServer {
	m.t.Logf("%s: adding response handler for %s", m.name, pattern)
	m.mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		m.logRequest(r, payload)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, payload)
	})
	return m
}

// ErrorResponse attaches a handler function that returns the given
// error code from requests to the URL pattern
func (m *MockServer) ErrorResponse(pattern string, errorCode int) *MockServer {
	m.t.Logf("%s: adding error response handler for %s", m.name, pattern)
	m.mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		m.logRequest(r, fmt.Sprintf("%d", errorCode))
		http.Error(w, "An error", errorCode)
	})
	return m
}

// Start runs the server
func (m *MockServer) Start() *MockServer {
	m.server = httptest.NewServer(m.mux)
	//catch all handler
	m.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		m.logRequest(r, "")
	})
	return m
}

// Stop closes the server down
func (m *MockServer) Stop() {
	m.server.Close()
}
