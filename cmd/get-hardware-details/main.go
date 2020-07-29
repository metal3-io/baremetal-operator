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

func main() {
	var nodeID string
	authConfig := clients.AuthConfig{
		Type: clients.AuthType(os.Getenv("IRONIC_AUTH_STRATEGY")),
	}

	switch authConfig.Type {
	case clients.HTTPBasicAuth:
		if len(os.Args) != 5 {
			fmt.Println("Usage: get-hardware-details <inspector URI> <inspector User> <inspector Password> <node UUID>")
			os.Exit(1)
		}
		authConfig.Username = os.Args[2]
		authConfig.Password = os.Args[3]
		nodeID = os.Args[4]
	default:
		if len(os.Args) != 3 {
			fmt.Println("Usage: get-hardware-details <inspector URI> <node UUID>")
			os.Exit(1)
		}
		nodeID = os.Args[2]
	}
	inspectorEndpoint := os.Args[1]
	inspector, err := clients.InspectorClient(inspectorEndpoint, authConfig)
	if err != nil {
		fmt.Printf("could not get inspector client: %s", err)
		os.Exit(1)
	}

	introData := introspection.GetIntrospectionData(inspector, nodeID)
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
