package ironic

import (
	"testing"
	"time"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/hardwareutils/bmc"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/clients"
	// We don't use this package directly here, but need it imported
	// so it registers its test fixture with the other BMC access
	// types.
	_ "github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/testbmc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	logz "sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func init() {
	logf.SetLogger(logz.New(logz.UseDevMode(true)))
	// Disable jitter in tests for deterministic assertions
	jitter = func(d time.Duration) time.Duration { return d }
}

var testProvisionerConfig = ironicConfig{
	deployKernelURL:  "http://deploy.test/ipa.kernel",
	deployRamdiskURL: "http://deploy.test/ipa.initramfs",
	deployISOURL:     "http://deploy.test/ipa.iso",
	maxBusyHosts:     20,
}

// Construct an ironicProvisioner directly, bypassing refreshCache.
// This is used by tests that are testing provisioner operations, not cache behavior.
func newProvisionerWithSettings(host metal3api.BareMetalHost, bmcCreds bmc.Credentials, publisher provisioner.EventPublisher, ironicURL string, ironicAuthSettings clients.AuthConfig) (*ironicProvisioner, error) {
	hostData := provisioner.BuildHostData(host, bmcCreds)

	tlsConf := clients.TLSConfig{}
	clientIronic, err := clients.IronicClient(ironicURL, ironicAuthSettings, tlsConf, 0)
	if err != nil {
		return nil, err
	}

	provisionerLogger := logf.Log.WithValues("host", ironicNodeName(hostData.ObjectMeta))
	p := &ironicProvisioner{
		config:                  testProvisionerConfig,
		objectMeta:              hostData.ObjectMeta,
		nodeID:                  hostData.ProvisionerID,
		bmcCreds:                hostData.BMCCredentials,
		bmcAddress:              hostData.BMCAddress,
		disableCertVerification: hostData.DisableCertificateVerification,
		bootMACAddress:          hostData.BootMACAddress,
		client:                  clientIronic,
		log:                     provisionerLogger,
		debugLog:                provisionerLogger.V(1),
		publisher:               publisher,
	}
	return p, nil
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

	prov, err := newProvisionerWithSettings(host, bmc.Credentials{}, nullEventPublisher, "https://ironic.test/v1/", clients.AuthConfig{Type: clients.NoAuth})
	require.NoError(t, err)
	assert.NotNil(t, prov)
}

func TestRedactSensitiveURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "empty is unchanged",
			in:   "",
			want: "",
		},
		{
			name: "plain URL without secrets is unchanged",
			in:   "https://example.com/images/image.qcow2",
			want: "https://example.com/images/image.qcow2",
		},
		{
			name: "benign query params are preserved as-is",
			in:   "https://example.com/image.qcow2?version=2&format=qcow2",
			want: "https://example.com/image.qcow2?version=2&format=qcow2",
		},
		{
			name: "embedded userinfo credentials are removed",
			in:   "https://user:pass@example.com/image.qcow2",
			want: "https://example.com/image.qcow2",
		},
		{
			name: "username-only userinfo is removed",
			in:   "https://user@example.com/image.qcow2",
			want: "https://example.com/image.qcow2",
		},
		{
			name: "AWS signature query param is redacted",
			in:   "https://example.com/image.qcow2?X-Amz-Signature=abc123&foo=bar",
			want: "https://example.com/image.qcow2?X-Amz-Signature=REDACTED&foo=bar",
		},
		{
			name: "token query param is redacted",
			in:   "https://example.com/image.qcow2?token=supersecret",
			want: "https://example.com/image.qcow2?token=REDACTED",
		},
		{
			name: "access_token query param is redacted",
			in:   "https://example.com/image.qcow2?access_token=supersecret",
			want: "https://example.com/image.qcow2?access_token=REDACTED",
		},
		{
			name: "redaction is case-insensitive on param name",
			in:   "https://example.com/image.qcow2?Signature=abc",
			want: "https://example.com/image.qcow2?Signature=REDACTED",
		},
		{
			name: "both userinfo and signed params are handled",
			in:   "https://user:pass@example.com/image.qcow2?sig=abc&keep=1",
			want: "https://example.com/image.qcow2?keep=1&sig=REDACTED",
		},
		{
			name: "GCS signed URL params are redacted",
			in:   "https://storage.googleapis.com/bucket/image.qcow2?X-Goog-Signature=abc&X-Goog-Credential=def",
			want: "https://storage.googleapis.com/bucket/image.qcow2?X-Goog-Credential=REDACTED&X-Goog-Signature=REDACTED",
		},
		{
			name: "Swift temp_url_sig is redacted",
			in:   "https://swift.example.com/v1/img.qcow2?temp_url_sig=abc&temp_url_expires=123",
			want: "https://swift.example.com/v1/img.qcow2?temp_url_expires=123&temp_url_sig=REDACTED",
		},
		{
			name: "unparseable input is returned unchanged",
			in:   "://not a url",
			want: "://not a url",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, redactSensitiveURL(tt.in))
		})
	}
}
