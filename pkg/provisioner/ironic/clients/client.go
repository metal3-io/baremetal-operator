package clients

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/httpbasic"
	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/noauth"
	"go.etcd.io/etcd/client/pkg/v3/transport"
)

var tlsConnectionTimeout = time.Second * 30

// TLSConfig contains the TLS configuration for the Ironic connection.
// Using Go default values for this will result in no additional trusted
// CA certificates and a secure connection.
// When specifying Certificate and Private key, TLS connection will use
// client certificate authentication.
type TLSConfig struct {
	TrustedCAFile         string
	ClientCertificateFile string
	ClientPrivateKeyFile  string
	InsecureSkipVerify    bool
	SkipClientSANVerify   bool
}

func updateHTTPClient(client *gophercloud.ServiceClient, tlsConf TLSConfig) error {
	tlsInfo := transport.TLSInfo{
		TrustedCAFile:       tlsConf.TrustedCAFile,
		CertFile:            tlsConf.ClientCertificateFile,
		KeyFile:             tlsConf.ClientPrivateKeyFile,
		InsecureSkipVerify:  tlsConf.InsecureSkipVerify,
		SkipClientSANVerify: tlsConf.SkipClientSANVerify,
	}
	if _, err := os.Stat(tlsConf.TrustedCAFile); err != nil {
		if os.IsNotExist(err) {
			tlsInfo.TrustedCAFile = ""
		} else {
			return err
		}
	}
	if _, err := os.Stat(tlsConf.ClientCertificateFile); err != nil {
		if os.IsNotExist(err) {
			tlsInfo.CertFile = ""
		} else {
			return err
		}
	}
	if _, err := os.Stat(tlsConf.ClientPrivateKeyFile); err != nil {
		if os.IsNotExist(err) {
			tlsInfo.KeyFile = ""
		} else {
			return err
		}
	}
	if tlsInfo.CertFile != "" && tlsInfo.KeyFile != "" {
		tlsInfo.ClientCertAuth = true
	}

	tlsTransport, err := transport.NewTransport(tlsInfo, tlsConnectionTimeout)
	if err != nil {
		return err
	}
	c := http.Client{
		Transport: tlsTransport,
	}
	client.HTTPClient = c
	return nil
}

// IronicClient creates a client for Ironic.
func IronicClient(ironicEndpoint string, auth AuthConfig, tls TLSConfig) (client *gophercloud.ServiceClient, err error) {
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
		err = fmt.Errorf("unknown auth type %s", auth.Type)
	}
	if err != nil {
		return
	}

	client.Microversion = baseline

	err = updateHTTPClient(client, tls)
	return
}
