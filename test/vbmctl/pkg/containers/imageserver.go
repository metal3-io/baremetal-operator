//go:build vbmctl
// +build vbmctl

package containers

import (
	"context"
	"fmt"
	"os"
	"strconv"

	vbmctlapi "github.com/metal3-io/baremetal-operator/test/vbmctl/pkg/api"
	container "github.com/moby/moby/api/types/container"
	mount "github.com/moby/moby/api/types/mount"
	network "github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
)

func CreateImageServerInstance(ctx context.Context, cfg *vbmctlapi.ImageServerConfig) error {
	containerPort, err := network.ParsePort(fmt.Sprintf("%d/tcp", cfg.ContainerPort))
	if err != nil {
		return fmt.Errorf("failed to parse container port: %w", err)
	}

	// Validate that the data directory exists and is a directory
	info, err := os.Stat(cfg.DataDir)
	if err != nil {
		return fmt.Errorf("failed to access image server data directory %q: %w", cfg.DataDir, err)
	} else if !info.IsDir() {
		return fmt.Errorf("image server data directory %q is not a directory", cfg.DataDir)
	}

	// Create the container
	opts := client.ContainerCreateOptions{
		Config: &container.Config{
			Image: cfg.Image,
			ExposedPorts: network.PortSet{
				containerPort: struct{}{},
			},
		},
		HostConfig: &container.HostConfig{
			Mounts: []mount.Mount{
				{
					Type:     mount.TypeBind,
					Source:   cfg.DataDir,
					Target:   cfg.ContainerDataDir,
					ReadOnly: false,
				},
			},
			PortBindings: network.PortMap{
				containerPort: []network.PortBinding{
					{
						HostPort: strconv.FormatUint(uint64(cfg.Port), 10),
					},
				},
			},
		},
		NetworkingConfig: nil,
		Platform:         nil,
		Name:             ensureVbmctlPrefix(cfg.ContainerName),
	}

	err = CreateRunningContainer(ctx, "image server", &opts)
	if err != nil {
		return fmt.Errorf("failed to create image server container: %w", err)
	}

	return nil
}

func DeleteImageServerInstance(ctx context.Context, containerName string) error {
	return DeleteContainer(ctx, "image server", ensureVbmctlPrefix(containerName))
}

func GetImageServerInfo(ctx context.Context, containerName string) (info string, err error) {
	return GetContainerInfo(ctx, ensureVbmctlPrefix(containerName))
}
