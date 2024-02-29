package ironic

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	logz "sigs.k8s.io/controller-runtime/pkg/log/zap"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
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
func newProvisionerWithSettings(host metal3api.BareMetalHost, bmcCreds bmc.Credentials, publisher provisioner.EventPublisher, ironicURL string, ironicAuthSettings clients.AuthConfig) (*ironicProvisioner, error) {
	hostData := provisioner.BuildHostData(host, bmcCreds)

	tlsConf := clients.TLSConfig{}
	clientIronic, err := clients.IronicClient(ironicURL, ironicAuthSettings, tlsConf)
	if err != nil {
		return nil, err
	}

	factory := newTestProvisionerFactory()
	factory.clientIronic = clientIronic
	return factory.ironicProvisioner(context.TODO(), hostData, publisher)
}

func makeHost() metal3api.BareMetalHost {
	rotational := true

	return metal3api.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myhost",
			Namespace: "myns",
			UID:       "27720611-e5d1-45d3-ba3a-222dcfaa4ca2",
		},
		Spec: metal3api.BareMetalHostSpec{
			BMC: metal3api.BMCDetails{
				Address: "test://test.bmc/",
			},
			Image: &metal3api.Image{
				URL: "not-empty",
			},
			Online:          true,
			HardwareProfile: "libvirt",
			RootDeviceHints: &metal3api.RootDeviceHints{
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
		Status: metal3api.BareMetalHostStatus{
			Provisioning: metal3api.ProvisionStatus{
				ID: "provisioning-id",
				// Place the hints in the status field to pretend the
				// controller has already reconciled partially.
				RootDeviceHints: &metal3api.RootDeviceHints{
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
				BootMode: metal3api.UEFI,
			},
			HardwareProfile: "libvirt",
		},
	}
}

func makeHostLiveIso() (host metal3api.BareMetalHost) {
	host = makeHost()
	format := "live-iso"
	host.Spec.Image.DiskFormat = &format
	return host
}

func makeHostCustomDeploy(only bool) (host metal3api.BareMetalHost) {
	host = makeHost()
	host.Spec.CustomDeploy = &metal3api.CustomDeploy{
		Method: "install_everything",
	}
	if only {
		host.Spec.Image = nil
	}
	return host
}

// Implements provisioner.EventPublisher to swallow events for tests.
func nullEventPublisher(_, _ string) {}

func TestNewNoBMCDetails(t *testing.T) {
	// Create a host without BMC details
	host := makeHost()
	host.Spec.BMC = metal3api.BMCDetails{}

	factory := newTestProvisionerFactory()
	prov, err := factory.NewProvisioner(context.TODO(), provisioner.BuildHostData(host, bmc.Credentials{}), nullEventPublisher)
	assert.Equal(t, nil, err)
	assert.NotEqual(t, nil, prov)
}
