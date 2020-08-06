// get-hardware-details is a tool that can be used to convert raw Ironic introspection data into the HardwareDetails
// type used by Metal3.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/gophercloud/gophercloud/openstack/baremetalintrospection/v1/introspection"
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

	inspector, err := clients.InspectorClient(opts.Endpoint, opts.AuthConfig)
	if err != nil {
		fmt.Printf("could not get inspector client: %s", err)
		os.Exit(1)
	}

	introData := introspection.GetIntrospectionData(inspector, opts.NodeID)
	data, err := introData.Extract()
	if err != nil {
		fmt.Printf("could not get introspection data: %s", err)
		os.Exit(1)
	}

	json, err := json.MarshalIndent(hardwaredetails.GetHardwareDetails(data), "", "\t")
	if err != nil {
		fmt.Printf("could not convert introspection data: %s", err)
		os.Exit(1)
	}

	fmt.Println(string(json))
}

func getOptions() (o options) {
	if len(os.Args) != 3 {
		fmt.Println("Usage: get-hardware-details <inspector URI> <node UUID>")
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
