package provisioner

import (
	"context"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
)

// SerialConsoleProvisioner is an optional provisioner capability. Provisioners
// that cannot manage consoles need not implement it or accept cleanup work.
type SerialConsoleProvisioner interface {
	// SetSerialConsole requests an asynchronous enable or disable operation.
	SetSerialConsole(ctx context.Context, enabled bool) error
	// GetSerialConsole returns the endpoint when the console is enabled. This
	// call can interrupt an active session and must not be used for polling.
	GetSerialConsole(ctx context.Context) (*metal3api.SerialConsoleEndpoint, error)
	// GetSerialConsoleState observes console state without accessing the
	// potentially disruptive endpoint API.
	GetSerialConsoleState(ctx context.Context) (SerialConsoleState, error)
}

// SerialConsoleState is a non-disruptive observation of backend console state.
type SerialConsoleState struct {
	Enabled bool
	Busy    bool
	// ID changes when the backend node, conductor or allocated port changes.
	ID string
}
