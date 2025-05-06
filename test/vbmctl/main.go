//go:build vbmctl
// +build vbmctl

package main

import (
	"bytes"
	"embed"
	"flag"
	"log"
	"os"
	"text/template"

	"github.com/dypflying/go-qcow2lib/qcow2"
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

// CreateLibvirtVM creates a new virtual machine with the given name,
// network name, and MAC address. It first creates a qcow2 file with a size
// of 3GB and defines it in the default storage pool. The function then connects
// to the libvirt daemon and uses a template to generate the VM's XML configuration.
// If the domain is successfully defined and created, the virtual machine is
// started. Errors during qcow2 file creation, volume creation, libvirt connection,
// template rendering, or domain creation are returned.
func CreateLibvirtVM(conn *libvirt.Connect, name, networkName, macAddress string) error {
	poolName := "default"
	poolPath := "/tmp/pool_oo"
	opts := make(map[string]any)
	opts[qcow2.OPT_SIZE] = 3 * (1 << 30) //nolint: mnd // qcow2 file's size is 3g
	opts[qcow2.OPT_FMT] = "qcow2"        // qcow2 format
	opts[qcow2.OPT_SUBCLUSTER] = true    // enable sub-cluster

	err := qcow2.Blk_Create("/tmp/"+name+".qcow2", opts)

	if err != nil {
		log.Println("Failed to create qcow2 file")
		log.Printf("Error occurred: %v\n", err)
		return err
	}

	if err = CreateVolume(conn, name, poolName, poolPath, 20); err != nil { //nolint: mnd
		return err
	}

	data := struct {
		Name       string
		Network    string
		MacAddress string
		PoolPath   string
	}{
		Name:       name,
		Network:    networkName,
		MacAddress: macAddress,
		PoolPath:   poolPath,
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

// CreateLibvirtVMWithReservedIPAddress creates a VM with the given MAC address, name, IP address
// and adds a DHCP host entry on the given network.
//
// It will return an error if the network does not exist, or if creating the VM
// or adding the DHCP host entry fails.
func CreateLibvirtVMWithReservedIPAddress(conn *libvirt.Connect, macAddress, name, ipAddress, networkName string) error {
	network, err := conn.LookupNetworkByName(networkName)
	if err != nil {
		return err
	}

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
	if err = CreateLibvirtVM(conn, name, networkName, macAddress); err != nil {
		log.Printf("Error occurred: %v\n", err)
		return err
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
		"ip-address", "192.168.222.122", "IP address of the VM on the network")
	var configFile = flag.String(
		"yaml-source-file", "", "yaml file where BMCS are defined. If this is set, ignore all other options")
	flag.Parse()
	var err error
	bmcs := []bmoe2e.BMC{}
	if *configFile == "" {
		bmc := bmoe2e.BMC{
			IPAddress:      *ipAddress,
			BootMacAddress: *macAddress,
			Name:           *name,
			NetworkName:    *networkName,
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
		if err = CreateLibvirtVMWithReservedIPAddress(conn, bmc.BootMacAddress, bmc.Name, bmc.IPAddress, "baremetal-e2e"); err != nil {
			log.Printf("Error occurred: %v\n", err)
			// Not using os.Exit here so that we still close the connection
			break
		}
	}
}
