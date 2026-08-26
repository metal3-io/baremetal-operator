/*

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package starlark

import (
	"context"
	"strings"
	"testing"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/s3rj1k/starlark-provisioner/starlib"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// Every host read shares one fetch helper, so each caller must still name itself
// when no client is configured.
func TestHostReadsRequireClient(t *testing.T) {
	var r KubeHostResolver

	ctx := context.Background()

	cases := []struct {
		name string
		call func() error
		want string
	}{
		{"read_host_secret", func() error {
			_, err := r.ReadHostSecret(ctx, "ns", "node-1", "userdata")
			return err
		}, "read_host_secret requires a Kubernetes client"},
		{"read_host_spec", func() error {
			_, err := r.ReadHostSpec(ctx, "ns", "node-1")
			return err
		}, "read_host_spec requires a Kubernetes client"},
		{"read_host_status", func() error {
			_, err := r.ReadHostStatus(ctx, "ns", "node-1")
			return err
		}, "read_host_status requires a Kubernetes client"},
		{"callback", func() error {
			_, _, err := r.BMCCredentials(ctx, "ns", "node-1")
			return err
		}, "callback requires a Kubernetes client"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.call()
			if err == nil {
				t.Fatal("call succeeded with no client")
			}

			if err.Error() != c.want {
				t.Errorf("error = %q, want %q", err.Error(), c.want)
			}
		})
	}
}

func TestHostReadsReportMissingHost(t *testing.T) {
	r := newCallbackResolver(t)

	if _, err := r.ReadHostSpec(context.Background(), "ns", "absent"); err == nil ||
		!strings.Contains(err.Error(), "get BareMetalHost") {
		t.Errorf("error = %v, want a get BareMetalHost wrap", err)
	}
}

func TestReadHostSpecAndStatus(t *testing.T) {
	host := hostWithBMC("ns", "node-1", "bmc-creds")
	host.Spec.Online = true
	host.Status.PoweredOn = true
	host.Status.HardwareProfile = "unknown"

	r := newCallbackResolver(t, host)
	ctx := context.Background()

	spec, err := r.ReadHostSpec(ctx, "ns", "node-1")
	if err != nil {
		t.Fatalf("ReadHostSpec: %v", err)
	}

	if spec["online"] != true {
		t.Errorf("spec online = %v, want true", spec["online"])
	}

	bmc, ok := spec["bmc"].(map[string]any)
	if !ok || bmc["credentialsName"] != "bmc-creds" {
		t.Errorf("spec bmc = %v, want the credentials name", spec["bmc"])
	}

	status, err := r.ReadHostStatus(ctx, "ns", "node-1")
	if err != nil {
		t.Fatalf("ReadHostStatus: %v", err)
	}

	if status["poweredOn"] != true {
		t.Errorf("status poweredOn = %v, want true", status["poweredOn"])
	}

	if status["hardwareProfile"] != "unknown" {
		t.Errorf("status hardwareProfile = %v, want unknown", status["hardwareProfile"])
	}
}

func TestReadHostSecretFields(t *testing.T) {
	host := hostWithBMC("ns", "node-1", "bmc-creds")
	host.Spec.UserData = &corev1.SecretReference{Name: "user-data", Namespace: "ns"}
	host.Spec.NetworkData = &corev1.SecretReference{Name: "network-data"}
	host.Spec.PreprovisioningNetworkDataName = "preprov-data"

	secrets := []*corev1.Secret{
		{
			ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "user-data"},
			Data:       map[string][]byte{"userData": []byte("#cloud-config")},
		},
		{
			// A secret may carry the payload under the generic value key instead.
			ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "network-data"},
			Data:       map[string][]byte{"value": []byte("links")},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "preprov-data"},
			Data:       map[string][]byte{"networkData": []byte("preprov links")},
		},
	}

	r := newCallbackResolver(t, host, secrets[0], secrets[1], secrets[2])
	ctx := context.Background()

	cases := map[string]string{
		"userdata":                   "#cloud-config",
		"UserData":                   "#cloud-config",
		"networkdata":                "links",
		"preprovisioningnetworkdata": "preprov links",
		"metadata":                   "",
	}

	for field, want := range cases {
		got, err := r.ReadHostSecret(ctx, "ns", "node-1", field)
		if err != nil {
			t.Errorf("ReadHostSecret(%q) failed, %v", field, err)

			continue
		}

		if got != want {
			t.Errorf("ReadHostSecret(%q) = %q, want %q", field, got, want)
		}
	}

	if _, err := r.ReadHostSecret(ctx, "ns", "node-1", "nonsense"); err == nil {
		t.Error("ReadHostSecret accepted an unknown field")
	}
}

func TestReadHostSecretRejectsForeignNamespace(t *testing.T) {
	host := hostWithBMC("ns", "node-1", "bmc-creds")
	host.Spec.UserData = &corev1.SecretReference{Name: "user-data", Namespace: "other"}

	r := newCallbackResolver(t, host)

	_, err := r.ReadHostSecret(context.Background(), "ns", "node-1", "userdata")
	if err == nil || !strings.Contains(err.Error(), "must be in BMH namespace ns") {
		t.Errorf("error = %v, want a namespace rejection", err)
	}
}

// The generic k8s_* builtins must not reach Secrets, so every entry point that
// resolves a GVK refuses them and points at the sanctioned reader.
func TestKubeGVKBlocksSecrets(t *testing.T) {
	cases := []struct {
		apiVersion string
		kind       string
		blocked    bool
	}{
		{"v1", "Secret", true},
		{"v1", "SecretList", true},
		{"v1", "ConfigMap", false},
		{"v1", "Pod", false},
		// Only core Secrets are blocked, a same named kind in another group is not one.
		{"example.com/v1", "Secret", false},
	}

	for _, tc := range cases {
		_, err := KubeGVK(tc.apiVersion, tc.kind)
		if tc.blocked {
			if err == nil {
				t.Errorf("KubeGVK(%q, %q) = nil, want error", tc.apiVersion, tc.kind)
			} else if !strings.Contains(err.Error(), "read_host_secret") {
				t.Errorf("KubeGVK(%q, %q) error %q does not point at read_host_secret", tc.apiVersion, tc.kind, err)
			}

			continue
		}

		if err != nil {
			t.Errorf("KubeGVK(%q, %q) = %v, want nil", tc.apiVersion, tc.kind, err)
		}
	}
}

// The block has to hold at the resolver entry points, not only in KubeGVK.
func TestSecretsUnreachableThroughKubeBuiltins(t *testing.T) {
	creds := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "bmc-creds"},
		Data:       map[string][]byte{"username": []byte("admin"), "password": []byte("s3cret")},
	}
	r := newCallbackResolver(t, creds)

	ctx := context.Background()

	cases := []struct {
		name string
		call func() error
	}{
		{"k8s_get", func() error {
			_, err := r.GetObject(ctx, "ns", "v1", "Secret", "bmc-creds", false)
			return err
		}},
		{"k8s_get uncached", func() error {
			_, err := r.GetObject(ctx, "ns", "v1", "Secret", "bmc-creds", true)
			return err
		}},
		{"k8s_list", func() error {
			_, err := r.ListObjects(ctx, "ns", "v1", "Secret", "")
			return err
		}},
		{"k8s_apply", func() error {
			_, err := r.ApplyObject(ctx, "ns", map[string]any{
				"apiVersion": "v1",
				"kind":       "Secret",
				"metadata":   map[string]any{"name": "bmc-creds"},
			}, "", false)

			return err
		}},
		{"k8s_patch", func() error {
			_, err := r.PatchObject(ctx, "ns", "v1", "Secret", "bmc-creds", "merge", []byte(`{}`), false)
			return err
		}},
		{"k8s_delete", func() error {
			return r.DeleteObject(ctx, "ns", "v1", "Secret", "bmc-creds")
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call()
			if err == nil {
				t.Fatal("Secret access succeeded, want error")
			}

			if !strings.Contains(err.Error(), "access to Secrets is not allowed") {
				t.Errorf("error = %v, want the Secret block", err)
			}
		})
	}
}

func TestBMCCredentialsWithoutCredentialsName(t *testing.T) {
	host := &metal3api.BareMetalHost{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "node-1"}}
	r := newCallbackResolver(t, host)

	if _, _, err := r.BMCCredentials(context.Background(), "ns", "node-1"); err == nil {
		t.Error("BMCCredentials succeeded with no credentials name")
	}
}

// hostWithMACs builds a host carrying a boot MAC and inspected NIC MACs.
func hostWithMACs(name, bootMAC string, nicMACs ...string) *metal3api.BareMetalHost {
	host := hostWithBMC("ns", name, "bmc-creds")
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

func TestFindHostsByMAC(t *testing.T) {
	t.Setenv("POD_NAMESPACE", "ns")

	boot := hostWithMACs("boot-host", "aa:bb:cc:dd:ee:01")
	inspected := hostWithMACs("nic-host", "", "aa:bb:cc:dd:ee:02", "aa:bb:cc:dd:ee:03")

	// Same MACs in another namespace, which the lookup must never return.
	foreignBoot := hostWithMACs("foreign-boot", "aa:bb:cc:dd:ee:01")
	foreignBoot.Namespace = "other"
	foreignNIC := hostWithMACs("foreign-nic", "", "aa:bb:cc:dd:ee:02")
	foreignNIC.Namespace = "other"

	r := newCallbackResolver(t, boot, inspected, foreignBoot, foreignNIC)
	ctx := context.Background()

	cases := []struct {
		name string
		macs []string
		want string
	}{
		{"boot MAC hit", []string{"aa:bb:cc:dd:ee:01"}, "boot-host"},
		{"inspected NIC hit", []string{"aa:bb:cc:dd:ee:02"}, "nic-host"},
		{"second inspected NIC hit", []string{"aa:bb:cc:dd:ee:03"}, "nic-host"},
		{"case and separator insensitive", []string{"AA-BB-CC-DD-EE-01"}, "boot-host"},
		{"unknown MAC", []string{"aa:bb:cc:dd:ee:ff"}, ""},
		{"no MACs", nil, ""},
		{"unparseable MAC", []string{"nonsense"}, ""},
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

// A boot MAC match outranks a NIC match on a different host, so the machine that
// declares the address wins over one that merely reported it during inspection.
func TestFindHostsByMACPrefersBootMAC(t *testing.T) {
	shared := "aa:bb:cc:dd:ee:01"
	viaNIC := hostWithMACs("nic-host", "", shared)
	viaBoot := hostWithMACs("boot-host", shared)

	r := newCallbackResolver(t, viaNIC, viaBoot)

	hosts, err := r.FindHostsByMAC(context.Background(), []string{shared})
	if err != nil {
		t.Fatalf("FindHostsByMAC: %v", err)
	}

	if len(hosts) != 2 {
		t.Fatalf("hosts = %+v, want both so the caller can log the conflict", hosts)
	}

	if hosts[0].Name != "boot-host" {
		t.Errorf("first host = %s, want boot-host to outrank the NIC match", hosts[0].Name)
	}
}

// A referenced Secret that carries neither the named key nor "value" is a
// misconfiguration, and returning "" would silently deploy a host with no data.
func TestReadHostSecretMissingKeyErrors(t *testing.T) {
	host := hostWithBMC("ns", "node-1", "bmc-creds")
	host.Spec.NetworkData = &corev1.SecretReference{Name: "network-data"}

	wrongKey := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "network-data"},
		Data:       map[string][]byte{"network-data": []byte("links")},
	}

	r := newCallbackResolver(t, host, wrongKey)

	got, err := r.ReadHostSecret(context.Background(), "ns", "node-1", "networkData")
	if err == nil {
		t.Fatalf("ReadHostSecret = %q, want an error naming the missing key", got)
	}

	for _, want := range []string{"networkData", "network-data"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %s", err, want)
		}
	}
}

// An unset reference is not a misconfiguration, so it still reads as empty.
func TestReadHostSecretUnsetRefStaysEmpty(t *testing.T) {
	r := newCallbackResolver(t, hostWithBMC("ns", "node-1", "bmc-creds"))

	got, err := r.ReadHostSecret(context.Background(), "ns", "node-1", "networkData")
	if err != nil || got != "" {
		t.Errorf("ReadHostSecret = (%q, %v), want (\"\", nil)", got, err)
	}
}

// The host supplied namespace wins, because an empty one would silently widen
// every scoped list to the whole cluster.
func TestKubeHostResolverNamespace(t *testing.T) {
	t.Setenv("POD_NAMESPACE", "from-env")

	if got := (&KubeHostResolver{PodNamespace: "from-host"}).Namespace(); got != "from-host" {
		t.Errorf("Namespace = %q, want the host supplied value", got)
	}

	if got := (&KubeHostResolver{}).Namespace(); got != "from-env" {
		t.Errorf("Namespace = %q, want the environment fallback", got)
	}
}

// The read side lists only the operator namespace, so registering a route for a
// host elsewhere would succeed and then never be servable.
func TestWriteServeRefusesForeignNamespace(t *testing.T) {
	r := newCallbackResolver(t, hostWithBMC("ns", "node-1", "bmc-creds"))

	err := r.WriteServe(context.Background(), "other", "node-1", "ks",
		starlib.ServeRegistration{ConfigMap: "cm", Key: "ks.cfg"})
	if err == nil {
		t.Fatal("WriteServe accepted a host outside the operator namespace")
	}

	if !strings.Contains(err.Error(), "only supported for hosts in ns") {
		t.Errorf("error = %v, want it to name the operator namespace", err)
	}

	// The supported case still works.
	if err := r.WriteServe(context.Background(), "ns", "node-1", "ks",
		starlib.ServeRegistration{ConfigMap: "cm", Key: "ks.cfg"}); err != nil {
		t.Errorf("WriteServe in the operator namespace: %v", err)
	}
}
