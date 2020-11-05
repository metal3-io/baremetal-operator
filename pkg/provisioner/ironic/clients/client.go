package clients

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/baremetal/httpbasic"
	"github.com/gophercloud/gophercloud/openstack/baremetal/noauth"
	httpbasicintrospection "github.com/gophercloud/gophercloud/openstack/baremetalintrospection/httpbasic"
	noauthintrospection "github.com/gophercloud/gophercloud/openstack/baremetalintrospection/noauth"
	"go.etcd.io/etcd/pkg/transport"
)

var tlsConnectionTimeout = time.Second * 30
var inspectorClient *gophercloud.ServiceClient
var ironicClient *gophercloud.ServiceClient

// TLSConfig contains the TLS configuration for the Ironic connection.
// Using Go default values for this will result in no additional trusted
// CA certificates and a secure connection.
type TLSConfig struct {
	TrustedCAFile      string
	InsecureSkipVerify bool
}

func updateHTTPClient(client *gophercloud.ServiceClient, tlsConf TLSConfig) (*gophercloud.ServiceClient, error) {
	tlsInfo := transport.TLSInfo{
		TrustedCAFile:      tlsConf.TrustedCAFile,
		InsecureSkipVerify: tlsConf.InsecureSkipVerify,
	}
	if _, err := os.Stat(tlsConf.TrustedCAFile); err != nil {
		if os.IsNotExist(err) {
			tlsInfo.TrustedCAFile = ""
		} else {
			return client, err
		}
	}
	tlsTransport, err := transport.NewTransport(tlsInfo, tlsConnectionTimeout)
	if err != nil {
		return client, err
	}
	c := http.Client{
		Transport: tlsTransport,
	}
	client.HTTPClient = c
	return client, nil
}

// IronicClient creates a client for Ironic
func IronicClient(ironicEndpoint string, auth AuthConfig, tls TLSConfig) (client *gophercloud.ServiceClient, err error) {
	if ironicClient != nil && ironicClient.Endpoint == ironicEndpoint {
		return ironicClient, nil
	}

	switch auth.Type {
	case NoAuth:
		client, err = noauth.NewBareMetalNoAuth(noauth.EndpointOpts{
			IronicEndpoint: ironicEndpoint,
		})
	case HTTPBasicAuth:
		client, err = httpbasic.NewBareMetalHTTPBasic(httpbasic.EndpointOpts{
			IronicEndpoint:     ironicEndpoint,
			IronicUser:         auth.Username,
			IronicUserPassword: auth.Password,
		})
	default:
		err = fmt.Errorf("Unknown auth type %s", auth.Type)
	}
	if err != nil {
		return
	}
	ironicClient = client
	return updateHTTPClient(ironicClient, tls)
}

// InspectorClient creates a client for Ironic Inspector
func InspectorClient(inspectorEndpoint string, auth AuthConfig, tls TLSConfig) (client *gophercloud.ServiceClient, err error) {
	if inspectorClient != nil && inspectorClient.Endpoint == inspectorEndpoint {
		return inspectorClient, nil
	}

	switch auth.Type {
	case NoAuth:
		client, err = noauthintrospection.NewBareMetalIntrospectionNoAuth(
			noauthintrospection.EndpointOpts{
				IronicInspectorEndpoint: inspectorEndpoint,
			})
	case HTTPBasicAuth:
		client, err = httpbasicintrospection.NewBareMetalIntrospectionHTTPBasic(httpbasicintrospection.EndpointOpts{
			IronicInspectorEndpoint:     inspectorEndpoint,
			IronicInspectorUser:         auth.Username,
			IronicInspectorUserPassword: auth.Password,
		})
	default:
		err = fmt.Errorf("Unknown auth type %s", auth.Type)
	}
	if err != nil {
		return
	}
	inspectorClient = client
	return updateHTTPClient(inspectorClient, tls)
}
