package clients

import (
	"fmt"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/baremetal/httpbasic"
	"github.com/gophercloud/gophercloud/openstack/baremetal/noauth"
	httpbasicintrospection "github.com/gophercloud/gophercloud/openstack/baremetalintrospection/httpbasic"
	noauthintrospection "github.com/gophercloud/gophercloud/openstack/baremetalintrospection/noauth"
)

// IronicClient creates a client for Ironic
func IronicClient(ironicEndpoint string) (client *gophercloud.ServiceClient, err error) {
	switch authStrategy {
	case NoAuth:
		client, err = noauth.NewBareMetalNoAuth(noauth.EndpointOpts{
			IronicEndpoint: ironicEndpoint,
		})
	case HTTPBasicAuth:
		client, err = httpbasic.NewBareMetalHTTPBasic(httpbasic.EndpointOpts{
			IronicEndpoint:     ironicEndpoint,
			IronicUser:         ironicUser,
			IronicUserPassword: ironicPassword,
		})
	default:
		err = fmt.Errorf("Unknown auth type %s", auth.Type)
	}
	return
}

// InspectorClient creates a client for Ironic Inspector
func InspectorClient(inspectorEndpoint string) (client *gophercloud.ServiceClient, err error) {
	switch authStrategy {
	case NoAuth:
		client, err = noauthintrospection.NewBareMetalIntrospectionNoAuth(
			noauthintrospection.EndpointOpts{
				IronicInspectorEndpoint: inspectorEndpoint,
			})
	case HTTPBasicAuth:
		client, err = httpbasicintrospection.NewBareMetalIntrospectionHTTPBasic(httpbasicintrospection.EndpointOpts{
			IronicInspectorEndpoint:     inspectorEndpoint,
			IronicInspectorUser:         inspectorUser,
			IronicInspectorUserPassword: inspectorPassword,
		})
	default:
		err = fmt.Errorf("Unknown auth type %s", auth.Type)
	}
	return
}
