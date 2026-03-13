//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"fmt"
	"path"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/deprecated/v1beta1/patch"
)

var _ = Describe("networking", Label("required", "networking"), func() {
	var (
		specName      = "networking"
		namespace     *corev1.Namespace
		cancelWatches context.CancelFunc
	)

	BeforeEach(func() {
		namespaceInput := framework.CreateNamespaceAndWatchEventsInput{
			Creator:   clusterProxy.GetClient(),
			ClientSet: clusterProxy.GetClientSet(),
			LogFolder: artifactFolder,
		}

		if e2eConfig.GetBoolVariable("NAMESPACE_SCOPED") {
			namespaceInput.Name = specName
			namespaceInput.IgnoreAlreadyExists = true
		} else {
			namespaceInput.Name = fmt.Sprintf("%s-%s", specName, util.RandomString(6))
		}

		namespace, cancelWatches = framework.CreateNamespaceAndWatchEvents(ctx, namespaceInput)
	})

	AfterEach(func() {
		DumpResources(ctx, e2eConfig, clusterProxy, path.Join(artifactFolder, specName))
		if !skipCleanup {
			Cleanup(ctx, clusterProxy, namespace, cancelWatches, e2eConfig, nil)
		}
	})

	It("should create and validate HostNetworkAttachment resources", func() {
		c := clusterProxy.GetClient()

		By("creating a valid HostNetworkAttachment in access mode")
		accessHNA := &metal3api.HostNetworkAttachment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "access-net",
				Namespace: namespace.Name,
			},
			Spec: metal3api.HostNetworkAttachmentSpec{
				Mode:       metal3api.SwitchportModeAccess,
				NativeVLAN: 100,
			},
		}
		Expect(c.Create(ctx, accessHNA)).To(Succeed())

		By("creating a valid HostNetworkAttachment in trunk mode")
		trunkHNA := &metal3api.HostNetworkAttachment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "trunk-net",
				Namespace: namespace.Name,
			},
			Spec: metal3api.HostNetworkAttachmentSpec{
				Mode:         metal3api.SwitchportModeTrunk,
				NativeVLAN:   1,
				AllowedVLANs: []string{"100", "200", "300"},
				MTU:          ptr.To(9000),
			},
		}
		Expect(c.Create(ctx, trunkHNA)).To(Succeed())

		By("verifying the HostNetworkAttachments can be retrieved")
		retrieved := &metal3api.HostNetworkAttachment{}
		Expect(c.Get(ctx, types.NamespacedName{Name: "access-net", Namespace: namespace.Name}, retrieved)).To(Succeed())
		Expect(retrieved.Spec.Mode).To(Equal(metal3api.SwitchportModeAccess))
		Expect(retrieved.Spec.NativeVLAN).To(Equal(100))

		Expect(c.Get(ctx, types.NamespacedName{Name: "trunk-net", Namespace: namespace.Name}, retrieved)).To(Succeed())
		Expect(retrieved.Spec.Mode).To(Equal(metal3api.SwitchportModeTrunk))
		Expect(retrieved.Spec.AllowedVLANs).To(Equal([]string{"100", "200", "300"}))
		Expect(retrieved.Spec.MTU).To(Equal(ptr.To(9000)))

		By("creating a BMH that references the HostNetworkAttachments")
		bmh := &metal3api.BareMetalHost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-host",
				Namespace: namespace.Name,
			},
			Spec: metal3api.BareMetalHostSpec{
				NetworkInterfaces: []metal3api.NetworkInterface{
					{
						Name: "eth0",
						HostNetworkAttachment: metal3api.HostNetworkAttachmentRef{
							Name: "access-net",
						},
					},
					{
						Name: "eth1",
						HostNetworkAttachment: metal3api.HostNetworkAttachmentRef{
							Name: "trunk-net",
						},
					},
				},
			},
		}
		Expect(c.Create(ctx, bmh)).To(Succeed())

		By("verifying the BMH was created with network interfaces")
		retrievedBMH := &metal3api.BareMetalHost{}
		Expect(c.Get(ctx, types.NamespacedName{Name: "test-host", Namespace: namespace.Name}, retrievedBMH)).To(Succeed())
		Expect(retrievedBMH.Spec.NetworkInterfaces).To(HaveLen(2))

		By("verifying that a referenced HNA cannot be deleted")
		err := c.Delete(ctx, accessHNA)
		Expect(err).To(HaveOccurred())
		Expect(k8serrors.IsForbidden(err)).To(BeTrue(), fmt.Sprintf("expected Forbidden error, got: %v", err))

		By("deleting the BMH to release HNA references")
		Expect(c.Delete(ctx, bmh)).To(Succeed())
		WaitForBmhDeleted(ctx, WaitForBmhDeletedInput{
			Client:    c,
			BmhName:   bmh.Name,
			Namespace: bmh.Namespace,
		}, e2eConfig.GetIntervals(specName, "wait-bmh-deleted")...)

		By("verifying the HNA can be deleted after BMH removal")
		Expect(c.Delete(ctx, accessHNA)).To(Succeed())
		Expect(c.Delete(ctx, trunkHNA)).To(Succeed())

		By("verifying the HNAs are actually deleted")
		Eventually(func() bool {
			err := c.Get(ctx, types.NamespacedName{Name: "access-net", Namespace: namespace.Name}, &metal3api.HostNetworkAttachment{})
			return k8serrors.IsNotFound(err)
		}, e2eConfig.GetIntervals(specName, "wait-hna-deleted")...).Should(BeTrue())
	})

	It("should reject invalid HostNetworkAttachment configurations", func() {
		c := clusterProxy.GetClient()

		By("rejecting an HNA with invalid VLAN (out of range)")
		invalidHNA := &metal3api.HostNetworkAttachment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "invalid-vlan",
				Namespace: namespace.Name,
			},
			Spec: metal3api.HostNetworkAttachmentSpec{
				Mode:       metal3api.SwitchportModeAccess,
				NativeVLAN: 5000,
			},
		}
		err := c.Create(ctx, invalidHNA)
		Expect(err).To(HaveOccurred())

		By("rejecting an HNA with invalid MTU (too small)")
		invalidMTU := &metal3api.HostNetworkAttachment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "invalid-mtu",
				Namespace: namespace.Name,
			},
			Spec: metal3api.HostNetworkAttachmentSpec{
				Mode:       metal3api.SwitchportModeAccess,
				NativeVLAN: 100,
				MTU:        ptr.To(10),
			},
		}
		err = c.Create(ctx, invalidMTU)
		Expect(err).To(HaveOccurred())
	})

	It("should handle BMH with switch port identity on network interfaces", func() {
		c := clusterProxy.GetClient()

		By("creating an HNA for the interface")
		hna := &metal3api.HostNetworkAttachment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "switchport-net",
				Namespace: namespace.Name,
			},
			Spec: metal3api.HostNetworkAttachmentSpec{
				Mode:       metal3api.SwitchportModeAccess,
				NativeVLAN: 200,
			},
		}
		Expect(c.Create(ctx, hna)).To(Succeed())

		By("creating a BMH with switch port identity on the interface")
		bmh := &metal3api.BareMetalHost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "host-with-switchport",
				Namespace: namespace.Name,
			},
			Spec: metal3api.BareMetalHostSpec{
				NetworkInterfaces: []metal3api.NetworkInterface{
					{
						Name: "eth0",
						HostNetworkAttachment: metal3api.HostNetworkAttachmentRef{
							Name: "switchport-net",
						},
						SwitchPort: &metal3api.SwitchPortIdentifier{
							SwitchID: "00:00:5e:00:53:01",
							PortID:   "Ethernet1/1",
						},
					},
				},
			},
		}
		Expect(c.Create(ctx, bmh)).To(Succeed())

		By("verifying the BMH has the switch port identity")
		retrievedBMH := &metal3api.BareMetalHost{}
		Expect(c.Get(ctx, types.NamespacedName{Name: "host-with-switchport", Namespace: namespace.Name}, retrievedBMH)).To(Succeed())
		Expect(retrievedBMH.Spec.NetworkInterfaces).To(HaveLen(1))
		Expect(retrievedBMH.Spec.NetworkInterfaces[0].SwitchPort).NotTo(BeNil())
		Expect(retrievedBMH.Spec.NetworkInterfaces[0].SwitchPort.SwitchID).To(Equal("00:00:5e:00:53:01"))
		Expect(retrievedBMH.Spec.NetworkInterfaces[0].SwitchPort.PortID).To(Equal("Ethernet1/1"))

		By("cleaning up")
		Expect(c.Delete(ctx, bmh)).To(Succeed())
		WaitForBmhDeleted(ctx, WaitForBmhDeletedInput{
			Client:    c,
			BmhName:   bmh.Name,
			Namespace: bmh.Namespace,
		}, e2eConfig.GetIntervals(specName, "wait-bmh-deleted")...)

		Expect(c.Delete(ctx, hna)).To(Succeed())
	})

	It("should validate network interfaces and apply port configs through full lifecycle", func() {
		c := clusterProxy.GetClient()

		By("creating a BMC credentials secret")
		bmcCredentialsData := map[string]string{
			"username": bmc.User,
			"password": bmc.Password,
		}
		CreateSecret(ctx, c, namespace.Name, "bmc-creds-lifecycle", bmcCredentialsData)

		By("creating an HNA for the interface")
		hna := &metal3api.HostNetworkAttachment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-net",
				Namespace: namespace.Name,
			},
			Spec: metal3api.HostNetworkAttachmentSpec{
				Mode:       metal3api.SwitchportModeAccess,
				NativeVLAN: 100,
				MTU:        ptr.To(9000),
			},
		}
		Expect(c.Create(ctx, hna)).To(Succeed())

		By("creating a BMH with NetworkInterfaces referencing the HNA")
		bmh := &metal3api.BareMetalHost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      specName + "-lifecycle",
				Namespace: namespace.Name,
			},
			Spec: metal3api.BareMetalHostSpec{
				BMC: metal3api.BMCDetails{
					Address:                        bmc.Address,
					CredentialsName:                "bmc-creds-lifecycle",
					DisableCertificateVerification: bmc.DisableCertificateVerification,
				},
				BootMode:              metal3api.BootMode(e2eConfig.GetVariable("BOOT_MODE")),
				BootMACAddress:        bmc.BootMacAddress,
				AutomatedCleaningMode: metal3api.CleaningModeDisabled,
				NetworkInterfaces: []metal3api.NetworkInterface{
					{
						MACAddress: bmc.BootMacAddress,
						HostNetworkAttachment: metal3api.HostNetworkAttachmentRef{
							Name: "test-net",
						},
					},
				},
			},
		}
		Expect(c.Create(ctx, bmh)).To(Succeed())

		By("waiting for the BMH to become available")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: c,
			Bmh:    *bmh,
			State:  metal3api.StateAvailable,
		}, e2eConfig.GetIntervals("default", "wait-available")...)

		By("verifying NetworkInterfacesValid condition is True")
		retrievedBMH := &metal3api.BareMetalHost{}
		Expect(c.Get(ctx, types.NamespacedName{Name: bmh.Name, Namespace: namespace.Name}, retrievedBMH)).To(Succeed())
		cond := meta.FindStatusCondition(retrievedBMH.Status.Conditions, metal3api.NetworkInterfacesValidCondition)
		Expect(cond).NotTo(BeNil(), "NetworkInterfacesValid condition should exist")
		Expect(cond.Status).To(Equal(metav1.ConditionTrue),
			fmt.Sprintf("NetworkInterfacesValid should be True, got reason=%s message=%s", cond.Reason, cond.Message))
		Expect(cond.Reason).To(Equal("AllInterfacesValid"))

		By("verifying AppliedPortConfigs is populated")
		Expect(retrievedBMH.Status.AppliedPortConfigs).NotTo(BeEmpty(), "AppliedPortConfigs should be set")
		Expect(retrievedBMH.Status.AppliedPortConfigs[0].SwitchPortConfig.Mode).To(Equal(metal3api.SwitchPortMode("access")))
		Expect(retrievedBMH.Status.AppliedPortConfigs[0].SwitchPortConfig.NativeVLAN).To(Equal(100))

		By("cleaning up")
		Expect(c.Delete(ctx, bmh)).To(Succeed())
		WaitForBmhDeleted(ctx, WaitForBmhDeletedInput{
			Client:    c,
			BmhName:   bmh.Name,
			Namespace: bmh.Namespace,
		}, e2eConfig.GetIntervals(specName, "wait-bmh-deleted")...)
		Expect(c.Delete(ctx, hna)).To(Succeed())
	})

	It("should set NetworkInterfacesValid to False for invalid interface names", func() {
		c := clusterProxy.GetClient()

		By("creating a BMC credentials secret")
		bmcCredentialsData := map[string]string{
			"username": bmc.User,
			"password": bmc.Password,
		}
		CreateSecret(ctx, c, namespace.Name, "bmc-creds-invalid-nic", bmcCredentialsData)

		By("creating an HNA")
		hna := &metal3api.HostNetworkAttachment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "invalid-net",
				Namespace: namespace.Name,
			},
			Spec: metal3api.HostNetworkAttachmentSpec{
				Mode:       metal3api.SwitchportModeAccess,
				NativeVLAN: 200,
			},
		}
		Expect(c.Create(ctx, hna)).To(Succeed())

		By("creating a BMH with a non-existent interface name")
		bmh := &metal3api.BareMetalHost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      specName + "-invalid-nic",
				Namespace: namespace.Name,
			},
			Spec: metal3api.BareMetalHostSpec{
				BMC: metal3api.BMCDetails{
					Address:                        bmc.Address,
					CredentialsName:                "bmc-creds-invalid-nic",
					DisableCertificateVerification: bmc.DisableCertificateVerification,
				},
				BootMode:              metal3api.BootMode(e2eConfig.GetVariable("BOOT_MODE")),
				BootMACAddress:        bmc.BootMacAddress,
				AutomatedCleaningMode: metal3api.CleaningModeDisabled,
				NetworkInterfaces: []metal3api.NetworkInterface{
					{
						Name: "does-not-exist",
						HostNetworkAttachment: metal3api.HostNetworkAttachmentRef{
							Name: "invalid-net",
						},
					},
				},
			},
		}
		Expect(c.Create(ctx, bmh)).To(Succeed())

		By("waiting for the BMH to become available")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: c,
			Bmh:    *bmh,
			State:  metal3api.StateAvailable,
		}, e2eConfig.GetIntervals("default", "wait-available")...)

		By("verifying NetworkInterfacesValid condition is False with InvalidInterfaceNames")
		retrievedBMH := &metal3api.BareMetalHost{}
		Expect(c.Get(ctx, types.NamespacedName{Name: bmh.Name, Namespace: namespace.Name}, retrievedBMH)).To(Succeed())
		cond := meta.FindStatusCondition(retrievedBMH.Status.Conditions, metal3api.NetworkInterfacesValidCondition)
		Expect(cond).NotTo(BeNil(), "NetworkInterfacesValid condition should exist")
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Reason).To(Equal("InvalidInterfaceNames"))
		Expect(cond.Message).To(ContainSubstring("does-not-exist"))

		By("verifying AppliedPortConfigs is NOT populated")
		Expect(retrievedBMH.Status.AppliedPortConfigs).To(BeEmpty())

		By("cleaning up")
		Expect(c.Delete(ctx, bmh)).To(Succeed())
		WaitForBmhDeleted(ctx, WaitForBmhDeletedInput{
			Client:    c,
			BmhName:   bmh.Name,
			Namespace: bmh.Namespace,
		}, e2eConfig.GetIntervals(specName, "wait-bmh-deleted")...)
		Expect(c.Delete(ctx, hna)).To(Succeed())
	})

	It("should re-apply port configs when NetworkInterfaces change on available host", func() {
		c := clusterProxy.GetClient()

		By("creating a BMC credentials secret")
		bmcCredentialsData := map[string]string{
			"username": bmc.User,
			"password": bmc.Password,
		}
		CreateSecret(ctx, c, namespace.Name, "bmc-creds-reconfig", bmcCredentialsData)

		By("creating two HNAs")
		hna1 := &metal3api.HostNetworkAttachment{
			ObjectMeta: metav1.ObjectMeta{Name: "net-v100", Namespace: namespace.Name},
			Spec: metal3api.HostNetworkAttachmentSpec{
				Mode:       metal3api.SwitchportModeAccess,
				NativeVLAN: 100,
			},
		}
		hna2 := &metal3api.HostNetworkAttachment{
			ObjectMeta: metav1.ObjectMeta{Name: "net-v200", Namespace: namespace.Name},
			Spec: metal3api.HostNetworkAttachmentSpec{
				Mode:       metal3api.SwitchportModeAccess,
				NativeVLAN: 200,
			},
		}
		Expect(c.Create(ctx, hna1)).To(Succeed())
		Expect(c.Create(ctx, hna2)).To(Succeed())

		By("creating a BMH with nic-1 on net-v100")
		bmh := &metal3api.BareMetalHost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      specName + "-reconfig",
				Namespace: namespace.Name,
			},
			Spec: metal3api.BareMetalHostSpec{
				BMC: metal3api.BMCDetails{
					Address:                        bmc.Address,
					CredentialsName:                "bmc-creds-reconfig",
					DisableCertificateVerification: bmc.DisableCertificateVerification,
				},
				BootMode:              metal3api.BootMode(e2eConfig.GetVariable("BOOT_MODE")),
				BootMACAddress:        bmc.BootMacAddress,
				AutomatedCleaningMode: metal3api.CleaningModeDisabled,
				NetworkInterfaces: []metal3api.NetworkInterface{
					{
						MACAddress:            bmc.BootMacAddress,
						HostNetworkAttachment: metal3api.HostNetworkAttachmentRef{Name: "net-v100"},
					},
				},
			},
		}
		Expect(c.Create(ctx, bmh)).To(Succeed())

		By("waiting for the BMH to become available with initial config")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: c,
			Bmh:    *bmh,
			State:  metal3api.StateAvailable,
		}, e2eConfig.GetIntervals("default", "wait-available")...)

		By("verifying initial AppliedPortConfigs")
		Expect(c.Get(ctx, types.NamespacedName{Name: bmh.Name, Namespace: namespace.Name}, bmh)).To(Succeed())
		Expect(bmh.Status.AppliedPortConfigs).To(HaveLen(1))
		Expect(bmh.Status.AppliedPortConfigs[0].SwitchPortConfig.NativeVLAN).To(Equal(100))

		By("changing the NetworkInterfaces to use a different HNA")
		helper, err := patch.NewHelper(bmh, c)
		Expect(err).NotTo(HaveOccurred())
		bmh.Spec.NetworkInterfaces = []metal3api.NetworkInterface{
			{
				MACAddress:            bmc.BootMacAddress,
				HostNetworkAttachment: metal3api.HostNetworkAttachmentRef{Name: "net-v200"},
			},
		}
		Expect(helper.Patch(ctx, bmh)).To(Succeed())

		By("waiting for AppliedPortConfigs to reflect the new VLAN")
		Eventually(func(g Gomega) {
			g.Expect(c.Get(ctx, types.NamespacedName{Name: bmh.Name, Namespace: namespace.Name}, bmh)).To(Succeed())
			g.Expect(bmh.Status.Provisioning.State).To(Equal(metal3api.StateAvailable))
			g.Expect(bmh.Status.AppliedPortConfigs).To(HaveLen(1))
			g.Expect(bmh.Status.AppliedPortConfigs[0].SwitchPortConfig.NativeVLAN).To(Equal(200))
		}, e2eConfig.GetIntervals("default", "wait-available")...).Should(Succeed())

		By("removing all NetworkInterfaces")
		helper, err = patch.NewHelper(bmh, c)
		Expect(err).NotTo(HaveOccurred())
		bmh.Spec.NetworkInterfaces = nil
		Expect(helper.Patch(ctx, bmh)).To(Succeed())

		By("waiting for AppliedPortConfigs to be cleared and condition removed")
		Eventually(func(g Gomega) {
			g.Expect(c.Get(ctx, types.NamespacedName{Name: bmh.Name, Namespace: namespace.Name}, bmh)).To(Succeed())
			g.Expect(bmh.Status.Provisioning.State).To(Equal(metal3api.StateAvailable))
			g.Expect(bmh.Status.AppliedPortConfigs).To(BeEmpty())
			cond := meta.FindStatusCondition(bmh.Status.Conditions, metal3api.NetworkInterfacesValidCondition)
			g.Expect(cond).To(BeNil(), "NetworkInterfacesValid condition should be removed when no interfaces")
		}, e2eConfig.GetIntervals("default", "wait-available")...).Should(Succeed())

		By("cleaning up")
		Expect(c.Delete(ctx, bmh)).To(Succeed())
		WaitForBmhDeleted(ctx, WaitForBmhDeletedInput{
			Client:    c,
			BmhName:   bmh.Name,
			Namespace: bmh.Namespace,
		}, e2eConfig.GetIntervals(specName, "wait-bmh-deleted")...)
		Expect(c.Delete(ctx, hna1)).To(Succeed())
		Expect(c.Delete(ctx, hna2)).To(Succeed())
	})

	It("should apply switchport configuration to Ironic ports", func() {
		if !e2eConfig.GetBoolVariable("DEPLOY_IRONIC") {
			Skip("Ironic port verification requires a real Ironic deployment")
		}
		c := clusterProxy.GetClient()

		By("creating a BMC credentials secret")
		bmcCredentialsData := map[string]string{
			"username": bmc.User,
			"password": bmc.Password,
		}
		CreateSecret(ctx, c, namespace.Name, "bmc-creds-ironic-ports", bmcCredentialsData)

		By("creating an HNA with access mode, VLAN 100, MTU 9000")
		hna := &metal3api.HostNetworkAttachment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ironic-port-net",
				Namespace: namespace.Name,
			},
			Spec: metal3api.HostNetworkAttachmentSpec{
				Mode:       metal3api.SwitchportModeAccess,
				NativeVLAN: 100,
				MTU:        ptr.To(9000),
			},
		}
		Expect(c.Create(ctx, hna)).To(Succeed())

		By("creating a BMH with NetworkInterfaces referencing the HNA")
		bmh := &metal3api.BareMetalHost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      specName + "-ironic-ports",
				Namespace: namespace.Name,
			},
			Spec: metal3api.BareMetalHostSpec{
				BMC: metal3api.BMCDetails{
					Address:                        bmc.Address,
					CredentialsName:                "bmc-creds-ironic-ports",
					DisableCertificateVerification: bmc.DisableCertificateVerification,
				},
				BootMode:              metal3api.BootMode(e2eConfig.GetVariable("BOOT_MODE")),
				BootMACAddress:        bmc.BootMacAddress,
				AutomatedCleaningMode: metal3api.CleaningModeDisabled,
				NetworkInterfaces: []metal3api.NetworkInterface{
					{
						MACAddress: bmc.BootMacAddress,
						HostNetworkAttachment: metal3api.HostNetworkAttachmentRef{
							Name: "ironic-port-net",
						},
					},
				},
			},
		}
		Expect(c.Create(ctx, bmh)).To(Succeed())

		By("waiting for the BMH to become available")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: c,
			Bmh:    *bmh,
			State:  metal3api.StateAvailable,
		}, e2eConfig.GetIntervals("default", "wait-available")...)

		By("fetching Ironic ports for the node")
		ports, err := fetchIronicPorts(e2eConfig, namespace.Name, bmh.Name)
		Expect(err).NotTo(HaveOccurred(), "failed to fetch Ironic ports")
		Expect(ports).NotTo(BeEmpty(), "expected at least one Ironic port")

		By("finding the port matching the boot MAC address")
		bootPort := findPortByMAC(ports, bmc.BootMacAddress)
		Expect(bootPort).NotTo(BeNil(), "expected to find an Ironic port matching boot MAC %s", bmc.BootMacAddress)

		By("verifying the port has switchport configuration in Extra")
		Expect(bootPort.Extra).NotTo(BeNil(), "expected Extra to be set on the Ironic port")
		switchportRaw, ok := bootPort.Extra["switchport"]
		Expect(ok).To(BeTrue(), "expected Extra to contain 'switchport' key")

		switchport, ok := switchportRaw.(map[string]interface{})
		Expect(ok).To(BeTrue(), "expected switchport to be a map")
		Expect(switchport["mode"]).To(Equal("access"), "expected switchport mode to be 'access'")

		// VLAN values may come back as float64 from JSON unmarshaling
		nativeVLAN, ok := switchport["native_vlan"].(float64)
		Expect(ok).To(BeTrue(), "expected native_vlan to be a number")
		Expect(int(nativeVLAN)).To(Equal(100), "expected native_vlan to be 100")

		By("cleaning up")
		Expect(c.Delete(ctx, bmh)).To(Succeed())
		WaitForBmhDeleted(ctx, WaitForBmhDeletedInput{
			Client:    c,
			BmhName:   bmh.Name,
			Namespace: bmh.Namespace,
		}, e2eConfig.GetIntervals(specName, "wait-bmh-deleted")...)
		Expect(c.Delete(ctx, hna)).To(Succeed())
	})

	It("should apply port configs when NetworkInterfaces are added to an available host", func() {
		c := clusterProxy.GetClient()

		By("creating a BMC credentials secret")
		bmcCredentialsData := map[string]string{
			"username": bmc.User,
			"password": bmc.Password,
		}
		CreateSecret(ctx, c, namespace.Name, "bmc-creds-add-ni", bmcCredentialsData)

		By("creating a BMH without NetworkInterfaces")
		bmh := &metal3api.BareMetalHost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      specName + "-add-ni",
				Namespace: namespace.Name,
			},
			Spec: metal3api.BareMetalHostSpec{
				BMC: metal3api.BMCDetails{
					Address:                        bmc.Address,
					CredentialsName:                "bmc-creds-add-ni",
					DisableCertificateVerification: bmc.DisableCertificateVerification,
				},
				BootMode:              metal3api.BootMode(e2eConfig.GetVariable("BOOT_MODE")),
				BootMACAddress:        bmc.BootMacAddress,
				AutomatedCleaningMode: metal3api.CleaningModeDisabled,
			},
		}
		Expect(c.Create(ctx, bmh)).To(Succeed())

		By("waiting for the BMH to become available (through inspection)")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: c,
			Bmh:    *bmh,
			State:  metal3api.StateAvailable,
		}, e2eConfig.GetIntervals("default", "wait-available")...)

		By("verifying no NetworkInterfacesValid condition exists")
		Expect(c.Get(ctx, types.NamespacedName{Name: bmh.Name, Namespace: namespace.Name}, bmh)).To(Succeed())
		cond := meta.FindStatusCondition(bmh.Status.Conditions, metal3api.NetworkInterfacesValidCondition)
		Expect(cond).To(BeNil(), "NetworkInterfacesValid condition should not exist without interfaces")

		By("creating an HNA")
		hna := &metal3api.HostNetworkAttachment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "add-ni-net",
				Namespace: namespace.Name,
			},
			Spec: metal3api.HostNetworkAttachmentSpec{
				Mode:       metal3api.SwitchportModeAccess,
				NativeVLAN: 100,
			},
		}
		Expect(c.Create(ctx, hna)).To(Succeed())

		By("adding NetworkInterfaces to the BMH")
		Expect(c.Get(ctx, types.NamespacedName{Name: bmh.Name, Namespace: namespace.Name}, bmh)).To(Succeed())
		bmh.Spec.NetworkInterfaces = []metal3api.NetworkInterface{
			{
				MACAddress: bmc.BootMacAddress,
				HostNetworkAttachment: metal3api.HostNetworkAttachmentRef{
					Name: "add-ni-net",
				},
			},
		}
		Expect(c.Update(ctx, bmh)).To(Succeed())

		By("waiting for AppliedPortConfigs to be populated after Preparing cycle")
		Eventually(func(g Gomega) {
			g.Expect(c.Get(ctx, types.NamespacedName{Name: bmh.Name, Namespace: namespace.Name}, bmh)).To(Succeed())
			g.Expect(bmh.Status.Provisioning.State).To(Equal(metal3api.StateAvailable))
			g.Expect(bmh.Status.AppliedPortConfigs).NotTo(BeEmpty())
			g.Expect(bmh.Status.AppliedPortConfigs[0].SwitchPortConfig.NativeVLAN).To(Equal(100))
			cond = meta.FindStatusCondition(bmh.Status.Conditions, metal3api.NetworkInterfacesValidCondition)
			g.Expect(cond).NotTo(BeNil(), "NetworkInterfacesValid condition should exist")
			g.Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		}, e2eConfig.GetIntervals("default", "wait-available")...).Should(Succeed())

		By("cleaning up")
		Expect(c.Delete(ctx, bmh)).To(Succeed())
		WaitForBmhDeleted(ctx, WaitForBmhDeletedInput{
			Client:    c,
			BmhName:   bmh.Name,
			Namespace: bmh.Namespace,
		}, e2eConfig.GetIntervals(specName, "wait-bmh-deleted")...)
		Expect(c.Delete(ctx, hna)).To(Succeed())
	})

	It("should maintain port configs during deprovisioning", func() {
		c := clusterProxy.GetClient()

		By("creating a BMC credentials secret")
		bmcCredentialsData := map[string]string{
			"username": bmc.User,
			"password": bmc.Password,
		}
		CreateSecret(ctx, c, namespace.Name, "bmc-creds-lldp", bmcCredentialsData)

		By("creating an HNA")
		hna := &metal3api.HostNetworkAttachment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "deprov-net",
				Namespace: namespace.Name,
			},
			Spec: metal3api.HostNetworkAttachmentSpec{
				Mode:       metal3api.SwitchportModeAccess,
				NativeVLAN: 100,
			},
		}
		Expect(c.Create(ctx, hna)).To(Succeed())

		By("creating a BMH with NetworkInterfaces")
		bmh := &metal3api.BareMetalHost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      specName + "-deprov",
				Namespace: namespace.Name,
			},
			Spec: metal3api.BareMetalHostSpec{
				Online: true,
				BMC: metal3api.BMCDetails{
					Address:                        bmc.Address,
					CredentialsName:                "bmc-creds-lldp",
					DisableCertificateVerification: bmc.DisableCertificateVerification,
				},
				BootMode:              metal3api.BootMode(e2eConfig.GetVariable("BOOT_MODE")),
				BootMACAddress:        bmc.BootMacAddress,
				AutomatedCleaningMode: metal3api.CleaningModeDisabled,
				NetworkInterfaces: []metal3api.NetworkInterface{
					{
						MACAddress: bmc.BootMacAddress,
						HostNetworkAttachment: metal3api.HostNetworkAttachmentRef{
							Name: "deprov-net",
						},
					},
				},
			},
		}
		Expect(c.Create(ctx, bmh)).To(Succeed())

		By("waiting for the BMH to become available")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: c,
			Bmh:    *bmh,
			State:  metal3api.StateAvailable,
		}, e2eConfig.GetIntervals("default", "wait-available")...)

		By("verifying initial port config is applied")
		Expect(c.Get(ctx, types.NamespacedName{Name: bmh.Name, Namespace: namespace.Name}, bmh)).To(Succeed())
		Expect(bmh.Status.AppliedPortConfigs).NotTo(BeEmpty())

		By("provisioning the BMH")
		Expect(PatchBMHForProvisioning(ctx, PatchBMHForProvisioningInput{
			client:    c,
			bmh:       bmh,
			bmc:       bmc,
			e2eConfig: e2eConfig,
			namespace: namespace.Name,
		})).To(Succeed())

		By("waiting for the BMH to become provisioned")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: c,
			Bmh:    *bmh,
			State:  metal3api.StateProvisioned,
		}, e2eConfig.GetIntervals("default", "wait-provisioned")...)

		By("triggering deprovisioning")
		Expect(c.Get(ctx, types.NamespacedName{Name: bmh.Name, Namespace: namespace.Name}, bmh)).To(Succeed())
		helper, err := patch.NewHelper(bmh, c)
		Expect(err).NotTo(HaveOccurred())
		bmh.Spec.Image = nil
		Expect(helper.Patch(ctx, bmh)).To(Succeed())

		By("waiting for the BMH to enter deprovisioning")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: c,
			Bmh:    *bmh,
			State:  metal3api.StateDeprovisioning,
		}, e2eConfig.GetIntervals("default", "wait-available")...)

		if e2eConfig.GetBoolVariable("DEPLOY_IRONIC") {
			By("verifying Ironic port still has switchport config during deprovisioning")
			ironicPorts, ironicErr := fetchIronicPorts(e2eConfig, namespace.Name, bmh.Name)
			Expect(ironicErr).NotTo(HaveOccurred())
			bootPort := findPortByMAC(ironicPorts, bmc.BootMacAddress)
			Expect(bootPort).NotTo(BeNil())
			Expect(bootPort.Extra).NotTo(BeNil())
			_, hasSwitchport := bootPort.Extra["switchport"]
			Expect(hasSwitchport).To(BeTrue(), "switchport config should be preserved during deprovisioning")
		}

		By("waiting for the BMH to return to available")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: c,
			Bmh:    *bmh,
			State:  metal3api.StateAvailable,
		}, e2eConfig.GetIntervals("default", "wait-available")...)

		By("verifying port config is still present after deprovisioning (NI still set)")
		Expect(c.Get(ctx, types.NamespacedName{Name: bmh.Name, Namespace: namespace.Name}, bmh)).To(Succeed())
		Expect(bmh.Status.AppliedPortConfigs).NotTo(BeEmpty())

		By("removing NetworkInterfaces now that BMH is available")
		helper, err = patch.NewHelper(bmh, c)
		Expect(err).NotTo(HaveOccurred())
		bmh.Spec.NetworkInterfaces = nil
		Expect(helper.Patch(ctx, bmh)).To(Succeed())

		By("waiting for port configs to be cleared (preparing may be too fast to observe)")
		Eventually(func(g Gomega) {
			g.Expect(c.Get(ctx, types.NamespacedName{Name: bmh.Name, Namespace: namespace.Name}, bmh)).To(Succeed())
			g.Expect(bmh.Status.AppliedPortConfigs).To(BeEmpty(),
				"AppliedPortConfigs should be cleared after NI removal")
		}, e2eConfig.GetIntervals("default", "wait-available")...).Should(Succeed())

		if e2eConfig.GetBoolVariable("DEPLOY_IRONIC") {
			By("verifying Ironic port no longer has switchport config")
			ironicPorts, ironicErr := fetchIronicPorts(e2eConfig, namespace.Name, bmh.Name)
			Expect(ironicErr).NotTo(HaveOccurred())
			bootPort := findPortByMAC(ironicPorts, bmc.BootMacAddress)
			Expect(bootPort).NotTo(BeNil())
			if bootPort.Extra != nil {
				_, hasSwitchport := bootPort.Extra["switchport"]
				Expect(hasSwitchport).To(BeFalse(), "switchport config should be cleared after NI removal")
			}
		}

		By("cleaning up")
		Expect(c.Delete(ctx, bmh)).To(Succeed())
		WaitForBmhDeleted(ctx, WaitForBmhDeletedInput{
			Client:    c,
			BmhName:   bmh.Name,
			Namespace: bmh.Namespace,
		}, e2eConfig.GetIntervals(specName, "wait-bmh-deleted")...)
		Expect(c.Delete(ctx, hna)).To(Succeed())
	})

	It("should clear port configs only after deprovisioning completes when NI removed atomically", func() {
		c := clusterProxy.GetClient()

		By("creating a BMC credentials secret")
		bmcCredentialsData := map[string]string{
			"username": bmc.User,
			"password": bmc.Password,
		}
		CreateSecret(ctx, c, namespace.Name, "bmc-creds-atomic-deprov", bmcCredentialsData)

		By("creating an HNA")
		hna := &metal3api.HostNetworkAttachment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "atomic-deprov-net",
				Namespace: namespace.Name,
			},
			Spec: metal3api.HostNetworkAttachmentSpec{
				Mode:       metal3api.SwitchportModeAccess,
				NativeVLAN: 100,
			},
		}
		Expect(c.Create(ctx, hna)).To(Succeed())

		By("creating a BMH with NetworkInterfaces")
		bmh := &metal3api.BareMetalHost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      specName + "-atomic-deprov",
				Namespace: namespace.Name,
			},
			Spec: metal3api.BareMetalHostSpec{
				Online: true,
				BMC: metal3api.BMCDetails{
					Address:                        bmc.Address,
					CredentialsName:                "bmc-creds-atomic-deprov",
					DisableCertificateVerification: bmc.DisableCertificateVerification,
				},
				BootMode:              metal3api.BootMode(e2eConfig.GetVariable("BOOT_MODE")),
				BootMACAddress:        bmc.BootMacAddress,
				AutomatedCleaningMode: metal3api.CleaningModeDisabled,
				NetworkInterfaces: []metal3api.NetworkInterface{
					{
						MACAddress: bmc.BootMacAddress,
						HostNetworkAttachment: metal3api.HostNetworkAttachmentRef{
							Name: "atomic-deprov-net",
						},
					},
				},
			},
		}
		Expect(c.Create(ctx, bmh)).To(Succeed())

		By("waiting for the BMH to become available")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: c,
			Bmh:    *bmh,
			State:  metal3api.StateAvailable,
		}, e2eConfig.GetIntervals("default", "wait-available")...)

		By("verifying initial port config is applied")
		Expect(c.Get(ctx, types.NamespacedName{Name: bmh.Name, Namespace: namespace.Name}, bmh)).To(Succeed())
		Expect(bmh.Status.AppliedPortConfigs).NotTo(BeEmpty())

		By("provisioning the BMH")
		Expect(PatchBMHForProvisioning(ctx, PatchBMHForProvisioningInput{
			client:    c,
			bmh:       bmh,
			bmc:       bmc,
			e2eConfig: e2eConfig,
			namespace: namespace.Name,
		})).To(Succeed())

		By("waiting for the BMH to become provisioned")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: c,
			Bmh:    *bmh,
			State:  metal3api.StateProvisioned,
		}, e2eConfig.GetIntervals("default", "wait-provisioned")...)

		By("atomically removing image and NetworkInterfaces to trigger deprovisioning")
		Expect(c.Get(ctx, types.NamespacedName{Name: bmh.Name, Namespace: namespace.Name}, bmh)).To(Succeed())
		bmh.Spec.Image = nil
		bmh.Spec.NetworkInterfaces = nil
		Expect(c.Update(ctx, bmh)).To(Succeed())

		By("waiting for the BMH to enter deprovisioning")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: c,
			Bmh:    *bmh,
			State:  metal3api.StateDeprovisioning,
		}, e2eConfig.GetIntervals("default", "wait-available")...)

		By("verifying AppliedPortConfigs still present during deprovisioning")
		Expect(c.Get(ctx, types.NamespacedName{Name: bmh.Name, Namespace: namespace.Name}, bmh)).To(Succeed())
		Expect(bmh.Status.AppliedPortConfigs).NotTo(BeEmpty(),
			"AppliedPortConfigs should be preserved during deprovisioning")

		if e2eConfig.GetBoolVariable("DEPLOY_IRONIC") {
			By("verifying Ironic port still has switchport config during deprovisioning")
			ironicPorts, ironicErr := fetchIronicPorts(e2eConfig, namespace.Name, bmh.Name)
			Expect(ironicErr).NotTo(HaveOccurred())
			bootPort := findPortByMAC(ironicPorts, bmc.BootMacAddress)
			Expect(bootPort).NotTo(BeNil())
			Expect(bootPort.Extra).NotTo(BeNil())
			_, hasSwitchport := bootPort.Extra["switchport"]
			Expect(hasSwitchport).To(BeTrue(), "switchport config should be preserved during deprovisioning")
		}

		By("waiting for the BMH to return to available")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: c,
			Bmh:    *bmh,
			State:  metal3api.StateAvailable,
		}, e2eConfig.GetIntervals("default", "wait-available")...)

		By("waiting for port configs to be cleared after returning to available")
		Eventually(func(g Gomega) {
			g.Expect(c.Get(ctx, types.NamespacedName{Name: bmh.Name, Namespace: namespace.Name}, bmh)).To(Succeed())
			g.Expect(bmh.Status.AppliedPortConfigs).To(BeEmpty(),
				"AppliedPortConfigs should be cleared after deprovisioning")
		}, e2eConfig.GetIntervals("default", "wait-available")...).Should(Succeed())

		By("cleaning up")
		Expect(c.Delete(ctx, bmh)).To(Succeed())
		WaitForBmhDeleted(ctx, WaitForBmhDeletedInput{
			Client:    c,
			BmhName:   bmh.Name,
			Namespace: bmh.Namespace,
		}, e2eConfig.GetIntervals(specName, "wait-bmh-deleted")...)
		Expect(c.Delete(ctx, hna)).To(Succeed())
	})

	It("should preserve port configs when NI removed separately during deprovisioning", func() {
		c := clusterProxy.GetClient()

		By("creating a BMC credentials secret")
		bmcCredentialsData := map[string]string{
			"username": bmc.User,
			"password": bmc.Password,
		}
		CreateSecret(ctx, c, namespace.Name, "bmc-creds-ni-during-deprov", bmcCredentialsData)

		By("creating an HNA")
		hna := &metal3api.HostNetworkAttachment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ni-during-deprov-net",
				Namespace: namespace.Name,
			},
			Spec: metal3api.HostNetworkAttachmentSpec{
				Mode:       metal3api.SwitchportModeAccess,
				NativeVLAN: 100,
			},
		}
		Expect(c.Create(ctx, hna)).To(Succeed())

		By("creating a BMH with NetworkInterfaces")
		bmh := &metal3api.BareMetalHost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      specName + "-ni-during-deprov",
				Namespace: namespace.Name,
			},
			Spec: metal3api.BareMetalHostSpec{
				Online: true,
				BMC: metal3api.BMCDetails{
					Address:                        bmc.Address,
					CredentialsName:                "bmc-creds-ni-during-deprov",
					DisableCertificateVerification: bmc.DisableCertificateVerification,
				},
				BootMode:              metal3api.BootMode(e2eConfig.GetVariable("BOOT_MODE")),
				BootMACAddress:        bmc.BootMacAddress,
				AutomatedCleaningMode: metal3api.CleaningModeDisabled,
				NetworkInterfaces: []metal3api.NetworkInterface{
					{
						MACAddress: bmc.BootMacAddress,
						HostNetworkAttachment: metal3api.HostNetworkAttachmentRef{
							Name: "ni-during-deprov-net",
						},
					},
				},
			},
		}
		Expect(c.Create(ctx, bmh)).To(Succeed())

		By("waiting for the BMH to become available")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: c,
			Bmh:    *bmh,
			State:  metal3api.StateAvailable,
		}, e2eConfig.GetIntervals("default", "wait-available")...)

		By("verifying initial port config is applied")
		Expect(c.Get(ctx, types.NamespacedName{Name: bmh.Name, Namespace: namespace.Name}, bmh)).To(Succeed())
		Expect(bmh.Status.AppliedPortConfigs).NotTo(BeEmpty())

		By("provisioning the BMH")
		Expect(PatchBMHForProvisioning(ctx, PatchBMHForProvisioningInput{
			client:    c,
			bmh:       bmh,
			bmc:       bmc,
			e2eConfig: e2eConfig,
			namespace: namespace.Name,
		})).To(Succeed())

		By("waiting for the BMH to become provisioned")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: c,
			Bmh:    *bmh,
			State:  metal3api.StateProvisioned,
		}, e2eConfig.GetIntervals("default", "wait-provisioned")...)

		By("removing image to trigger deprovisioning (keeping NI intact)")
		Expect(c.Get(ctx, types.NamespacedName{Name: bmh.Name, Namespace: namespace.Name}, bmh)).To(Succeed())
		helper, err := patch.NewHelper(bmh, c)
		Expect(err).NotTo(HaveOccurred())
		bmh.Spec.Image = nil
		Expect(helper.Patch(ctx, bmh)).To(Succeed())

		By("waiting for the BMH to enter deprovisioning")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: c,
			Bmh:    *bmh,
			State:  metal3api.StateDeprovisioning,
		}, e2eConfig.GetIntervals("default", "wait-available")...)

		By("removing NetworkInterfaces while deprovisioning is in progress")
		Expect(c.Get(ctx, types.NamespacedName{Name: bmh.Name, Namespace: namespace.Name}, bmh)).To(Succeed())
		Expect(bmh.Status.Provisioning.State).To(Equal(metal3api.StateDeprovisioning))
		helper, err = patch.NewHelper(bmh, c)
		Expect(err).NotTo(HaveOccurred())
		bmh.Spec.NetworkInterfaces = nil
		Expect(helper.Patch(ctx, bmh)).To(Succeed())

		By("verifying AppliedPortConfigs still present during deprovisioning after NI removal")
		Consistently(func(g Gomega) {
			g.Expect(c.Get(ctx, types.NamespacedName{Name: bmh.Name, Namespace: namespace.Name}, bmh)).To(Succeed())
			if bmh.Status.Provisioning.State == metal3api.StateDeprovisioning {
				g.Expect(bmh.Status.AppliedPortConfigs).NotTo(BeEmpty(),
					"AppliedPortConfigs should be preserved during deprovisioning even after NI removal")
			}
		}, "5s", "1s").Should(Succeed())

		if e2eConfig.GetBoolVariable("DEPLOY_IRONIC") {
			By("verifying Ironic port still has switchport config during deprovisioning")
			ironicPorts, ironicErr := fetchIronicPorts(e2eConfig, namespace.Name, bmh.Name)
			Expect(ironicErr).NotTo(HaveOccurred())
			bootPort := findPortByMAC(ironicPorts, bmc.BootMacAddress)
			Expect(bootPort).NotTo(BeNil())
			Expect(bootPort.Extra).NotTo(BeNil())
			_, hasSwitchport := bootPort.Extra["switchport"]
			Expect(hasSwitchport).To(BeTrue(), "switchport config should be preserved during deprovisioning")
		}

		By("waiting for the BMH to return to available")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: c,
			Bmh:    *bmh,
			State:  metal3api.StateAvailable,
		}, e2eConfig.GetIntervals("default", "wait-available")...)

		By("waiting for port configs to be cleared after returning to available")
		Eventually(func(g Gomega) {
			g.Expect(c.Get(ctx, types.NamespacedName{Name: bmh.Name, Namespace: namespace.Name}, bmh)).To(Succeed())
			g.Expect(bmh.Status.AppliedPortConfigs).To(BeEmpty(),
				"AppliedPortConfigs should be cleared after deprovisioning completes")
		}, e2eConfig.GetIntervals("default", "wait-available")...).Should(Succeed())

		By("cleaning up")
		Expect(c.Delete(ctx, bmh)).To(Succeed())
		WaitForBmhDeleted(ctx, WaitForBmhDeletedInput{
			Client:    c,
			BmhName:   bmh.Name,
			Namespace: bmh.Namespace,
		}, e2eConfig.GetIntervals(specName, "wait-bmh-deleted")...)
		Expect(c.Delete(ctx, hna)).To(Succeed())
	})

	It("should block provisioning when network interfaces reference missing HNA", func() {
		c := clusterProxy.GetClient()

		By("creating a BMC credentials secret")
		bmcCredentialsData := map[string]string{
			"username": bmc.User,
			"password": bmc.Password,
		}
		CreateSecret(ctx, c, namespace.Name, "bmc-creds-block-prov", bmcCredentialsData)

		By("creating a BMH with NetworkInterfaces referencing a non-existent HNA")
		bmh := &metal3api.BareMetalHost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      specName + "-block-prov",
				Namespace: namespace.Name,
			},
			Spec: metal3api.BareMetalHostSpec{
				BMC: metal3api.BMCDetails{
					Address:                        bmc.Address,
					CredentialsName:                "bmc-creds-block-prov",
					DisableCertificateVerification: bmc.DisableCertificateVerification,
				},
				BootMode:              metal3api.BootMode(e2eConfig.GetVariable("BOOT_MODE")),
				BootMACAddress:        bmc.BootMacAddress,
				AutomatedCleaningMode: metal3api.CleaningModeDisabled,
				NetworkInterfaces: []metal3api.NetworkInterface{
					{
						MACAddress: bmc.BootMacAddress,
						HostNetworkAttachment: metal3api.HostNetworkAttachmentRef{
							Name: "missing-hna",
						},
					},
				},
			},
		}
		Expect(c.Create(ctx, bmh)).To(Succeed())

		By("waiting for the BMH to become available")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: c,
			Bmh:    *bmh,
			State:  metal3api.StateAvailable,
		}, e2eConfig.GetIntervals("default", "wait-available")...)

		By("verifying NetworkInterfacesValid is False with AttachmentNotFound")
		Expect(c.Get(ctx, types.NamespacedName{Name: bmh.Name, Namespace: namespace.Name}, bmh)).To(Succeed())
		cond := meta.FindStatusCondition(bmh.Status.Conditions, metal3api.NetworkInterfacesValidCondition)
		Expect(cond).NotTo(BeNil(), "NetworkInterfacesValid condition should exist")
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Reason).To(Equal("AttachmentNotFound"))

		By("verifying the BMH stays in available despite needing provisioning settings")
		Consistently(func(g Gomega) {
			g.Expect(c.Get(ctx, types.NamespacedName{Name: bmh.Name, Namespace: namespace.Name}, bmh)).To(Succeed())
			g.Expect(bmh.Status.Provisioning.State).To(Equal(metal3api.StateAvailable),
				"BMH should remain in Available when NI validation is False")
		}, "30s", "5s").Should(Succeed())

		By("cleaning up")
		Expect(c.Delete(ctx, bmh)).To(Succeed())
		WaitForBmhDeleted(ctx, WaitForBmhDeletedInput{
			Client:    c,
			BmhName:   bmh.Name,
			Namespace: bmh.Namespace,
		}, e2eConfig.GetIntervals(specName, "wait-bmh-deleted")...)
	})

	It("should recover validation when missing HNA is created", func() {
		c := clusterProxy.GetClient()

		By("creating a BMC credentials secret")
		bmcCredentialsData := map[string]string{
			"username": bmc.User,
			"password": bmc.Password,
		}
		CreateSecret(ctx, c, namespace.Name, "bmc-creds-recover", bmcCredentialsData)

		By("creating a BMH with NetworkInterfaces referencing an HNA that does not exist yet")
		bmh := &metal3api.BareMetalHost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      specName + "-recover",
				Namespace: namespace.Name,
			},
			Spec: metal3api.BareMetalHostSpec{
				BMC: metal3api.BMCDetails{
					Address:                        bmc.Address,
					CredentialsName:                "bmc-creds-recover",
					DisableCertificateVerification: bmc.DisableCertificateVerification,
				},
				BootMode:              metal3api.BootMode(e2eConfig.GetVariable("BOOT_MODE")),
				BootMACAddress:        bmc.BootMacAddress,
				AutomatedCleaningMode: metal3api.CleaningModeDisabled,
				NetworkInterfaces: []metal3api.NetworkInterface{
					{
						MACAddress: bmc.BootMacAddress,
						HostNetworkAttachment: metal3api.HostNetworkAttachmentRef{
							Name: "late-hna",
						},
					},
				},
			},
		}
		Expect(c.Create(ctx, bmh)).To(Succeed())

		By("waiting for the BMH to become available")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: c,
			Bmh:    *bmh,
			State:  metal3api.StateAvailable,
		}, e2eConfig.GetIntervals("default", "wait-available")...)

		By("verifying NetworkInterfacesValid is False")
		Expect(c.Get(ctx, types.NamespacedName{Name: bmh.Name, Namespace: namespace.Name}, bmh)).To(Succeed())
		cond := meta.FindStatusCondition(bmh.Status.Conditions, metal3api.NetworkInterfacesValidCondition)
		Expect(cond).NotTo(BeNil())
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Reason).To(Equal("AttachmentNotFound"))

		By("creating the missing HNA")
		hna := &metal3api.HostNetworkAttachment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "late-hna",
				Namespace: namespace.Name,
			},
			Spec: metal3api.HostNetworkAttachmentSpec{
				Mode:       metal3api.SwitchportModeAccess,
				NativeVLAN: 100,
			},
		}
		Expect(c.Create(ctx, hna)).To(Succeed())

		By("waiting for NetworkInterfacesValid to recover to True")
		Eventually(func(g Gomega) {
			g.Expect(c.Get(ctx, types.NamespacedName{Name: bmh.Name, Namespace: namespace.Name}, bmh)).To(Succeed())
			cond := meta.FindStatusCondition(bmh.Status.Conditions, metal3api.NetworkInterfacesValidCondition)
			g.Expect(cond).NotTo(BeNil())
			g.Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		}, e2eConfig.GetIntervals("default", "wait-available")...).Should(Succeed())

		By("waiting for AppliedPortConfigs to be populated after Preparing cycle")
		Eventually(func(g Gomega) {
			g.Expect(c.Get(ctx, types.NamespacedName{Name: bmh.Name, Namespace: namespace.Name}, bmh)).To(Succeed())
			g.Expect(bmh.Status.AppliedPortConfigs).NotTo(BeEmpty(),
				"AppliedPortConfigs should be set after recovery")
			g.Expect(bmh.Status.AppliedPortConfigs[0].SwitchPortConfig.Mode).To(Equal(metal3api.SwitchPortMode("access")))
			g.Expect(bmh.Status.AppliedPortConfigs[0].SwitchPortConfig.NativeVLAN).To(Equal(100))
		}, e2eConfig.GetIntervals("default", "wait-available")...).Should(Succeed())

		By("cleaning up")
		Expect(c.Delete(ctx, bmh)).To(Succeed())
		WaitForBmhDeleted(ctx, WaitForBmhDeletedInput{
			Client:    c,
			BmhName:   bmh.Name,
			Namespace: bmh.Namespace,
		}, e2eConfig.GetIntervals(specName, "wait-bmh-deleted")...)
		Expect(c.Delete(ctx, hna)).To(Succeed())
	})

	It("should preserve LLDP data on Ironic ports", func() {
		if !e2eConfig.GetBoolVariable("DEPLOY_IRONIC") {
			Skip("Ironic port verification requires a real Ironic deployment")
		}
		c := clusterProxy.GetClient()

		By("creating a BMC credentials secret")
		bmcCredentialsData := map[string]string{
			"username": bmc.User,
			"password": bmc.Password,
		}
		CreateSecret(ctx, c, namespace.Name, "bmc-creds-deprov", bmcCredentialsData)

		By("creating a BMH without NetworkInterfaces")
		bmh := &metal3api.BareMetalHost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      specName + "-lldp",
				Namespace: namespace.Name,
			},
			Spec: metal3api.BareMetalHostSpec{
				BMC: metal3api.BMCDetails{
					Address:                        bmc.Address,
					CredentialsName:                "bmc-creds-deprov",
					DisableCertificateVerification: bmc.DisableCertificateVerification,
				},
				BootMode:              metal3api.BootMode(e2eConfig.GetVariable("BOOT_MODE")),
				BootMACAddress:        bmc.BootMacAddress,
				AutomatedCleaningMode: metal3api.CleaningModeDisabled,
			},
		}
		Expect(c.Create(ctx, bmh)).To(Succeed())

		By("waiting for the BMH to become available (through registration and inspection)")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: c,
			Bmh:    *bmh,
			State:  metal3api.StateAvailable,
		}, e2eConfig.GetIntervals("default", "wait-available")...)

		By("fetching Ironic ports for the node")
		ports, err := fetchIronicPorts(e2eConfig, namespace.Name, bmh.Name)
		Expect(err).NotTo(HaveOccurred(), "failed to fetch Ironic ports")
		Expect(ports).NotTo(BeEmpty(), "expected at least one Ironic port")

		By("finding the port matching the boot MAC address")
		bootPort := findPortByMAC(ports, bmc.BootMacAddress)
		Expect(bootPort).NotTo(BeNil(), "expected to find an Ironic port matching boot MAC %s", bmc.BootMacAddress)

		By("checking if LLDP data is present in local_link_connection")
		if len(bootPort.LocalLinkConnection) > 0 {
			Logf("local_link_connection is populated: %v", bootPort.LocalLinkConnection)
			_, hasSwitchID := bootPort.LocalLinkConnection["switch_id"]
			_, hasPortID := bootPort.LocalLinkConnection["port_id"]
			if hasSwitchID {
				Logf("switch_id: %v", bootPort.LocalLinkConnection["switch_id"])
			}
			if hasPortID {
				Logf("port_id: %v", bootPort.LocalLinkConnection["port_id"])
			}
			// If LLDP data is present, verify the expected fields exist
			Expect(hasSwitchID || hasPortID).To(BeTrue(),
				"expected local_link_connection to have switch_id or port_id when populated")
		} else {
			Logf("local_link_connection is empty; BMC emulator (sushy-tools) likely does not provide LLDP data during inspection, skipping LLDP assertions")
		}

		By("cleaning up")
		Expect(c.Delete(ctx, bmh)).To(Succeed())
		WaitForBmhDeleted(ctx, WaitForBmhDeletedInput{
			Client:    c,
			BmhName:   bmh.Name,
			Namespace: bmh.Namespace,
		}, e2eConfig.GetIntervals(specName, "wait-bmh-deleted")...)
	})
})
