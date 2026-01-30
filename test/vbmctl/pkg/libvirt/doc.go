//go:build vbmctl
// +build vbmctl

// Package libvirt provides a wrapper around the libvirt Go bindings for
// managing virtual machines, storage pools, volumes, and networks.
//
// This package is designed to be used by vbmctl for creating and managing
// virtual bare metal environments. It provides high-level abstractions
// over the low-level libvirt API.
//
// # VM Management
//
// Virtual machines can be created using VMManager:
//
//	conn, err := libvirtgo.NewConnect("qemu:///system")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer conn.Close()
//
//	manager, err := libvirt.NewVMManager(conn, libvirt.VMManagerOptions{
//	    PoolName: "default",
//	    PoolPath: "/var/lib/libvirt/images",
//	})
//
//	vm, err := manager.Create(ctx, api.VMConfig{
//	    Name:   "test-vm",
//	    Memory: 4096,
//	    VCPUs:  2,
//	    Volumes: []api.VolumeConfig{
//	        {Name: "root", Size: 20},
//	    },
//	})
//
// # Storage Management
//
// Storage pools and volumes can be managed using PoolManager:
//
//	poolManager := libvirt.NewPoolManager(conn)
//	pool, err := poolManager.Create(ctx, api.PoolConfig{
//	    Name: "my-pool",
//	    Path: "/var/lib/libvirt/my-pool",
//	})
//
// # Error Handling
//
// All operations return errors that can be inspected for specific failure modes.
// The package defines sentinel errors for common cases like resource not found
// or resource already exists.
package libvirt
