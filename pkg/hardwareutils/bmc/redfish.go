package bmc

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const serialConsoleDiscoveryTimeout = 10 * time.Second

func init() {
	schemes := []string{"http", "https"}
	RegisterFactory(redfish, newRedfishAccessDetails, schemes)
	RegisterFactory("ilo5-redfish", newRedfishAccessDetails, schemes)
	RegisterFactory("idrac-redfish", newRedfishiDracAccessDetails, schemes)
}

func redfishDetails(parsedURL *url.URL, disableCertificateVerification bool) *redfishAccessDetails {
	return &redfishAccessDetails{
		bmcType:                        parsedURL.Scheme,
		host:                           parsedURL.Host,
		path:                           parsedURL.Path,
		disableCertificateVerification: disableCertificateVerification,
	}
}

func newRedfishAccessDetails(parsedURL *url.URL, disableCertificateVerification bool) (AccessDetails, error) {
	return redfishDetails(parsedURL, disableCertificateVerification), nil
}

func newRedfishiDracAccessDetails(parsedURL *url.URL, disableCertificateVerification bool) (AccessDetails, error) {
	return &redfishiDracAccessDetails{
		*redfishDetails(parsedURL, disableCertificateVerification),
	}, nil
}

type redfishAccessDetails struct {
	bmcType                        string
	host                           string
	path                           string
	disableCertificateVerification bool
}

type redfishiDracAccessDetails struct {
	redfishAccessDetails
}

const redfishDefaultScheme = "https"

func (a *redfishAccessDetails) Type() string {
	return a.bmcType
}

// NeedsMAC returns false because the Redfish driver in Ironic can
// pre-populate MAC addresses during inspection, so a BootMACAddress is not
// required up front regardless of the boot method. This matches the existing
// behaviour of the plain IPMI driver, which also boots via iPXE yet already
// returns false: agent-based inspection reports the MACs back to Ironic
// rather than relying on a pre-created port. When inspection is disabled a
// MAC is still required, but that requirement is enforced by the callers via
// host.InspectionDisabled(), not by this driver-level flag.
func (a *redfishAccessDetails) NeedsMAC() bool {
	return false
}

func (a *redfishAccessDetails) Driver() string {
	return redfish
}

func (a *redfishAccessDetails) DisableCertificateVerification() bool {
	return a.disableCertificateVerification
}

func getRedfishAddress(bmcType, host string) string {
	redfishAddress := []string{}
	schemes := strings.Split(bmcType, "+")
	if len(schemes) > 1 {
		redfishAddress = append(redfishAddress, schemes[1])
	} else {
		redfishAddress = append(redfishAddress, redfishDefaultScheme)
	}
	redfishAddress = append(redfishAddress, "://")
	redfishAddress = append(redfishAddress, host)
	return strings.Join(redfishAddress, "")
}

// DriverInfo returns a data structure to pass as the DriverInfo
// parameter when creating a node in Ironic. The structure is
// pre-populated with the access information, and the caller is
// expected to add any other information that might be needed (such as
// the kernel and ramdisk locations).
func (a *redfishAccessDetails) DriverInfo(bmcCreds Credentials) map[string]interface{} {
	result := map[string]interface{}{
		"redfish_username": bmcCreds.Username,
		"redfish_password": bmcCreds.Password,
		"redfish_address":  getRedfishAddress(a.bmcType, a.host),
	}
	trimmedPath := strings.Trim(a.path, "/")
	if trimmedPath != "" && trimmedPath != "redfish/v1" {
		result["redfish_system_id"] = a.path
	}

	if a.disableCertificateVerification {
		result["redfish_verify_ca"] = false
	}

	return result
}

// DiscoverIPMISOL discovers the IPMI SOL endpoint advertised by the Redfish
// ComputerSystem. Redfish and IPMI are separate protocols, but a BMC may
// expose both and use the same credentials for them.
func (a *redfishAccessDetails) DiscoverIPMISOL(ctx context.Context, creds Credentials) (*IPMISOLInfo, error) {
	defaultTransport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, errors.New("unsupported default HTTP transport")
	}
	transport := defaultTransport.Clone()
	if a.disableCertificateVerification {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // explicitly requested by the BMH
	}
	client := &http.Client{Transport: transport, Timeout: serialConsoleDiscoveryTimeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}
	defer transport.CloseIdleConnections()
	return a.discoverIPMISOL(ctx, client, creds)
}

func (a *redfishAccessDetails) discoverIPMISOL(ctx context.Context, client *http.Client, creds Credentials) (*IPMISOLInfo, error) {
	path := strings.Trim(a.path, "/")
	if path == "" || path == "redfish/v1" {
		path = "redfish/v1/Systems"
	}

	origin, err := url.Parse(getRedfishAddress(a.bmcType, a.host))
	if err != nil {
		return nil, fmt.Errorf("parsing Redfish origin: %w", err)
	}
	response, err := redfishGET(ctx, client, origin, "/"+path, creds)
	if err != nil {
		return nil, err
	}

	// A root Redfish address points at a collection. Resolve the first system
	// before looking for the SerialConsole property.
	if members, ok := response["Members"].([]interface{}); ok {
		if len(members) == 0 {
			return nil, nil //nolint:nilnil // No advertised capability.
		}
		if len(members) != 1 {
			return nil, errors.New("multiple Redfish systems; specify a ComputerSystem in the BMC address")
		}
		member, ok := members[0].(map[string]interface{})
		if !ok {
			return nil, errors.New("invalid Redfish Systems member")
		}
		memberPath, ok := member["@odata.id"].(string)
		if !ok || memberPath == "" {
			return nil, errors.New("redfish Systems member has no @odata.id")
		}
		memberURL, linkErr := redfishResourceURL(origin, memberPath)
		if linkErr != nil {
			return nil, linkErr
		}
		response, err = redfishGET(ctx, client, origin, memberURL.String(), creds)
		if err != nil {
			return nil, err
		}
		path = strings.Trim(memberURL.Path, "/")
	}

	systemConsole, _ := response["SerialConsole"].(map[string]interface{})
	systemIPMI, _ := systemConsole["IPMI"].(map[string]interface{})
	if enabled, advertised := systemIPMI["ServiceEnabled"].(bool); advertised && !enabled {
		// A generic Manager capability must not override an explicit denial
		// of SOL on the selected ComputerSystem.
		return nil, nil //nolint:nilnil // The selected system explicitly disables SOL.
	}
	if info, found, discoveryErr := redfishIPMISOL(response); found || discoveryErr != nil {
		if discoveryErr != nil {
			return nil, discoveryErr
		}
		if info.Address == "" {
			info.Address = a.hostName()
		}
		return info, nil
	}

	// Some Redfish implementations expose serial console capabilities on the
	// Manager rather than the ComputerSystem. Find the manager associated with
	// this system through Links.ManagerForServers.
	managers, err := redfishGET(ctx, client, origin, "/redfish/v1/Managers", creds)
	if err != nil {
		return nil, err
	}
	managerMembers, ok := managers["Members"].([]interface{})
	if !ok {
		return nil, nil //nolint:nilnil // No advertised capability.
	}
	for _, rawMember := range managerMembers {
		member, ok := rawMember.(map[string]interface{})
		if !ok {
			continue
		}
		managerPath, ok := member["@odata.id"].(string)
		if !ok || managerPath == "" {
			continue
		}
		manager, err := redfishGET(ctx, client, origin, managerPath, creds)
		if err != nil {
			return nil, err
		}
		if !redfishManagerForServer(manager, path, origin) {
			continue
		}
		if info, found, err := redfishIPMISOL(manager); found || err != nil {
			if err != nil {
				return nil, err
			}
			if info.Address == "" {
				info.Address = a.hostName()
			}
			return info, nil
		}
		if info, found, managerErr := a.discoverManagerIPMISOL(ctx, client, origin, manager, creds); found || managerErr != nil {
			return info, managerErr
		}
	}

	return nil, nil //nolint:nilnil // No advertised capability.
}

func redfishIPMISOL(resource map[string]interface{}) (*IPMISOLInfo, bool, error) {
	serialConsole, ok := resource["SerialConsole"].(map[string]interface{})
	if !ok {
		return nil, false, nil
	}
	if enabled, ok := serialConsole["ServiceEnabled"].(bool); ok && !enabled {
		return nil, false, nil
	}
	if ipmi, ok := serialConsole["IPMI"].(map[string]interface{}); ok {
		if enabled, ok := ipmi["ServiceEnabled"].(bool); ok && !enabled {
			return nil, false, nil
		}
		port, ok := ipmi["Port"].(float64)
		if !ok || math.IsNaN(port) || port < 1 || port > 65535 || math.Trunc(port) != port {
			return nil, true, errors.New("redfish IPMI SOL capability does not contain a valid port")
		}
		return &IPMISOLInfo{Port: int(port)}, true, nil
	}

	if oem, ok := serialConsole["Oem"].(map[string]interface{}); ok {
		if sushyTools, ok := oem["SushyTools"].(map[string]interface{}); ok {
			port, ok := sushyTools["IPMITerminalPort"].(float64)
			if !ok || math.IsNaN(port) || port < 1 || port > 65535 || math.Trunc(port) != port {
				return nil, true, errors.New("redfish IPMI SOL capability does not contain a valid terminal port")
			}
			address, _ := sushyTools["Host"].(string)
			return &IPMISOLInfo{Address: address, Port: int(port)}, true, nil
		}
	}
	return nil, false, nil
}

// Standard Manager serial-console metadata advertises connection types, not
// a nested IPMI endpoint. Its linked ManagerNetworkProtocol supplies the port.
// A shared Manager is ambiguous: do not guess which system its SOL selects.
func (a *redfishAccessDetails) discoverManagerIPMISOL(ctx context.Context, client *http.Client, origin *url.URL, manager map[string]interface{}, creds Credentials) (*IPMISOLInfo, bool, error) {
	serial, ok := manager["SerialConsole"].(map[string]interface{})
	if !ok || serial["ServiceEnabled"] != true {
		return nil, false, nil
	}
	types, _ := serial["ConnectTypesSupported"].([]interface{})
	supportsIPMI := false
	for _, connection := range types {
		if connection == "IPMI" {
			supportsIPMI = true
		}
	}
	if !supportsIPMI {
		return nil, false, nil
	}
	links, _ := manager["Links"].(map[string]interface{})
	servers, _ := links["ManagerForServers"].([]interface{})
	if len(servers) != 1 {
		return nil, false, nil
	}
	reference, _ := manager["NetworkProtocol"].(map[string]interface{})
	link, _ := reference["@odata.id"].(string)
	if link == "" {
		return nil, false, nil
	}
	network, err := redfishGET(ctx, client, origin, link, creds)
	if err != nil {
		return nil, false, err
	}
	ipmi, _ := network["IPMI"].(map[string]interface{})
	if ipmi["ProtocolEnabled"] != true {
		return nil, false, nil
	}
	port, ok := ipmi["Port"].(float64)
	if !ok || math.IsNaN(port) || port < 1 || port > 65535 || math.Trunc(port) != port {
		return nil, true, errors.New("redfish Manager IPMI protocol does not contain a valid port")
	}
	return &IPMISOLInfo{Address: a.hostName(), Port: int(port)}, true, nil
}

func redfishManagerForServer(manager map[string]interface{}, systemPath string, origin *url.URL) bool {
	links, ok := manager["Links"].(map[string]interface{})
	if !ok {
		return false
	}
	servers, ok := links["ManagerForServers"].([]interface{})
	if !ok {
		return false
	}
	for _, rawServer := range servers {
		server, ok := rawServer.(map[string]interface{})
		if ok {
			link, _ := server["@odata.id"].(string)
			resource, err := redfishResourceURL(origin, link)
			if link != "" && err == nil && resource.Path == "/"+strings.Trim(systemPath, "/") {
				return true
			}
		}
	}
	return false
}

func (a *redfishAccessDetails) hostName() string {
	parsed, err := url.Parse("//" + a.host)
	if err != nil {
		return a.host
	}
	return parsed.Hostname()
}

func redfishResourceURL(origin *url.URL, link string) (*url.URL, error) {
	reference, err := url.Parse(link)
	if err != nil {
		return nil, fmt.Errorf("parsing Redfish resource link: %w", err)
	}
	endpoint := origin.ResolveReference(reference)
	// Resolve links as URI references, never by string concatenation. A link
	// such as @other-host/path must remain a path on the configured BMC, not
	// turn that BMC's authority into URL user information for another host.
	if endpoint.User != nil || endpoint.Fragment != "" || endpoint.Scheme != origin.Scheme || !strings.EqualFold(endpoint.Host, origin.Host) {
		return nil, errors.New("redfish resource link must stay on the configured BMC origin")
	}
	return endpoint, nil
}

func redfishGET(ctx context.Context, client *http.Client, origin *url.URL, link string, creds Credentials) (map[string]interface{}, error) {
	endpoint, err := redfishResourceURL(origin, link)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("creating Redfish request: %w", err)
	}
	request.SetBasicAuth(creds.Username, creds.Password)
	response, err := client.Do(request) //nolint:gosec // BMC addresses are administrator-controlled; discovery stays on this origin and redirects are disabled.
	if err != nil {
		return nil, fmt.Errorf("querying Redfish endpoint: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("redfish endpoint returned HTTP %d", response.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding Redfish response: %w", err)
	}
	return result, nil
}

func (a *redfishAccessDetails) BIOSInterface() string {
	return ""
}

// That can be either pxe or redfish-virtual-media.
func (a *redfishAccessDetails) BootInterface() string {
	return ipxe
}

func (a *redfishAccessDetails) FirmwareInterface() string {
	return redfish
}

func (a *redfishAccessDetails) ManagementInterface() string {
	return ""
}

func (a *redfishAccessDetails) PowerInterface() string {
	return ""
}

func (a *redfishAccessDetails) RAIDInterface() string {
	return redfish
}

func (a *redfishAccessDetails) VendorInterface() string {
	return ""
}

func (a *redfishAccessDetails) SupportsSecureBoot() bool {
	return true
}

func (a *redfishAccessDetails) InspectInterface() string {
	return redfish
}

func (a *redfishAccessDetails) SupportsISOPreprovisioningImage() bool {
	return false
}

func (a *redfishAccessDetails) RequiresProvisioningNetwork() bool {
	return true
}

// iDrac Redfish Overrides.
func (a *redfishiDracAccessDetails) Driver() string {
	return idrac
}

func (a *redfishiDracAccessDetails) BIOSInterface() string {
	return redfish
}

func (a *redfishiDracAccessDetails) BootInterface() string {
	return ipxe
}

func (a *redfishiDracAccessDetails) FirmwareInterface() string {
	return redfish
}

func (a *redfishiDracAccessDetails) InspectInterface() string {
	// FIXME(dtantsur): this should be using idracRedfish, but it's not currently enabled in ironic-image.
	return redfish
}

func (a *redfishiDracAccessDetails) ManagementInterface() string {
	return idracRedfish
}

func (a *redfishiDracAccessDetails) PowerInterface() string {
	return redfish
}

func (a *redfishiDracAccessDetails) RAIDInterface() string {
	return idracRedfish
}

func (a *redfishiDracAccessDetails) VendorInterface() string {
	return redfish
}
