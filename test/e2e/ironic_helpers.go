//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/httpbasic"
	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/nodes"
	. "github.com/onsi/gomega"
)

// CreateIronicClient creates a Gophercloud client for accessing the Ironic API.
// Only valid when DEPLOY_IRONIC is true. Requires IRONIC_USERNAME, IRONIC_PASSWORD,
// IRONIC_PROVISIONING_IP, IRONIC_PROVISIONING_PORT, and IRONIC_CLIENT_TIMEOUT in the config.
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
		Timeout: e2eConfig.GetDurationVariable("IRONIC_CLIENT_TIMEOUT"),
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

// IronicSecurityConfig holds the parameters needed to verify Ironic TLS and basic-auth security.
type IronicSecurityConfig struct {
	// Host is the Ironic API host in "host:port" form.
	Host string
	// Username is the Ironic basic-auth username.
	Username string
	// Password is the Ironic basic-auth password.
	Password string
	// ClientTimeout is the HTTP client timeout used during verification requests.
	ClientTimeout time.Duration
}

// VerifyIronicSecurityConfig verifies that the Ironic API endpoint is protected by TLS
// and basic authentication. It checks that:
//   - The server presents a TLS certificate
//   - Unauthenticated requests are rejected with HTTP 401
//   - Requests with incorrect credentials are rejected with HTTP 401
//   - Requests with correct credentials succeed with HTTP 200
func VerifyIronicSecurityConfig(ctx context.Context, config IronicSecurityConfig) {
	ironicEndpoint := fmt.Sprintf("https://%s/v1", config.Host)
	username := config.Username
	password := config.Password

	Logf("Verifying Ironic TLS and basic-auth configuration at %s", ironicEndpoint)

	// Verify TLS: connect directly and confirm the server completes a handshake
	// and presents a certificate. InsecureSkipVerify is required because Ironic
	// uses a self-signed certificate in the test environment.
	tlsDialer := &tls.Dialer{
		Config: &tls.Config{
			InsecureSkipVerify: true, // #nosec G402 Self-signed certificates in the test environment
		},
	}
	rawConn, err := tlsDialer.DialContext(ctx, "tcp", config.Host)
	Expect(err).NotTo(HaveOccurred(), "Ironic should accept TLS connections at %s", config.Host)
	//nolint:forcetypeassert
	tlsConn := rawConn.(*tls.Conn)
	state := tlsConn.ConnectionState()
	_ = rawConn.Close()
	Expect(state.HandshakeComplete).To(BeTrue(), "TLS handshake with Ironic should be complete")
	Expect(state.PeerCertificates).NotTo(BeEmpty(), "Ironic server should present a TLS certificate")
	Logf("Ironic TLS certificate subject: %s", state.PeerCertificates[0].Subject)

	// Create client for the basic-auth HTTP checks.
	httpClient := &http.Client{
		Timeout: config.ClientTimeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true, // #nosec G402 Self-signed certificates in the test environment
			},
		},
	}

	// Use /v1/nodes for the auth checks: /v1 is intentionally public (version
	// discovery), while /v1/nodes is a protected resource that requires auth.
	nodesEndpoint := fmt.Sprintf("https://%s/v1/nodes", config.Host)

	// No credentials: expect HTTP 401.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, nodesEndpoint, http.NoBody)
	Expect(err).NotTo(HaveOccurred())
	resp, err := httpClient.Do(req)
	Expect(err).NotTo(HaveOccurred(), "Ironic should be reachable at %s", nodesEndpoint)
	_ = resp.Body.Close()
	Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized),
		"Ironic should return HTTP 401 when accessed without credentials")

	// Wrong credentials: expect HTTP 401.
	req, err = http.NewRequestWithContext(ctx, http.MethodGet, nodesEndpoint, http.NoBody)
	Expect(err).NotTo(HaveOccurred())
	req.SetBasicAuth("wrong-user", "wrong-password")
	resp, err = httpClient.Do(req)
	Expect(err).NotTo(HaveOccurred())
	_ = resp.Body.Close()
	Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized),
		"Ironic should return HTTP 401 when accessed with wrong credentials")

	// Correct credentials: expect HTTP 200.
	req, err = http.NewRequestWithContext(ctx, http.MethodGet, nodesEndpoint, http.NoBody)
	Expect(err).NotTo(HaveOccurred())
	req.SetBasicAuth(username, password)
	resp, err = httpClient.Do(req)
	Expect(err).NotTo(HaveOccurred())
	_ = resp.Body.Close()
	Expect(resp.StatusCode).To(Equal(http.StatusOK),
		"Ironic should return HTTP 200 when accessed with correct credentials")

	Logf("Ironic TLS and basic-auth verification passed")
}

// RedfishResetBios resets BIOS in sushy-tools (or similar implementation).
func RedfishResetBios(ctx context.Context, bmc BMC) {
	address, err := url.Parse(bmc.Address)
	Expect(err).NotTo(HaveOccurred())
	if schemeParts := strings.Split(address.Scheme, "+"); len(schemeParts) > 1 {
		address.Scheme = schemeParts[1]
	}
	endpoint := address.String() + "/BIOS/Actions/Bios.ResetBios"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, http.NoBody)
	Expect(err).NotTo(HaveOccurred())
	resp, err := http.DefaultClient.Do(req)
	Expect(err).NotTo(HaveOccurred())
	defer resp.Body.Close()
	Expect(resp.StatusCode).To(Equal(http.StatusNoContent))
}
