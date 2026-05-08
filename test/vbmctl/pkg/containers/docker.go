//go:build vbmctl
// +build vbmctl

package containers

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/metal3-io/baremetal-operator/test/vbmctl/pkg/config"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

var ErrNetworkExists = errors.New("cannot create network, it already exists")
var ErrNetworkNotFound = errors.New("given network was not found")

func ensureVbmctlPrefix(name string) string {
	if !strings.HasPrefix(name, "vbmctl-") {
		return "vbmctl-" + name
	}
	return name
}

func getContainerStatus(ctx context.Context, containerName string, apiClient *client.Client) (status container.ContainerState, uuid string, err error) {
	// Use a name filter (anchored regex for exact match) and limit to 1
	// to avoid fetching all containers. Returns empty strings without error if not found.
	list, err := apiClient.ContainerList(ctx, client.ContainerListOptions{
		All:     true,
		Filters: make(client.Filters).Add("name", "^/"+containerName+"$"),
		Limit:   1,
	})
	if err != nil || len(list.Items) == 0 {
		return "", "", err
	}

	return list.Items[0].State, list.Items[0].ID, nil
}

func CreateRunningContainer(ctx context.Context, humanName string, opts *client.ContainerCreateOptions) error {
	apiClient, err := client.New(client.FromEnv, client.WithUserAgent("vbmctl/"+config.Version))
	if err != nil {
		return err
	}
	defer apiClient.Close()

	// need to check if the container is already running to avoid creating multiple instances
	status, containerID, err := getContainerStatus(ctx, opts.Name, apiClient)
	if err != nil {
		return fmt.Errorf("failed to get status for container %q: %w", opts.Name, err)
	}
	if containerID != "" {
		//nolint:forbidigo // CLI output is intentional
		fmt.Printf("Found %s container: %s (id: %s) status: %s\n", humanName, opts.Name, containerID[:12], status)
		if status == "running" {
			return nil
		}
		// container exists but is not running, we can start it below
	} else {
		// Pull the image if it is not already present locally
		if _, err := apiClient.ImageInspect(ctx, opts.Config.Image); err != nil {
			//nolint:forbidigo // CLI output is intentional
			fmt.Printf("Pulling image %s ...\n", opts.Config.Image)
			rc, err := apiClient.ImagePull(ctx, opts.Config.Image, client.ImagePullOptions{})
			if err != nil {
				return fmt.Errorf("failed to pull image %q for %s container: %w", opts.Config.Image, humanName, err)
			}
			if err := rc.Wait(ctx); err != nil {
				return fmt.Errorf("failed to pull image %q: %w", opts.Config.Image, err)
			}
		}

		// Create the container
		resp, err := apiClient.ContainerCreate(ctx, *opts)
		if err != nil {
			return fmt.Errorf("failed to create %s container: %w", humanName, err)
		}
		containerID = resp.ID
	}

	if _, err := apiClient.ContainerStart(ctx, containerID, client.ContainerStartOptions{}); err != nil {
		return fmt.Errorf("failed to start the %s container: %w", humanName, err)
	}
	//nolint:forbidigo // CLI output is intentional
	fmt.Printf("Started the %s container: %s (id: %s)\n", humanName, opts.Name, containerID[:12])

	return nil
}

func DeleteContainer(ctx context.Context, realName string, containerName string) error {
	apiClient, err := client.New(client.FromEnv, client.WithUserAgent("vbmctl/"+config.Version))
	if err != nil {
		return err
	}
	defer apiClient.Close()

	if _, err := apiClient.ContainerRemove(ctx, containerName, client.ContainerRemoveOptions{Force: true}); err != nil {
		return fmt.Errorf("failed to delete %s container %q: %w", realName, containerName, err)
	}
	//nolint:forbidigo // CLI output is intentional
	fmt.Printf("Deleted the %s container: %s\n", realName, containerName)

	return nil
}

func GetContainerInfo(ctx context.Context, containerName string) (info string, err error) {
	apiClient, err := client.New(client.FromEnv, client.WithUserAgent("vbmctl/"+config.Version))
	if err != nil {
		return "", err
	}
	defer apiClient.Close()

	status, containerID, err := getContainerStatus(ctx, containerName, apiClient)
	if err != nil {
		return "", fmt.Errorf("failed to get status for container %q: %w", containerName, err)
	}
	containerInfo := "'not found'"
	if containerID != "" {
		containerInfo = fmt.Sprintf("'%s' (id: %s)", status, containerID[:12])
	}
	return fmt.Sprintf("%s %s", containerName, containerInfo), nil
}

// CreateNetwork creates a new Docker network with the given name and options.
// If the network already exists, the network ID will be returned with error
// ErrNetworkExists.
func CreateNetwork(ctx context.Context, name string, opts *client.NetworkCreateOptions) (string, error) {
	apiClient, err := client.New(client.FromEnv, client.WithUserAgent("vbmctl/"+config.Version))
	if err != nil {
		return "", err
	}
	defer apiClient.Close()

	networkID, err := GetNetworkByName(ctx, name)
	if !errors.Is(err, ErrNetworkNotFound) {
		return "", err
	}
	if networkID != "" {
		return networkID, ErrNetworkExists
	}

	createdNet, err := apiClient.NetworkCreate(ctx, name, *opts)
	if err != nil {
		return "", fmt.Errorf("failed to create network %s: %w", name, err)
	}
	return createdNet.ID, nil
}

// GetNetworkByName finds the network with given name. If multiple networks
// is found, the first match will be returned.
func GetNetworkByName(ctx context.Context, networkName string) (string, error) {
	apiClient, err := client.New(client.FromEnv, client.WithUserAgent("vbmctl/"+config.Version))
	if err != nil {
		return "", err
	}
	defer apiClient.Close()

	filters := client.Filters{}
	filters.Add("name", networkName)
	networks, err := apiClient.NetworkList(ctx, client.NetworkListOptions{
		Filters: filters,
	})
	if err != nil {
		return "", fmt.Errorf("failed to look for Docker network with name %s: %w", networkName, err)
	}
	if len(networks.Items) == 0 {
		return "", ErrNetworkNotFound
	}
	if len(networks.Items) > 1 {
		log.Printf("Warning: found %d networks with name %s", len(networks.Items), networkName)
	}
	return networks.Items[0].Network.ID, nil
}

// DeleteNetwork deletes network with given ID. An error is returned if the network
// could not be deleted.
func DeleteNetwork(ctx context.Context, networkID string, opts *client.NetworkRemoveOptions) error {
	apiClient, err := client.New(client.FromEnv, client.WithUserAgent("vbmctl/"+config.Version))
	if err != nil {
		return err
	}
	defer apiClient.Close()

	_, err = apiClient.NetworkRemove(ctx, networkID, *opts)
	if err != nil {
		return fmt.Errorf("failed to remove Docker network %s: %w", networkID, err)
	}

	return nil
}
