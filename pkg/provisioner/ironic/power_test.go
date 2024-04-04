package ironic

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/nodes"
	"github.com/stretchr/testify/assert"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/hardwareutils/bmc"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/clients"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/testserver"
)

func TestPowerOn(t *testing.T) {
	nodeUUID := "33ce8659-7400-4c68-9535-d10766f07a58"
	cases := []struct {
		name   string
		force  bool
		ironic *testserver.IronicMock

		expectedDirty        bool
		expectedError        bool
		expectedRequestAfter int
		expectedErrorResult  bool
	}{
		{
			name: "node-already-power-on",
			ironic: testserver.NewIronic(t).Node(nodes.Node{
				PowerState: powerOn,
				UUID:       nodeUUID,
			}),
		},
		{
			name: "waiting-for-target-power-on",
			ironic: testserver.NewIronic(t).Node(nodes.Node{
				PowerState:       powerOff,
				TargetPowerState: powerOn,
				UUID:             nodeUUID,
			}),
			expectedDirty:        true,
			expectedRequestAfter: 10,
		},
		{
			name: "power-on normal",
			ironic: testserver.NewIronic(t).WithDefaultResponses().Node(nodes.Node{
				PowerState:           powerOff,
				TargetPowerState:     powerOff,
				TargetProvisionState: "",
				UUID:                 nodeUUID,
			}),
			expectedDirty: true,
		},
		{
			name: "power-on wait for Provisioning state",
			ironic: testserver.NewIronic(t).Node(nodes.Node{
				PowerState:           powerOff,
				TargetPowerState:     powerOff,
				TargetProvisionState: string(nodes.TargetDeleted),
				UUID:                 nodeUUID,
			}),
			expectedRequestAfter: 10,
			expectedDirty:        true,
		},
		{
			name: "power-on wait for locked host",
			ironic: testserver.NewIronic(t).Node(nodes.Node{
				PowerState:           powerOff,
				TargetPowerState:     powerOff,
				TargetProvisionState: "",
				UUID:                 nodeUUID,
			}).WithNodeStatesPower(nodeUUID, http.StatusConflict).WithNodeStatesPowerUpdate(nodeUUID, http.StatusConflict),
			expectedRequestAfter: 10,
			expectedDirty:        true,
		},
		{
			name: "power-on with LastError",
			ironic: testserver.NewIronic(t).Node(nodes.Node{
				PowerState:       powerOff,
				TargetPowerState: powerOff,
				UUID:             nodeUUID,
				LastError:        "power on failed",
			}),
			expectedRequestAfter: 0,
			expectedDirty:        false,
			expectedErrorResult:  true,
		},
		{
			name:  "power-on with LastError",
			force: true,
			ironic: testserver.NewIronic(t).Node(nodes.Node{
				PowerState:       powerOff,
				TargetPowerState: powerOff,
				UUID:             nodeUUID,
				LastError:        "power on failed",
			}),
			expectedError:       true,
			expectedErrorResult: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.ironic != nil {
				tc.ironic.Start()
				defer tc.ironic.Stop()
			}

			host := makeHost()
			host.Status.Provisioning.ID = nodeUUID
			publisher := func(reason, message string) {}
			auth := clients.AuthConfig{Type: clients.NoAuth}
			prov, err := newProvisionerWithSettings(host, bmc.Credentials{}, publisher, tc.ironic.Endpoint(), auth)
			if err != nil {
				t.Fatalf("could not create provisioner: %s", err)
			}

			result, err := prov.PowerOn(tc.force)

			assert.Equal(t, tc.expectedDirty, result.Dirty)
			assert.Equal(t, time.Second*time.Duration(tc.expectedRequestAfter), result.RequeueAfter)
			if !tc.expectedError {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
			if tc.expectedErrorResult {
				assert.Contains(t, result.ErrorMessage, "PowerOn operation failed")
			}
		})
	}
}

func TestPowerOff(t *testing.T) {
	hardPowerOffReason := "Host powered off"
	softPowerOffReason := "Host soft powered off"
	nodeUUID := "33ce8659-7400-4c68-9535-d10766f07a58"
	cases := []struct {
		name   string
		ironic *testserver.IronicMock
		force  bool

		expectedDirty        bool
		expectedError        bool
		expectedRequestAfter int
		expectedErrorResult  bool
		expectedReason       string
		rebootMode           metal3api.RebootMode
	}{
		{
			name: "node-already-power-off",
			ironic: testserver.NewIronic(t).Node(nodes.Node{
				PowerState: powerOff,
				UUID:       nodeUUID,
			}),
		},
		{
			name: "waiting-for-target-power-off",
			ironic: testserver.NewIronic(t).Node(nodes.Node{
				PowerState:       powerOn,
				TargetPowerState: powerOff,
				UUID:             nodeUUID,
			}),
			expectedDirty:        true,
			expectedRequestAfter: 10,
		},
		{
			name: "power-off normal",
			ironic: testserver.NewIronic(t).WithDefaultResponses().Node(nodes.Node{
				PowerState:           powerOn,
				TargetProvisionState: "",
				UUID:                 nodeUUID,
			}),
			rebootMode:     metal3api.RebootModeSoft,
			expectedDirty:  true,
			expectedReason: softPowerOffReason,
		},
		{
			name: "power-off hard",
			ironic: testserver.NewIronic(t).WithDefaultResponses().Node(nodes.Node{
				PowerState:           powerOn,
				TargetProvisionState: "",
				UUID:                 nodeUUID,
			}),
			expectedDirty:  true,
			rebootMode:     metal3api.RebootModeHard,
			expectedReason: hardPowerOffReason,
		},
		{
			name: "power-off wait for Provisioning state",
			ironic: testserver.NewIronic(t).Node(nodes.Node{
				PowerState:           powerOn,
				TargetProvisionState: string(nodes.TargetDeleted),
				UUID:                 nodeUUID,
			}),
			expectedRequestAfter: 10,
			expectedDirty:        true,
		},
		{
			name: "power-off wait for locked host",
			ironic: testserver.NewIronic(t).Node(nodes.Node{
				PowerState:           powerOn,
				TargetProvisionState: "",
				UUID:                 nodeUUID,
			}).WithNodeStatesPower(nodeUUID, http.StatusConflict).WithNodeStatesPowerUpdate(nodeUUID, http.StatusConflict),
			expectedRequestAfter: 10,
			expectedDirty:        true,
		},
		{
			name: "power-off soft with force",
			ironic: testserver.NewIronic(t).WithDefaultResponses().Node(nodes.Node{
				PowerState:           powerOn,
				TargetProvisionState: "",
				UUID:                 nodeUUID,
			}),
			rebootMode:     metal3api.RebootModeSoft,
			force:          true,
			expectedDirty:  true,
			expectedReason: hardPowerOffReason,
		},
		{
			name: "power-off hard with LastError",
			ironic: testserver.NewIronic(t).WithDefaultResponses().Node(nodes.Node{
				PowerState:           powerOn,
				TargetProvisionState: "",
				UUID:                 nodeUUID,
				LastError:            "hard power off failed",
			}),
			rebootMode:          metal3api.RebootModeHard,
			expectedDirty:       false,
			expectedErrorResult: true,
		},
		{
			name: "power-off hard with force",
			ironic: testserver.NewIronic(t).WithDefaultResponses().Node(nodes.Node{
				PowerState:           powerOn,
				TargetProvisionState: "",
				UUID:                 nodeUUID,
				LastError:            "hard power off failed",
			}),
			rebootMode:     metal3api.RebootModeHard,
			force:          true,
			expectedDirty:  true,
			expectedReason: hardPowerOffReason,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.ironic != nil {
				tc.ironic.Start()
				defer tc.ironic.Stop()
			}

			host := makeHost()
			host.Status.Provisioning.ID = nodeUUID
			var eventReasons []string
			publisher := func(reason, message string) {
				eventReasons = append(eventReasons, message)
			}
			auth := clients.AuthConfig{Type: clients.NoAuth}
			prov, err := newProvisionerWithSettings(host, bmc.Credentials{}, publisher, tc.ironic.Endpoint(), auth)
			if err != nil {
				t.Fatalf("could not create provisioner: %s", err)
			}

			// We pass the RebootMode type here to define the reboot action
			result, err := prov.PowerOff(tc.rebootMode, tc.force)

			assert.Equal(t, tc.expectedDirty, result.Dirty)
			assert.Equal(t, time.Second*time.Duration(tc.expectedRequestAfter), result.RequeueAfter)
			if !tc.expectedError {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
			if tc.expectedErrorResult {
				assert.Contains(t, result.ErrorMessage, "hard power off failed")
			}
			if tc.expectedReason != "" {
				assert.Len(t, eventReasons, 1)
				assert.Contains(t, eventReasons, tc.expectedReason)
			} else {
				assert.Empty(t, eventReasons)
			}
		})
	}
}

func TestSoftPowerOffFallback(t *testing.T) {
	nodeUUID := "33ce8659-7400-4c68-9535-d10766f07a58"
	node := nodes.Node{
		PowerState: powerOn,
		UUID:       nodeUUID,
	}
	ironic := testserver.NewIronic(t).Node(node).WithNodeStatesPowerUpdate(nodeUUID, http.StatusBadRequest)
	ironic.Start()
	defer ironic.Stop()

	host := makeHost()
	host.Status.Provisioning.ID = nodeUUID
	var eventReasons []string
	publisher := func(reason, message string) {
		eventReasons = append(eventReasons, message)
	}
	auth := clients.AuthConfig{Type: clients.NoAuth}
	prov, err := newProvisionerWithSettings(host, bmc.Credentials{}, publisher, ironic.Endpoint(), auth)
	if err != nil {
		t.Fatalf("could not create provisioner: %s", err)
	}

	_, err = prov.PowerOff(metal3api.RebootModeSoft, false)
	assert.Error(t, err)
	assert.False(t, errors.As(err, &softPowerOffUnsupportedError{}))

	_, err = prov.changePower(&node, nodes.PowerOff)
	assert.Error(t, err)
	assert.False(t, errors.As(err, &softPowerOffUnsupportedError{}))

	_, err = prov.changePower(&node, nodes.SoftPowerOff)
	assert.Error(t, err)
	assert.True(t, errors.As(err, &softPowerOffUnsupportedError{}))
}
