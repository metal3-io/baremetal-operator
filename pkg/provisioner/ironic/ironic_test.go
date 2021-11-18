package ironic

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	logz "sigs.k8s.io/controller-runtime/pkg/log/zap"

	metal3v1alpha1 "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/hardwareutils/bmc"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/clients"

	// We don't use this package directly here, but need it imported
	// so it registers its test fixture with the other BMC access
	// types.
	_ "github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/testbmc"
)

func init() {
	logf.SetLogger(logz.New(logz.UseDevMode(true)))
}

func newTestProvisionerFactory() ironicProvisionerFactory {
	return ironicProvisionerFactory{
		log: logf.Log,
		config: ironicConfig{
			deployKernelURL:  "http://deploy.test/ipa.kernel",
			deployRamdiskURL: "http://deploy.test/ipa.initramfs",
			deployISOURL:     "http://deploy.test/ipa.iso",
			maxBusyHosts:     20,
		},
	}
}

// A private function to construct an ironicProvisioner (rather than a
// Provisioner interface) in a consistent way for tests.
func newProvisionerWithSettings(host metal3v1alpha1.BareMetalHost, bmcCreds bmc.Credentials, publisher provisioner.EventPublisher, ironicURL string, ironicAuthSettings clients.AuthConfig, inspectorURL string, inspectorAuthSettings clients.AuthConfig) (*ironicProvisioner, error) {
	hostData := provisioner.BuildHostData(host, bmcCreds)

	tlsConf := clients.TLSConfig{}
	clientIronic, err := clients.IronicClient(ironicURL, ironicAuthSettings, tlsConf)
	if err != nil {
		return nil, err
	}

	clientInspector, err := clients.InspectorClient(inspectorURL, inspectorAuthSettings, tlsConf)
	if err != nil {
		return nil, err
	}

	factory := newTestProvisionerFactory()
	factory.clientIronic = clientIronic
	factory.clientInspector = clientInspector
	return factory.ironicProvisioner(hostData, publisher)
}

func makeHost() metal3v1alpha1.BareMetalHost {
	rotational := true

	return metal3v1alpha1.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myhost",
			Namespace: "myns",
			UID:       "27720611-e5d1-45d3-ba3a-222dcfaa4ca2",
		},
		Spec: metal3v1alpha1.BareMetalHostSpec{
			BMC: metal3v1alpha1.BMCDetails{
				Address: "test://test.bmc/",
			},
			Image: &metal3v1alpha1.Image{
				URL: "not-empty",
			},
			Online:          true,
			HardwareProfile: "libvirt",
			RootDeviceHints: &metal3v1alpha1.RootDeviceHints{
				DeviceName:         "userd_devicename",
				HCTL:               "1:2:3:4",
				Model:              "userd_model",
				Vendor:             "userd_vendor",
				SerialNumber:       "userd_serial",
				MinSizeGigabytes:   40,
				WWN:                "userd_wwn",
				WWNWithExtension:   "userd_with_extension",
				WWNVendorExtension: "userd_vendor_extension",
				Rotational:         &rotational,
			},
		},
		Status: metal3v1alpha1.BareMetalHostStatus{
			Provisioning: metal3v1alpha1.ProvisionStatus{
				ID: "provisioning-id",
				// Place the hints in the status field to pretend the
				// controller has already reconciled partially.
				RootDeviceHints: &metal3v1alpha1.RootDeviceHints{
					DeviceName:         "userd_devicename",
					HCTL:               "1:2:3:4",
					Model:              "userd_model",
					Vendor:             "userd_vendor",
					SerialNumber:       "userd_serial",
					MinSizeGigabytes:   40,
					WWN:                "userd_wwn",
					WWNWithExtension:   "userd_with_extension",
					WWNVendorExtension: "userd_vendor_extension",
					Rotational:         &rotational,
				},
				BootMode: metal3v1alpha1.UEFI,
			},
			HardwareProfile: "libvirt",
		},
	}
}

func makeHostLiveIso() (host metal3v1alpha1.BareMetalHost) {
	host = makeHost()
	format := "live-iso"
	host.Spec.Image.DiskFormat = &format
	return host
}

func makeHostCustomDeploy(only bool) (host metal3v1alpha1.BareMetalHost) {
	host = makeHost()
	host.Spec.CustomDeploy = &metal3v1alpha1.CustomDeploy{
		Method: "install_everything",
	}
	if only {
		host.Spec.Image = nil
	}
	return host
}

// Implements provisioner.EventPublisher to swallow events for tests.
func nullEventPublisher(reason, message string) {}

func TestNewNoBMCDetails(t *testing.T) {
	// Create a host without BMC details
	host := makeHost()
	host.Spec.BMC = metal3v1alpha1.BMCDetails{}

	factory := newTestProvisionerFactory()
	prov, err := factory.NewProvisioner(provisioner.BuildHostData(host, bmc.Credentials{}), nullEventPublisher)
	assert.Equal(t, nil, err)
	assert.NotEqual(t, nil, prov)
}
