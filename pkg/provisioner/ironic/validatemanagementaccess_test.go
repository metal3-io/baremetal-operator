package ironic

import (
	"net/http"
	"testing"

	"github.com/gophercloud/gophercloud/openstack/baremetal/v1/nodes"
	"github.com/gophercloud/gophercloud/openstack/baremetal/v1/ports"
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

	result, err := prov.ValidateManagementAccess(false, false)
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
		Node(nodes.Node{
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

	result, err := prov.ValidateManagementAccess(false, false)
	if err != nil {
		t.Fatalf("error from ValidateManagementAccess: %s", err)
	}
	assert.Equal(t, "", result.ErrorMessage)
}

func TestValidateManagementAccessCreateNode(t *testing.T) {
	// Create a host without a bootMACAddress and with a BMC that
	// does not require one.
	host := makeHost()
	host.Spec.BootMACAddress = ""
	host.Status.Provisioning.ID = "" // so we don't lookup by uuid

	var createdNode *nodes.Node

	createCallback := func(node nodes.Node) {
		createdNode = &node
	}

	ironic := testserver.NewIronic(t).Ready().CreateNodes(createCallback).NoNode(host.Name)
	ironic.Start()
	defer ironic.Stop()

	auth := clients.AuthConfig{Type: clients.NoAuth}
	prov, err := newProvisionerWithSettings(host, bmc.Credentials{}, nullEventPublisher,
		ironic.Endpoint(), auth, testserver.NewInspector(t).Endpoint(), auth,
	)
	if err != nil {
		t.Fatalf("could not create provisioner: %s", err)
	}

	result, err := prov.ValidateManagementAccess(false, false)
	if err != nil {
		t.Fatalf("error from ValidateManagementAccess: %s", err)
	}
	assert.Equal(t, "", result.ErrorMessage)
	assert.NotEqual(t, "", createdNode.UUID)
	assert.Equal(t, createdNode.UUID, host.Status.Provisioning.ID)
}

func TestValidateManagementAccessExistingNode(t *testing.T) {
	// Create a host without a bootMACAddress and with a BMC that
	// does not require one.
	host := makeHost()
	host.Spec.BootMACAddress = ""
	host.Status.Provisioning.ID = "" // so we don't lookup by uuid

	createCallback := func(node nodes.Node) {
		t.Fatal("create callback should not be invoked for existing node")
	}

	ironic := testserver.NewIronic(t).Ready().CreateNodes(createCallback).Node(nodes.Node{
		Name: host.Name,
		UUID: "uuid",
	})
	ironic.Start()
	defer ironic.Stop()

	auth := clients.AuthConfig{Type: clients.NoAuth}
	prov, err := newProvisionerWithSettings(host, bmc.Credentials{}, nullEventPublisher,
		ironic.Endpoint(), auth, testserver.NewInspector(t).Endpoint(), auth,
	)
	if err != nil {
		t.Fatalf("could not create provisioner: %s", err)
	}

	result, err := prov.ValidateManagementAccess(false, false)
	if err != nil {
		t.Fatalf("error from ValidateManagementAccess: %s", err)
	}
	assert.Equal(t, "", result.ErrorMessage)
	assert.Equal(t, "uuid", host.Status.Provisioning.ID)
}

func TestValidateManagementAccessExistingNodeContinue(t *testing.T) {
	statuses := []nodes.ProvisionState{
		nodes.Manageable,
		nodes.Available,
		nodes.Active,
		nodes.DeployWait,
		nodes.Deploying,
		nodes.DeployFail,
		nodes.DeployDone,
		nodes.Deleting,
		nodes.Deleted,
		nodes.Cleaning,
		nodes.CleanWait,
		nodes.CleanFail,
		nodes.Error,
		nodes.Rebuild,
		nodes.Inspecting,
		nodes.InspectFail,
		nodes.InspectWait,
		nodes.Adopting,
		nodes.AdoptFail,
		nodes.Rescue,
		nodes.RescueFail,
		nodes.Rescuing,
		nodes.UnrescueFail,
	}

	for _, status := range statuses {
		t.Run(string(status), func(t *testing.T) {
			// Create a host without a bootMACAddress and with a BMC that
			// does not require one.
			host := makeHost()
			host.Spec.BootMACAddress = ""
			host.Status.Provisioning.ID = "" // so we don't lookup by uuid

			createCallback := func(node nodes.Node) {
				t.Fatal("create callback should not be invoked for existing node")
			}

			ironic := testserver.NewIronic(t).Ready().CreateNodes(createCallback).Node(nodes.Node{
				Name:           host.Name,
				UUID:           "", // to match status in host
				ProvisionState: string(status),
			})
			ironic.Start()
			defer ironic.Stop()

			auth := clients.AuthConfig{Type: clients.NoAuth}
			prov, err := newProvisionerWithSettings(host, bmc.Credentials{}, nullEventPublisher,
				ironic.Endpoint(), auth, testserver.NewInspector(t).Endpoint(), auth,
			)
			if err != nil {
				t.Fatalf("could not create provisioner: %s", err)
			}

			result, err := prov.ValidateManagementAccess(false, false)
			if err != nil {
				t.Fatalf("error from ValidateManagementAccess: %s", err)
			}
			assert.Equal(t, "", result.ErrorMessage)
			assert.Equal(t, false, result.Dirty)
		})
	}
}

func TestValidateManagementAccessExistingNodeWaiting(t *testing.T) {
	statuses := []nodes.ProvisionState{
		nodes.Enroll,
		nodes.Verifying,
	}

	for _, status := range statuses {
		t.Run(string(status), func(t *testing.T) {
			// Create a host without a bootMACAddress and with a BMC that
			// does not require one.
			host := makeHost()
			host.Spec.BootMACAddress = ""
			host.Status.Provisioning.ID = "uuid"

			createCallback := func(node nodes.Node) {
				t.Fatal("create callback should not be invoked for existing node")
			}

			node := nodes.Node{
				Name:           host.Name,
				UUID:           "uuid", // to match status in host
				ProvisionState: string(status),
			}
			ironic := testserver.NewIronic(t).Ready().CreateNodes(createCallback).Node(node).WithNodeStatesProvisionUpdate(node.UUID)
			ironic.Start()
			defer ironic.Stop()

			auth := clients.AuthConfig{Type: clients.NoAuth}
			prov, err := newProvisionerWithSettings(host, bmc.Credentials{}, nullEventPublisher,
				ironic.Endpoint(), auth, testserver.NewInspector(t).Endpoint(), auth,
			)
			if err != nil {
				t.Fatalf("could not create provisioner: %s", err)
			}

			result, err := prov.ValidateManagementAccess(false, false)
			if err != nil {
				t.Fatalf("error from ValidateManagementAccess: %s", err)
			}
			assert.Equal(t, "", result.ErrorMessage)
			assert.Equal(t, true, result.Dirty)
		})
	}
}

func TestValidateManagementAccessNewCredentials(t *testing.T) {
	// Create a host without a bootMACAddress and with a BMC that
	// does not require one.
	host := makeHost()
	host.Spec.BootMACAddress = ""
	host.Status.Provisioning.ID = "" // so we don't lookup by uuid

	ironic := testserver.NewIronic(t).
		Node(
			nodes.Node{
				Name: host.Name,
				UUID: "uuid",
			}).
		NodeUpdate(
			nodes.Node{
				Name: host.Name,
				UUID: "uuid",
				DriverInfo: map[string]interface{}{
					"test_address": "test.bmc",
				},
			})
	ironic.Start()
	defer ironic.Stop()

	auth := clients.AuthConfig{Type: clients.NoAuth}
	prov, err := newProvisionerWithSettings(host, bmc.Credentials{}, nullEventPublisher,
		ironic.Endpoint(), auth, testserver.NewInspector(t).Endpoint(), auth,
	)
	if err != nil {
		t.Fatalf("could not create provisioner: %s", err)
	}

	result, err := prov.ValidateManagementAccess(true, false)
	if err != nil {
		t.Fatalf("error from ValidateManagementAccess: %s", err)
	}
	assert.Equal(t, "", result.ErrorMessage)
	assert.Equal(t, "uuid", host.Status.Provisioning.ID)

	updates := ironic.GetLastNodeUpdateRequestFor("uuid")
	newValues := updates[0].Value.(map[string]interface{})
	assert.Equal(t, "test.bmc", newValues["test_address"])
}

func TestValidateManagementAccessLinkExistingIronicNodeByMAC(t *testing.T) {
	// Create an Ironic node, and then create a host with a matching MAC
	// Test to see if the node was found, and if the link is made

	existingNode := nodes.Node{
		UUID: "33ce8659-7400-4c68-9535-d10766f07a58",
	}

	existingNodePort := ports.Port{
		NodeUUID: existingNode.UUID,
		Address:  "11:11:11:11:11:11",
	}

	createCallback := func(node nodes.Node) {
		t.Fatal("create callback should not be invoked for existing node")
	}

	ironic := testserver.NewIronic(t).Ready().CreateNodes(createCallback).Node(existingNode).Port(existingNodePort)
	ironic.AddDefaultResponse("/v1/nodes/myhost", "GET", http.StatusNotFound, "")
	ironic.AddDefaultResponse("/v1/nodes/"+existingNode.UUID, "PATCH", http.StatusOK, "{}")
	ironic.Start()
	defer ironic.Stop()

	host := makeHost()
	host.Spec.BootMACAddress = "11:11:11:11:11:11"
	host.Status.Provisioning.ID = "" // so we don't lookup by uuid

	auth := clients.AuthConfig{Type: clients.NoAuth}
	prov, err := newProvisionerWithSettings(host, bmc.Credentials{}, nullEventPublisher,
		ironic.Endpoint(), auth, testserver.NewInspector(t).Endpoint(), auth,
	)
	if err != nil {
		t.Fatalf("could not create provisioner: %s", err)
	}

	result, err := prov.ValidateManagementAccess(false, false)
	if err != nil {
		t.Fatalf("error from ValidateManagementAccess: %s", err)
	}
	assert.Equal(t, "", result.ErrorMessage)
	assert.NotEqual(t, "", host.Status.Provisioning.ID)
}

func TestValidateManagementAccessExistingPortWithWrongUUID(t *testing.T) {
	// Create a node, and a port.  The port has a node uuid that doesn't match the node.
	// ValidateManagementAccess should return an error.

	existingNode := nodes.Node{
		UUID: "33ce8659-7400-4c68-9535-d10766f07a58",
	}

	existingNodePort := ports.Port{
		NodeUUID: "random-wrong-id",
		Address:  "11:11:11:11:11:11",
	}

	createCallback := func(node nodes.Node) {
		t.Fatal("create callback should not be invoked for existing node")
	}

	ironic := testserver.NewIronic(t).Ready().CreateNodes(createCallback).Node(existingNode).Port(existingNodePort)
	ironic.AddDefaultResponse("/v1/nodes/myhost", "GET", http.StatusNotFound, "")
	ironic.AddDefaultResponse("/v1/nodes/random-wrong-id", "GET", http.StatusNotFound, "")
	ironic.Start()
	defer ironic.Stop()

	host := makeHost()
	host.Spec.BootMACAddress = "11:11:11:11:11:11"
	host.Status.Provisioning.ID = "" // so we don't lookup by uuid

	auth := clients.AuthConfig{Type: clients.NoAuth}
	prov, err := newProvisionerWithSettings(host, bmc.Credentials{}, nullEventPublisher,
		ironic.Endpoint(), auth, testserver.NewInspector(t).Endpoint(), auth,
	)
	if err != nil {
		t.Fatalf("could not create provisioner: %s", err)
	}

	_, err = prov.ValidateManagementAccess(false, false)
	assert.EqualError(t, err, "failed to find existing host: port exists but linked node doesn't random-wrong-id: Resource not found")
}

func TestValidateManagementAccessExistingPortButHasName(t *testing.T) {
	// Create a node, and a port.
	// The port is linked to the node.
	// The port address matches the BMH BootMACAddress.
	// The node has a name, and the name doesn't match the BMH.
	// ValidateManagementAccess should return an error.

	existingNode := nodes.Node{
		UUID: "33ce8659-7400-4c68-9535-d10766f07a58",
		Name: "wrong-name",
	}

	existingNodePort := ports.Port{
		NodeUUID: existingNode.UUID,
		Address:  "11:11:11:11:11:11",
	}

	createCallback := func(node nodes.Node) {
		t.Fatal("create callback should not be invoked for existing node")
	}

	ironic := testserver.NewIronic(t).Ready().CreateNodes(createCallback).Node(existingNode).Port(existingNodePort)
	ironic.AddDefaultResponse("/v1/nodes/myhost", "GET", http.StatusNotFound, "")
	ironic.Start()
	defer ironic.Stop()

	host := makeHost()
	host.Spec.BootMACAddress = "11:11:11:11:11:11"
	host.Status.Provisioning.ID = "" // so we don't lookup by uuid

	auth := clients.AuthConfig{Type: clients.NoAuth}
	prov, err := newProvisionerWithSettings(host, bmc.Credentials{}, nullEventPublisher,
		ironic.Endpoint(), auth, testserver.NewInspector(t).Endpoint(), auth,
	)
	if err != nil {
		t.Fatalf("could not create provisioner: %s", err)
	}

	_, err = prov.ValidateManagementAccess(false, false)
	assert.EqualError(t, err, "failed to find existing host: node found by MAC but has a name: wrong-name")
}

func TestValidateManagementAccessAddTwoHostsWithSameMAC(t *testing.T) {

	existingNode := nodes.Node{
		UUID: "33ce8659-7400-4c68-9535-d10766f07a58",
		Name: "myhost",
	}

	existingNodePort := ports.Port{
		NodeUUID: existingNode.UUID,
		Address:  "11:11:11:11:11:11",
	}

	createCallback := func(node nodes.Node) {
		t.Fatal("create callback should not be invoked for existing node")
	}

	ironic := testserver.NewIronic(t).Ready().CreateNodes(createCallback).Node(existingNode).Port(existingNodePort)
	ironic.Start()
	defer ironic.Stop()

	host := makeHost()

	// This value is differen than the port that actually exists
	host.Spec.BootMACAddress = "22:22:22:22:22:22"
	host.Status.Provisioning.ID = "" // so we don't lookup by uuid

	auth := clients.AuthConfig{Type: clients.NoAuth}
	prov, err := newProvisionerWithSettings(host, bmc.Credentials{}, nullEventPublisher,
		ironic.Endpoint(), auth, testserver.NewInspector(t).Endpoint(), auth,
	)
	if err != nil {
		t.Fatalf("could not create provisioner: %s", err)
	}

	result, err := prov.ValidateManagementAccess(false, false)
	if err != nil {
		t.Fatalf("error from ValidateManagementAccess: %s", err)
	}
	assert.Equal(t, "", result.ErrorMessage)
	assert.NotEqual(t, "", host.Status.Provisioning.ID)
}
