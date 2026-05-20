//go:build vbmctl
// +build vbmctl

package containers

import (
	"context"
	"fmt"
	"os"
	"strconv"

	vbmctlapi "github.com/metal3-io/baremetal-operator/test/vbmctl/pkg/api"
	"github.com/metal3-io/baremetal-operator/test/vbmctl/pkg/config"
	container "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

// volumeMountsToBinds converts a slice of VolumeMount to Docker bind strings in the form "hostPath:bindSpec".
func volumeMountsToBinds(mounts []vbmctlapi.VolumeMount) []string {
	if len(mounts) == 0 {
		return nil
	}
	binds := make([]string, 0, len(mounts))
	for _, m := range mounts {
		binds = append(binds, fmt.Sprintf("%s:%s", m.HostPath, m.BindSpec))
	}
	return binds
}

// envMapToSlice converts a map of environment variables to a slice in the form "KEY=VALUE".
func envMapToSlice(envMap map[string]string) []string {
	if len(envMap) == 0 {
		return nil
	}
	envSlice := make([]string, 0, len(envMap))
	for key, value := range envMap {
		envSlice = append(envSlice, fmt.Sprintf("%s=%s", key, value))
	}
	return envSlice
}

func createEmulatorInstance(ctx context.Context, cfg *vbmctlapi.BMCEmulatorConfig) error {
	// Create the container
	opts := client.ContainerCreateOptions{
		Config: &container.Config{
			Image: cfg.Image,
			Env:   envMapToSlice(cfg.Env),
			Cmd:   cfg.Cmd,
		},
		HostConfig: &container.HostConfig{
			NetworkMode: "host",
			Binds:       volumeMountsToBinds(cfg.VolumeMounts),
		},
		NetworkingConfig: nil,
		Platform:         nil,
		Name:             cfg.ContainerName,
	}

	err := CreateRunningContainer(ctx, "BMC emulator", &opts)
	if err != nil {
		return fmt.Errorf("failed to create BMC emulator container: %w", err)
	}

	return nil
}

func deleteEmulatorInstance(ctx context.Context, containerName string) error {
	return DeleteContainer(ctx, "BMC emulator", containerName)
}

func createVBMCEmulatorInstance(ctx context.Context, cfg *vbmctlapi.BMCEmulatorConfig) error {
	// Fill in configuration
	cfg.ContainerName = ensureVbmctlPrefix(config.BMCEmulatorTypeVBMC)
	cfg.VolumeMounts = []vbmctlapi.VolumeMount{
		{HostPath: "/var/run/libvirt/libvirt-sock", BindSpec: "/var/run/libvirt/libvirt-sock"},
		{HostPath: "/var/run/libvirt/libvirt-sock-ro", BindSpec: "/var/run/libvirt/libvirt-sock-ro"},
	}
	cfg.Env = map[string]string{}
	cfg.Cmd = nil

	return createEmulatorInstance(ctx, cfg)
}

func deleteVBMCEmulatorInstance(ctx context.Context) error {
	return deleteEmulatorInstance(ctx, ensureVbmctlPrefix(config.BMCEmulatorTypeVBMC))
}

func getVBMCEmulatorInfo(ctx context.Context) (info string, err error) {
	return GetContainerInfo(ctx, ensureVbmctlPrefix(config.BMCEmulatorTypeVBMC))
}

func createSushyToolsEmulatorInstance(ctx context.Context, cfg *vbmctlapi.BMCEmulatorConfig) error {
	// Validate that the config file if it is specified exists and is a file
	if cfg.ConfigFile != "" {
		info, err := os.Stat(cfg.ConfigFile)
		if err != nil {
			return fmt.Errorf("failed to access sushy-tools config file %q: %w", cfg.ConfigFile, err)
		} else if info.IsDir() {
			return fmt.Errorf("sushy-tools config file %q is a directory", cfg.ConfigFile)
		}
	}

	// Fill in configuration
	cfg.ContainerName = ensureVbmctlPrefix(config.BMCEmulatorTypeSushyTools)
	cfg.VolumeMounts = []vbmctlapi.VolumeMount{
		{HostPath: "/var/run/libvirt", BindSpec: "/var/run/libvirt:Z"},
	}
	cfg.Env = map[string]string{}
	cfg.Cmd = []string{"sushy-emulator"}

	// If a config file is specified, set the environment variable and volume mount for it.
	// We use ":Z" in the bind spec to ensure proper SELinux labeling in case the host is
	// running with SELinux enabled.
	if cfg.ConfigFile != "" {
		cfg.Env["SUSHY_EMULATOR_CONFIG"] = "/etc/sushy/sushy-emulator.conf"
		cfg.VolumeMounts = append(cfg.VolumeMounts, vbmctlapi.VolumeMount{HostPath: cfg.ConfigFile, BindSpec: "/etc/sushy/sushy-emulator.conf:Z"})
	}

	// Set command-line arguments for the emulator based on the provided configuration.
	if cfg.ListenAddress != "" {
		cfg.Cmd = append(cfg.Cmd, "--interface", cfg.ListenAddress)
	}

	if cfg.ListenPort != 0 {
		cfg.Cmd = append(cfg.Cmd, "--port", strconv.FormatUint(uint64(cfg.ListenPort), 10))
	}

	// Overwrite specific configuration with values provided by vbmctl
	cfg.Cmd = append(cfg.Cmd, "--storage-pool", cfg.StoragePool)
	cfg.Cmd = append(cfg.Cmd, "--libvirt-uri", cfg.LibvirtURI)

	return createEmulatorInstance(ctx, cfg)
}

func deleteSushyToolsEmulatorInstance(ctx context.Context) error {
	return deleteEmulatorInstance(ctx, ensureVbmctlPrefix(config.BMCEmulatorTypeSushyTools))
}

func getSushyToolsEmulatorInfo(ctx context.Context) (info string, err error) {
	return GetContainerInfo(ctx, ensureVbmctlPrefix(config.BMCEmulatorTypeSushyTools))
}

func CreateBMCEmulatorInstance(ctx context.Context, cfg *vbmctlapi.BMCEmulatorConfig) error {
	switch cfg.Type {
	case config.BMCEmulatorTypeVBMC:
		return createVBMCEmulatorInstance(ctx, cfg)
	case config.BMCEmulatorTypeSushyTools:
		return createSushyToolsEmulatorInstance(ctx, cfg)
	default:
		return fmt.Errorf("unsupported BMC emulator type: %s", cfg.Type)
	}
}

func DeleteBMCEmulatorInstance(ctx context.Context, emulatorType string) error {
	switch emulatorType {
	case config.BMCEmulatorTypeVBMC:
		return deleteVBMCEmulatorInstance(ctx)
	case config.BMCEmulatorTypeSushyTools:
		return deleteSushyToolsEmulatorInstance(ctx)
	default:
		return fmt.Errorf("unsupported BMC emulator type: %s", emulatorType)
	}
}

func GetBMCEmulatorInfo(ctx context.Context, emulatorType string) (info string, err error) {
	switch emulatorType {
	case config.BMCEmulatorTypeVBMC:
		return getVBMCEmulatorInfo(ctx)
	case config.BMCEmulatorTypeSushyTools:
		return getSushyToolsEmulatorInfo(ctx)
	default:
		return "", fmt.Errorf("unsupported BMC emulator type: %s", emulatorType)
	}
}
