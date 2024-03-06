package e2e

import (
	"context"
	"encoding/json"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/util"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
)

const hardwareDetails = `
{
  "cpu": {
    "arch": "x86_64",
    "count": 2,
    "flags": [
      "3dnowprefetch",
      "abm",
      "adx",
      "aes",
      "apic",
      "arat",
      "arch_capabilities",
      "avx",
      "avx2",
      "avx_vnni",
      "bmi1",
      "bmi2",
      "clflush",
      "clflushopt",
      "clwb",
      "cmov",
      "constant_tsc",
      "cpuid",
      "cpuid_fault",
      "cx16",
      "cx8",
      "de",
      "ept",
      "ept_ad",
      "erms",
      "f16c",
      "flexpriority",
      "fma",
      "fpu",
      "fsgsbase",
      "fsrm",
      "fxsr",
      "gfni",
      "hypervisor",
      "ibpb",
      "ibrs",
      "ibrs_enhanced",
      "invpcid",
      "lahf_lm",
      "lm",
      "mca",
      "mce",
      "md_clear",
      "mmx",
      "movbe",
      "movdir64b",
      "movdiri",
      "msr",
      "mtrr",
      "nopl",
      "nx",
      "ospke",
      "pae",
      "pat",
      "pclmulqdq",
      "pdpe1gb",
      "pge",
      "pku",
      "pni",
      "popcnt",
      "pse",
      "pse36",
      "rdpid",
      "rdrand",
      "rdseed",
      "rdtscp",
      "rep_good",
      "sep",
      "serialize",
      "sha_ni",
      "smap",
      "smep",
      "ss",
      "ssbd",
      "sse",
      "sse2",
      "sse4_1",
      "sse4_2",
      "ssse3",
      "stibp",
      "syscall",
      "tpr_shadow",
      "tsc",
      "tsc_adjust",
      "tsc_deadline_timer",
      "tsc_known_freq",
      "umip",
      "vaes",
      "vme",
      "vmx",
      "vnmi",
      "vpclmulqdq",
      "vpid",
      "waitpkg",
      "x2apic",
      "xgetbv1",
      "xsave",
      "xsavec",
      "xsaveopt",
      "xsaves",
      "xtopology"
    ],
    "model": "12th Gen Intel(R) Core(TM) i9-12900H"
  },
  "firmware": {
    "bios": {
      "date": "04/01/2014",
      "vendor": "SeaBIOS",
      "version": "1.15.0-1"
    }
  },
  "hostname": "bmo-e2e-0",
  "nics": [
    {
      "ip": "192.168.222.122",
      "mac": "00:60:2f:31:81:01",
      "model": "0x1af4 0x0001",
      "name": "enp1s0",
      "pxe": true
    },
    {
      "ip": "fe80::570a:edf2:a3a7:4eb8%enp1s0",
      "mac": "00:60:2f:31:81:01",
      "model": "0x1af4 0x0001",
      "name": "enp1s0",
      "pxe": true
    }
  ],
  "ramMebibytes": 4096,
  "storage": [
    {
      "alternateNames": [
        "/dev/vda",
        "/dev/disk/by-path/pci-0000:04:00.0"
      ],
      "name": "/dev/disk/by-path/pci-0000:04:00.0",
      "rotational": true,
      "sizeBytes": 21474836480,
      "type": "HDD",
      "vendor": "0x1af4"
    }
  ],
  "systemVendor": {
    "manufacturer": "QEMU",
    "productName": "Standard PC (Q35 + ICH9, 2009)"
  }
}
`

var _ = Describe("External Inspection", Label("required", "external-inspection"), func() {
	var (
		specName      = "external-inspection"
		secretName    = "bmc-credentials"
		namespace     *corev1.Namespace
		cancelWatches context.CancelFunc
	)
	BeforeEach(func() {
		namespace, cancelWatches = framework.CreateNamespaceAndWatchEvents(ctx, framework.CreateNamespaceAndWatchEventsInput{
			Creator:   clusterProxy.GetClient(),
			ClientSet: clusterProxy.GetClientSet(),
			Name:      fmt.Sprintf("%s-%s", specName, util.RandomString(6)),
			LogFolder: artifactFolder,
		})
	})

	It("should skip inspection and become available when a BMH has annotations with hardware details and inspection disabled", func() {
		By("Creating a secret with BMH credentials")
		bmcCredentialsData := map[string]string{
			"username": bmc.User,
			"password": bmc.Password,
		}
		CreateSecret(ctx, clusterProxy.GetClient(), namespace.Name, secretName, bmcCredentialsData)

		By("creating a BMH with inspection disabled and hardware details added")
		bmh := metal3api.BareMetalHost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      specName + "-inspect",
				Namespace: namespace.Name,
				Annotations: map[string]string{
					metal3api.InspectAnnotationPrefix:   "disabled",
					metal3api.HardwareDetailsAnnotation: hardwareDetails,
				},
			},
			Spec: metal3api.BareMetalHostSpec{
				BMC: metal3api.BMCDetails{
					Address:         bmc.Address,
					CredentialsName: "bmc-credentials",
				},
				BootMode:       metal3api.Legacy,
				BootMACAddress: bmc.BootMacAddress,
			},
		}
		err := clusterProxy.GetClient().Create(ctx, &bmh)
		Expect(err).NotTo(HaveOccurred())

		By("waiting for the BMH to become available")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: clusterProxy.GetClient(),
			Bmh:    bmh,
			State:  metal3api.StateAvailable,
		}, e2eConfig.GetIntervals(specName, "wait-available")...)

		By("checking that the BMH was not inspected")
		key := types.NamespacedName{Namespace: bmh.Namespace, Name: bmh.Name}
		Expect(clusterProxy.GetClient().Get(ctx, key, &bmh)).To(Succeed())
		Expect(bmh.Status.OperationHistory.Inspect.Start.IsZero()).To(BeTrue())

		By("checking that the hardware details match what was in the annotation")
		hwStatusJSON, err := json.Marshal(bmh.Status.HardwareDetails)
		Expect(err).NotTo(HaveOccurred())
		Expect(hwStatusJSON).To(MatchJSON(hardwareDetails))
	})

	AfterEach(func() {
		cleanup(ctx, clusterProxy, namespace, cancelWatches, e2eConfig.GetIntervals("default", "wait-namespace-deleted")...)
	})
})
