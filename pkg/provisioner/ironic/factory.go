package ironic

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/go-logr/logr"
	"github.com/gophercloud/gophercloud/v2"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/clients"
)

type ironicProvisionerFactory struct {
	log    logr.Logger
	config ironicConfig

	// Keep pointers to ironic client configured with the global
	// auth settings to reuse the connection between reconcilers.
	clientIronic *gophercloud.ServiceClient
}

func NewProvisionerFactory(logger logr.Logger, havePreprovImgBuilder bool) provisioner.Factory {
	factory := ironicProvisionerFactory{
		log: logger.WithName("ironic"),
	}

	err := factory.init(havePreprovImgBuilder)
	if err != nil {
		factory.log.Error(err, "Cannot start ironic provisioner")
		os.Exit(1)
	}
	return factory
}

func (f *ironicProvisionerFactory) init(havePreprovImgBuilder bool) error {
	ironicAuth, err := clients.LoadAuth()
	if err != nil {
		return err
	}

	f.config, err = loadConfigFromEnv(havePreprovImgBuilder)
	if err != nil {
		return err
	}

	ironicEndpoint, err := loadEndpointsFromEnv()
	if err != nil {
		return err
	}

	tlsConf := loadTLSConfigFromEnv()

	f.log.Info("ironic settings",
		"endpoint", ironicEndpoint,
		"ironicAuthType", ironicAuth.Type,
		"deployKernelURL", f.config.deployKernelURL,
		"deployRamdiskURL", f.config.deployRamdiskURL,
		"deployISOURL", f.config.deployISOURL,
		"liveISOForcePersistentBootDevice", f.config.liveISOForcePersistentBootDevice,
		"kernelAndRamdiskForcePersistentBootDevice", f.config.kernelAndRamdiskForcePersistentBootDevice,
		"CACertFile", tlsConf.TrustedCAFile,
		"ClientCertFile", tlsConf.ClientCertificateFile,
		"ClientPrivKeyFile", tlsConf.ClientPrivateKeyFile,
		"TLSInsecure", tlsConf.InsecureSkipVerify,
		"SkipClientSANVerify", tlsConf.SkipClientSANVerify,
	)

	f.clientIronic, err = clients.IronicClient(
		ironicEndpoint, ironicAuth, tlsConf)
	if err != nil {
		return err
	}

	return nil
}

func (f ironicProvisionerFactory) ironicProvisioner(ctx context.Context, hostData provisioner.HostData, publisher provisioner.EventPublisher) (*ironicProvisioner, error) {
	provisionerLogger := f.log.WithValues("host", ironicNodeName(hostData.ObjectMeta))

	p := &ironicProvisioner{
		config:                  f.config,
		objectMeta:              hostData.ObjectMeta,
		nodeID:                  hostData.ProvisionerID,
		bmcCreds:                hostData.BMCCredentials,
		bmcAddress:              hostData.BMCAddress,
		disableCertVerification: hostData.DisableCertificateVerification,
		bootMACAddress:          hostData.BootMACAddress,
		client:                  f.clientIronic,
		log:                     provisionerLogger,
		debugLog:                provisionerLogger.V(1),
		publisher:               publisher,
		ctx:                     ctx,
	}

	return p, nil
}

// NewProvisioner returns a new Ironic Provisioner using the global
// configuration for finding the Ironic services.
func (f ironicProvisionerFactory) NewProvisioner(ctx context.Context, hostData provisioner.HostData, publisher provisioner.EventPublisher) (provisioner.Provisioner, error) {
	return f.ironicProvisioner(ctx, hostData, publisher)
}

func loadConfigFromEnv(havePreprovImgBuilder bool) (ironicConfig, error) {
	c := ironicConfig{
		havePreprovImgBuilder: havePreprovImgBuilder,
	}

	c.deployKernelURL = os.Getenv("DEPLOY_KERNEL_URL")
	c.deployRamdiskURL = os.Getenv("DEPLOY_RAMDISK_URL")
	c.deployISOURL = os.Getenv("DEPLOY_ISO_URL")
	if !havePreprovImgBuilder {
		// NOTE(dtantsur): with a PreprovisioningImage controller, it makes sense to set only the kernel.
		// Without it, either both or neither must be set.
		if (c.deployKernelURL == "" && c.deployRamdiskURL != "") ||
			(c.deployKernelURL != "" && c.deployRamdiskURL == "") {
			return c, errors.New("DEPLOY_KERNEL_URL and DEPLOY_RAMDISK_URL can only be set together")
		}
	}
	if c.deployKernelURL == "" && c.deployRamdiskURL != "" {
		return c, errors.New("DEPLOY_RAMDISK_URL requires DEPLOY_KERNEL_URL to be set also")
	}

	c.maxBusyHosts = 20
	if maxHostsStr := os.Getenv("PROVISIONING_LIMIT"); maxHostsStr != "" {
		value, err := strconv.Atoi(maxHostsStr)
		if err != nil {
			return c, fmt.Errorf("invalid value set for variable PROVISIONING_LIMIT=%s", maxHostsStr)
		}
		c.maxBusyHosts = value
	}

	if liveISOForcePersistentBootDevice := os.Getenv("LIVE_ISO_FORCE_PERSISTENT_BOOT_DEVICE"); liveISOForcePersistentBootDevice != "" {
		if liveISOForcePersistentBootDevice != "Default" && liveISOForcePersistentBootDevice != "Always" && liveISOForcePersistentBootDevice != "Never" {
			return c, errors.New("invalid value for variable LIVE_ISO_FORCE_PERSISTENT_BOOT_DEVICE, must be one of Default, Always or Never")
		}
		c.liveISOForcePersistentBootDevice = liveISOForcePersistentBootDevice
	}
	if kernelAndRamdiskForcePersistentBootDevice := os.Getenv("KERNEL_AND_RAMDISK_FORCE_PERSISTENT_BOOT_DEVICE"); kernelAndRamdiskForcePersistentBootDevice != "" {
		if kernelAndRamdiskForcePersistentBootDevice != "Default" && kernelAndRamdiskForcePersistentBootDevice != "Always" && kernelAndRamdiskForcePersistentBootDevice != "Never" {
			return c, errors.New("invalid value for variable KERNEL_AND_RAMDISK_FORCE_PERSISTENT_BOOT_DEVICE, must be one of Default, Always or Never")
		}
		c.kernelAndRamdiskForcePersistentBootDevice = kernelAndRamdiskForcePersistentBootDevice
	}

	c.externalURL = os.Getenv("IRONIC_EXTERNAL_URL_V6")

	// Let's see if externalURL looks like a URL
	if c.externalURL != "" {
		_, externalURLParseErr := url.Parse(c.externalURL)

		if externalURLParseErr != nil {
			return c, externalURLParseErr
		}
	}

	c.provNetDisabled = strings.ToLower(os.Getenv("PROVISIONING_NETWORK_DISABLED")) == "true"

	return c, nil
}

func loadEndpointsFromEnv() (ironicEndpoint string, err error) {
	ironicEndpoint = os.Getenv("IRONIC_ENDPOINT")
	if ironicEndpoint == "" {
		err = errors.New("no IRONIC_ENDPOINT variable set")
	}
	return
}

func loadTLSConfigFromEnv() clients.TLSConfig {
	ironicCACertFile := os.Getenv("IRONIC_CACERT_FILE")
	if ironicCACertFile == "" {
		ironicCACertFile = "/opt/metal3/certs/ca/tls.crt"
	}
	ironicClientCertFile := os.Getenv("IRONIC_CLIENT_CERT_FILE")
	if ironicClientCertFile == "" {
		ironicClientCertFile = "/opt/metal3/certs/client/tls.crt"
	}
	ironicClientPrivKeyFile := os.Getenv("IRONIC_CLIENT_PRIVATE_KEY_FILE")
	if ironicClientPrivKeyFile == "" {
		ironicClientPrivKeyFile = "/opt/metal3/certs/client/tls.key"
	}
	insecure := false
	ironicInsecureStr := os.Getenv("IRONIC_INSECURE")
	if strings.EqualFold(ironicInsecureStr, "true") {
		insecure = true
	}
	skipClientSANVerify := false
	ironicSkipClientSANVerifyStr := os.Getenv("IRONIC_SKIP_CLIENT_SAN_VERIFY")
	if strings.EqualFold(ironicSkipClientSANVerifyStr, "true") {
		skipClientSANVerify = true
	}
	return clients.TLSConfig{
		TrustedCAFile:         ironicCACertFile,
		ClientCertificateFile: ironicClientCertFile,
		ClientPrivateKeyFile:  ironicClientPrivKeyFile,
		InsecureSkipVerify:    insecure,
		SkipClientSANVerify:   skipClientSANVerify,
	}
}
