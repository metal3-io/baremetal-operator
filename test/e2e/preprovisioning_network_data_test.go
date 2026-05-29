//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"fmt"
	"net"
	"path"
	"strings"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	metal3bmc "github.com/metal3-io/baremetal-operator/pkg/hardwareutils/bmc"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Preprovisioning Network Data", Label("required", "pp-network-data"), func() {
	var (
		specName      = "pp-network-data"
		namespace     *corev1.Namespace
		cancelWatches context.CancelFunc
		toCleanup     []client.Object
	)

	BeforeEach(func() {
		toCleanup = nil

		accessDetails, err := metal3bmc.NewAccessDetails(bmc.Address, false)
		Expect(err).NotTo(HaveOccurred())
		if !accessDetails.SupportsISOPreprovisioningImage() {
			Skip("BMC does not support virtual media, required for preprovisioning network data. BMC address: " + bmc.Address)
		}
		if !e2eConfig.GetBoolVariable("DEPLOY_IRONIC") {
			Skip("Preprovisioning network data test requires a real Ironic deployment")
		}

		namespace, cancelWatches = framework.CreateNamespaceAndWatchEvents(ctx, framework.CreateNamespaceAndWatchEventsInput{
			Creator:             clusterProxy.GetClient(),
			ClientSet:           clusterProxy.GetClientSet(),
			Name:                specName,
			LogFolder:           artifactFolder,
			IgnoreAlreadyExists: true,
		})
	})

	It("should apply preprovisioning network data during inspection", func() {
		bmhName := specName
		secretName := bmhName + "-bmc"
		networkDataSecretName := bmhName + "-network-data"

		By("Creating a secret with BMH credentials")
		bmcCredentialsData := map[string]string{
			"username": bmc.User,
			"password": bmc.Password,
		}
		secret := CreateSecret(ctx, clusterProxy.GetClient(), namespace.Name, secretName, bmcCredentialsData)
		toCleanup = append(toCleanup, secret)

		By("Creating a network data secret with static IP configuration")
		// Derive a test IP from bmc.IPAddress by flipping the top bit of the
		// last octet. This keeps us on the same subnet while avoiding collisions
		// with the real address (e.g. 192.168.222.122 -> 192.168.222.250).
		ip := net.ParseIP(bmc.IPAddress).To4()
		Expect(ip).NotTo(BeNil(), "failed to parse BMC IP address %q", bmc.IPAddress)
		ip[3] ^= 0x80
		staticIP := ip.String()

		networkData := fmt.Sprintf(`{
  "links": [
    {"id": "iface0", "type": "phy", "ethernet_mac_address": "%s"}
  ],
  "networks": [
    {
      "id": "network0",
      "link": "iface0",
      "type": "ipv4",
      "ip_address": "%s",
      "netmask": "255.255.255.0",
      "network_id": "test-pp-network"
    }
  ],
  "services": []
}`, strings.ToUpper(bmc.BootMacAddress), staticIP)
		CreateSecret(ctx, clusterProxy.GetClient(), namespace.Name, networkDataSecretName, map[string]string{
			"networkData": networkData,
		})

		By("Creating a BMH with preprovisioning network data")
		bmh := metal3api.BareMetalHost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      bmhName,
				Namespace: namespace.Name,
			},
			Spec: metal3api.BareMetalHostSpec{
				BMC: metal3api.BMCDetails{
					Address:                        bmc.Address,
					CredentialsName:                secretName,
					DisableCertificateVerification: bmc.DisableCertificateVerification,
				},
				BootMode:                       metal3api.BootMode(e2eConfig.GetVariable("BOOT_MODE")),
				BootMACAddress:                 bmc.BootMacAddress,
				PreprovisioningNetworkDataName: networkDataSecretName,
			},
		}
		Expect(clusterProxy.GetClient().Create(ctx, &bmh)).To(Succeed())
		toCleanup = append(toCleanup, &bmh)

		By("Waiting for the BMH to be in inspecting state")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: clusterProxy.GetClient(),
			Bmh:    bmh,
			State:  metal3api.StateInspecting,
		}, e2eConfig.GetIntervals(specName, "wait-inspecting")...)

		By("Waiting for the BMH to become available")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: clusterProxy.GetClient(),
			Bmh:    bmh,
			State:  metal3api.StateAvailable,
		}, e2eConfig.GetIntervals(specName, "wait-available")...)

		By("Verifying that HardwareData contains a NIC with the preprovisioned static IP")
		hwData := metal3api.HardwareData{}
		key := types.NamespacedName{Namespace: namespace.Name, Name: bmhName}
		Expect(clusterProxy.GetClient().Get(ctx, key, &hwData)).To(Succeed())
		Expect(hwData.Spec.HardwareDetails).NotTo(BeNil())
		Expect(hwData.Spec.HardwareDetails.NIC).To(
			ContainElement(HaveField("IP", staticIP)),
			"Expected a NIC with the preprovisioned static IP "+staticIP)
	})

	AfterEach(func() {
		CollectSerialLogs(bmc.Name, path.Join(artifactFolder, specName))
		DumpResources(ctx, e2eConfig, clusterProxy, path.Join(artifactFolder, specName))
		if !skipCleanup {
			Cleanup(ctx, clusterProxy, namespace, cancelWatches, e2eConfig, toCleanup)
		}
	})
})
