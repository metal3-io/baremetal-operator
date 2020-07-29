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
	o.AuthConfig.Type = clients.AuthType(os.Getenv("IRONIC_AUTH_STRATEGY"))

	switch o.AuthConfig.Type {
	case clients.HTTPBasicAuth:
		if len(os.Args) != 5 {
			fmt.Println("Usage: get-hardware-details <inspector URI> <inspector User> <inspector Password> <node UUID>")
			os.Exit(1)
		}
		o.AuthConfig.Username = os.Args[2]
		o.AuthConfig.Password = os.Args[3]
		o.NodeID = os.Args[4]
	default:
		if len(os.Args) != 3 {
			fmt.Println("Usage: get-hardware-details <inspector URI> <node UUID>")
			os.Exit(1)
		}
		o.NodeID = os.Args[2]
	}
	o.Endpoint = os.Args[1]
	return
}
