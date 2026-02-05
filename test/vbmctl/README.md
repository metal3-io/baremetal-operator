# vbmctl - Virtual Bare Metal Controller

`vbmctl` is a tool for creating and managing virtual bare metal environments for
testing and development purposes. It is designed to be as simple to use as
`kind` while providing comprehensive functionality for simulating bare metal
infrastructure.

## Implementation Status

This tool is under active development.

| Feature | Status |
|---------|--------|
| Project structure & CLI | ✅ Implemented |
| Configuration system | ✅ Implemented |
| VM management (create/delete/list) | ✅ Implemented |
| Storage pool management | ✅ Implemented |
| Volume management | ✅ Implemented |
| `vbmctl create bml` / `vbmctl delete bml` | ✅ Implemented |
| `vbmctl status` | ✅ Implemented (basic) |
| State management (persistent state) | ❌ TODO |

## Features

- **VM Management**: Create, delete, and manage virtual machines using libvirt ✅
- **Configurable Resources**: Memory, vCPUs, and volumes are fully configurable ✅
- **DHCP Reservation**: Reserve IP addresses for VMs via DHCP ✅
- **Library Support**: Can be imported as a Go module for programmatic use ✅

## Build Tags

The `vbmctl` build tag is used to isolate libvirt-dependent code. This allows
developers to work on other parts of BMO without needing to install libvirt
development headers.

```bash
# Build/test packages that DON'T require libvirt (no special tag needed)
go build ./pkg/api/... ./pkg/config/...
go test ./pkg/api/... ./pkg/config/...

# Build/test ALL packages including libvirt (requires libvirt-dev)
go build -tags vbmctl ./...
go test -tags vbmctl ./...
```

## Usage

### CLI Commands

```bash
# Initialize a configuration file
vbmctl config init

# View current configuration
vbmctl config view

# Create a single virtual machine
vbmctl create vm --name test-vm --memory 4096 --vcpus 2

# Create VM with custom options
vbmctl create vm \
  --name bmo-e2e-0 \
  --memory 8192 \
  --vcpus 4 \
  --volume-size 50 \
  --network baremetal-e2e \
  --mac-address 00:60:2f:31:81:01 \
  --ip-address 192.168.222.100

# Create a bare metal lab (all VMs defined in spec.vms of the config file)
vbmctl create bml

# Check status
vbmctl status

# Delete a VM (also deletes its volumes)
vbmctl delete vm test-vm

# Delete the bare metal lab (all VMs defined in spec.vms of the config file)
vbmctl delete bml

# Show help
vbmctl --help
```

### Bare Metal Lab Workflow

The recommended workflow is to define your VMs in a configuration file and use
the `bml` (bare metal lab) commands:

```bash
# 1. Initialize a config file
vbmctl config init

# 2. Edit vbmctl.yaml to define your VMs in spec.vms (see example below)

# 3. Create the bare metal lab
vbmctl create bml

# 4. Use your VMs for testing...

# 5. Clean up (deletes VMs and their volumes)
vbmctl delete bml
```

### Configuration File

vbmctl supports YAML configuration files. Create a `vbmctl.yaml` file:

```yaml
apiVersion: vbmctl.metal3.io/v1alpha1
kind: Config
spec:
  libvirt:
    uri: "qemu:///system"
  pool:
    name: "baremetal-e2e"
    path: "/tmp/pool_oo"
  # VMs to create with 'vbmctl create bml'
  vms:
  - name: "bmo-e2e-0"
    memory: 4096      # Memory in MB
    vcpus: 2          # Number of vCPUs
    volumes:
    - name: "root"
      size: 20      # Size in GB
    - name: "data"
      size: 10
    networks:
    - network: "baremetal-e2e"
      macAddress: "00:60:2f:31:81:01"
      ipAddress: "192.168.222.100"  # Optional: reserve via DHCP
  - name: "bmo-e2e-1"
    memory: 4096
    vcpus: 2
    volumes:
    - name: "root"
      size: 20
    - name: "data"
      size: 10
    networks:
    - network: "baremetal-e2e"
      macAddress: "00:60:2f:31:81:02"
      ipAddress: "192.168.222.101"
```

The `spec.vms` section defines the VMs that will be created when you run `vbmctl
create bml` and deleted when you run `vbmctl delete bml`.

## Library Usage

vbmctl can be imported as a Go module for programmatic use in tests or other
applications:

```go
import (
    "context"

    "github.com/metal3-io/baremetal-operator/test/vbmctl/pkg/api"
    "github.com/metal3-io/baremetal-operator/test/vbmctl/pkg/config"
    "github.com/metal3-io/baremetal-operator/test/vbmctl/pkg/libvirt"
)

func main() {
    ctx := context.Background()

    // Connect to libvirt
    conn, err := libvirt.NewConnection("qemu:///system")
    if err != nil {
        log.Fatal(err)
    }
    defer conn.Close()

    // Create VM manager
    vmManager, err := libvirt.NewVMManager(conn, libvirt.VMManagerOptions{
        PoolName: "baremetal-e2e",
        PoolPath: "/tmp/pool_oo",
    })
    if err != nil {
        log.Fatal(err)
    }

    // Create a VM
    vm, err := vmManager.Create(ctx, api.VMConfig{
        Name:   "test-vm",
        Memory: 4096,  // MB
        VCPUs:  2,
        Volumes: []api.VolumeConfig{
            {Name: "root", Size: 20},  // GB
        },
        Networks: []api.NetworkAttachment{
            {
                Network:    "baremetal-e2e",
                MACAddress: "00:60:2f:31:81:01",
            },
        },
    })
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Created VM: %s (UUID: %s)\n", vm.Config.Name, vm.UUID)
}
```
