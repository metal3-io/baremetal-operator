// get-hardware-details is a tool that can be used to convert raw Ironic introspection data into the HardwareDetails
// type used by Metal3.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"

	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/nodes"
	"k8s.io/klog/v2"

	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/clients"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/hardwaredetails"
)

type options struct {
	Endpoint   string
	AuthConfig clients.AuthConfig
	NodeID     string
}

func main() {
	opts := getOptions()
	ironicTrustedCAFile := os.Getenv("IRONIC_CACERT_FILE")
	ironicInsecureStr := os.Getenv("IRONIC_INSECURE")
	ironicInsecure := false
	if strings.EqualFold(ironicInsecureStr, "true") {
		ironicInsecure = true
	}

	tlsConf := clients.TLSConfig{
		TrustedCAFile:      ironicTrustedCAFile,
		InsecureSkipVerify: ironicInsecure,
	}

	endpoint := opts.Endpoint
	parsedEndpoint, err := url.Parse(endpoint)
	if err != nil {
		fmt.Printf("invalid ironic endpoint: %s", err)
		os.Exit(1)
	}

	// Previously, this command accepted the Inspector endpoint. But since
	// we're transitioning to not using Inspector directly, it now requires
	// the Ironic endpoint. Try to handle the transition by checking for
	// the well-known Inspector port and replacing it with the Ironic port.
	if parsedEndpoint.Port() == "5050" {
		parsedEndpoint.Host = net.JoinHostPort(parsedEndpoint.Hostname(), "6385")
		endpoint = parsedEndpoint.String()
	}

	ironic, err := clients.IronicClient(endpoint, opts.AuthConfig, tlsConf)
	if err != nil {
		fmt.Printf("could not get ironic client: %s", err)
		os.Exit(1)
	}

	introData := nodes.GetInventory(context.TODO(), ironic, opts.NodeID)
	data, err := introData.Extract()
	if err != nil {
		fmt.Printf("could not get inspection data: %s", err)
		os.Exit(1)
	}

	json, err := json.MarshalIndent(hardwaredetails.GetHardwareDetails(data, klog.NewKlogr()), "", "\t")
	if err != nil {
		fmt.Printf("could not convert inspection data: %s", err)
		os.Exit(1)
	}

	fmt.Println(string(json))
}

func getOptions() (o options) {
	if len(os.Args) != 3 {
		fmt.Println("Usage: get-hardware-details <ironic URI> <node UUID>")
		os.Exit(1)
	}

	var err error
	o.Endpoint, o.AuthConfig, err = clients.ConfigFromEndpointURL(os.Args[1])
	if err != nil {
		fmt.Printf("Error: %s\n", err)
		os.Exit(1)
	}
	o.NodeID = os.Args[2]
	return
}
