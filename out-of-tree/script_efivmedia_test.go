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

// Cover for scripts/efi-vmedia.star against a stub Ironic. Load time validation only
// checks the entry points, so a bad internal helper call needs a real registration.

package starlark

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	"github.com/metal3-io/baremetal-operator/pkg/hardwareutils/bmc"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	"github.com/s3rj1k/starlark-provisioner/starlib"
	"github.com/s3rj1k/starlark-provisioner/starscript"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// noSecrets satisfies the host resolver the script uses for network data, with
// nothing configured, which is the common case for a plain virtual media host.
type noSecrets struct{}

func (noSecrets) ReadHostSecret(_ context.Context, _, _, _ string) (string, error) {
	return "", nil
}

func (noSecrets) ReadHostSpec(_ context.Context, _, _ string) (map[string]any, error) {
	return map[string]any{}, nil
}

func (noSecrets) ReadHostStatus(_ context.Context, _, _ string) (map[string]any, error) {
	return map[string]any{}, nil
}

// ironicStub answers the calls register makes and records the created node.
func ironicStub(t *testing.T, created map[string]any) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/nodes":
			body, _ := io.ReadAll(r.Body)

			var node map[string]any
			if err := json.Unmarshal(body, &node); err != nil {
				t.Errorf("node body is not JSON: %v", err)
			}

			for k, v := range node {
				created[k] = v
			}

			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"uuid":"node-uuid-1","provision_state":"enroll"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/ports":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	return srv
}

// efiProvisioner loads efi-vmedia.star after the Ironic endpoint is in the
// environment, since the script reads it at module load.
func efiProvisioner(t *testing.T, endpoint, bmcAddress string) *starlarkProvisioner {
	t.Helper()

	t.Setenv("IRONIC_ENDPOINT", endpoint)
	t.Setenv("DEPLOY_KERNEL_URL", "http://images/kernel")
	t.Setenv("DEPLOY_RAMDISK_URL", "http://images/initramfs")

	globals, err := starscript.LoadScript(filepath.Join("scripts", "efi-vmedia.star"),
		starlib.Builtins(), starlib.ThreadPrint(logr.Discard()), starlib.MaxExecutionSteps)
	if err != nil {
		t.Fatalf("LoadScript: %v", err)
	}

	return &starlarkProvisioner{
		globals:        globals,
		log:            logr.Discard(),
		secretResolver: noSecrets{},
		hostData: provisioner.HostData{
			ObjectMeta:     metav1.ObjectMeta{Namespace: "ns", Name: "node-1", UID: "uid-1"},
			BMCAddress:     bmcAddress,
			BMCCredentials: bmc.Credentials{Username: "admin", Password: "s3cret"},
			BootMACAddress: "aa:bb:cc:dd:ee:01",
		},
	}
}

// An address with no system path is legal upstream, and the parsed driver_info
// then carries no redfish_system_id. Indexing that key aborts registration.
func TestEFIVmediaRegistersAddressWithoutSystemPath(t *testing.T) {
	cases := map[string]struct {
		address    string
		wantAddr   string
		wantSysID  bool
		wantVerify bool
	}{
		"no path at all": {
			address: "redfish-virtualmedia+https://10.0.0.1", wantAddr: "https://10.0.0.1", wantSysID: false, wantVerify: true,
		},
		"bare redfish root": {
			address: "redfish-virtualmedia://10.0.0.1/redfish/v1", wantAddr: "https://10.0.0.1", wantSysID: false, wantVerify: true,
		},
		"full system path": {
			address: "redfish-virtualmedia://10.0.0.1/redfish/v1/Systems/1", wantAddr: "https://10.0.0.1", wantSysID: true, wantVerify: true,
		},
		"plaintext transport is preserved": {
			address: "redfish-virtualmedia+http://10.0.0.1/redfish/v1/Systems/1", wantAddr: "http://10.0.0.1", wantSysID: true, wantVerify: true,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			created := map[string]any{}

			srv := ironicStub(t, created)
			p := efiProvisioner(t, srv.URL, tc.address)

			result, provID, err := p.Register(context.Background(), provisioner.ManagementAccessData{}, false, false)
			if err != nil {
				t.Fatalf("Register: %v", err)
			}

			if result.ErrorMessage != "" {
				t.Fatalf("Register reported %q", result.ErrorMessage)
			}

			if provID != "node-uuid-1" {
				t.Errorf("provID = %q, want the created node uuid", provID)
			}

			di, ok := created["driver_info"].(map[string]any)
			if !ok {
				t.Fatalf("created node has no driver_info, got %v", created)
			}

			if di["redfish_address"] != tc.wantAddr {
				t.Errorf("redfish_address = %v, want %q", di["redfish_address"], tc.wantAddr)
			}

			if _, present := di["redfish_system_id"]; present != tc.wantSysID {
				t.Errorf("redfish_system_id present = %v, want %v", present, tc.wantSysID)
			}

			if di["redfish_verify_ca"] != tc.wantVerify {
				t.Errorf("redfish_verify_ca = %v, want %v", di["redfish_verify_ca"], tc.wantVerify)
			}

			// The credentials have to reach Ironic, it cannot read them back later.
			if di["redfish_username"] != "admin" || di["redfish_password"] != "s3cret" {
				t.Errorf("credentials missing from driver_info, got %v", di)
			}
		})
	}
}

// disableCertificateVerification on the host must reach Ironic, since the CRD
// field is otherwise silently ignored.
func TestEFIVmediaHonoursDisableCertificateVerification(t *testing.T) {
	created := map[string]any{}

	srv := ironicStub(t, created)
	p := efiProvisioner(t, srv.URL, "redfish-virtualmedia://10.0.0.1/redfish/v1/Systems/1")
	p.hostData.DisableCertificateVerification = true

	if _, _, err := p.Register(context.Background(), provisioner.ManagementAccessData{}, false, false); err != nil {
		t.Fatalf("Register: %v", err)
	}

	di, ok := created["driver_info"].(map[string]any)
	if !ok {
		t.Fatalf("no driver_info in %v", created)
	}

	if di["redfish_verify_ca"] != false {
		t.Errorf("redfish_verify_ca = %v, want false when verification is disabled", di["redfish_verify_ca"])
	}
}

// A non redfish-virtualmedia address is still refused, the exclusivity contract
// this script enforces everywhere else.
func TestEFIVmediaRejectsForeignScheme(t *testing.T) {
	srv := ironicStub(t, map[string]any{})
	p := efiProvisioner(t, srv.URL, "ipmi://10.0.0.1")

	_, _, err := p.Register(context.Background(), provisioner.ManagementAccessData{}, false, false)
	if err == nil {
		t.Fatal("Register accepted an IPMI address")
	}

	if !strings.Contains(err.Error(), "unsupported BMC scheme") {
		t.Errorf("err = %v, want a scheme rejection", err)
	}
}
