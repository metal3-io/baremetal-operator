// SPDX-License-Identifier: Apache-2.0

package kube_test

import (
	"testing"
	"time"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"metal3.local/anaconda/internal/core"
	"metal3.local/anaconda/internal/kube"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	testHost    = "node-1"
	testUID     = "bmh-uid"
	testMAC     = "aa:bb:cc:dd:ee:01"
	testDisk    = "vda"
	testSecret  = "node-1-ks"
	testFailure = "disk sda not found"
)

// Only hints that name a udev link survive into a kickstart. The rest select by
// property, which Ironic can do and kickstart cannot.
func TestRootDeviceSpec(t *testing.T) {
	cases := map[string]struct {
		hints *metal3api.RootDeviceHints
		want  string
	}{
		"no hints at all":     {hints: nil},
		"device name":         {hints: &metal3api.RootDeviceHints{DeviceName: "/dev/" + testDisk}, want: testDisk},
		"by-path device name": {hints: &metal3api.RootDeviceHints{DeviceName: "/dev/disk/by-path/pci-0000:01:00.0"}, want: "disk/by-path/pci-0000:01:00.0"},
		"wwn gains its prefix": {
			hints: &metal3api.RootDeviceHints{WWN: "5000c500a1b2c3d4"},
			want:  "disk/by-id/wwn-0x5000c500a1b2c3d4",
		},
		"wwn keeps the prefix it has": {
			hints: &metal3api.RootDeviceHints{WWN: "0x5000c500a1b2c3d4"},
			want:  "disk/by-id/wwn-0x5000c500a1b2c3d4",
		},
		// The extension is part of the same link, so the longer hint has to win or
		// the spec names a device that does not exist.
		"extension beats bare wwn": {
			hints: &metal3api.RootDeviceHints{WWN: "0x5000c500", WWNWithExtension: "0x5000c500a1b2"},
			want:  "disk/by-id/wwn-0x5000c500a1b2",
		},
		// The transport prefix is not in the hint, so the serial anchors the tail.
		"serial globs the transport prefix": {
			hints: &metal3api.RootDeviceHints{SerialNumber: "S3Z1NB0K"},
			want:  "disk/by-id/*S3Z1NB0K",
		},
		"device name beats everything": {
			hints: &metal3api.RootDeviceHints{DeviceName: "/dev/sda", WWN: "0x5000c500", SerialNumber: "S3Z1NB0K"},
			want:  "sda",
		},
		// Property hints name no udev link. Returning a bare disk/by-id/* would
		// match every disk on the machine and hand clearpart the whole array.
		"property hints are not expressible": {
			hints: &metal3api.RootDeviceHints{Model: "SAMSUNG", Vendor: "ATA", MinSizeGigabytes: 400, HCTL: "0:0:0:0"},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := kube.RootDeviceSpec(tc.hints); got != tc.want {
				t.Errorf("RootDeviceSpec() = %q, want %q", got, tc.want)
			}
		})
	}
}

func newResolver(t *testing.T, objs ...client.Object) *kube.HostResolver {
	t.Helper()

	// The fixtures put every object in "ns", so the operator namespace has to
	// match or the scoped lists look at somewhere else entirely.
	t.Setenv("POD_NAMESPACE", "ns")

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add corev1 to scheme: %v", err)
	}

	if err := metal3api.AddToScheme(scheme); err != nil {
		t.Fatalf("add metal3api to scheme: %v", err)
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()

	return &kube.HostResolver{
		Client:    c,
		APIReader: c,
	}
}

// bmh returns a bare BareMetalHost. Nothing in kube reads the BMC block, so the
// lookups are driven by name, namespace and MACs alone.
func bmh(namespace, name string) *metal3api.BareMetalHost {
	return &metal3api.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name, UID: testUID},
	}
}

func hostWithMACs(name, bootMAC string, nicMACs ...string) *metal3api.BareMetalHost {
	host := bmh("ns", name)
	host.UID = types.UID("uid-" + name)
	host.Spec.BootMACAddress = bootMAC

	if len(nicMACs) > 0 {
		nics := make([]metal3api.NIC, 0, len(nicMACs))
		for _, m := range nicMACs {
			nics = append(nics, metal3api.NIC{MAC: m})
		}

		host.Status.HardwareDetails = &metal3api.HardwareDetails{NIC: nics}
	}

	return host
}

func TestInstallReportRoundTrip(t *testing.T) {
	ctx := t.Context()
	r := newResolver(t, bmh("ns", testHost))

	if got, err := r.ReadInstallReport(ctx, "ns", testHost); err != nil || got != nil {
		t.Fatalf("ReadInstallReport before any = (%v, %v), want (nil, nil)", got, err)
	}

	if err := r.WriteInstallReport(ctx, "ns", testHost, core.InstallReport{Message: testFailure}); err != nil {
		t.Fatalf("WriteInstallReport: %v", err)
	}

	host := &metal3api.BareMetalHost{}
	key := types.NamespacedName{Namespace: "ns", Name: testHost}

	if err := r.Client.Get(ctx, key, host); err != nil {
		t.Fatalf("get host: %v", err)
	}

	// Annotations rather than a Secret, so nothing is created and BMO's own
	// permissions are enough.
	if host.Annotations[kube.InstallResultAnnotation] != kube.InstallResultFailed {
		t.Errorf("result annotation = %q, want %q", host.Annotations[kube.InstallResultAnnotation], kube.InstallResultFailed)
	}

	if host.Annotations[kube.InstallMessageAnnotation] != testFailure {
		t.Errorf("message annotation = %q", host.Annotations[kube.InstallMessageAnnotation])
	}

	got, err := r.ReadInstallReport(ctx, "ns", testHost)
	if err != nil || got == nil || got.Succeeded || got.Message != testFailure {
		t.Fatalf("ReadInstallReport = (%+v, %v), want the failure back", got, err)
	}

	// A success overwrites the failure and drops the stale message with it.
	if werr := r.WriteInstallReport(ctx, "ns", testHost, core.InstallReport{Succeeded: true}); werr != nil {
		t.Fatalf("second WriteInstallReport: %v", werr)
	}

	if got, _ = r.ReadInstallReport(ctx, "ns", testHost); got == nil || !got.Succeeded || got.Message != "" {
		t.Errorf("ReadInstallReport = %+v, want a clean success", got)
	}

	// A reinstall that still reads the last run's success finishes on its first
	// pass without installing, so starting one has to drop the verdict.
	if cerr := r.ClearInstallReport(ctx, "ns", testHost); cerr != nil {
		t.Fatalf("ClearInstallReport: %v", cerr)
	}

	if got, err = r.ReadInstallReport(ctx, "ns", testHost); err != nil || got != nil {
		t.Errorf("after clear = (%v, %v), want (nil, nil)", got, err)
	}
}

// The install clock is BMO's own, stamped when the host entered provisioning.
func TestInstallStartedAtRoundTrips(t *testing.T) {
	ctx := t.Context()

	// BMO stamps this on entering provisioning, which survives a host parked on
	// a refusal for hours, so the clock must not come from it.
	host := bmh("ns", testHost)
	host.Status.OperationHistory.Provision.Start = metav1.NewTime(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))

	r := newResolver(t, host)

	if got, err := r.InstallStartedAt(ctx, "ns", testHost); err != nil || !got.IsZero() {
		t.Fatalf("InstallStartedAt = (%v, %v), want zero before an install starts", got, err)
	}

	at := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	if err := r.MarkInstallStarted(ctx, "ns", testHost, at); err != nil {
		t.Fatalf("MarkInstallStarted: %v", err)
	}

	got, err := r.InstallStartedAt(ctx, "ns", testHost)
	if err != nil || !got.Equal(at) {
		t.Fatalf("InstallStartedAt = (%v, %v), want %v", got, err, at)
	}

	// Clearing has to drop the stamp too, or a reinstall times out on the last
	// run's clock before anaconda has finished booting.
	if err := r.ClearInstallReport(ctx, "ns", testHost); err != nil {
		t.Fatalf("ClearInstallReport: %v", err)
	}

	if got, _ := r.InstallStartedAt(ctx, "ns", testHost); !got.IsZero() {
		t.Errorf("InstallStartedAt = %v after clear, want zero", got)
	}
}

func TestFindHostsByMAC(t *testing.T) {
	t.Setenv("POD_NAMESPACE", "ns")

	boot := hostWithMACs("boot-host", testMAC)
	inspected := hostWithMACs("nic-host", "", "aa:bb:cc:dd:ee:02")

	r := newResolver(t, boot, inspected)
	ctx := t.Context()

	cases := []struct {
		name string
		want string
		macs []string
	}{
		{"boot MAC hit", "boot-host", []string{testMAC}},
		// A host that reported the MAC during inspection but declares no boot
		// MAC cannot be provisioned, so serving it a kickstart is pointless.
		{"inspected NIC is not a match", "", []string{"aa:bb:cc:dd:ee:02"}},
		{"case and separator insensitive", "boot-host", []string{"AA-BB-CC-DD-EE-01"}},
		{"unknown MAC", "", []string{"aa:bb:cc:dd:ee:ff"}},
		{"no MACs", "", nil},
		{"unparseable MAC", "", []string{"nonsense"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hosts, err := r.FindHostsByMAC(ctx, tc.macs)
			if err != nil {
				t.Fatalf("FindHostsByMAC: %v", err)
			}

			if tc.want == "" {
				if len(hosts) != 0 {
					t.Fatalf("hosts = %+v, want none", hosts)
				}

				return
			}

			if len(hosts) != 1 || hosts[0].Name != tc.want {
				t.Fatalf("hosts = %+v, want just %s", hosts, tc.want)
			}

			if hosts[0].UID != "uid-"+tc.want {
				t.Errorf("host ref = %+v, want the UID filled in", hosts[0])
			}
		})
	}
}

// Two hosts declaring the same boot MAC is a conflict only their operator can
// resolve, so both come back and the caller logs it before taking the first.
func TestFindHostsByMACReportsACollision(t *testing.T) {
	shared := testMAC
	r := newResolver(t, hostWithMACs("host-a", shared), hostWithMACs("host-b", shared))

	hosts, err := r.FindHostsByMAC(t.Context(), []string{shared})
	if err != nil {
		t.Fatalf("FindHostsByMAC: %v", err)
	}

	if len(hosts) != 2 {
		t.Fatalf("hosts = %+v, want both so the caller can log the conflict", hosts)
	}
}

// The lookup is what decides a host's namespace for every later read, so it
// must not be confined to whichever namespace the operator happens to run in.
func TestFindHostsByMACSpansNamespaces(t *testing.T) {
	elsewhere := hostWithMACs("far-host", testMAC)
	elsewhere.Namespace = "other"

	r := newResolver(t, elsewhere)

	hosts, err := r.FindHostsByMAC(t.Context(), []string{testMAC})
	if err != nil {
		t.Fatalf("FindHostsByMAC: %v", err)
	}

	if len(hosts) != 1 || hosts[0].Namespace != "other" {
		t.Fatalf("hosts = %+v, want the host found in its own namespace", hosts)
	}
}

// ksSecret builds a kickstart Secret the way bmh-create.sh does, under the one
// key BMO accepts and the plugin reads.
func ksSecret(namespace, name, body string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
		Data:       map[string][]byte{core.KickstartSecretKey: []byte(body)},
	}
}

// Every way of not finding a kickstart has to read as not found. An error would
// 500 the route and drop the machine to an interactive prompt.
func TestReadKickstart(t *testing.T) {
	const body = "text\nrootpw --plaintext toor\n"

	cases := map[string]struct {
		secretName string
		seed       client.Object
		want       string
		wantFound  bool
	}{
		"host names no secret": {secretName: ""},
		"secret is absent":     {secretName: "gone"},
		"secret has no value key": {
			secretName: testSecret,
			seed: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: testSecret},
				Data:       map[string][]byte{"networkData": []byte("not ours")},
			},
		},
		"secret carries the kickstart": {
			secretName: testSecret,
			seed:       ksSecret("ns", testSecret, body),
			want:       body, wantFound: true,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			objs := []client.Object{bmh("ns", testHost)}
			if tc.seed != nil {
				objs = append(objs, tc.seed)
			}

			r := newResolver(t, objs...)

			got, found, err := r.ReadKickstart(t.Context(), "ns", tc.secretName)
			if err != nil {
				t.Fatalf("ReadKickstart: %v", err)
			}

			if found != tc.wantFound || got != tc.want {
				t.Errorf("= (%q, %v), want (%q, %v)", got, found, tc.want, tc.wantFound)
			}
		})
	}
}

// Without a client nothing can be read, and returning empty would look like a
// host with no kickstart, which installs the inert fallback instead.
func TestReadKickstartWithoutAClientErrors(t *testing.T) {
	r := &kube.HostResolver{}

	if _, _, err := r.ReadKickstart(t.Context(), "ns", testSecret); err == nil {
		t.Error("ReadKickstart succeeded with no client, want an error")
	}
}

// BMO caches only Secrets carrying its own label, so a kickstart Secret is never
// in that cache at any age. Reading through the cached client would never find it.
func TestCallbackReaderPrefersTheUncachedReader(t *testing.T) {
	r := newResolver(t, bmh("ns", testHost))
	if r.CallbackReader() != r.APIReader {
		t.Error("CallbackReader did not return the uncached APIReader")
	}

	r.APIReader = nil
	if r.CallbackReader() != r.Client {
		t.Error("CallbackReader did not fall back to the cached client")
	}
}

// A host with no HardwareData is the normal case, so it must not be an error.
// An error here would loop the host in inspecting instead of letting it through.
func TestHostHardwareData(t *testing.T) {
	r := newResolver(t, bmh("ns", testHost))

	got, err := r.HostHardwareData(t.Context(), "ns", testHost)
	if err != nil || got != nil {
		t.Fatalf("= (%v, %v), want (nil, nil) when no CR exists", got, err)
	}

	recorded := &metal3api.HardwareData{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: testHost},
		Spec:       metal3api.HardwareDataSpec{HardwareDetails: &metal3api.HardwareDetails{Hostname: "recorded"}},
	}

	r = newResolver(t, bmh("ns", testHost), recorded)

	got, err = r.HostHardwareData(t.Context(), "ns", testHost)
	if err != nil || got == nil || got.Hostname != "recorded" {
		t.Errorf("= (%+v, %v), want the recorded details", got, err)
	}
}

// BeginInstall refuses on this, so it has to answer for the host's own Secret
// rather than for any Secret that happens to exist.
func TestHasKickstart(t *testing.T) {
	host := bmh("ns", testHost)
	host.Spec.PreprovisioningNetworkDataName = testSecret

	r := newResolver(t, host, ksSecret("ns", testSecret, "text\n"))

	if got, err := r.HasKickstart(t.Context(), "ns", testHost); err != nil || !got {
		t.Errorf("= (%v, %v), want true when the Secret carries the key", got, err)
	}

	// The same host with the Secret gone must not claim to have one.
	bare := newResolver(t, bmh("ns", testHost))
	if got, err := bare.HasKickstart(t.Context(), "ns", testHost); err != nil || got {
		t.Errorf("= (%v, %v), want false when the host names no Secret", got, err)
	}
}

// This is the disk clearpart wipes, so it comes from the host's own hints and
// renders through the same spec the kickstart is given.
func TestHostInstallDisk(t *testing.T) {
	host := bmh("ns", testHost)
	host.Spec.RootDeviceHints = &metal3api.RootDeviceHints{DeviceName: "/dev/" + testDisk}

	r := newResolver(t, host)

	if got, err := r.HostInstallDisk(t.Context(), "ns", testHost); err != nil || got != testDisk {
		t.Errorf("= (%q, %v), want %q", got, err, testDisk)
	}

	// No hints is empty rather than a guess, BeginInstall turns that into a refusal.
	bare := newResolver(t, bmh("ns", testHost))
	if got, err := bare.HostInstallDisk(t.Context(), "ns", testHost); err != nil || got != "" {
		t.Errorf("= (%q, %v), want empty for a host with no hints", got, err)
	}
}

// The UID in the callback path is the only thing authenticating a report, so
// reading the wrong one would accept somebody else's.
func TestHostUID(t *testing.T) {
	r := newResolver(t, bmh("ns", testHost))

	if got, err := r.HostUID(t.Context(), "ns", testHost); err != nil || got != testUID {
		t.Errorf("= (%q, %v), want %q", got, err, testUID)
	}

	if _, err := r.HostUID(t.Context(), "ns", "no-such-host"); err == nil {
		t.Error("HostUID succeeded for a host that does not exist")
	}
}
