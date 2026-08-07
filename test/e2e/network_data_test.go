//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"path"
	"strings"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const networkDataTemplate = `{
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
      "network_id": "test-network"
    }
  ],
  "services": []
}`

type simpleNetworkData struct {
	Links    []map[string]string `json:"links"`
	Networks []map[string]string `json:"networks"`
}

func getNetworkData(specName, connectIP string) simpleNetworkData {
	sshClient := EstablishSSHConnection(e2eConfig, connectIP)
	defer sshClient.Close()

	var output string
	Eventually(func(g Gomega) {
		command := "sudo mkdir -p /mnt/cdtest && sudo mount $(blkid -L config-2) /mnt/cdtest && cat /mnt/cdtest/openstack/latest/network_data.json"
		var err error
		output, err = executeSSHCommand(sshClient, command)
		g.Expect(err).NotTo(HaveOccurred(), "Failed to read network_data.json from the config drive")
	}, e2eConfig.GetIntervals(specName, "wait-user-data")...).Should(Succeed())
	Expect(output).NotTo(BeEmpty())

	var parsed simpleNetworkData
	Expect(json.Unmarshal([]byte(output), &parsed)).To(Succeed(), "network_data.json is not valid JSON: "+output)
	return parsed
}

func getNewIPAddress() string {
	// Derive a test IP from bmc.IPAddress by flipping the top bit of the
	// last octet. This keeps us on the same subnet while avoiding collisions
	// with the real address (e.g. 192.168.222.122 -> 192.168.222.250).
	ip := net.ParseIP(bmc.IPAddress).To4()
	Expect(ip).NotTo(BeNil(), "failed to parse BMC IP address %q", bmc.IPAddress)
	ip[3] ^= 0x80
	return ip.String()
}

var _ = Describe("Network Data", Label("required", "network-data", "ironic"), func() {
	var (
		specName      = "network-data"
		namespace     *corev1.Namespace
		cancelWatches context.CancelFunc
		toCleanup     []client.Object
	)

	BeforeEach(func() {
		toCleanup = nil

		namespace, cancelWatches = framework.CreateNamespaceAndWatchEvents(ctx, framework.CreateNamespaceAndWatchEventsInput{
			Creator:             clusterProxy.GetClient(),
			ClientSet:           clusterProxy.GetClientSet(),
			Name:                specName,
			LogFolder:           artifactFolder,
			IgnoreAlreadyExists: true,
		})
	})

	It("should apply preprovisioning network data during inspection and provisioning", func() {
		if !bmc.AccessDetails.SupportsISOPreprovisioningImage() {
			Skip("BMC does not support virtual media, required for network data test. BMC address: " + bmc.Address)
		}
		if !e2eConfig.GetBoolVariable("DEPLOY_IRONIC") {
			Skip("Preprovisioning network data test requires a real Ironic deployment")
		}

		bmhName := specName + "-pp"
		secretName := bmhName + "-bmc"
		networkDataSecretName := bmhName + "-network-data"
		expectedMAC := strings.ToUpper(bmc.BootMacAddress)

		By("Creating a secret with BMH credentials")
		bmcCredentialsData := map[string]string{
			"username": bmc.User,
			"password": bmc.Password,
		}
		secret := CreateSecret(ctx, clusterProxy.GetClient(), namespace.Name, secretName, bmcCredentialsData)
		toCleanup = append(toCleanup, secret)

		By("Creating a network data secret with static IP configuration")
		staticIP := getNewIPAddress()
		networkData := fmt.Sprintf(networkDataTemplate, expectedMAC, staticIP)
		netSecret := CreateSecret(ctx, clusterProxy.GetClient(), namespace.Name, networkDataSecretName, map[string]string{
			"networkData": networkData,
		})
		toCleanup = append(toCleanup, netSecret)

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
				AutomatedCleaningMode:          metal3api.CleaningModeDisabled,
				BootMode:                       metal3api.BootMode(e2eConfig.GetVariable("BOOT_MODE")),
				BootMACAddress:                 bmc.BootMacAddress,
				Online:                         true,
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

		By("Verifying that HardwareData contains a NIC with the static IP from network data")
		hwData := metal3api.HardwareData{}
		key := types.NamespacedName{Namespace: namespace.Name, Name: bmhName}
		Expect(clusterProxy.GetClient().Get(ctx, key, &hwData)).To(Succeed())
		Expect(hwData.Spec.HardwareDetails).NotTo(BeNil())
		Expect(hwData.Spec.HardwareDetails.NIC).To(
			ContainElement(HaveField("IP", staticIP)),
			"Expected a NIC with the static IP from network data: "+staticIP)

		By("Provisioning the BMH to verify network data defaults to preprovisioning network data")
		var userDataSecret *corev1.SecretReference
		if e2eConfig.GetVariable("SSH_CHECK_PROVISIONED") == "true" {
			userDataSecretName := bmhName + "-user-data"
			sshPubKeyPath := e2eConfig.GetVariable("SSH_PUB_KEY")
			obj := createSSHSetupUserdata(ctx, clusterProxy.GetClient(), namespace.Name, userDataSecretName, sshPubKeyPath, bmc.IPAddress)
			toCleanup = append(toCleanup, obj)
			userDataSecret = &corev1.SecretReference{
				Name:      userDataSecretName,
				Namespace: namespace.Name,
			}
		}
		Expect(PatchBMHForProvisioning(ctx, PatchBMHForProvisioningInput{
			client:         clusterProxy.GetClient(),
			bmh:            &bmh,
			bmc:            bmc,
			e2eConfig:      e2eConfig,
			namespace:      namespace.Name,
			userDataSecret: userDataSecret,
		})).To(Succeed())

		By("Waiting for the BMH to be provisioned")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: clusterProxy.GetClient(),
			Bmh:    bmh,
			State:  metal3api.StateProvisioned,
		}, e2eConfig.GetIntervals(specName, "wait-provisioned")...)

		if e2eConfig.GetVariable("SSH_CHECK_PROVISIONED") == "true" {
			By("Verifying the network data file on the config drive")
			parsed := getNetworkData(specName, bmc.IPAddress)
			Expect(parsed.Links).To(ContainElement(HaveKeyWithValue("ethernet_mac_address", expectedMAC)))
			Expect(parsed.Networks).To(ContainElement(HaveKeyWithValue("ip_address", staticIP)))
		} else {
			Logf("WARNING: Skipping SSH check since SSH_CHECK_PROVISIONED != true")
		}
	})

	It("should apply network data during provisioning", func() {
		if bmc.IPAddress == "" {
			// Fixture tests don't have an IP address we could use
			Skip("network data test requires configuring a static IP address")
		}

		bmhName := specName + "-nd"
		secretName := bmhName + "-bmc"
		networkDataSecretName := bmhName + "-network-data"
		expectedMAC := strings.ToUpper(bmc.BootMacAddress)

		By("Creating a secret with BMH credentials")
		bmcCredentialsData := map[string]string{
			"username": bmc.User,
			"password": bmc.Password,
		}
		secret := CreateSecret(ctx, clusterProxy.GetClient(), namespace.Name, secretName, bmcCredentialsData)
		toCleanup = append(toCleanup, secret)

		By("Creating a network data secret with static IP configuration")
		staticIP := getNewIPAddress()
		networkData := fmt.Sprintf(networkDataTemplate, expectedMAC, staticIP)
		netSecret := CreateSecret(ctx, clusterProxy.GetClient(), namespace.Name, networkDataSecretName, map[string]string{
			"networkData": networkData,
		})
		toCleanup = append(toCleanup, netSecret)

		By("Creating a BMH with network data set, inspection and cleaning disabled")
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
				AutomatedCleaningMode: metal3api.CleaningModeDisabled,
				BootMode:              metal3api.BootMode(e2eConfig.GetVariable("BOOT_MODE")),
				BootMACAddress:        bmc.BootMacAddress,
				InspectionMode:        metal3api.InspectionModeDisabled,
				NetworkData: &corev1.SecretReference{
					Name: networkDataSecretName,
				},
				Online: true,
			},
		}
		Expect(clusterProxy.GetClient().Create(ctx, &bmh)).To(Succeed())
		toCleanup = append(toCleanup, &bmh)

		By("Waiting for the BMH to become available")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: clusterProxy.GetClient(),
			Bmh:    bmh,
			State:  metal3api.StateAvailable,
		}, e2eConfig.GetIntervals(specName, "wait-available")...)

		By("Provisioning the BMH to verify network data")
		var userDataSecret *corev1.SecretReference
		if e2eConfig.GetVariable("SSH_CHECK_PROVISIONED") == "true" {
			userDataSecretName := bmhName + "-user-data"
			sshPubKeyPath := e2eConfig.GetVariable("SSH_PUB_KEY")
			obj := createSSHSetupUserdata(ctx, clusterProxy.GetClient(), namespace.Name, userDataSecretName, sshPubKeyPath, bmc.IPAddress)
			toCleanup = append(toCleanup, obj)
			userDataSecret = &corev1.SecretReference{
				Name:      userDataSecretName,
				Namespace: namespace.Name,
			}
		}
		Expect(PatchBMHForProvisioning(ctx, PatchBMHForProvisioningInput{
			client:         clusterProxy.GetClient(),
			bmh:            &bmh,
			bmc:            bmc,
			e2eConfig:      e2eConfig,
			namespace:      namespace.Name,
			userDataSecret: userDataSecret,
		})).To(Succeed())

		By("Waiting for the BMH to be provisioned")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: clusterProxy.GetClient(),
			Bmh:    bmh,
			State:  metal3api.StateProvisioned,
		}, e2eConfig.GetIntervals(specName, "wait-provisioned")...)

		if e2eConfig.GetVariable("SSH_CHECK_PROVISIONED") == "true" {
			By("Verifying the network data file on the config drive")
			parsed := getNetworkData(specName, bmc.IPAddress)
			Expect(parsed.Links).To(ContainElement(HaveKeyWithValue("ethernet_mac_address", expectedMAC)))
			Expect(parsed.Networks).To(ContainElement(HaveKeyWithValue("ip_address", staticIP)))
		} else {
			Logf("WARNING: Skipping SSH check since SSH_CHECK_PROVISIONED != true")
		}
	})

	AfterEach(func() {
		CollectSerialLogs(bmc.Name, path.Join(artifactFolder, specName))
		DumpResources(ctx, e2eConfig, clusterProxy, path.Join(artifactFolder, specName))
		if !skipCleanup {
			Cleanup(ctx, clusterProxy, namespace, cancelWatches, e2eConfig, toCleanup)
		}
	})
})
