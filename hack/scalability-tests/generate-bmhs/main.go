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

// generate-bmhs produces Kubernetes YAML manifests containing BareMetalHost
// resources and their corresponding BMC credential secrets for scalability
// testing. It generates N hosts pointing to a sushy-tools fake backend,
// with inspection and automated cleaning disabled.
//
// Usage:
//
//	go run ./hack/scalability-tests/generate-bmhs -num-hosts=100
//	go run ./hack/scalability-tests/generate-bmhs -num-hosts=50 -bmc-address=10.0.0.1
//	go run ./hack/scalability-tests/generate-bmhs -num-hosts=200 -output=bmhs.yaml
//	go run ./hack/scalability-tests/generate-bmhs -systems-from-sushy=http://192.168.222.1:8000
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

const (
	httpTimeout     = 10 * time.Second
	defaultNumHosts = 50
	defaultBMCPort  = 8000
	macByteMask     = 0xFF
	macShiftHigh    = 16
	macShiftMid     = 8
)

type config struct {
	numHosts        int
	namespace       string
	bmcAddress      string
	bmcPort         int
	bmcUser         string
	bmcPassword     string
	bootMode        string
	output          string
	systemsFromSuhy string
}

func main() {
	cfg := parseFlags()

	if err := run(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func parseFlags() config {
	cfg := config{}

	flag.IntVar(&cfg.numHosts, "num-hosts", defaultNumHosts, "Number of BareMetalHost resources to generate")
	flag.StringVar(&cfg.namespace, "namespace", "scalability-test", "Kubernetes namespace for the resources")
	flag.StringVar(&cfg.bmcAddress, "bmc-address", "192.168.222.1", "BMC emulator IP address")
	flag.IntVar(&cfg.bmcPort, "bmc-port", defaultBMCPort, "BMC emulator port")
	flag.StringVar(&cfg.bmcUser, "bmc-user", "admin", "BMC username")
	flag.StringVar(&cfg.bmcPassword, "bmc-password", "password", "BMC password")
	flag.StringVar(&cfg.bootMode, "boot-mode", "UEFI", "Boot mode (UEFI, UEFISecureBoot, legacy)")
	flag.StringVar(&cfg.output, "output", "", "Output file (default: stdout)")
	flag.StringVar(&cfg.systemsFromSuhy, "systems-from-sushy", "", "Query sushy-tools at this URL for real system IDs")

	flag.Parse()

	return cfg
}

func run(cfg config) error {
	// Determine system IDs
	systemIDs, err := getSystemIDs(cfg)
	if err != nil {
		return fmt.Errorf("getting system IDs: %w", err)
	}

	if len(systemIDs) < cfg.numHosts {
		return fmt.Errorf("not enough systems: got %d, need %d", len(systemIDs), cfg.numHosts)
	}

	// Determine output writer
	var w io.Writer
	if cfg.output != "" {
		f, err := os.Create(cfg.output)
		if err != nil {
			return fmt.Errorf("creating output file: %w", err)
		}
		defer f.Close()
		w = f
	} else {
		w = os.Stdout
	}

	// Generate namespace
	if err := writeNamespace(w, cfg.namespace); err != nil {
		return err
	}

	// Generate BMH resources
	for i := range cfg.numHosts {
		if err := writeSecret(w, cfg, i); err != nil {
			return err
		}
		if err := writeBMH(w, cfg, systemIDs[i], i); err != nil {
			return err
		}
	}

	if cfg.output != "" {
		fmt.Fprintf(os.Stderr, "Generated %d BMH manifests to %s\n", cfg.numHosts, cfg.output)
	}

	return nil
}

// getSystemIDs returns the Redfish system ID paths to use for each host.
func getSystemIDs(cfg config) ([]string, error) {
	if cfg.systemsFromSuhy != "" {
		return fetchSystemIDsFromSushy(cfg.systemsFromSuhy, cfg.numHosts)
	}

	// Generate synthetic system ID paths
	ids := make([]string, cfg.numHosts)
	for i := range cfg.numHosts {
		ids[i] = fmt.Sprintf("/redfish/v1/Systems/%08d", i)
	}
	return ids, nil
}

// redfishSystemsResponse is the minimal Redfish response for /redfish/v1/Systems.
type redfishSystemsResponse struct {
	Members []struct {
		OdataID string `json:"@odata.id"` //nolint:tagliatelle // Redfish API uses this name
	} `json:"Members"` //nolint:tagliatelle // Redfish API uses this name
}

// fetchSystemIDsFromSushy queries a sushy-tools instance for available system IDs.
func fetchSystemIDsFromSushy(baseURL string, count int) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), httpTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/redfish/v1/Systems", http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	client := &http.Client{Timeout: httpTimeout}
	resp, err := client.Do(req) //nolint:gosec // URL is from a trusted CLI flag, not user-facing input
	if err != nil {
		return nil, fmt.Errorf("querying sushy-tools: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sushy-tools returned status %d", resp.StatusCode)
	}

	var systems redfishSystemsResponse
	if err := json.NewDecoder(resp.Body).Decode(&systems); err != nil {
		return nil, fmt.Errorf("decoding sushy-tools response: %w", err)
	}

	if len(systems.Members) < count {
		return nil, fmt.Errorf("sushy-tools has %d systems, need %d", len(systems.Members), count)
	}

	ids := make([]string, count)
	for i := range count {
		ids[i] = systems.Members[i].OdataID
	}
	return ids, nil
}

// generateMAC produces a deterministic, locally-administered MAC address from an index.
func generateMAC(index int) string {
	return fmt.Sprintf("02:00:00:%02x:%02x:%02x",
		(index>>macShiftHigh)&macByteMask,
		(index>>macShiftMid)&macByteMask,
		index&macByteMask,
	)
}

func writeYAML(w io.Writer, obj any) error {
	data, err := yaml.Marshal(obj)
	if err != nil {
		return fmt.Errorf("marshaling YAML: %w", err)
	}
	if _, err := fmt.Fprintf(w, "---\n%s", data); err != nil {
		return fmt.Errorf("writing YAML: %w", err)
	}
	return nil
}

type namespaceManifest struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
}

func writeNamespace(w io.Writer, namespace string) error {
	ns := namespaceManifest{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Namespace",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}
	return writeYAML(w, ns)
}

func writeSecret(w io.Writer, cfg config, index int) error {
	secretName := fmt.Sprintf("bmc-secret-%04d", index)

	secret := corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: cfg.namespace,
		},
		StringData: map[string]string{
			"username": cfg.bmcUser,
			"password": cfg.bmcPassword,
		},
	}
	return writeYAML(w, secret)
}

type bareMetalHostManifest struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              metal3api.BareMetalHostSpec `json:"spec,omitempty"`
}

func writeBMH(w io.Writer, cfg config, systemID string, index int) error {
	hostName := fmt.Sprintf("scale-host-%04d", index)
	secretName := fmt.Sprintf("bmc-secret-%04d", index)
	mac := generateMAC(index)
	hostPort := net.JoinHostPort(cfg.bmcAddress, strconv.Itoa(cfg.bmcPort))
	bmcAddr := fmt.Sprintf("redfish+http://%s%s", hostPort, systemID)

	spec := metal3api.BareMetalHostSpec{
		Online: true,
		BMC: metal3api.BMCDetails{
			Address:                        bmcAddr,
			CredentialsName:                secretName,
			DisableCertificateVerification: true,
		},
		BootMACAddress:        mac,
		BootMode:              metal3api.BootMode(cfg.bootMode),
		AutomatedCleaningMode: metal3api.CleaningModeDisabled,
		InspectionMode:        metal3api.InspectionModeDisabled,
	}

	bmh := bareMetalHostManifest{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "metal3.io/v1alpha1",
			Kind:       "BareMetalHost",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      hostName,
			Namespace: cfg.namespace,
		},
		Spec: spec,
	}

	return writeYAML(w, bmh)
}
