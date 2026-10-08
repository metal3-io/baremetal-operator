//go:build vbmctl
// +build vbmctl

package containers

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	vbmctlapi "github.com/metal3-io/baremetal-operator/test/vbmctl/pkg/api"
	config "github.com/metal3-io/baremetal-operator/test/vbmctl/pkg/config"
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

// parseSushyConfig extracts key=value pairs from a sushy-tools config file.
// The file is valid Python, so inline comments (#) and string prefixes
// (u, b, r, f) are handled accordingly.
func parseSushyConfig(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	lineRe := regexp.MustCompile(`^\s*([A-Z][A-Z_0-9]*)\s*=\s*(.+)$`)

	result := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		// Skip full-line comments and blank lines.
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		m := lineRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		key := m[1]
		rawVal := strings.TrimSpace(m[2])

		val, ok := parsePythonValue(rawVal)
		if !ok {
			continue
		}
		result[key] = val
	}
	return result, scanner.Err()
}

// parsePythonValue extracts a value from the RHS of a Python assignment.
// It handles:
//   - Quoted strings with optional prefix (u, b, r, f): u'val', "val", etc.
//   - Bare values (integers, True/False): strips inline comments.
func parsePythonValue(raw string) (string, bool) {
	if raw == "" {
		return "", false
	}

	// Check for a quoted string (with optional u/b/r/f prefix).
	s := raw
	if len(s) > 1 && strings.ContainsRune("ubfrUBFR", rune(s[0])) && (s[1] == '\'' || s[1] == '"') {
		s = s[1:] // skip prefix
	}

	if s != "" && (s[0] == '\'' || s[0] == '"') {
		quote := s[0]
		// Find the matching closing quote.
		end := strings.IndexByte(s[1:], quote)
		if end < 0 {
			return "", false // unterminated string
		}
		return s[1 : end+1], true
	}

	// Bare value (number, bool, etc.) — strip inline comment.
	if idx := strings.IndexByte(raw, '#'); idx >= 0 {
		raw = strings.TrimSpace(raw[:idx])
	}
	if raw == "" {
		return "", false
	}
	return raw, true
}

func parseSushyEmulatorConfigFile(configPath string) (string, uint16, error) {
	cfg, err := parseSushyConfig(configPath)
	if err != nil {
		return "", 0, fmt.Errorf("failed to parse sushy-tools config file %q: %w", configPath, err)
	}

	listenAddress := cfg["SUSHY_EMULATOR_LISTEN_IP"]
	portStr := cfg["SUSHY_EMULATOR_LISTEN_PORT"]

	var listenPort uint16
	if portStr != "" {
		port, err := strconv.ParseUint(portStr, 10, 16)
		if err != nil {
			return "", 0, fmt.Errorf("invalid SUSHY_EMULATOR_LISTEN_PORT value %q: %w", portStr, err)
		}
		listenPort = uint16(port)
	}

	return listenAddress, listenPort, nil
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

func WaitForBMCEmulatorReadiness(ctx context.Context, cfg *vbmctlapi.BMCEmulatorConfig) error {
	// default retry policy (keeps backward compatibility)
	const (
		defaultMaxAttempts    = 5
		defaultRequestTimeout = 3 * time.Second
		defaultRetryDelay     = 2 * time.Second
	)

	return WaitForBMCEmulatorReadinessWithPolicy(ctx, cfg, defaultMaxAttempts, defaultRequestTimeout, defaultRetryDelay)
}

// WaitForBMCEmulatorReadinessWithPolicy waits for the emulator using an injectable
// retry policy. Tests should call this with small values to exercise retry
// exhaustion and cancellation without long sleeps.
func WaitForBMCEmulatorReadinessWithPolicy(ctx context.Context, cfg *vbmctlapi.BMCEmulatorConfig, maxAttempts int, requestTimeout, retryDelay time.Duration) error {
	const successStatusLimit = 400

	if cfg == nil || cfg.Type != vbmctlapi.BMCEmulatorTypeSushyTools {
		return nil
	}

	endpoint := BuildBMCEmulatorHealthCheckURL(cfg)
	//nolint:forbidigo // CLI output is intentional
	fmt.Printf("Waiting for BMC emulator to become reachable at %s ... ", endpoint)

	httpClient := &http.Client{Timeout: requestTimeout}
	for attempt := range maxAttempts {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, http.NoBody)
		if err != nil {
			return err
		}

		//nolint:gosec // endpoint is from config under test
		resp, err := httpClient.Do(req)
		// If the context was cancelled/expired during the request, propagate
		// that error immediately so callers see the real cause (including on
		// the final attempt).
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if err == nil {
			if resp.StatusCode < successStatusLimit {
				_ = resp.Body.Close()
				//nolint:forbidigo // CLI output is intentional
				fmt.Println("BMC emulator is reachable.")
				return nil
			}
			_ = resp.Body.Close()
		}

		if attempt == maxAttempts-1 {
			break
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(retryDelay):
		}
	}

	info, infoErr := GetBMCEmulatorInfo(ctx, cfg.Type)
	if infoErr != nil {
		//nolint:forbidigo // CLI output is intentional
		fmt.Printf("Failed to get BMC emulator status: %v\n", infoErr)
	} else {
		//nolint:forbidigo // CLI output is intentional
		fmt.Printf("BMC emulator status: %s\n", info)
	}

	return fmt.Errorf("BMC emulator did not become reachable at %s", endpoint)
}

func BuildBMCEmulatorHealthCheckURL(cfg *vbmctlapi.BMCEmulatorConfig) string {
	if cfg == nil {
		return ""
	}
	sushyCfg := &cfg.SushyToolsConfig

	// Set default listen address and port to match those in the VBMCTL
	// configuration file because they overwrite other settings.
	listenAddress := sushyCfg.ListenAddress
	listenPort := sushyCfg.ListenPort

	// If the values are still empty, check next the sushy-tools configuration
	// file if it is specified. If it does, we will attempt to read the listen
	// address and port from it.
	if sushyCfg.ConfigFile != "" && (listenAddress == "" || listenPort == 0) {
		parsedAddress, parsedPort, err := parseSushyEmulatorConfigFile(sushyCfg.ConfigFile)
		if err == nil {
			if listenAddress == "" {
				if parsedAddress != "" {
					listenAddress = parsedAddress
				} else {
					// If no listen address is specified in the config file,
					// sushy-tools defaults to "127.0.0.1".
					listenAddress = "127.0.0.1"
				}
			}
			if listenPort == 0 {
				listenPort = parsedPort
			}
		}
	}

	// Finally, fallback to the default values if they are still empty.
	if listenAddress == "" {
		listenAddress = config.DefaultNetworkAddress
	}
	if listenPort == 0 {
		listenPort = config.DefaultBMCEmulatorSushyToolsListenPort
	}

	return "http://" + net.JoinHostPort(listenAddress, strconv.Itoa(int(listenPort))) + "/redfish/v1/"
}
