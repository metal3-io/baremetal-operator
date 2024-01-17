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
		name   string
		ironic *testserver.IronicMock

		expectedIronicCalls string
		expectedIsReady     bool
		expectedError       string
	}{
		{
			name:                "IsReady",
			ironic:              testserver.NewIronic(t).WithDrivers(),
			expectedIronicCalls: "/v1/;/v1/drivers;",
			expectedIsReady:     true,
		},
		{
			name:                "NoDriversLoaded",
			ironic:              testserver.NewIronic(t),
			expectedIronicCalls: "/v1/;/v1/drivers;",
		},
		{
			name:            "IronicDown",
			expectedIsReady: false,
		},
		{
			name:                "IronicNotOk",
			ironic:              testserver.NewIronic(t).NotReady(http.StatusInternalServerError),
			expectedIsReady:     false,
			expectedIronicCalls: "/v1/;",
		},
		{
			name:                "IronicNotOkAndNotExpected",
			ironic:              testserver.NewIronic(t).NotReady(http.StatusBadGateway),
			expectedIsReady:     false,
			expectedIronicCalls: "/v1/;",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.ironic != nil {
				tc.ironic.Start()
				defer tc.ironic.Stop()
			}

			auth := clients.AuthConfig{Type: clients.NoAuth}

			ironicEndpoint := tc.ironic.Endpoint()
			prov, err := newProvisionerWithSettings(makeHost(), bmc.Credentials{}, nil, ironicEndpoint, auth)
			if err != nil {
				t.Fatalf("could not create provisioner: %s", err)
			}

			ready, err := prov.TryInit()
			if err != nil {
				t.Fatalf("could not determine ready state: %s", err)
			}

			if tc.ironic != nil {
				assert.Equal(t, tc.expectedIronicCalls, tc.ironic.Requests, "ironic calls")
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
