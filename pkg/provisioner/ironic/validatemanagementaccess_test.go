package ironic

import (
	"testing"

	"github.com/gophercloud/gophercloud/openstack/baremetal/v1/nodes"
	"github.com/stretchr/testify/assert"

	"github.com/metal3-io/baremetal-operator/pkg/bmc"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/clients"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/testserver"
)

func TestValidateManagementAccessNoMAC(t *testing.T) {
	// Create a host without a bootMACAddress and with a BMC that
	// requires one.
	host := makeHost()
	host.Spec.BMC.Address = "test-needs-mac://"
	host.Spec.BootMACAddress = ""
	host.Status.Provisioning.ID = "" // so we don't lookup by uuid

	ironic := testserver.NewIronic(t).Ready().NoNode(host.Name)
	ironic.Start()
	defer ironic.Stop()

	auth := clients.AuthConfig{Type: clients.NoAuth}
	prov, err := newProvisionerWithSettings(host, bmc.Credentials{}, nil,
		ironic.Endpoint(), auth, testserver.NewInspector(t).Endpoint(), auth,
	)
	if err != nil {
		t.Fatalf("could not create provisioner: %s", err)
	}

	result, err := prov.ValidateManagementAccess(false)
	if err != nil {
		t.Fatalf("error from ValidateManagementAccess: %s", err)
	}
	assert.Contains(t, result.ErrorMessage, "requires a BootMACAddress")
}

func TestValidateManagementAccessMACOptional(t *testing.T) {
	// Create a host without a bootMACAddress and with a BMC that
	// does not require one.
	host := makeHost()
	host.Spec.BootMACAddress = ""

	// Set up ironic server to return the node
	ironic := testserver.NewIronic(t).Ready().
		WithNode(nodes.Node{
			Name: host.Name,
			UUID: host.Status.Provisioning.ID,
		})
	ironic.Start()
	defer ironic.Stop()

	auth := clients.AuthConfig{Type: clients.NoAuth}
	prov, err := newProvisionerWithSettings(host, bmc.Credentials{}, nil,
		ironic.Endpoint(), auth, testserver.NewInspector(t).Endpoint(), auth,
	)
	if err != nil {
		t.Fatalf("could not create provisioner: %s", err)
	}

	result, err := prov.ValidateManagementAccess(false)
	if err != nil {
		t.Fatalf("error from ValidateManagementAccess: %s", err)
	}
	assert.Equal(t, "", result.ErrorMessage)
}
