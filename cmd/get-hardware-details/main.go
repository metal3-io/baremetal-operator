// get-hardware-details is a tool that can be used to convert raw Ironic introspection data into the HardwareDetails
// type used by Metal3.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"strings"

	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/nodes"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/clients"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/hardwaredetails"
	"k8s.io/klog/v2"
)

const (
	reqOptArgs = 3
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
	var ironicInsecure bool
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
		log.Fatalf("invalid ironic endpoint: %s", err)
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
		log.Fatalf("could not get ironic client: %s", err)
	}

	introData := nodes.GetInventory(context.TODO(), ironic, opts.NodeID)
	data, err := introData.Extract()
	if err != nil {
		log.Fatalf("could not get inspection data: %s", err)
	}

	json, err := json.MarshalIndent(hardwaredetails.GetHardwareDetails(data, klog.NewKlogr()), "", "\t")
	if err != nil {
		log.Fatalf("could not convert inspection data: %s", err)
	}

	//nolint:forbidigo
	fmt.Println(string(json))
}

func getOptions() (o options) {
	if len(os.Args) != reqOptArgs {
		log.Fatalln("Usage: get-hardware-details <ironic URI> <node UUID>")
	}

	var err error
	o.Endpoint, o.AuthConfig, err = clients.ConfigFromEndpointURL(os.Args[1])
	if err != nil {
		log.Fatalf("Error: %s\n", err)
	}
	o.NodeID = os.Args[2]
	return
}
