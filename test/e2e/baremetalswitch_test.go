//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"fmt"
	"path"
	"strings"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	configSecretName     = "metal3-switch-configs"
	credentialSecretName = "metal3-switch-credentials"
	credentialPath       = "/etc/ironic/switch-credentials"
	switchConfigKey      = "switch-configs.conf"
	switchConfigHeader   = "# This file is managed by the Baremetal Operator\n\n"
)

// createLabeledSecret creates a Kubernetes secret with the environment.metal3.io: baremetal
// label required by BMO's cache selector. The label ensures the secret is visible to the
// controller, which uses a filtered cache that only includes secrets with this label.
func createLabeledSecret(ctx context.Context, c client.Client, namespace, name string, data map[string]string) {
	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"environment.metal3.io": "baremetal",
			},
		},
		StringData: data,
	}
	Expect(c.Create(ctx, &secret)).NotTo(HaveOccurred(), fmt.Sprintf("Failed to create secret '%s/%s'", namespace, name))
}

// createEmptyLabeledSecret creates an empty secret with the baremetal label.
// Used for the output secrets that the controller updates.
func createEmptyLabeledSecret(ctx context.Context, c client.Client, namespace, name string) {
	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"environment.metal3.io": "baremetal",
			},
		},
		Data: map[string][]byte{},
	}
	Expect(c.Create(ctx, &secret)).NotTo(HaveOccurred(), fmt.Sprintf("Failed to create empty secret '%s/%s'", namespace, name))
}

// getSwitchConfig retrieves the switch config from the config secret.
func getSwitchConfig(ctx context.Context, c client.Client, namespace string) string {
	secret := &corev1.Secret{}
	key := types.NamespacedName{Namespace: namespace, Name: configSecretName}
	Expect(c.Get(ctx, key, secret)).To(Succeed())
	return string(secret.Data[switchConfigKey])
}

// getCredentialSecret retrieves the credential secret data.
func getCredentialSecret(ctx context.Context, c client.Client, namespace string) map[string][]byte {
	secret := &corev1.Secret{}
	key := types.NamespacedName{Namespace: namespace, Name: credentialSecretName}
	Expect(c.Get(ctx, key, secret)).To(Succeed())
	return secret.Data
}

var _ = Describe("baremetalswitch", Serial, Label("required", "baremetalswitch"), func() {
	var (
		specName      = "baremetalswitch"
		namespace     *corev1.Namespace
		cancelWatches context.CancelFunc
	)

	BeforeEach(func() {
		namespace, cancelWatches = framework.CreateNamespaceAndWatchEvents(ctx, framework.CreateNamespaceAndWatchEventsInput{
			Creator:             clusterProxy.GetClient(),
			ClientSet:           clusterProxy.GetClientSet(),
			Name:                specName,
			LogFolder:           artifactFolder,
			IgnoreAlreadyExists: true,
		})

		By("creating empty output secrets for the controller")
		createEmptyLabeledSecret(ctx, clusterProxy.GetClient(), namespace.Name, configSecretName)
		createEmptyLabeledSecret(ctx, clusterProxy.GetClient(), namespace.Name, credentialSecretName)
	})

	It("should manage switch config from BareMetalSwitch resources", func() {
		c := clusterProxy.GetClient()

		By("creating a credential secret for switch-1")
		createLabeledSecret(ctx, c, namespace.Name, "switch1-creds", map[string]string{
			"username": "admin",
			"password": "secret123",
		})

		By("creating the first BareMetalSwitch resource")
		switch1 := &metal3api.BareMetalSwitch{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "switch-1",
				Namespace: namespace.Name,
			},
			Spec: metal3api.BareMetalSwitchSpec{
				Address:    "192.168.1.1",
				MACAddress: "00:00:5e:00:53:01",
				DeviceType: "netmiko_cisco_ios",
				Credentials: metal3api.SwitchCredentials{
					Type:       metal3api.SwitchCredentialTypePassword,
					SecretName: "switch1-creds",
				},
			},
		}
		Expect(c.Create(ctx, switch1)).To(Succeed())

		By("verifying the config secret contains the first switch")
		Eventually(func(g Gomega) {
			config := getSwitchConfig(ctx, c, namespace.Name)
			g.Expect(config).To(ContainSubstring("[switch:switch-1]"))
			g.Expect(config).To(ContainSubstring("address=192.168.1.1"))
			g.Expect(config).To(ContainSubstring("mac_address=00:00:5e:00:53:01"))
			g.Expect(config).To(ContainSubstring("driver_type=generic-switch"))
			g.Expect(config).To(ContainSubstring("device_type=netmiko_cisco_ios"))
			g.Expect(config).To(ContainSubstring("username=admin"))
			g.Expect(config).To(ContainSubstring("password=secret123"))
		}, e2eConfig.GetIntervals(specName, "wait-secret-updated")...).Should(Succeed())

		By("creating a credential secret for switch-2")
		createLabeledSecret(ctx, c, namespace.Name, "switch2-creds", map[string]string{
			"username": "root",
			"password": "p@ssw0rd",
		})

		By("creating a second BareMetalSwitch resource")
		switch2 := &metal3api.BareMetalSwitch{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "switch-2",
				Namespace: namespace.Name,
			},
			Spec: metal3api.BareMetalSwitchSpec{
				Address:    "192.168.1.2",
				MACAddress: "00:00:5e:00:53:02",
				DeviceType: "netmiko_dell_os10",
				Credentials: metal3api.SwitchCredentials{
					Type:       metal3api.SwitchCredentialTypePassword,
					SecretName: "switch2-creds",
				},
			},
		}
		Expect(c.Create(ctx, switch2)).To(Succeed())

		By("verifying the config secret contains both switches sorted by name")
		Eventually(func(g Gomega) {
			config := getSwitchConfig(ctx, c, namespace.Name)
			g.Expect(config).To(ContainSubstring("[switch:switch-1]"))
			g.Expect(config).To(ContainSubstring("[switch:switch-2]"))
			// Verify sorted order: switch-1 comes before switch-2
			idx1 := strings.Index(config, "[switch:switch-1]")
			idx2 := strings.Index(config, "[switch:switch-2]")
			g.Expect(idx1).To(BeNumerically("<", idx2))
		}, e2eConfig.GetIntervals(specName, "wait-secret-updated")...).Should(Succeed())

		By("deleting the first switch")
		Expect(c.Delete(ctx, switch1)).To(Succeed())

		By("verifying the config secret contains only the second switch")
		Eventually(func(g Gomega) {
			config := getSwitchConfig(ctx, c, namespace.Name)
			g.Expect(config).NotTo(ContainSubstring("[switch:switch-1]"))
			g.Expect(config).To(ContainSubstring("[switch:switch-2]"))
		}, e2eConfig.GetIntervals(specName, "wait-secret-updated")...).Should(Succeed())

		By("deleting the second switch")
		Expect(c.Delete(ctx, switch2)).To(Succeed())

		By("verifying the config secret contains only the header")
		Eventually(func(g Gomega) {
			config := getSwitchConfig(ctx, c, namespace.Name)
			g.Expect(config).To(Equal(switchConfigHeader))
		}, e2eConfig.GetIntervals(specName, "wait-secret-updated")...).Should(Succeed())

		By("creating a password-auth switch with credentials")
		createLabeledSecret(ctx, c, namespace.Name, "pw-switch-creds", map[string]string{
			"username": "admin",
			"password": "secret123",
		})
		pwSwitch := &metal3api.BareMetalSwitch{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pw-switch",
				Namespace: namespace.Name,
			},
			Spec: metal3api.BareMetalSwitchSpec{
				Address:    "192.168.1.1",
				MACAddress: "aa:bb:cc:dd:ee:01",
				DeviceType: "netmiko_cisco_ios",
				Credentials: metal3api.SwitchCredentials{
					Type:       metal3api.SwitchCredentialTypePassword,
					SecretName: "pw-switch-creds",
				},
			},
		}
		Expect(c.Create(ctx, pwSwitch)).To(Succeed())

		By("verifying password-auth config contains password field")
		Eventually(func(g Gomega) {
			config := getSwitchConfig(ctx, c, namespace.Name)
			g.Expect(config).To(ContainSubstring("[switch:pw-switch]"))
			g.Expect(config).To(ContainSubstring("password=secret123"))
			g.Expect(config).NotTo(ContainSubstring("key_file="))
		}, e2eConfig.GetIntervals(specName, "wait-secret-updated")...).Should(Succeed())

		By("creating a publickey-auth switch with SSH key credential")
		createLabeledSecret(ctx, c, namespace.Name, "pk-switch-creds", map[string]string{
			"username":       "sshuser",
			"ssh-privatekey": "-----BEGIN RSA PRIVATE KEY-----\ntest-key-data\n-----END RSA PRIVATE KEY-----\n",
		})
		pkSwitch := &metal3api.BareMetalSwitch{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pk-switch",
				Namespace: namespace.Name,
			},
			Spec: metal3api.BareMetalSwitchSpec{
				Address:    "192.168.1.2",
				MACAddress: "aa:bb:cc:dd:ee:02",
				DeviceType: "netmiko_dell_os10",
				Credentials: metal3api.SwitchCredentials{
					Type:       metal3api.SwitchCredentialTypePublicKey,
					SecretName: "pk-switch-creds",
				},
			},
		}
		Expect(c.Create(ctx, pkSwitch)).To(Succeed())

		By("verifying publickey-auth config contains key_file field")
		expectedKeyFile := credentialPath + "/aa-bb-cc-dd-ee-02.key"
		Eventually(func(g Gomega) {
			config := getSwitchConfig(ctx, c, namespace.Name)
			g.Expect(config).To(ContainSubstring("[switch:pk-switch]"))
			g.Expect(config).To(ContainSubstring("key_file=" + expectedKeyFile))
			g.Expect(config).To(ContainSubstring("username=sshuser"))
		}, e2eConfig.GetIntervals(specName, "wait-secret-updated")...).Should(Succeed())

		By("verifying the credentials secret contains the SSH key file")
		Eventually(func(g Gomega) {
			credData := getCredentialSecret(ctx, c, namespace.Name)
			g.Expect(credData).To(HaveKey("aa-bb-cc-dd-ee-02.key"))
			g.Expect(string(credData["aa-bb-cc-dd-ee-02.key"])).To(ContainSubstring("BEGIN RSA PRIVATE KEY"))
		}, e2eConfig.GetIntervals(specName, "wait-secret-updated")...).Should(Succeed())

		By("cleaning up credential type test switches")
		Expect(c.Delete(ctx, pwSwitch)).To(Succeed())
		Expect(c.Delete(ctx, pkSwitch)).To(Succeed())

		By("verifying config is empty after credential type test cleanup")
		Eventually(func(g Gomega) {
			config := getSwitchConfig(ctx, c, namespace.Name)
			g.Expect(config).To(Equal(switchConfigHeader))
		}, e2eConfig.GetIntervals(specName, "wait-secret-updated")...).Should(Succeed())

		By("creating a password-auth switch with initial credentials for rotation test")
		createLabeledSecret(ctx, c, namespace.Name, "rotating-creds", map[string]string{
			"username": "admin",
			"password": "old-password",
		})
		rotatingSwitch := &metal3api.BareMetalSwitch{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "rotating-switch",
				Namespace: namespace.Name,
			},
			Spec: metal3api.BareMetalSwitchSpec{
				Address:    "192.168.1.1",
				MACAddress: "00:00:5e:00:53:01",
				DeviceType: "netmiko_cisco_ios",
				Credentials: metal3api.SwitchCredentials{
					Type:       metal3api.SwitchCredentialTypePassword,
					SecretName: "rotating-creds",
				},
			},
		}
		Expect(c.Create(ctx, rotatingSwitch)).To(Succeed())

		By("verifying config has the original password")
		Eventually(func(g Gomega) {
			config := getSwitchConfig(ctx, c, namespace.Name)
			g.Expect(config).To(ContainSubstring("password=old-password"))
		}, e2eConfig.GetIntervals(specName, "wait-secret-updated")...).Should(Succeed())

		By("updating the credential secret with a new password")
		credSecret := &corev1.Secret{}
		credKey := types.NamespacedName{Namespace: namespace.Name, Name: "rotating-creds"}
		Expect(c.Get(ctx, credKey, credSecret)).To(Succeed())
		credSecret.Data["password"] = []byte("new-password")
		Expect(c.Update(ctx, credSecret)).To(Succeed())

		By("verifying config is updated with the new password")
		Eventually(func(g Gomega) {
			config := getSwitchConfig(ctx, c, namespace.Name)
			g.Expect(config).To(ContainSubstring("password=new-password"))
			g.Expect(config).NotTo(ContainSubstring("password=old-password"))
		}, e2eConfig.GetIntervals(specName, "wait-secret-updated")...).Should(Succeed())

		By("cleaning up rotation test switch")
		Expect(c.Delete(ctx, rotatingSwitch)).To(Succeed())
		Eventually(func(g Gomega) {
			config := getSwitchConfig(ctx, c, namespace.Name)
			g.Expect(config).To(Equal(switchConfigHeader))
		}, e2eConfig.GetIntervals(specName, "wait-secret-updated")...).Should(Succeed())

		By("creating a BareMetalSwitch referencing a non-existent credential secret")
		missingCredsSwitch := &metal3api.BareMetalSwitch{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "missing-creds-switch",
				Namespace: namespace.Name,
			},
			Spec: metal3api.BareMetalSwitchSpec{
				Address:    "192.168.1.1",
				MACAddress: "00:00:5e:00:53:01",
				DeviceType: "netmiko_cisco_ios",
				Credentials: metal3api.SwitchCredentials{
					Type:       metal3api.SwitchCredentialTypePassword,
					SecretName: "nonexistent-creds",
				},
			},
		}
		Expect(c.Create(ctx, missingCredsSwitch)).To(Succeed())

		By("verifying the config secret contains only the header (bad switch is skipped)")
		Eventually(func(g Gomega) {
			config := getSwitchConfig(ctx, c, namespace.Name)
			g.Expect(config).To(Equal(switchConfigHeader))
			g.Expect(config).NotTo(ContainSubstring("[switch:missing-creds-switch]"))
		}, e2eConfig.GetIntervals(specName, "wait-secret-updated")...).Should(Succeed())

		By("creating the missing credential secret")
		createLabeledSecret(ctx, c, namespace.Name, "nonexistent-creds", map[string]string{
			"username": "admin",
			"password": "secret123",
		})

		By("verifying the config secret is now populated")
		Eventually(func(g Gomega) {
			config := getSwitchConfig(ctx, c, namespace.Name)
			g.Expect(config).To(ContainSubstring("[switch:missing-creds-switch]"))
			g.Expect(config).To(ContainSubstring("password=secret123"))
		}, e2eConfig.GetIntervals(specName, "wait-secret-updated")...).Should(Succeed())

		By("cleaning up missing creds test switch")
		Expect(c.Delete(ctx, missingCredsSwitch)).To(Succeed())
		Eventually(func(g Gomega) {
			config := getSwitchConfig(ctx, c, namespace.Name)
			g.Expect(config).To(Equal(switchConfigHeader))
		}, e2eConfig.GetIntervals(specName, "wait-secret-updated")...).Should(Succeed())

		By("creating credential secrets for three switches")
		for _, name := range []string{"a-creds", "b-creds", "c-creds"} {
			createLabeledSecret(ctx, c, namespace.Name, name, map[string]string{
				"username": "admin",
				"password": "pass-" + name,
			})
		}

		By("creating three BareMetalSwitch resources")
		switches := make([]*metal3api.BareMetalSwitch, 3)
		for i, name := range []string{"switch-a", "switch-b", "switch-c"} {
			switches[i] = &metal3api.BareMetalSwitch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace.Name,
				},
				Spec: metal3api.BareMetalSwitchSpec{
					Address:    fmt.Sprintf("192.168.1.%d", i+1),
					MACAddress: fmt.Sprintf("00:00:5e:00:53:%02d", i+1),
					DeviceType: "netmiko_cisco_ios",
					Credentials: metal3api.SwitchCredentials{
						Type:       metal3api.SwitchCredentialTypePassword,
						SecretName: fmt.Sprintf("%c-creds", 'a'+rune(i)),
					},
				},
			}
			Expect(c.Create(ctx, switches[i])).To(Succeed())
		}

		By("verifying all three switches appear in config sorted by name")
		Eventually(func(g Gomega) {
			config := getSwitchConfig(ctx, c, namespace.Name)
			g.Expect(config).To(ContainSubstring("[switch:switch-a]"))
			g.Expect(config).To(ContainSubstring("[switch:switch-b]"))
			g.Expect(config).To(ContainSubstring("[switch:switch-c]"))
			// Verify sorted order
			idxA := strings.Index(config, "[switch:switch-a]")
			idxB := strings.Index(config, "[switch:switch-b]")
			idxC := strings.Index(config, "[switch:switch-c]")
			g.Expect(idxA).To(BeNumerically("<", idxB))
			g.Expect(idxB).To(BeNumerically("<", idxC))
		}, e2eConfig.GetIntervals(specName, "wait-secret-updated")...).Should(Succeed())

		By("deleting switch-b")
		Expect(c.Delete(ctx, switches[1])).To(Succeed())

		By("verifying config has only switch-a and switch-c")
		Eventually(func(g Gomega) {
			config := getSwitchConfig(ctx, c, namespace.Name)
			g.Expect(config).To(ContainSubstring("[switch:switch-a]"))
			g.Expect(config).NotTo(ContainSubstring("[switch:switch-b]"))
			g.Expect(config).To(ContainSubstring("[switch:switch-c]"))
		}, e2eConfig.GetIntervals(specName, "wait-secret-updated")...).Should(Succeed())

		By("deleting switch-a and switch-c")
		Expect(c.Delete(ctx, switches[0])).To(Succeed())
		Expect(c.Delete(ctx, switches[2])).To(Succeed())

		By("verifying config has only the header comment")
		Eventually(func(g Gomega) {
			config := getSwitchConfig(ctx, c, namespace.Name)
			g.Expect(config).To(Equal(switchConfigHeader))
		}, e2eConfig.GetIntervals(specName, "wait-secret-updated")...).Should(Succeed())

		By("creating a credential secret for optional fields test")
		createLabeledSecret(ctx, c, namespace.Name, "optional-creds", map[string]string{
			"username": "admin",
			"password": "secret123",
		})

		By("creating a switch with port set")
		switchWithPort := &metal3api.BareMetalSwitch{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "switch-with-port",
				Namespace: namespace.Name,
			},
			Spec: metal3api.BareMetalSwitchSpec{
				Address:    "192.168.1.1",
				MACAddress: "00:00:5e:00:53:01",
				DeviceType: "netmiko_cisco_ios",
				Port:       ptr.To(int32(8443)),
				Credentials: metal3api.SwitchCredentials{
					Type:       metal3api.SwitchCredentialTypePassword,
					SecretName: "optional-creds",
				},
			},
		}
		Expect(c.Create(ctx, switchWithPort)).To(Succeed())

		By("verifying config contains port=8443")
		Eventually(func(g Gomega) {
			config := getSwitchConfig(ctx, c, namespace.Name)
			g.Expect(config).To(ContainSubstring("[switch:switch-with-port]"))
			g.Expect(config).To(ContainSubstring("port=8443"))
		}, e2eConfig.GetIntervals(specName, "wait-secret-updated")...).Should(Succeed())

		By("creating a switch without port set")
		switchNoPort := &metal3api.BareMetalSwitch{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "switch-no-port",
				Namespace: namespace.Name,
			},
			Spec: metal3api.BareMetalSwitchSpec{
				Address:    "192.168.1.2",
				MACAddress: "00:00:5e:00:53:02",
				DeviceType: "netmiko_dell_os10",
				Credentials: metal3api.SwitchCredentials{
					Type:       metal3api.SwitchCredentialTypePassword,
					SecretName: "optional-creds",
				},
			},
		}
		Expect(c.Create(ctx, switchNoPort)).To(Succeed())

		By("verifying the no-port switch config does NOT contain port=")
		Eventually(func(g Gomega) {
			config := getSwitchConfig(ctx, c, namespace.Name)
			g.Expect(config).To(ContainSubstring("[switch:switch-no-port]"))
			// Extract the switch-no-port section and verify it doesn't contain port=
			sections := strings.Split(config, "[switch:")
			for _, section := range sections {
				if strings.HasPrefix(section, "switch-no-port]") {
					g.Expect(section).NotTo(ContainSubstring("port="))
				}
			}
		}, e2eConfig.GetIntervals(specName, "wait-secret-updated")...).Should(Succeed())

		By("creating a switch with disableCertificateVerification set")
		switchInsecure := &metal3api.BareMetalSwitch{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "switch-insecure",
				Namespace: namespace.Name,
			},
			Spec: metal3api.BareMetalSwitchSpec{
				Address:                        "192.168.1.3",
				MACAddress:                     "00:00:5e:00:53:03",
				DeviceType:                     "netmiko_cisco_ios",
				DisableCertificateVerification: ptr.To(true),
				Credentials: metal3api.SwitchCredentials{
					Type:       metal3api.SwitchCredentialTypePassword,
					SecretName: "optional-creds",
				},
			},
		}
		Expect(c.Create(ctx, switchInsecure)).To(Succeed())

		By("verifying config contains insecure=true for the insecure switch")
		Eventually(func(g Gomega) {
			config := getSwitchConfig(ctx, c, namespace.Name)
			g.Expect(config).To(ContainSubstring("[switch:switch-insecure]"))
			// Extract the switch-insecure section and verify it contains insecure=true
			sections := strings.Split(config, "[switch:")
			for _, section := range sections {
				if strings.HasPrefix(section, "switch-insecure]") {
					g.Expect(section).To(ContainSubstring("insecure=true"))
				}
			}
		}, e2eConfig.GetIntervals(specName, "wait-secret-updated")...).Should(Succeed())
	})

	AfterEach(func() {
		DumpResources(ctx, e2eConfig, clusterProxy, path.Join(artifactFolder, specName))
		if !skipCleanup {
			isNamespaced := e2eConfig.GetBoolVariable("NAMESPACE_SCOPED")
			Cleanup(ctx, clusterProxy, namespace, cancelWatches, isNamespaced, e2eConfig.GetIntervals("default", "wait-namespace-deleted")...)
		}
	})
})
