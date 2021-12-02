package ironic

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/metal3-io/baremetal-operator/pkg/hardwareutils/bmc"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/clients"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/testserver"
)

func TestProvisionerIsReady(t *testing.T) {

	cases := []struct {
		name      string
		ironic    *testserver.IronicMock
		inspector *testserver.InspectorMock

		expectedIronicCalls    string
		expectedInspectorCalls string
		expectedIsReady        bool
		expectedError          string
	}{
		{
			name:                   "IsReady",
			ironic:                 testserver.NewIronic(t).Ready().WithDrivers(),
			inspector:              testserver.NewInspector(t).Ready(),
			expectedIronicCalls:    "/v1;/v1/drivers;",
			expectedInspectorCalls: "/v1;",
			expectedIsReady:        true,
		},
		{
			name:                "NoDriversLoaded",
			ironic:              testserver.NewIronic(t).Ready(),
			inspector:           testserver.NewInspector(t).Ready(),
			expectedIronicCalls: "/v1;/v1/drivers;",
		},
		{
			name:            "IronicDown",
			inspector:       testserver.NewInspector(t).Ready(),
			expectedIsReady: false,
		},
		{
			name:                "InspectorDown",
			ironic:              testserver.NewIronic(t).Ready().WithDrivers(),
			expectedIronicCalls: "/v1;/v1/drivers;",
			expectedIsReady:     false,
		},
		{
			name:                "IronicNotOk",
			ironic:              testserver.NewIronic(t).NotReady(http.StatusInternalServerError),
			inspector:           testserver.NewInspector(t).Ready(),
			expectedIsReady:     false,
			expectedIronicCalls: "/v1;",
		},
		{
			name:                "IronicNotOkAndNotExpected",
			ironic:              testserver.NewIronic(t).NotReady(http.StatusBadGateway),
			inspector:           testserver.NewInspector(t).Ready(),
			expectedIsReady:     false,
			expectedIronicCalls: "/v1;",
		},
		{
			name:                   "InspectorNotOk",
			ironic:                 testserver.NewIronic(t).Ready().WithDrivers(),
			inspector:              testserver.NewInspector(t).NotReady(http.StatusInternalServerError),
			expectedIsReady:        false,
			expectedIronicCalls:    "/v1;/v1/drivers;",
			expectedInspectorCalls: "/v1;",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.ironic != nil {
				tc.ironic.Start()
				defer tc.ironic.Stop()
			}

			if tc.inspector != nil {
				tc.inspector.Start()
				defer tc.inspector.Stop()
			}

			auth := clients.AuthConfig{Type: clients.NoAuth}

			ironicEndpoint := tc.ironic.Endpoint()
			inspectorEndpoint := tc.inspector.Endpoint()
			prov, err := newProvisionerWithSettings(makeHost(), bmc.Credentials{}, nil,
				ironicEndpoint, auth, inspectorEndpoint, auth,
			)
			if err != nil {
				t.Fatalf("could not create provisioner: %s", err)
			}

			ready, err := prov.IsReady()
			if err != nil {
				t.Fatalf("could not determine ready state: %s", err)
			}

			if tc.ironic != nil {
				assert.Equal(t, tc.expectedIronicCalls, tc.ironic.Requests, "ironic calls")
			}
			if tc.inspector != nil {
				assert.Equal(t, tc.expectedInspectorCalls, tc.inspector.Requests, "inspector calls")
			}

			if tc.expectedError != "" {
				assert.Regexp(t, tc.expectedError, err, "error message")
			} else {
				assert.Nil(t, err)
				assert.Equal(t, tc.expectedIsReady, ready, "ready flag")
			}
		})
	}
}
