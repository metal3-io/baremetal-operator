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

	extraMounts, err := containerMounts(cfg.ExtraMounts)
	if err != nil {
		return err
	}

	exposedPorts := network.PortSet{
		containerPort: struct{}{},
	}
	portBindings := network.PortMap{
		containerPort: []network.PortBinding{
			{
				HostPort: strconv.FormatUint(uint64(cfg.Port), 10),
			},
		},
	}
	mounts := append([]mount.Mount{
		{
			Type:     mount.TypeBind,
			Source:   cfg.DataDir,
			Target:   cfg.ContainerDataDir,
			ReadOnly: false,
		},
	}, extraMounts...)

	for _, p := range cfg.ExtraPorts {
		extraContainerPort, portErr := network.ParsePort(fmt.Sprintf("%d/tcp", p.ContainerPort))
		if portErr != nil {
			return fmt.Errorf("failed to parse extra container port %d: %w", p.ContainerPort, portErr)
		}
		exposedPorts[extraContainerPort] = struct{}{}
		portBindings[extraContainerPort] = []network.PortBinding{
			{HostPort: strconv.FormatUint(uint64(p.HostPort), 10)},
		}
	}

	// Create the container
	opts := client.ContainerCreateOptions{
		Config: &container.Config{
			Image:        cfg.Image,
			ExposedPorts: exposedPorts,
		},
		HostConfig: &container.HostConfig{
			Mounts:       mounts,
			PortBindings: portBindings,
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

// containerMounts validates the host paths of the given extra mounts and
// converts them to Docker bind mount specifications.
func containerMounts(extra []vbmctlapi.ContainerMount) ([]mount.Mount, error) {
	mounts := make([]mount.Mount, 0, len(extra))
	for _, m := range extra {
		if _, err := os.Stat(m.HostPath); err != nil {
			return nil, fmt.Errorf("failed to access extra mount host path %q: %w", m.HostPath, err)
		}
		mounts = append(mounts, mount.Mount{
			Type:     mount.TypeBind,
			Source:   m.HostPath,
			Target:   m.ContainerPath,
			ReadOnly: m.ReadOnly,
		})
	}
	return mounts, nil
}

func DeleteImageServerInstance(ctx context.Context, containerName string) error {
	return DeleteContainer(ctx, "image server", ensureVbmctlPrefix(containerName))
}

func GetImageServerInfo(ctx context.Context, containerName string) (info string, err error) {
	return GetContainerInfo(ctx, ensureVbmctlPrefix(containerName))
}
