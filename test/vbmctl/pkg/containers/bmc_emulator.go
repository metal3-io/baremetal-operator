//go:build vbmctl
// +build vbmctl

package containers

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"

	vbmctlapi "github.com/metal3-io/baremetal-operator/test/vbmctl/pkg/api"
	container "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

type bmcEmulatorInstance struct {
	// Image is the container image to use for the BMC emulator.
	image string

	// Command for the BMC emulator container.
	cmd []string

	// Environment variables for the emulator container.
	env map[string]string

	// Container name for the BMC emulator.
	containerName string

	// List of host-to-container volume bindings.
	volumeMounts []volumeMount
}

// VolumeMount represents a single host-to-container volume binding.
type volumeMount struct {
	// HostPath is the path on the host to mount.
	hostPath string

	// BindSpec is the container-side bind specification, e.g.
	// "/container/path" or "/container/path:Z".
	bindSpec string
}

// volumeMountsToBinds converts a slice of VolumeMount to Docker bind strings in the form "hostPath:bindSpec".
func volumeMountsToBinds(mounts []volumeMount) []string {
	if len(mounts) == 0 {
		return nil
	}
	binds := make([]string, 0, len(mounts))
	for _, m := range mounts {
		binds = append(binds, fmt.Sprintf("%s:%s", m.hostPath, m.bindSpec))
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

func createEmulatorInstance(ctx context.Context, instance *bmcEmulatorInstance) error {
	// Create the container
	opts := client.ContainerCreateOptions{
		Config: &container.Config{
			Image: instance.image,
			Env:   envMapToSlice(instance.env),
			Cmd:   instance.cmd,
		},
		HostConfig: &container.HostConfig{
			NetworkMode: "host",
			Binds:       volumeMountsToBinds(instance.volumeMounts),
		},
		NetworkingConfig: nil,
		Platform:         nil,
		Name:             instance.containerName,
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
	instance := &bmcEmulatorInstance{
		image:         cfg.Image,
		containerName: ensureVbmctlPrefix(vbmctlapi.BMCEmulatorTypeVBMC),
		volumeMounts: []volumeMount{
			{hostPath: "/var/run/libvirt/libvirt-sock", bindSpec: "/var/run/libvirt/libvirt-sock"},
			{hostPath: "/var/run/libvirt/libvirt-sock-ro", bindSpec: "/var/run/libvirt/libvirt-sock-ro"},
		},
		env: map[string]string{},
		cmd: nil,
	}

	return createEmulatorInstance(ctx, instance)
}

func deleteVBMCEmulatorInstance(ctx context.Context) error {
	return deleteEmulatorInstance(ctx, ensureVbmctlPrefix(vbmctlapi.BMCEmulatorTypeVBMC))
}

func getVBMCEmulatorInfo(ctx context.Context) (info string, err error) {
	return GetContainerInfo(ctx, ensureVbmctlPrefix(vbmctlapi.BMCEmulatorTypeVBMC))
}

func createSushyToolsEmulatorInstance(ctx context.Context, cfg *vbmctlapi.BMCEmulatorConfig) error {
	// Validate that the config file if it is specified exists and is a file
	sushyCfg := &cfg.SushyToolsConfig
	if sushyCfg.ConfigFile != "" {
		info, err := os.Stat(sushyCfg.ConfigFile)
		if err != nil {
			return fmt.Errorf("failed to access sushy-tools config file %q: %w", sushyCfg.ConfigFile, err)
		} else if info.IsDir() {
			return fmt.Errorf("sushy-tools config file %q is a directory", sushyCfg.ConfigFile)
		}
	}

	// Fill in configuration
	instance := &bmcEmulatorInstance{
		image:         cfg.Image,
		containerName: ensureVbmctlPrefix(vbmctlapi.BMCEmulatorTypeSushyTools),
		volumeMounts: []volumeMount{
			{hostPath: "/var/run/libvirt", bindSpec: "/var/run/libvirt:Z"},
		},
		env: map[string]string{},
		cmd: []string{"sushy-emulator"},
	}

	// If a config file is specified, set the environment variable and volume mount for it.
	// We use ":Z" in the bind spec to ensure proper SELinux labeling in case the host is
	// running with SELinux enabled.
	if sushyCfg.ConfigFile != "" {
		instance.env["SUSHY_EMULATOR_CONFIG"] = "/etc/sushy/sushy-emulator.conf"
		instance.volumeMounts = append(instance.volumeMounts, volumeMount{hostPath: sushyCfg.ConfigFile, bindSpec: "/etc/sushy/sushy-emulator.conf:Z"})
	}

	// Set command-line arguments for the emulator based on the provided configuration.
	if sushyCfg.ListenAddress != "" {
		instance.cmd = append(instance.cmd, "--interface", sushyCfg.ListenAddress)
	}

	if sushyCfg.ListenPort != 0 {
		instance.cmd = append(instance.cmd, "--port", strconv.FormatUint(uint64(sushyCfg.ListenPort), 10))
	}

	// Overwrite specific configuration with values provided by vbmctl
	instance.cmd = append(instance.cmd, "--storage-pool", sushyCfg.StoragePool)
	instance.cmd = append(instance.cmd, "--libvirt-uri", sushyCfg.LibvirtURI)

	return createEmulatorInstance(ctx, instance)
}

func deleteSushyToolsEmulatorInstance(ctx context.Context) error {
	return deleteEmulatorInstance(ctx, ensureVbmctlPrefix(vbmctlapi.BMCEmulatorTypeSushyTools))
}

func getSushyToolsEmulatorInfo(ctx context.Context) (info string, err error) {
	return GetContainerInfo(ctx, ensureVbmctlPrefix(vbmctlapi.BMCEmulatorTypeSushyTools))
}

func CreateBMCEmulatorInstance(ctx context.Context, cfg *vbmctlapi.BMCEmulatorConfig) error {
	if cfg == nil {
		return errors.New("invalid BMC emulator configuration")
	}

	switch cfg.Type {
	case vbmctlapi.BMCEmulatorTypeVBMC:
		return createVBMCEmulatorInstance(ctx, cfg)
	case vbmctlapi.BMCEmulatorTypeSushyTools:
		return createSushyToolsEmulatorInstance(ctx, cfg)
	default:
		return fmt.Errorf("unsupported BMC emulator type: %s", cfg.Type)
	}
}

func DeleteBMCEmulatorInstance(ctx context.Context, emulatorType string) error {
	switch emulatorType {
	case vbmctlapi.BMCEmulatorTypeVBMC:
		return deleteVBMCEmulatorInstance(ctx)
	case vbmctlapi.BMCEmulatorTypeSushyTools:
		return deleteSushyToolsEmulatorInstance(ctx)
	default:
		return fmt.Errorf("unsupported BMC emulator type: %s", emulatorType)
	}
}

func GetBMCEmulatorInfo(ctx context.Context, emulatorType string) (info string, err error) {
	switch emulatorType {
	case vbmctlapi.BMCEmulatorTypeVBMC:
		return getVBMCEmulatorInfo(ctx)
	case vbmctlapi.BMCEmulatorTypeSushyTools:
		return getSushyToolsEmulatorInfo(ctx)
	default:
		return "", fmt.Errorf("unsupported BMC emulator type: %s", emulatorType)
	}
}
