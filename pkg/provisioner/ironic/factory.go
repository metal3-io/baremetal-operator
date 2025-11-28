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
	"github.com/metal3-io/baremetal-operator/pkg/secretutils"
	ironicv1alpha1 "github.com/metal3-io/ironic-standalone-operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ironicProvisionerFactory struct {
	log    logr.Logger
	config ironicConfig

	// Keep pointers to ironic client configured with the global
	// auth settings to reuse the connection between reconcilers.
	clientIronic *gophercloud.ServiceClient

	// Kubernetes client for reading Ironic CR
	k8sClient client.Client
	apiReader client.Reader

	// Ironic CR configuration
	ironicName      string
	ironicNamespace string
}

func NewProvisionerFactory(logger logr.Logger, havePreprovImgBuilder bool) (provisioner.Factory, error) {
	factory := ironicProvisionerFactory{
		log: logger.WithName("ironic"),
	}

	err := factory.init(havePreprovImgBuilder)
	return factory, err
}

func NewProvisionerFactoryWithClient(logger logr.Logger, havePreprovImgBuilder bool, k8sClient client.Client, apiReader client.Reader, ironicName, ironicNamespace string) (provisioner.Factory, error) {
	factory := ironicProvisionerFactory{
		log:             logger.WithName("ironic"),
		k8sClient:       k8sClient,
		apiReader:       apiReader,
		ironicName:      ironicName,
		ironicNamespace: ironicNamespace,
	}

	err := factory.init(havePreprovImgBuilder)
	return factory, err
}

func (f *ironicProvisionerFactory) init(havePreprovImgBuilder bool) error {
	var err error
	f.config, err = loadConfigFromEnv(havePreprovImgBuilder)
	if err != nil {
		return err
	}

	if f.ironicName != "" && f.ironicNamespace != "" {
		f.log.Info("will use Ironic resource configuration",
			"ironicName", f.ironicName,
			"ironicNamespace", f.ironicNamespace,
			"deployKernelURL", f.config.deployKernelURL,
			"deployRamdiskURL", f.config.deployRamdiskURL,
			"deployISOURL", f.config.deployISOURL,
			"liveISOForcePersistentBootDevice", f.config.liveISOForcePersistentBootDevice,
		)
		// NOTE(dtantsur): the Ironic object will be loaded from the client cache on each reconciliation, so exiting here.
		return nil
	}

	f.log.V(1).Info("will use environment variables configuration")
	// For environment variable mode, validate configuration early and create a static client
	ironicAuth, err := clients.LoadAuth()
	if err != nil {
		return err
	}

	ironicEndpoint, err := loadEndpointsFromEnv()
	if err != nil {
		return err
	}

	tlsConf := loadTLSConfigFromEnv()
	f.log.Info("ironic settings from environment variables",
		"endpoint", ironicEndpoint,
		"ironicAuthType", ironicAuth.Type,
		"deployKernelURL", f.config.deployKernelURL,
		"deployRamdiskURL", f.config.deployRamdiskURL,
		"deployISOURL", f.config.deployISOURL,
		"liveISOForcePersistentBootDevice", f.config.liveISOForcePersistentBootDevice,
		"CACertFile", tlsConf.TrustedCAFile,
		"ClientCertFile", tlsConf.ClientCertificateFile,
		"ClientPrivKeyFile", tlsConf.ClientPrivateKeyFile,
		"TLSInsecure", tlsConf.InsecureSkipVerify,
		"SkipClientSANVerify", tlsConf.SkipClientSANVerify,
	)

	f.clientIronic, err = clients.IronicClient(ironicEndpoint, ironicAuth, tlsConf)
	if err != nil {
		return err
	}

	return nil
}

func (f ironicProvisionerFactory) ironicProvisioner(ctx context.Context, hostData provisioner.HostData, publisher provisioner.EventPublisher) (*ironicProvisioner, error) {
	provisionerLogger := f.log.WithValues("host", ironicNodeName(hostData.ObjectMeta))

	var ironicClient *gophercloud.ServiceClient

	// Check if we should use Ironic CR configuration (fetch fresh config on each provisioner creation)
	if f.ironicName != "" && f.ironicNamespace != "" && f.k8sClient != nil {
		ironicEndpoint, ironicAuth, tlsConf, err := f.loadConfigFromIronicCR(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to load configuration from Ironic resource %s/%s: %w", f.ironicNamespace, f.ironicName, err)
		}

		provisionerLogger.Info("ironic settings from Ironic resource",
			"ironicName", f.ironicName,
			"ironicNamespace", f.ironicNamespace,
			"endpoint", ironicEndpoint,
			"CACertFile", tlsConf.TrustedCAFile,
		)

		ironicClient, err = clients.IronicClient(ironicEndpoint, ironicAuth, tlsConf)
		if err != nil {
			return nil, fmt.Errorf("failed to create a client from Ironic resource %s/%s: %w", f.ironicNamespace, f.ironicName, err)
		}
	} else {
		// Use the pre-configured client from environment variables
		ironicClient = f.clientIronic
	}

	p := &ironicProvisioner{
		config:                  f.config,
		objectMeta:              hostData.ObjectMeta,
		nodeID:                  hostData.ProvisionerID,
		bmcCreds:                hostData.BMCCredentials,
		bmcAddress:              hostData.BMCAddress,
		disableCertVerification: hostData.DisableCertificateVerification,
		bootMACAddress:          hostData.BootMACAddress,
		client:                  ironicClient,
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

	if forcePersistentBootDevice := os.Getenv("LIVE_ISO_FORCE_PERSISTENT_BOOT_DEVICE"); forcePersistentBootDevice != "" {
		if forcePersistentBootDevice != "Default" && forcePersistentBootDevice != "Always" && forcePersistentBootDevice != "Never" {
			return c, errors.New("invalid value for variable LIVE_ISO_FORCE_PERSISTENT_BOOT_DEVICE, must be one of Default, Always or Never")
		}
		c.liveISOForcePersistentBootDevice = forcePersistentBootDevice
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

func ironicUnreadyReason(ironic *ironicv1alpha1.Ironic) string {
	cond := meta.FindStatusCondition(ironic.Status.Conditions, string(ironicv1alpha1.IronicStatusReady))
	if cond == nil || cond.ObservedGeneration != ironic.Generation {
		return "reconciliation hasn't started yet"
	}
	if cond.Status != metav1.ConditionTrue {
		return cond.Message
	}
	return ""
}

func (f *ironicProvisionerFactory) loadConfigFromIronicCR(ctx context.Context) (endpoint string, auth clients.AuthConfig, tlsConfig clients.TLSConfig, err error) {
	sm := secretutils.NewSecretManager(ctx, f.log, f.k8sClient, f.apiReader)

	// Get the Ironic CR
	ironicCR := &ironicv1alpha1.Ironic{}
	key := types.NamespacedName{
		Name:      f.ironicName,
		Namespace: f.ironicNamespace,
	}

	err = f.k8sClient.Get(ctx, key, ironicCR)
	if err != nil {
		return "", clients.AuthConfig{}, clients.TLSConfig{}, fmt.Errorf("failed to get Ironic resource %s/%s: %w", f.ironicNamespace, f.ironicName, err)
	}

	// Check if the Ironic CR is ready
	if unreadyReason := ironicUnreadyReason(ironicCR); unreadyReason != "" {
		return "", clients.AuthConfig{}, clients.TLSConfig{}, fmt.Errorf("ironic resource %s/%s is not ready: %s", f.ironicNamespace, f.ironicName, unreadyReason)
	}

	endpoint = fmt.Sprintf("http://%s.%s.svc", f.ironicName, f.ironicNamespace)

	// Handle TLS
	if ironicCR.Spec.TLS.CertificateName != "" {
		endpoint = fmt.Sprintf("https://%s.%s.svc", f.ironicName, f.ironicNamespace)
		tlsConfig, err = f.loadTLSConfigFromIronicCR(&sm, ironicCR)
		if err != nil {
			return "", clients.AuthConfig{}, clients.TLSConfig{}, err
		}
	}

	// Handle authentication
	auth, err = f.loadAuthConfigFromIronicCR(&sm, ironicCR)
	if err != nil {
		return "", clients.AuthConfig{}, clients.TLSConfig{}, err
	}

	return endpoint, auth, tlsConfig, nil
}

func (f *ironicProvisionerFactory) loadAuthConfigFromIronicCR(sm *secretutils.SecretManager, ironicCR *ironicv1alpha1.Ironic) (clients.AuthConfig, error) {
	key := types.NamespacedName{
		Name:      ironicCR.Spec.APICredentialsName,
		Namespace: ironicCR.Namespace,
	}

	secret, err := sm.ObtainSecret(key)
	if err != nil {
		return clients.AuthConfig{}, fmt.Errorf("failed to get auth secret %s/%s: %w", key.Namespace, key.Name, err)
	}

	username, usernameExists := secret.Data["username"]
	password, passwordExists := secret.Data["password"]

	if !usernameExists || !passwordExists {
		return clients.AuthConfig{}, fmt.Errorf("auth secret %s/%s must contain 'username' and 'password' keys", key.Namespace, key.Name)
	}

	return clients.AuthConfig{
		Type:     clients.HTTPBasicAuth,
		Username: string(username),
		Password: string(password),
	}, nil
}

func (f *ironicProvisionerFactory) loadTLSConfigFromIronicCR(sm *secretutils.SecretManager, ironicCR *ironicv1alpha1.Ironic) (clients.TLSConfig, error) {
	// Allow client TLS configuration from the environment
	tlsConfig := loadTLSConfigFromEnv()

	key := types.NamespacedName{
		Name:      ironicCR.Spec.TLS.CertificateName,
		Namespace: ironicCR.Namespace,
	}

	secret, err := sm.ObtainSecret(key)
	if err != nil {
		return clients.TLSConfig{}, fmt.Errorf("failed to get TLS secret %s/%s: %w", ironicCR.Namespace, ironicCR.Spec.TLS.CertificateName, err)
	}

	caCert := secret.Data["tls.crt"]
	if caCert != nil {
		caFile, err := writeTempFile("ironic-ca-", caCert)
		if err != nil {
			return clients.TLSConfig{}, fmt.Errorf("failed to write CA certificate: %w", err)
		}
		tlsConfig.TrustedCAFile = caFile
	}

	return tlsConfig, nil
}

func writeTempFile(prefix string, data []byte) (string, error) {
	tmpFile, err := os.CreateTemp("", prefix)
	if err != nil {
		return "", err
	}
	defer tmpFile.Close()

	_, err = tmpFile.Write(data)
	if err != nil {
		return "", err
	}

	return tmpFile.Name(), nil
}
