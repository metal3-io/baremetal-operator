package ironic

import (
	"net/http"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/noauth"
	"github.com/metal3-io/baremetal-operator/pkg/hardwareutils/bmc"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/testserver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
			ironic:              testserver.NewIronic(t).WithNoDrivers(),
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
			ironicEndpoint := "https://ironic.example.com"
			if tc.ironic != nil {
				tc.ironic.Start()
				defer tc.ironic.Stop()
				ironicEndpoint = tc.ironic.Endpoint()
			}

			client, err := noauth.NewBareMetalNoAuth(noauth.EndpointOpts{
				IronicEndpoint: ironicEndpoint,
			})
			require.NoError(t, err)

			factory := ironicProvisionerFactory{
				log:          logr.Discard(),
				cache:        new(ironicProvisionerCache),
				cacheTTL:     5 * time.Second,
				clientIronic: client,
			}

			hostData := provisioner.BuildHostData(makeHost(), bmc.Credentials{})
			prov, err := factory.NewProvisioner(t.Context(), hostData, nil)

			if tc.ironic != nil {
				assert.Equal(t, tc.expectedIronicCalls, tc.ironic.Requests, "ironic calls")
			}

			if tc.expectedError != "" {
				assert.Regexp(t, tc.expectedError, err, "error message")
			} else if tc.expectedIsReady {
				require.NoError(t, err)
				assert.NotNil(t, prov)
			} else {
				assert.ErrorIs(t, err, provisioner.ErrNotReady)
			}
		})
	}
}
