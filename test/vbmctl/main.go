//go:build vbmctl
// +build vbmctl

package main

import (
	"bytes"
	"embed"
	"flag"
	"log"
	"os"
	"strconv"
	"text/template"

	bmoe2e "github.com/metal3-io/baremetal-operator/test/e2e"
	"libvirt.org/go/libvirt"
)

const (
	filePerm777 = 0777
)

var (
	//go:embed templates/*.tpl
	templateFiles embed.FS
)

func RenderTemplate(inputFile string, data interface{}) (string, error) {
	tmpl, err := template.ParseFS(templateFiles, inputFile)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer

	if err = tmpl.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// CreateVolumePool creates a volume pool with specified name if a pool with
// that name does not exist yet.
func CreateVolumePool(conn *libvirt.Connect, poolName, poolPath string) (*libvirt.StoragePool, error) {
	pool, err := conn.LookupStoragePoolByName(poolName)

	if err == nil {
		log.Println("Pool already exists")
		return pool, nil
	}

	if err = os.Mkdir(poolPath, filePerm777); err != nil && !os.IsExist(err) {
		log.Println("Cannot determine the state of the poolPath")
		return nil, err
	}

	data := struct {
		PoolName string
		PoolPath string
	}{
		PoolName: poolName,
		PoolPath: poolPath,
	}

	poolCfg, err := RenderTemplate("templates/pool.xml.tpl", data)

	if err != nil {
		log.Println("Failed to read pool XML file")
		log.Printf("Error occurred: %v\n", err)
		return nil, err
	}

	// Create the volume pool
	pool, err = conn.StoragePoolDefineXML(poolCfg, 0)

	if err != nil {
		log.Println("Failed to create volume pool")
		log.Printf("Error occurred: %v\n", err)
		return nil, err
	}

	if err = pool.SetAutostart(true); err != nil {
		log.Println("Failed to Set the pool autostart")
		log.Printf("Error occurred: %v\n", err)
		return nil, err
	}

	if err = pool.Create(0); err != nil {
		log.Println("Failed to Start the pool")
		log.Printf("Error occurred: %v\n", err)
		return nil, err
	}

	log.Println("Volume pool created successfully")
	return pool, nil
}

func CreateVolume(conn *libvirt.Connect, volumeName, poolName, poolPath string, capacityInGB int) error {
	pool, err := CreateVolumePool(conn, poolName, poolPath)

	if err != nil {
		log.Println("Failed to create storage pool")
		log.Printf("Error occurred: %v\n", err)
		return err
	}

	data := struct {
		VolumeName         string
		VolumeCapacityInGB int
	}{
		VolumeName:         volumeName,
		VolumeCapacityInGB: capacityInGB,
	}

	volumeCfg, err := RenderTemplate("templates/volume.xml.tpl", data)

	if err != nil {
		log.Println("Failed to read volume XML file")
		log.Printf("Error occurred: %v\n", err)
		return err
	}

	// Create the volume
	_, err = pool.StorageVolCreateXML(volumeCfg, 0)

	if err != nil {
		log.Println("Failed to create volume")
		log.Printf("Error occurred: %v\n", err)
		return err
	}

	log.Println("Volume created successfully")
	return nil
}

// CreateLibvirtVM creates a new virtual machine based on the bmc details.
// The VM is defined only, not started. Two volumes are also created for the VM.
func CreateLibvirtVM(conn *libvirt.Connect, bmc *bmoe2e.BMC) error {
	if err := ReserveIPAddresses(conn, bmc.Name, bmc.Networks); err != nil {
		log.Printf("Error occurred: %v\n", err)
		return err
	}
	poolName := "baremetal-e2e"
	poolPath := "/tmp/pool_oo"
	if err := CreateVolume(conn, bmc.Name+"-1", poolName, poolPath, 20); err != nil { //nolint: mnd
		return err
	}

	if err := CreateVolume(conn, bmc.Name+"-2", poolName, poolPath, 20); err != nil { //nolint: mnd
		return err
	}

	data := struct {
		Name     string
		Networks []bmoe2e.Network
		PoolPath string
	}{
		Name:     bmc.Name,
		Networks: bmc.Networks,
		PoolPath: poolPath,
	}

	vmCfg, err := RenderTemplate("templates/VM.xml.tpl", data)

	if err != nil {
		return err
	}

	_, err = conn.DomainDefineXML(vmCfg)

	if err != nil {
		log.Println("Failed to define domain")
		log.Printf("Error occurred: %v\n", err)
		return err
	}

	log.Println("Domain created successfully")
	return nil
}

// updateNetwork is a helper function for CreateLibvirtVMWithReservedIPAddress.
// It updates the network with a DHCP host entry.
func updateNetwork(network *libvirt.Network, macAddress, name, ipAddress string) error {
	xmlTpl, err := template.New("xml").Parse("<host mac='{{ .MacAddress }}' name='{{ .Name }}' ip='{{ .IPAddress }}' />")

	if err != nil {
		return err
	}

	data := struct {
		MacAddress string
		Name       string
		IPAddress  string
	}{
		MacAddress: macAddress,
		Name:       name,
		IPAddress:  ipAddress,
	}

	var buf bytes.Buffer

	err = xmlTpl.Execute(&buf, data)

	if err != nil {
		log.Println("Failed to create BMC")
		log.Printf("Error occurred: %v\n", err)
		return err
	}

	if err = network.Update(
		libvirt.NETWORK_UPDATE_COMMAND_ADD_LAST,
		libvirt.NETWORK_SECTION_IP_DHCP_HOST,
		-1,
		buf.String(),
		libvirt.NETWORK_UPDATE_AFFECT_LIVE|libvirt.NETWORK_UPDATE_AFFECT_CONFIG,
	); err != nil {
		log.Printf("Error occurred: %v\n", err)
		return err
	}
	return nil
}

// ReserveIPAddresses adds a DHCP host entry for all networks that
// specify an IP address.
func ReserveIPAddresses(conn *libvirt.Connect, hostName string, networks []bmoe2e.Network) error {
	for i, net := range networks {
		// Checking if this network has IP specified.
		if net.IPAddress != "" {
			network, err := conn.LookupNetworkByName(net.Name)
			if err != nil {
				return err
			}
			err = updateNetwork(network, net.MacAddress, hostName+"-"+strconv.Itoa(i), net.IPAddress)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func main() {
	var name = flag.String(
		"name", "BMH-0", "The name of the VM to create")
	var networkName = flag.String(
		"network-name", "baremetal-e2e", "The name of the network that the new VM should be attached to")
	var macAddress = flag.String(
		"mac-address", "00:60:2f:31:81:01", "Mac address of the VM on the network")
	var ipAddress = flag.String(
		"ip-address", "", "IP address of the VM on the network")
	var configFile = flag.String(
		"yaml-source-file", "", "yaml file where BMCS are defined. If this is set, ignore all other options")
	flag.Parse()
	var err error
	bmcs := []bmoe2e.BMC{}
	if *configFile == "" {
		bmc := bmoe2e.BMC{
			BootMacAddress: *macAddress,
			Name:           *name,
			Networks: []bmoe2e.Network{
				{
					Name:       *networkName,
					MacAddress: *macAddress,
					IPAddress:  *ipAddress,
				},
			},
		}
		bmcs = append(bmcs, bmc)
	} else {
		bmcs, err = bmoe2e.LoadBMCConfig(*configFile)
		if err != nil {
			log.Fatalf("Error occurred: %v\n", err)
		}
	}

	// Connect to Libvirt
	conn, err := libvirt.NewConnect("qemu:///system")
	if err != nil {
		log.Fatalf("Error occurred: %v\n", err)
	}
	defer conn.Close()

	for _, bmc := range bmcs {
		if err = CreateLibvirtVM(conn, &bmc); err != nil {
			log.Printf("Error occurred: %v\n", err)
			break
		}
	}
}
