//go:build vbmctl
// +build vbmctl

// Package containers provides a wrapper around the Docker/Moby client for
// managing containers and image servers.
//
// This package is designed to be used by vbmctl for creating and managing
// container-based services in virtual bare metal environments. It provides
// high-level abstractions over the low-level Docker API.
//
// # Container Management
//
// Containers can be created and started using CreateRunningContainer:
//
//	opts := &client.ContainerCreateOptions{
//	    Name: "bmc-emulator",
//	    Config: &container.Config{
//	        Image: "my-bmc:latest",
//	        Env:   []string{"BMC_USER=admin"},
//	    },
//	}
//	err := containers.CreateRunningContainer(ctx, "BMC emulator", opts)
//
// The function checks whether the container already exists before creating it,
// and starts it regardless of whether it was freshly created or was previously
// stopped.
//
// Containers can be removed forcefully using DeleteContainer:
//
//	err := containers.DeleteContainer(ctx, "BMC emulator", "bmc-emulator")
//
// The current status and short ID of a container can be retrieved using
// GetContainerInfo:
//
//	info, err := containers.GetContainerInfo(ctx, "bmc-emulator")
//	// info is e.g. "bmc-emulator 'running' (id: a1b2c3d4e5f6)" or "'not found'"
//
// # Image Server Management
//
// An image server container can be provisioned using
// CreateImageServerInstance:
//
//	err := containers.CreateImageServerInstance(ctx, &api.ImageServerConfig{
//	    Image:             "nginx:latest",
//	    ContainerName:     "vbmctl-image-server",
//	    DataDir:           "/var/lib/vbmctl/images",
//	    ContainerDataDir:  "/data",
//	    Port:              8080,
//	    ContainerPort:     80,
//	})
//
// # BMC Emulator Management
//
// A BMC emulator container can be provisioned using
// CreateBMCEmulatorInstance:
//
// For the "vbmc" type, only the Image field is required.
//
//	err := containers.CreateBMCEmulatorInstance(ctx, &api.BMCEmulatorConfig{
//	    Type:          "vbmc",
//	    Image:         "my-vbmc:latest",
//	})
//
// For the "sushy-tools" type, ConfigFile must point to a configuration file
// that already exists on the host. That file will be bind-mounted into the
// container.
//
//	err := containers.CreateBMCEmulatorInstance(ctx, &api.BMCEmulatorConfig{
//	    Type:          "sushy-tools",
//	    Image:         "my-sushy-tools:latest",
//	    ConfigFile:    "/path/to/existing/config/file",
//	})
//
// # Error Handling
//
// All operations return standard Go errors that can be inspected for specific
// failure modes. Callers should handle these errors according to their needs,
// for example by checking the underlying Docker API error where appropriate.
package containers
