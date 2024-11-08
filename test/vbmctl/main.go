//go:build vbmctl
// +build vbmctl

package main

import (
	"bytes"
	"embed"
	"flag"
	"fmt"
	"os"
	"text/template"

	"github.com/dypflying/go-qcow2lib/qcow2"
	bmoe2e "github.com/metal3-io/baremetal-operator/test/e2e"
	"libvirt.org/go/libvirt"
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
func CreateVolumePool(poolName, poolPath string) (*libvirt.StoragePool, error) {
	// Connect to libvirt daemon
	conn, err := libvirt.NewConnect("qemu:///system")
	if err != nil {
		fmt.Println("Failed to connect to qemu:///system")
		return nil, err
	}
	defer conn.Close()

	pool, err := conn.LookupStoragePoolByName(poolName)

	if err == nil {
		fmt.Println("Pool already exists")
		return pool, nil
	}

	if err = os.Mkdir(poolPath, 0777); err != nil && !os.IsExist(err) {
		fmt.Println("Cannot determine the state of the poolPath")
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
		fmt.Println("Failed to read pool XML file")
		fmt.Printf("Error occurred: %v\n", err)
		return nil, err
	}

	// Create the volume pool
	pool, err = conn.StoragePoolDefineXML(poolCfg, 0)

	if err != nil {
		fmt.Println("Failed to create volume pool")
		fmt.Printf("Error occurred: %v\n", err)
		return nil, err
	}

	if err = pool.SetAutostart(true); err != nil {
		fmt.Println("Failed to Set the pool autostart")
		fmt.Printf("Error occurred: %v\n", err)
		return nil, err
	}

	if err = pool.Create(0); err != nil {
		fmt.Println("Failed to Start the pool")
		fmt.Printf("Error occurred: %v\n", err)
		return nil, err
	}

	fmt.Println("Volume pool created successfully")
	return pool, nil
}

func CreateVolume(volumeName, poolName, poolPath string, capacityInGB int) error {
	// Connect to libvirt daemon
	conn, err := libvirt.NewConnect("qemu:///system")
	if err != nil {
		fmt.Println("Failed to connect to qemu:///system")
		return err
	}
	defer conn.Close()

	pool, err := CreateVolumePool(poolName, poolPath)

	if err != nil {
		fmt.Println("Failed to create storage pool")
		fmt.Printf("Error occurred: %v\n", err)
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
		fmt.Println("Failed to read volume XML file")
		fmt.Printf("Error occurred: %v\n", err)
		return err
	}

	// Create the volume
	_, err = pool.StorageVolCreateXML(volumeCfg, 0)

	if err != nil {
		fmt.Println("Failed to create volume")
		fmt.Printf("Error occurred: %v\n", err)
		return err
	}

	fmt.Println("Volume created successfully")
	return nil
}

// CreateLibvirtVM creates a new virtual machine with the given hostname,
// network name, and MAC address. It first creates a qcow2 file with a size
// of 3GB and defines it in the default storage pool. The function then connects
// to the libvirt daemon and uses a template to generate the VM's XML configuration.
// If the domain is successfully defined and created, the virtual machine is
// started. Errors during qcow2 file creation, volume creation, libvirt connection,
// template rendering, or domain creation are returned.
func CreateLibvirtVM(hostName, networkName, macAddress string) error {
	poolName := "default"
	poolPath := "/tmp/pool_oo"
	opts := make(map[string]any)
	opts[qcow2.OPT_SIZE] = 3 * (1 << 30) // qcow2 file's size is 3g
	opts[qcow2.OPT_FMT] = "qcow2"        // qcow2 format
	opts[qcow2.OPT_SUBCLUSTER] = true    // enable sub-cluster

	err := qcow2.Blk_Create("/tmp/"+hostName+".qcow2", opts)

	if err != nil {
		fmt.Println("Failed to create qcow2 file")
		fmt.Printf("Error occurred: %v\n", err)
		return err
	}

	if err = CreateVolume(hostName, poolName, poolPath, 20); err != nil {
		return err
	}

	conn, err := libvirt.NewConnect("qemu:///system")
	if err != nil {
		fmt.Println("Failed to connect to qemu:///system")
		return err
	}
	defer conn.Close()

	data := struct {
		HostName   string
		Network    string
		MacAddress string
		PoolPath   string
	}{
		HostName:   hostName,
		Network:    networkName,
		MacAddress: macAddress,
		PoolPath:   poolPath,
	}

	vmCfg, err := RenderTemplate("templates/VM.xml.tpl", data)

	if err != nil {
		return err
	}

	dom, err := conn.DomainDefineXML(vmCfg)

	if err != nil {
		fmt.Println("Failed to define domain")
		fmt.Printf("Error occurred: %v\n", err)
		return err
	}

	if err := dom.Create(); err != nil {
		fmt.Println("Failed to create domain")
		fmt.Printf("Error occurred: %v\n", err)
		return err
	}

	fmt.Println("Domain created successfully")
	return nil
}

// CreateLibvirtBMC creates a VM with the given MAC address, hostname, IP address
// and adds a DHCP host entry on the given network.
//
// It will return an error if the network does not exist, or if creating the VM
// or adding the DHCP host entry fails.
func CreateLibvirtBMC(macAddress, hostName, ipAddress, networkName string) error {
	var err error
	conn, err := libvirt.NewConnect("qemu:///system")
	if err != nil {
		return err
	}
	defer conn.Close()

	network, err := conn.LookupNetworkByName(networkName)
	if err != nil {
		return err
	}

	xmlTpl, err := template.New("xml").Parse("<host mac='{{ .MacAddress }}' name='{{ .HostName }}' ip='{{ .IPAddress }}' />")

	if err != nil {
		return err
	}

	data := struct {
		MacAddress string
		HostName   string
		IPAddress  string
	}{
		MacAddress: macAddress,
		HostName:   hostName,
		IPAddress:  ipAddress,
	}

	var buf bytes.Buffer

	err = xmlTpl.Execute(&buf, data)

	if err != nil {
		fmt.Println("Failed to create BMC")
		fmt.Printf("Error occurred: %v\n", err)
		return err
	}

	if err = network.Update(
		libvirt.NETWORK_UPDATE_COMMAND_ADD_LAST,
		libvirt.NETWORK_SECTION_IP_DHCP_HOST,
		-1,
		buf.String(),
		libvirt.NETWORK_UPDATE_AFFECT_LIVE|libvirt.NETWORK_UPDATE_AFFECT_CONFIG,
	); err != nil {
		fmt.Printf("Error occurred: %v\n", err)
		return err
	}
	if err = CreateLibvirtVM(hostName, networkName, macAddress); err != nil {
		fmt.Printf("Error occurred: %v\n", err)
		return err
	}
	return nil
}

func main() {
	var vmName = flag.String(
		"vm-name", "VM-1", "The name of the VM to create")
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
	if *configFile == "" {
		if err = CreateLibvirtBMC(*macAddress, *vmName, *ipAddress, *networkName); err != nil {
			fmt.Printf("Error occurred: %v\n", err)
			os.Exit(1)
		}
	} else {
		bmcs, err := bmoe2e.LoadBMCConfig(*configFile)
		if err != nil {
			os.Exit(1)
		}
		for _, bmc := range *bmcs {
			if err = CreateLibvirtBMC(bmc.BootMacAddress, bmc.HostName, bmc.IPAddress, "baremetal-e2e"); err != nil {
				fmt.Printf("Error occurred: %v\n", err)
				os.Exit(1)
			}
		}
	}
}
