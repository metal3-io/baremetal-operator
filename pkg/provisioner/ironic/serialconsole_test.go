package ironic

import (
	"encoding/json"
	"net/http"
	"sync"
	"testing"

	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/nodes"
	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/hardwareutils/bmc"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/clients"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/testserver"
	"github.com/stretchr/testify/require"
)

func TestSerialConsole(t *testing.T) {
	const nodeUUID = "33ce8659-7400-4c68-9535-d10766f07a58"

	ironic := testserver.NewIronic(t).WithNodeConsole(nodeUUID, "tcp://192.0.2.10:2023").
		Node(nodes.Node{UUID: nodeUUID, ConsoleInterface: serialConsoleInterface, ProvisionState: "active", DriverInfo: map[string]any{
			"ipmi_address": "192.0.2.20", "ipmi_username": "admin", "ipmi_password": "password", "ipmi_port": "623", "ipmi_priv_level": "ADMINISTRATOR",
		}})
	ironic.Start()
	defer ironic.Stop()

	host := makeHost()
	host.Spec.BMC.Address = "ipmi://192.0.2.20"
	host.Status.Provisioning.ID = nodeUUID
	prov, err := newProvisionerWithSettings(host, bmc.Credentials{Username: "admin", Password: "password"}, nil, ironic.Endpoint(), clients.AuthConfig{Type: clients.NoAuth})
	require.NoError(t, err)

	require.NoError(t, prov.SetSerialConsole(t.Context(), true))

	body := ironic.GetLastNodeConsoleRequestFor(nodeUUID)
	require.Equal(t, map[string]bool{"enabled": true}, body)

	console, err := prov.GetSerialConsole(t.Context())
	require.NoError(t, err)
	require.Equal(t, &metal3api.SerialConsoleEndpoint{Host: "192.0.2.10", Port: 2023}, console)
}

func TestSerialConsoleStateDoesNotReadConsoleEndpoint(t *testing.T) {
	node := nodes.Node{UUID: "node-id", ConsoleEnabled: true, Conductor: "conductor-1",
		DriverInternalInfo: map[string]any{"allocated_ipmi_terminal_port": 3023}, Reservation: "conductor-1"}
	ironic := testserver.NewIronic(t).Node(node)
	ironic.Start()
	defer ironic.Stop()
	host := makeHost()
	host.Status.Provisioning.ID = node.UUID
	prov, err := newProvisionerWithSettings(host, bmc.Credentials{}, nil, ironic.Endpoint(), clients.AuthConfig{Type: clients.NoAuth})
	require.NoError(t, err)
	state, err := prov.GetSerialConsoleState(t.Context())
	require.NoError(t, err)
	require.True(t, state.Enabled)
	require.True(t, state.Busy)
	require.Contains(t, state.ID, "node-id/conductor-1/3023")
	// There is deliberately no /states/console response configured.
}

func TestSerialConsoleEndpointValidation(t *testing.T) {
	for _, endpoint := range []string{"http://192.0.2.10:2023", "tcp://192.0.2.10", "tcp://192.0.2.10:65536", "tcp://192.0.2.10:0", "tcp://user:password@192.0.2.10:2023", "tcp://192.0.2.10:2023/path"} {
		t.Run(endpoint, func(t *testing.T) {
			ironic := testserver.NewIronic(t).WithNodeConsole("node-id", endpoint)
			ironic.Start()
			defer ironic.Stop()
			host := makeHost()
			host.Status.Provisioning.ID = "node-id"
			prov, err := newProvisionerWithSettings(host, bmc.Credentials{}, nil, ironic.Endpoint(), clients.AuthConfig{Type: clients.NoAuth})
			require.NoError(t, err)
			_, err = prov.GetSerialConsole(t.Context())
			require.Error(t, err)
		})
	}
}

func TestSerialConsoleDisableDoesNotRequireBMCCredentials(t *testing.T) {
	ironic := testserver.NewIronic(t).Node(nodes.Node{UUID: "node-id", ConsoleEnabled: true}).WithNodeConsole("node-id", "tcp://192.0.2.10:2023")
	ironic.Start()
	defer ironic.Stop()
	host := makeHost()
	host.Spec.BMC.Address = ""
	host.Status.Provisioning.ID = "node-id"
	prov, err := newProvisionerWithSettings(host, bmc.Credentials{}, nil, ironic.Endpoint(), clients.AuthConfig{Type: clients.NoAuth})
	require.NoError(t, err)
	require.NoError(t, prov.SetSerialConsole(t.Context(), false))
	require.Equal(t, map[string]bool{"enabled": false}, ironic.GetLastNodeConsoleRequestFor("node-id"))
}

func TestSerialConsoleDriverInfo(t *testing.T) {
	host := makeHost()
	host.Spec.BMC.Address = "redfish://192.0.2.10/redfish/v1/Systems/1"
	host.Spec.BMC.SerialConsoleAddress = "ipmi://192.0.2.20:6233?cipher_suite=3"
	creds := bmc.Credentials{Username: "redfish-user", Password: "redfish-password", SerialConsoleUsername: "sol-user", SerialConsolePassword: "sol-password"}
	prov, err := newProvisionerWithSettings(host, creds, nil, "http://ironic.test/v1/", clients.AuthConfig{Type: clients.NoAuth})
	require.NoError(t, err)
	info, err := prov.serialConsoleDriverInfo(t.Context())
	require.NoError(t, err)
	require.Equal(t, "192.0.2.20", info["ipmi_address"])
	require.Equal(t, "6233", info["ipmi_port"])
	require.Equal(t, "sol-user", info["ipmi_username"])
	require.Equal(t, "sol-password", info["ipmi_password"])
	require.NotContains(t, info, "ipmi_terminal_port")
	require.Equal(t, 3, info["ipmi_cipher_suite"])
}

func TestSerialConsolePreservesDriverInfoDuringRegistration(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		rotated, separateSOL bool
	}{
		{name: "rotated-management", rotated: true},
		{name: "separate-SOL", rotated: true, separateSOL: true},
		{name: "unchanged-mixed-addresses"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			host := makeHost()
			host.Spec.BMC.Address = "redfish://bmc.example/redfish/v1/Systems/1"
			node := nodes.Node{UUID: host.Status.Provisioning.ID, Name: ironicNodeName(host.ObjectMeta), ProvisionState: "active", ConsoleInterface: serialConsoleInterface,
				DriverInfo: map[string]any{"redfish_address": "https://bmc.example", "redfish_system_id": "/redfish/v1/Systems/1", "redfish_username": "redfish-user", "redfish_password": "******", "ipmi_address": "192.0.2.20", "ipmi_port": 6233, "ipmi_priv_level": "OPERATOR", "ipmi_username": "old-user", "ipmi_password": "******", "ipmi_cipher_suite": 3}}
			ironic := testserver.NewIronic(t)
			var mutex sync.Mutex
			var updates []nodes.UpdateOperation
			ironic.Handler("/v1/nodes/"+node.UUID, func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPatch {
					var operations []nodes.UpdateOperation
					if err := json.NewDecoder(r.Body).Decode(&operations); err != nil {
						t.Error(err)
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					mutex.Lock()
					updates = append(updates, operations...)
					mutex.Unlock()
				}
				if err := json.NewEncoder(w).Encode(node); err != nil {
					t.Error(err)
				}
			})
			ironic.Start()
			defer ironic.Stop()
			creds := bmc.Credentials{Username: "redfish-user", Password: "new-password"}
			wantUsername, wantPassword := creds.Username, creds.Password
			if tc.separateSOL {
				creds.SerialConsoleUsername, creds.SerialConsolePassword = "sol-user", "sol-password"
				wantUsername, wantPassword = creds.SerialConsoleUsername, creds.SerialConsolePassword
			}
			prov, err := newProvisionerWithSettings(host, creds, func(string, string) {}, ironic.Endpoint(), clients.AuthConfig{Type: clients.NoAuth})
			require.NoError(t, err)
			result, _, err := prov.Register(t.Context(), provisioner.ManagementAccessData{}, tc.rotated, false)
			require.NoError(t, err)
			require.Empty(t, result.ErrorMessage)
			mutex.Lock()
			defer mutex.Unlock()
			found := false
			for _, update := range updates {
				if update.Path != "/driver_info" {
					continue
				}
				found = true
				info, ok := update.Value.(map[string]interface{})
				require.True(t, ok)
				require.Equal(t, "192.0.2.20", info["ipmi_address"])
				require.EqualValues(t, 6233, info["ipmi_port"])
				require.EqualValues(t, 3, info["ipmi_cipher_suite"])
				require.Equal(t, wantUsername, info["ipmi_username"])
				require.Equal(t, wantPassword, info["ipmi_password"])
				require.Equal(t, creds.Password, info["redfish_password"])
			}
			require.Equal(t, tc.rotated, found, "only credential rotation should replace the complete driver_info")
		})
	}
}

func TestSerialConsoleRefusesActiveInterfaceChange(t *testing.T) {
	ironic := testserver.NewIronic(t).Node(nodes.Node{UUID: "node-id", ConsoleInterface: "no-console", ProvisionState: "active"})
	ironic.Start()
	defer ironic.Stop()
	host := makeHost()
	host.Status.Provisioning.ID = "node-id"
	prov, err := newProvisionerWithSettings(host, bmc.Credentials{}, nil, ironic.Endpoint(), clients.AuthConfig{Type: clients.NoAuth})
	require.NoError(t, err)
	require.ErrorContains(t, prov.SetSerialConsole(t.Context(), true), "before provisioning")
}

func TestSerialConsoleHandlesBackendConflicts(t *testing.T) {
	ironic := testserver.NewIronic(t).Node(nodes.Node{UUID: "node-id", ConsoleEnabled: true})
	ironic.ResponseWithCode("/v1/nodes/node-id/states/console:PUT", "{}", http.StatusConflict)
	ironic.Start()
	defer ironic.Stop()
	host := makeHost()
	host.Status.Provisioning.ID = "node-id"
	prov, err := newProvisionerWithSettings(host, bmc.Credentials{}, nil, ironic.Endpoint(), clients.AuthConfig{Type: clients.NoAuth})
	require.NoError(t, err)
	require.ErrorIs(t, prov.SetSerialConsole(t.Context(), false), provisioner.ErrNodeIsBusy)
}
