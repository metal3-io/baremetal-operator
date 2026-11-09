//go:build vbmctl
// +build vbmctl

package libvirt

import (
	"context"
	"errors"
	"testing"

	vbmctlapi "github.com/metal3-io/baremetal-operator/test/vbmctl/pkg/api"
	"libvirt.org/go/libvirt"
)

// newTestVMManager returns a VMManager without a pool manager. The tests
// using it cover only what happens before the storage pool is used.
func newTestVMManager(t *testing.T, conn *fakeConnection) *VMManager {
	t.Helper()
	renderer, err := NewTemplateRenderer()
	if err != nil {
		t.Fatalf("failed to create template renderer: %v", err)
	}
	return &VMManager{conn: conn, renderer: renderer, opts: DefaultVMManagerOptions()}
}

func TestCreateVM(t *testing.T) {
	lookupErr := libvirt.Error{Code: libvirt.ERR_INTERNAL_ERROR, Domain: libvirt.FROM_DOMAIN, Message: "lookup failed"}

	tests := []struct {
		name    string
		conn    *fakeConnection
		wantErr error
		wantMsg string
	}{
		{
			name:    "fails if the VM exists",
			conn:    &fakeConnection{domains: []string{"bmo-e2e-0"}},
			wantErr: ErrAlreadyExists,
			wantMsg: "VM bmo-e2e-0 already exists",
		},
		{
			name:    "returns the lookup error",
			conn:    &fakeConnection{lookupErr: lookupErr},
			wantErr: lookupErr,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestVMManager(t, tt.conn)
			_, err := m.Create(context.Background(), vbmctlapi.VMConfig{Name: "bmo-e2e-0"})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected error wrapping %v, got: %v", tt.wantErr, err)
			}
			if tt.wantMsg != "" && err.Error() != tt.wantMsg {
				t.Errorf("expected error %q, got %q", tt.wantMsg, err.Error())
			}
			if tt.conn.defined != 0 {
				t.Errorf("expected no define calls, got %d", tt.conn.defined)
			}
		})
	}
}
