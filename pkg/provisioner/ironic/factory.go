package ironic

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/gophercloud/gophercloud"
	logz "sigs.k8s.io/controller-runtime/pkg/log/zap"

	metal3v1alpha1 "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/bmc"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/clients"
)

var (
	log                       = logz.New().WithName("provisioner").WithName("ironic")
	deployKernelURL           string
	deployRamdiskURL          string
	deployISOURL              string
	ironicEndpoint            string
	inspectorEndpoint         string
	ironicTrustedCAFile       string
	ironicClientCertFile      string
	ironicClientPrivKeyFile   string
	ironicInsecure            bool
	ironicSkipClientSANVerify bool
	ironicAuth                clients.AuthConfig
	inspectorAuth             clients.AuthConfig
	maxBusyHosts              int = 20

	// Keep pointers to ironic and inspector clients configured with
	// the global auth settings to reuse the connection between
	// reconcilers.
	clientIronicSingleton    *gophercloud.ServiceClient
	clientInspectorSingleton *gophercloud.ServiceClient
)

func init() {
	// NOTE(dhellmann): Use Fprintf() to report errors instead of
	// logging, because logging is not configured yet in init().

	var authErr error
	ironicAuth, inspectorAuth, authErr = clients.LoadAuth()
	if authErr != nil {
		fmt.Fprintf(os.Stderr, "Cannot start: %s\n", authErr)
		os.Exit(1)
	}

	err := loadConfigFromEnv()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Cannot start: %s\n", err)
		os.Exit(1)
	}
}

type ironicProvisionerFactory struct{}

func NewProvisionerFactory() provisioner.Factory {
	factory := ironicProvisionerFactory{}
	log.Info("ironic settings",
		"endpoint", ironicEndpoint,
		"ironicAuthType", ironicAuth.Type,
		"inspectorEndpoint", inspectorEndpoint,
		"inspectorAuthType", inspectorAuth.Type,
		"deployKernelURL", deployKernelURL,
		"deployRamdiskURL", deployRamdiskURL,
		"deployISOURL", deployISOURL,
	)
	return factory
}

// A private function to construct an ironicProvisioner (rather than a
// Provisioner interface) in a consistent way for tests.
func newProvisionerWithSettings(host metal3v1alpha1.BareMetalHost, bmcCreds bmc.Credentials, publisher provisioner.EventPublisher, ironicURL string, ironicAuthSettings clients.AuthConfig, inspectorURL string, inspectorAuthSettings clients.AuthConfig) (*ironicProvisioner, error) {
	hostData := provisioner.BuildHostData(host, bmcCreds)

	tlsConf := clients.TLSConfig{
		TrustedCAFile:         ironicTrustedCAFile,
		ClientCertificateFile: ironicClientCertFile,
		ClientPrivateKeyFile:  ironicClientPrivKeyFile,
		InsecureSkipVerify:    ironicInsecure,
		SkipClientSANVerify:   ironicSkipClientSANVerify,
	}
	clientIronic, err := clients.IronicClient(ironicURL, ironicAuthSettings, tlsConf)
	if err != nil {
		return nil, err
	}

	clientInspector, err := clients.InspectorClient(inspectorURL, inspectorAuthSettings, tlsConf)
	if err != nil {
		return nil, err
	}

	return newProvisionerWithIronicClients(hostData, publisher,
		clientIronic, clientInspector)
}

func newProvisionerWithIronicClients(hostData provisioner.HostData, publisher provisioner.EventPublisher, clientIronic *gophercloud.ServiceClient, clientInspector *gophercloud.ServiceClient) (*ironicProvisioner, error) {
	// Ensure we have a microversion high enough to get the features
	// we need.
	clientIronic.Microversion = "1.56"

	provisionerLogger := log.WithValues("host", ironicNodeName(hostData.ObjectMeta))

	p := &ironicProvisioner{
		objectMeta:              hostData.ObjectMeta,
		nodeID:                  hostData.ProvisionerID,
		bmcCreds:                hostData.BMCCredentials,
		bmcAddress:              hostData.BMCAddress,
		disableCertVerification: hostData.DisableCertificateVerification,
		bootMACAddress:          hostData.BootMACAddress,
		client:                  clientIronic,
		inspector:               clientInspector,
		log:                     provisionerLogger,
		debugLog:                provisionerLogger.V(1),
		publisher:               publisher,
	}

	return p, nil
}

// NewProvisioner returns a new Ironic Provisioner using the global
// configuration for finding the Ironic services.
func (f ironicProvisionerFactory) NewProvisioner(hostData provisioner.HostData, publisher provisioner.EventPublisher) (provisioner.Provisioner, error) {
	var err error
	if clientIronicSingleton == nil || clientInspectorSingleton == nil {
		tlsConf := clients.TLSConfig{
			TrustedCAFile:         ironicTrustedCAFile,
			ClientCertificateFile: ironicClientCertFile,
			ClientPrivateKeyFile:  ironicClientPrivKeyFile,
			InsecureSkipVerify:    ironicInsecure,
			SkipClientSANVerify:   ironicSkipClientSANVerify,
		}
		clientIronicSingleton, err = clients.IronicClient(
			ironicEndpoint, ironicAuth, tlsConf)
		if err != nil {
			return nil, err
		}

		clientInspectorSingleton, err = clients.InspectorClient(
			inspectorEndpoint, inspectorAuth, tlsConf)
		if err != nil {
			return nil, err
		}
	}
	return newProvisionerWithIronicClients(hostData, publisher,
		clientIronicSingleton, clientInspectorSingleton)
}

func loadConfigFromEnv() error {
	deployKernelURL = os.Getenv("DEPLOY_KERNEL_URL")
	deployRamdiskURL = os.Getenv("DEPLOY_RAMDISK_URL")
	deployISOURL = os.Getenv("DEPLOY_ISO_URL")
	if deployISOURL == "" && (deployKernelURL == "" || deployRamdiskURL == "") {
		return errors.New("Either DEPLOY_KERNEL_URL and DEPLOY_RAMDISK_URL or DEPLOY_ISO_URL must be set")
	}
	if (deployKernelURL == "" && deployRamdiskURL != "") || (deployKernelURL != "" && deployRamdiskURL == "") {
		return errors.New("DEPLOY_KERNEL_URL and DEPLOY_RAMDISK_URL can only be set together")
	}
	ironicEndpoint = os.Getenv("IRONIC_ENDPOINT")
	if ironicEndpoint == "" {
		return errors.New("No IRONIC_ENDPOINT variable set")
	}
	inspectorEndpoint = os.Getenv("IRONIC_INSPECTOR_ENDPOINT")
	if inspectorEndpoint == "" {
		return errors.New("No IRONIC_INSPECTOR_ENDPOINT variable set")
	}
	ironicTrustedCAFile = os.Getenv("IRONIC_CACERT_FILE")
	if ironicTrustedCAFile == "" {
		ironicTrustedCAFile = "/opt/metal3/certs/ca/tls.crt"
	}
	ironicClientCertFile = os.Getenv("IRONIC_CLIENT_CERT_FILE")
	if ironicClientCertFile == "" {
		ironicClientCertFile = "/opt/metal3/certs/client/tls.crt"
	}
	ironicClientPrivKeyFile = os.Getenv("IRONIC_CLIENT_PRIVATE_KEY_FILE")
	if ironicClientPrivKeyFile == "" {
		ironicClientPrivKeyFile = "/opt/metal3/certs/client/tls.key"
	}
	ironicInsecureStr := os.Getenv("IRONIC_INSECURE")
	if strings.ToLower(ironicInsecureStr) == "true" {
		ironicInsecure = true
	}
	ironicSkipClientSANVerifyStr := os.Getenv("IRONIC_SKIP_CLIENT_SAN_VERIFY")
	if strings.ToLower(ironicSkipClientSANVerifyStr) == "true" {
		ironicSkipClientSANVerify = true
	}

	if maxHostsStr := os.Getenv("PROVISIONING_LIMIT"); maxHostsStr != "" {
		value, err := strconv.Atoi(maxHostsStr)
		if err != nil {
			return fmt.Errorf("Invalid value set for variable PROVISIONING_LIMIT=%s", maxHostsStr)
		}
		maxBusyHosts = value
	}

	return nil
}
