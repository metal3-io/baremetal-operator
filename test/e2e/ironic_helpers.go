//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"slices"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/httpbasic"
	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/nodes"
	. "github.com/onsi/gomega"
)

// CreateIronicClient creates a Gophercloud client for accessing the Ironic API.
// Only valid when DEPLOY_IRONIC is true (requires IRONIC_USERNAME, IRONIC_PASSWORD,
// IRONIC_PROVISIONING_IP, and IRONIC_PROVISIONING_PORT in the config).
func CreateIronicClient(e2eConfig *Config) *gophercloud.ServiceClient {
	ironicIP := e2eConfig.GetVariable("IRONIC_PROVISIONING_IP")
	ironicPort := e2eConfig.GetVariable("IRONIC_PROVISIONING_PORT")
	ironicEndpoint := fmt.Sprintf("https://%s/v1", net.JoinHostPort(ironicIP, ironicPort))

	username := e2eConfig.GetVariable("IRONIC_USERNAME")
	password := e2eConfig.GetVariable("IRONIC_PASSWORD")

	client, err := httpbasic.NewBareMetalHTTPBasic(httpbasic.EndpointOpts{
		IronicEndpoint:     ironicEndpoint,
		IronicUser:         username,
		IronicUserPassword: password,
	})
	Expect(err).NotTo(HaveOccurred(), "Failed to create Ironic client")

	client.Microversion = "1.89"

	client.HTTPClient = http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true, // #nosec G402 Self-signed certificates in the test environment
			},
		},
	}

	return client
}

// IronicNodeName returns the Ironic node name corresponding to a BMH.
func IronicNodeName(namespace, bmhName string) string {
	return namespace + "~" + bmhName
}

// WaitForIronicNodeProvisionStateInput is the input for WaitForIronicNodeProvisionState.
type WaitForIronicNodeProvisionStateInput struct {
	Client   *gophercloud.ServiceClient
	NodeName string
	States   []nodes.ProvisionState
}

// WaitForIronicNodeProvisionState polls the Ironic API until the node's provision
// state matches one of the specified target states.
func WaitForIronicNodeProvisionState(ctx context.Context, input WaitForIronicNodeProvisionStateInput, intervals ...interface{}) {
	Logf("Waiting for Ironic node %s to reach one of %v", input.NodeName, input.States)

	Eventually(func(g Gomega) {
		ironicNode, err := nodes.Get(ctx, input.Client, input.NodeName).Extract()
		g.Expect(err).NotTo(HaveOccurred(), "Failed to get Ironic node %s", input.NodeName)

		currentState := nodes.ProvisionState(ironicNode.ProvisionState)
		g.Expect(slices.Contains(input.States, currentState)).To(BeTrue(),
			"Ironic node %s is in state %s, expected one of %v", input.NodeName, currentState, input.States)
	}, intervals...).Should(Succeed())
}
