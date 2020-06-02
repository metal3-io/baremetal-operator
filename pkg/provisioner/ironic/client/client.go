package client

import (
	"fmt"
	"os"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/baremetal/noauth"
	noauthintrospection "github.com/gophercloud/gophercloud/openstack/baremetalintrospection/noauth"
)

// IronicEndpoint is the location of the Ironic API service.
var IronicEndpoint string

// InspectorEndpoint is the location of the Ironic Inspector API.
var InspectorEndpoint string

func init() {
	IronicEndpoint = os.Getenv("IRONIC_ENDPOINT")
	if IronicEndpoint == "" {
		fmt.Fprintf(os.Stderr, "Cannot start: No IRONIC_ENDPOINT variable set\n")
		os.Exit(1)
	}
	InspectorEndpoint = os.Getenv("IRONIC_INSPECTOR_ENDPOINT")
	if InspectorEndpoint == "" {
		fmt.Fprintf(os.Stderr, "Cannot start: No IRONIC_INSPECTOR_ENDPOINT variable set")
		os.Exit(1)
	}
}

// New creates a new ironic client
func New() (client *gophercloud.ServiceClient, err error) {
	client, err = noauth.NewBareMetalNoAuth(noauth.EndpointOpts{
		IronicEndpoint: IronicEndpoint,
	})
	if err != nil {
		return nil, err
	}
	// Ensure we have a microversion high enough to get the features
	// we need.
	client.Microversion = "1.56"
	return client, nil
}

// NewInspector creates a new ironic-inspecctor client
func NewInspector() (client *gophercloud.ServiceClient, err error) {
	client, err = noauthintrospection.NewBareMetalIntrospectionNoAuth(
		noauthintrospection.EndpointOpts{
			IronicInspectorEndpoint: InspectorEndpoint,
		})
	if err != nil {
		return nil, err
	}
	return client, nil
}
