//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/util/deprecated/v1beta1/patch"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// Stamped by the plugin when the ISO goes into the virtual media drive.
	// BeginInstall clears any recorded verdict just before it, so a callback
	// posted earlier than this would be thrown away.
	installStartedAnnotation = "anaconda.metal3.io/install-started"
	// Key the plugin reads the kickstart from, see core.KickstartSecretKey.
	kickstartSecretKey = "value"
	// Emitted by the compiled in fallback, which is what the plugin serves when
	// it cannot resolve the caller. Seeing it means the render did not happen.
	kickstartFallbackMarker = "refusing to install"
)

// anacondaKickstart is served to the machine and never actually run, since the
// e2e ISO is not an installer. It exists to prove the template was rendered for
// this host, so it may only name variables the plugin passes.
const anacondaKickstart = `text
network --bootproto=dhcp --activate --hostname={{ .Name }}
ignoredisk --only-use={{ .InstallDisk }}
clearpart --all --initlabel --drives={{ .InstallDisk }}
autopart --type=lvm --nohome

%post --erroronfail --interpreter=/bin/bash
curl -fsS --retry 5 -X POST -H "Content-Type: application/json" \
  -d '{"status":"installed"}' "{{ .CallbackURL }}"
%end
`

// runCurlPod runs curl inside the cluster and returns what it printed. The e2e
// process runs outside the cluster and there is no port-forward helper, so this
// is how metrics_service_test.go reaches an in-cluster endpoint too.
func runCurlPod(namespace, name string, args ...string) string {
	cl := clusterProxy.GetClient()

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{
				{
					Name:    "curl",
					Image:   "curlimages/curl:7.87.0",
					Command: append([]string{"curl"}, args...),
				},
			},
		},
	}
	Expect(cl.Create(ctx, pod)).To(Succeed(), "Failed to create pod "+name)

	Eventually(func(g Gomega) {
		key := types.NamespacedName{Name: name, Namespace: namespace}
		got := &corev1.Pod{}
		g.Expect(cl.Get(ctx, key, got)).To(Succeed())
		g.Expect(got.Status.Phase).To(Equal(corev1.PodSucceeded), name+" did not succeed")
	}, 5*time.Minute, 5*time.Second).Should(Succeed())

	req := clusterProxy.GetClientSet().CoreV1().Pods(namespace).GetLogs(name, &corev1.PodLogOptions{})
	logs, err := req.Stream(ctx)
	Expect(err).NotTo(HaveOccurred(), "Failed to get logs of "+name)
	defer logs.Close()

	out, err := io.ReadAll(logs)
	Expect(err).NotTo(HaveOccurred(), "Failed to read logs of "+name)

	return string(out)
}

var _ = Describe("Anaconda", Label("anaconda"), func() {
	var (
		specName      = "anaconda-ops"
		namespace     *corev1.Namespace
		cancelWatches context.CancelFunc
		imageURL      string
		baseURL       string
		kickstartID   string
		toCleanup     []client.Object
	)

	BeforeEach(func() {
		toCleanup = nil

		// The provisioner is chosen per deployment, so this spec only makes
		// sense against a BMO running the anaconda plugin. Read the map rather
		// than GetVariable, which fails the spec when the key is absent.
		if e2eConfig.Variables["PROVISIONER"] != "anaconda" {
			Skip("BMO is not running the anaconda provisioner, skipping")
		}

		// Anaconda deploys by inserting virtual media and nothing else, so an
		// address it cannot drive is not worth booting a machine for.
		if !strings.HasPrefix(bmc.Address, "redfish-virtualmedia") {
			Skip("anaconda requires a redfish-virtualmedia BMC. BMC address: " + bmc.Address)
		}

		imageURL = e2eConfig.GetVariable("ISO_IMAGE_URL")
		baseURL = e2eConfig.GetVariable("ANACONDA_BASE_URL")
		kickstartID = e2eConfig.GetVariable("ANACONDA_KICKSTART_ID")

		namespace, cancelWatches = framework.CreateNamespaceAndWatchEvents(ctx, framework.CreateNamespaceAndWatchEventsInput{
			Creator:             clusterProxy.GetClient(),
			ClientSet:           clusterProxy.GetClientSet(),
			Name:                specName,
			LogFolder:           artifactFolder,
			IgnoreAlreadyExists: true,
		})
	})

	It("should serve a kickstart, finish on the callback and then deprovision", func() {
		By("Creating a secret with BMH credentials")
		bmcCredentialsData := map[string]string{
			"username": bmc.User,
			"password": bmc.Password,
		}
		secret := CreateSecret(ctx, clusterProxy.GetClient(), namespace.Name, "bmc-credentials", bmcCredentialsData)
		toCleanup = append(toCleanup, secret)

		By("Creating the kickstart secret the plugin serves")
		kickstartSecret := CreateSecret(ctx, clusterProxy.GetClient(), namespace.Name, "kickstart",
			map[string]string{kickstartSecretKey: anacondaKickstart})
		toCleanup = append(toCleanup, kickstartSecret)

		By("Creating a BMH with a live ISO, a boot MAC and root device hints")
		bmh := metal3api.BareMetalHost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      specName,
				Namespace: namespace.Name,
			},
			Spec: metal3api.BareMetalHostSpec{
				Online: true,
				BMC: metal3api.BMCDetails{
					Address:                        bmc.Address,
					CredentialsName:                "bmc-credentials",
					DisableCertificateVerification: bmc.DisableCertificateVerification,
				},
				Image: &metal3api.Image{
					URL:        imageURL,
					DiskFormat: ptr.To("live-iso"),
				},
				BootMode: metal3api.BootMode(e2eConfig.GetVariable("BOOT_MODE")),
				// Anaconda refuses to install without all three of these, each
				// with an error naming the field.
				BootMACAddress:                 bmc.BootMacAddress,
				RootDeviceHints:                &bmc.RootDeviceHints,
				PreprovisioningNetworkDataName: "kickstart",
				AutomatedCleaningMode:          metal3api.CleaningModeDisabled,
			},
		}
		Expect(clusterProxy.GetClient().Create(ctx, &bmh)).To(Succeed())
		toCleanup = append(toCleanup, &bmh)

		By("Waiting for the BMH to be in provisioning state")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: clusterProxy.GetClient(),
			Bmh:    bmh,
			State:  metal3api.StateProvisioning,
		}, e2eConfig.GetIntervals(specName, "wait-provisioning")...)

		By("Waiting for the ISO to be in the drive")
		var provisioned metal3api.BareMetalHost
		Eventually(func(g Gomega) {
			key := types.NamespacedName{Name: bmh.Name, Namespace: bmh.Namespace}
			g.Expect(clusterProxy.GetClient().Get(ctx, key, &provisioned)).To(Succeed())
			g.Expect(provisioned.Annotations).To(HaveKey(installStartedAnnotation),
				"the plugin has not stamped the install start yet")
		}, e2eConfig.GetIntervals(specName, "wait-provisioning")...).Should(Succeed())

		By("Fetching the kickstart the way anaconda would")
		// The MAC headers are the only thing the route matches on, and they
		// need inst.ks.sendmac on the kernel command line to be sent at all.
		kickstart := runCurlPod(namespace.Name, "curl-kickstart",
			"-sS", "-H", "X-RHN-Provisioning-MAC-0: eth0 "+bmc.BootMacAddress,
			fmt.Sprintf("%s/ks/%s", baseURL, kickstartID))

		Expect(kickstart).NotTo(ContainSubstring(kickstartFallbackMarker),
			"the plugin served the fallback instead of this host's kickstart")
		Expect(kickstart).To(ContainSubstring(bmh.Name), "kickstart was not rendered for this host")

		// Only a deviceName hint renders to something predictable. A wwn or
		// serial hint becomes a by-id path, and an empty expectation here would
		// pass against anything at all.
		if disk := strings.TrimPrefix(bmc.RootDeviceHints.DeviceName, "/dev/"); disk != "" {
			Expect(kickstart).To(ContainSubstring(disk),
				"kickstart does not name the disk the root device hints resolve to")
		}

		By("Reporting the install finished, which anaconda would do from %post")
		status := runCurlPod(namespace.Name, "curl-callback",
			"-sS", "-o", "/dev/null", "-w", "%{http_code}",
			"-X", "POST", "-H", "Content-Type: application/json",
			"-d", `{"status":"installed"}`,
			fmt.Sprintf("%s/callback/%s/%s/%s", baseURL, provisioned.UID, bmh.Namespace, bmh.Name))
		Expect(status).To(ContainSubstring("200"), "the callback was rejected")

		// Only reached once the plugin has taken the host down, ejected the
		// media and pointed the boot override back at the disk.
		By("Waiting for the BMH to become provisioned")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: clusterProxy.GetClient(),
			Bmh:    bmh,
			State:  metal3api.StateProvisioned,
		}, e2eConfig.GetIntervals(specName, "wait-provisioned")...)

		By("Triggering the deprovisioning of the BMH")
		helper, err := patch.NewHelper(&bmh, clusterProxy.GetClient())
		Expect(err).NotTo(HaveOccurred())
		bmh.Spec.Image = nil
		Expect(helper.Patch(ctx, &bmh)).To(Succeed())

		By("Waiting for the BMH to be in deprovisioning state")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: clusterProxy.GetClient(),
			Bmh:    bmh,
			State:  metal3api.StateDeprovisioning,
		}, e2eConfig.GetIntervals(specName, "wait-deprovisioning")...)

		By("Waiting for the BMH to become available again")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: clusterProxy.GetClient(),
			Bmh:    bmh,
			State:  metal3api.StateAvailable,
		}, e2eConfig.GetIntervals(specName, "wait-available")...)
	})

	AfterEach(func() {
		CollectSerialLogs(bmc.Name, path.Join(artifactFolder, specName))
		DumpResources(ctx, e2eConfig, clusterProxy, path.Join(artifactFolder, specName))
		if !skipCleanup {
			Cleanup(ctx, clusterProxy, namespace, cancelWatches, e2eConfig, toCleanup)
		}
	})
})
