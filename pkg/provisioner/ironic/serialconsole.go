package ironic

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/nodes"
	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/hardwareutils/bmc"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/clients"
)

const (
	serialConsoleInterface = "ipmitool-socat"
	ipmiDriver             = "ipmi"
)

var _ provisioner.SerialConsoleProvisioner = (*ironicProvisioner)(nil)

// GetSerialConsoleState does not call console show: IPMISocatConsole.get_console
// deactivates SOL and can interrupt a connected client.
func (p *ironicProvisioner) GetSerialConsoleState(ctx context.Context) (provisioner.SerialConsoleState, error) {
	node, err := p.getNode(ctx)
	if err != nil {
		return provisioner.SerialConsoleState{}, err
	}
	return provisioner.SerialConsoleState{
		Enabled: node.ConsoleEnabled,
		Busy:    node.Reservation != "",
		ID: fmt.Sprintf("%s/%s/%v/%v", node.UUID, node.Conductor,
			node.DriverInternalInfo["allocated_ipmi_terminal_port"], node.DriverInfo["ipmi_terminal_port"]),
	}, nil
}

// SetSerialConsole requests an asynchronous state change. Callers must observe
// completion with GetSerialConsoleState before exposing or forgetting access.
func (p *ironicProvisioner) SetSerialConsole(ctx context.Context, enabled bool) error {
	node, err := p.getNode(ctx)
	if err != nil {
		return err
	}
	if node.Reservation != "" {
		return provisioner.ErrNodeIsBusy
	}
	if enabled {
		if err = p.configureSerialConsole(ctx, node); err != nil {
			return err
		}
	}
	// Gophercloud drains and closes the response body.
	_, err = p.client.Put(ctx, p.client.ServiceURL("nodes", p.nodeID, "states", "console"), //nolint:bodyclose // Gophercloud closes the body.
		map[string]bool{"enabled": enabled}, nil, &gophercloud.RequestOpts{OkCodes: []int{http.StatusAccepted}})
	if gophercloud.ResponseCodeIs(err, http.StatusConflict) {
		return provisioner.ErrNodeIsBusy
	}
	return err
}

func (p *ironicProvisioner) configureSerialConsole(ctx context.Context, node *nodes.Node) error {
	if node.ConsoleInterface != serialConsoleInterface {
		// Changing interfaces on an active node requires maintenance mode. Do not
		// silently put running machines into maintenance just to open a console.
		switch nodes.ProvisionState(node.ProvisionState) {
		case nodes.Enroll, nodes.Manageable, nodes.Available:
		case nodes.Active:
			if !node.Maintenance {
				return fmt.Errorf("configure Ironic console_interface=%s before provisioning (or while in maintenance)", serialConsoleInterface)
			}
		default:
			return provisioner.ErrNodeIsBusy
		}
	}
	info, err := p.serialConsoleDriverInfo(ctx)
	if err != nil {
		return err
	}
	updater := clients.UpdateOptsBuilder(p.log)
	updater.SetTopLevelOpt("console_interface", serialConsoleInterface, node.ConsoleInterface)
	updater.SetDriverInfoOpts(clients.UpdateOptsData(info), node)
	// Passwords returned by Ironic are masked, so send the actual password
	// when starting a session. We do not update these settings while connected.
	_, success, result, err := p.tryUpdateNode(ctx, node, updater)
	if err != nil {
		return err
	}
	if !success {
		if result.ErrorMessage != "" {
			return errors.New(result.ErrorMessage)
		}
		return provisioner.ErrNodeIsBusy
	}
	return nil
}

func (p *ironicProvisioner) serialConsoleDriverInfo(ctx context.Context) (map[string]any, error) {
	access, err := p.bmcAccess()
	if err != nil {
		return nil, err
	}
	creds := p.bmcCreds
	if creds.SerialConsoleUsername != "" || creds.SerialConsolePassword != "" {
		creds.Username, creds.Password = creds.SerialConsoleUsername, creds.SerialConsolePassword
	}
	if err = creds.Validate(); err != nil {
		return nil, err
	}
	if p.serialConsoleAddress != "" {
		if err = bmc.ValidateSerialConsoleAddress(p.serialConsoleAddress); err != nil {
			return nil, err
		}
		if access.Driver() == ipmiDriver {
			return nil, errors.New("IPMI-managed hosts must reuse their management connection for SOL")
		}
		access, err = bmc.NewAccessDetails(p.serialConsoleAddress, false)
		if err != nil {
			return nil, err
		}
		if access.Driver() != ipmiDriver {
			return nil, errors.New("serialConsoleAddress must use IPMI")
		}
	}
	info := access.DriverInfo(creds)
	managementAccess, err := p.bmcAccess()
	if err != nil {
		return nil, err
	}
	if managementAccess.Driver() == ipmiDriver && (creds.Username != p.bmcCreds.Username || creds.Password != p.bmcCreds.Password) {
		return nil, errors.New("IPMI-managed hosts must reuse management credentials for SOL")
	}
	if info["ipmi_address"] == nil {
		discoverer, ok := access.(bmc.IPMISOLDiscoverer)
		if !ok {
			return nil, errors.New("BMC does not support IPMI SOL discovery; set bmc.serialConsoleAddress")
		}
		// Discover using management credentials, not the optional SOL credentials.
		sol, err := discoverer.DiscoverIPMISOL(ctx, p.bmcCreds)
		if err != nil {
			return nil, fmt.Errorf("discovering IPMI SOL: %w", err)
		}
		if sol == nil {
			return nil, errors.New("no IPMI SOL endpoint advertised; set bmc.serialConsoleAddress")
		}
		info = map[string]any{"ipmi_address": sol.Address, "ipmi_port": sol.Port,
			"ipmi_username": creds.Username, "ipmi_password": creds.Password, "ipmi_priv_level": "ADMINISTRATOR"}
	}
	result := map[string]any{}
	for _, key := range []string{"ipmi_address", "ipmi_port", "ipmi_username", "ipmi_password", "ipmi_priv_level", "ipmi_cipher_suite"} {
		if value, ok := info[key]; ok {
			result[key] = value
		}
	}
	// ipmi_port is the BMC's UDP port, NOT the conductor's TCP listener.
	// Leave ipmi_terminal_port unset so Ironic allocates from console.port_range.
	return result, nil
}

// preserveConsoleDriverInfo prevents BMH registration/credential rotation from
// erasing discovered SOL settings. Management IPMI settings take precedence.
func preserveConsoleDriverInfo(info map[string]any, node *nodes.Node, creds bmc.Credentials) {
	if node.ConsoleInterface != serialConsoleInterface || info["ipmi_address"] != nil {
		return
	}
	for _, key := range []string{"ipmi_address", "ipmi_port", "ipmi_priv_level", "ipmi_cipher_suite"} {
		if value, ok := node.DriverInfo[key]; ok {
			info[key] = value
		}
	}
	if info["ipmi_address"] != nil {
		info["ipmi_username"], info["ipmi_password"] = creds.Username, creds.Password
		if creds.SerialConsoleUsername != "" && creds.SerialConsolePassword != "" {
			info["ipmi_username"], info["ipmi_password"] = creds.SerialConsoleUsername, creds.SerialConsolePassword
		}
	}
}

// GetSerialConsole retrieves an endpoint only when establishing a session.
// Repeated calls can disconnect the current client and must be avoided.
func (p *ironicProvisioner) GetSerialConsole(ctx context.Context) (*metal3api.SerialConsoleEndpoint, error) {
	if p.nodeID == "" {
		return nil, provisioner.ErrNeedsRegistration
	}
	response := struct {
		Enabled bool `json:"console_enabled"` //nolint:tagliatelle // Ironic API
		Info    struct {
			URL string `json:"url"`
		} `json:"console_info"` //nolint:tagliatelle // Ironic API
	}{}
	_, err := p.client.Get(ctx, p.client.ServiceURL("nodes", p.nodeID, "states", "console"), &response, nil) //nolint:bodyclose // Gophercloud closes the body.
	if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
		return nil, provisioner.ErrNeedsRegistration
	}
	if err != nil {
		return nil, err
	}
	if !response.Enabled {
		return nil, provisioner.ErrNotReady
	}
	endpoint, err := url.Parse(response.Info.URL)
	if err != nil {
		return nil, fmt.Errorf("invalid serial console URL: %w", err)
	}
	if endpoint.Scheme != "tcp" || endpoint.Hostname() == "" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" || endpoint.Path != "" {
		return nil, errors.New("expected a TCP console endpoint from Ironic")
	}
	port, err := strconv.ParseInt(endpoint.Port(), 10, 32)
	if err != nil || port < 1 || port > 65535 {
		return nil, errors.New("invalid serial console port")
	}
	return &metal3api.SerialConsoleEndpoint{Host: endpoint.Hostname(), Port: int32(port)}, nil
}
