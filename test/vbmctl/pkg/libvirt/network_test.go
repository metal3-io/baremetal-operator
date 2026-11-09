//go:build vbmctl
// +build vbmctl

package libvirt

import (
	"bytes"
	"context"
	"errors"
	"log"
	"strings"
	"testing"

	vbmctlapi "github.com/metal3-io/baremetal-operator/test/vbmctl/pkg/api"
	"libvirt.org/go/libvirt"
)

func newTestNetworkManager(t *testing.T, conn *fakeConnection) *NetworkManager {
	t.Helper()
	renderer, err := NewTemplateRenderer()
	if err != nil {
		t.Fatalf("failed to create template renderer: %v", err)
	}
	return &NetworkManager{conn: conn, renderer: renderer}
}

func TestCreateNetwork(t *testing.T) {
	lookupErr := libvirt.Error{Code: libvirt.ERR_INTERNAL_ERROR, Domain: libvirt.FROM_NETWORK, Message: "lookup failed"}

	tests := []struct {
		name        string
		conn        *fakeConnection
		wantErr     error
		wantMsg     string
		wantDefined int
	}{
		{
			name:        "fails if the network exists",
			conn:        &fakeConnection{networks: []string{"baremetal-e2e"}},
			wantErr:     ErrAlreadyExists,
			wantMsg:     "network baremetal-e2e already exists",
			wantDefined: 0,
		},
		{
			name:        "returns the lookup error",
			conn:        &fakeConnection{lookupErr: lookupErr},
			wantErr:     lookupErr,
			wantDefined: 0,
		},
		{
			name:        "tries to define the network if it does not exist",
			conn:        &fakeConnection{networks: []string{"provisioning"}},
			wantErr:     errFakeDefine,
			wantDefined: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestNetworkManager(t, tt.conn)
			_, err := m.CreateNetwork(context.Background(), vbmctlapi.NetworkConfig{Name: "baremetal-e2e"})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected error wrapping %v, got: %v", tt.wantErr, err)
			}
			if tt.wantMsg != "" && err.Error() != tt.wantMsg {
				t.Errorf("expected error %q, got %q", tt.wantMsg, err.Error())
			}
			if tt.conn.defined != tt.wantDefined {
				t.Errorf("expected %d define call(s), got %d", tt.wantDefined, tt.conn.defined)
			}
		})
	}
}

func TestCreateNetworksExisting(t *testing.T) {
	conn := &fakeConnection{networks: []string{"provisioning"}}
	m := newTestNetworkManager(t, conn)

	_, err := m.CreateNetworks(context.Background(), []vbmctlapi.NetworkConfig{
		{Name: "provisioning"},
		{Name: "baremetal-e2e"},
	})
	if !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("expected error wrapping ErrAlreadyExists, got: %v", err)
	}
	want := "failed to create network provisioning: network provisioning already exists"
	if err.Error() != want {
		t.Errorf("expected error %q, got %q", want, err.Error())
	}
	if conn.defined != 0 {
		t.Errorf("expected no define calls, got %d", conn.defined)
	}
}

// TestDeleteNetworksReportsEveryFailure checks that DeleteNetworks keeps going
// after a failure, returns the errors of all networks it could not delete and
// does not print them as deleted. Destroying an empty handle from
// fakeConnection fails, so both existing networks fail to be deleted.
func TestDeleteNetworksReportsEveryFailure(t *testing.T) {
	var out bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&out)
	t.Cleanup(func() { log.SetOutput(prev) })

	conn := &fakeConnection{networks: []string{"provisioning", "external"}}
	m := newTestNetworkManager(t, conn)

	err := m.DeleteNetworks(context.Background(), []string{"provisioning", "missing", "external"})
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, name := range []string{"provisioning", "external"} {
		if !strings.Contains(err.Error(), "failed to destroy network "+name) {
			t.Errorf("expected the error to name network %s, got: %v", name, err)
		}
	}
	if strings.Contains(err.Error(), "missing") {
		t.Errorf("a network that does not exist must not be an error, got: %v", err)
	}
	if strings.Contains(out.String(), "Deleted libvirt network") {
		t.Errorf("no network was deleted, but the output says otherwise:\n%s", out.String())
	}
}
