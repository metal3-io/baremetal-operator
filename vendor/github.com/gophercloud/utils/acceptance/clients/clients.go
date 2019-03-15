// Package clients contains functions for creating service clients
// for utils services.
// That clients can be used in acceptance tests.
package clients

import (
	"os"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	"github.com/gophercloud/utils/gnocchi"
)

// NewGnocchiV1Client returns a *ServiceClient for making calls
// to the Gnocchi v1 API.
// An error will be returned if authentication or client
// creation was not possible.
func NewGnocchiV1Client() (*gophercloud.ServiceClient, error) {
	ao, err := openstack.AuthOptionsFromEnv()
	if err != nil {
		return nil, err
	}

	client, err := openstack.AuthenticatedClient(ao)
	if err != nil {
		return nil, err
	}

	return gnocchi.NewGnocchiV1(client, gophercloud.EndpointOpts{
		Region: os.Getenv("OS_REGION_NAME"),
	})
}
