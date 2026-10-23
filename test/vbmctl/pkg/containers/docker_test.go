//go:build vbmctl
// +build vbmctl

package containers

import (
	"context"
	"errors"
	"slices"
	"testing"

	vbmctlapi "github.com/metal3-io/baremetal-operator/test/vbmctl/pkg/api"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

// fakeContainerID is the ID of every container the fake client creates.
const fakeContainerID = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"

// fakeDockerClient is a Docker client for unit tests. It knows the containers
// in containers and has every image, so it never pulls one.
type fakeDockerClient struct {
	// containers maps the names of existing containers to their state.
	containers map[string]container.ContainerState
	// listErr is returned by ContainerList if set.
	listErr error
	// startErr is returned by ContainerStart if set.
	startErr error
	// removeErr is returned by ContainerRemove if set.
	removeErr error

	// created, started and removed record the calls to ContainerCreate,
	// ContainerStart and ContainerRemove.
	created []client.ContainerCreateOptions
	started []string
	removed []string
}

func (f *fakeDockerClient) ContainerList(_ context.Context, options client.ContainerListOptions) (client.ContainerListResult, error) {
	if f.listErr != nil {
		return client.ContainerListResult{}, f.listErr
	}
	var result client.ContainerListResult
	for name, state := range f.containers {
		if !options.All && state != container.StateRunning {
			continue
		}
		if options.Filters["name"]["^/"+name+"$"] {
			result.Items = append(result.Items, container.Summary{ID: fakeContainerID, Names: []string{"/" + name}, State: state})
		}
	}
	return result, nil
}

func (f *fakeDockerClient) ImageInspect(_ context.Context, _ string, _ ...client.ImageInspectOption) (client.ImageInspectResult, error) {
	return client.ImageInspectResult{}, nil
}

func (f *fakeDockerClient) ImagePull(_ context.Context, _ string, _ client.ImagePullOptions) (client.ImagePullResponse, error) {
	return nil, errors.New("fake client cannot pull images")
}

func (f *fakeDockerClient) ContainerCreate(_ context.Context, options client.ContainerCreateOptions) (client.ContainerCreateResult, error) {
	f.created = append(f.created, options)
	return client.ContainerCreateResult{ID: fakeContainerID}, nil
}

func (f *fakeDockerClient) ContainerStart(_ context.Context, containerID string, _ client.ContainerStartOptions) (client.ContainerStartResult, error) {
	f.started = append(f.started, containerID)
	return client.ContainerStartResult{}, f.startErr
}

func (f *fakeDockerClient) ContainerRemove(_ context.Context, containerID string, _ client.ContainerRemoveOptions) (client.ContainerRemoveResult, error) {
	f.removed = append(f.removed, containerID)
	return client.ContainerRemoveResult{}, f.removeErr
}

func (f *fakeDockerClient) Close() error {
	return nil
}

// useFakeDockerClient makes CreateRunningContainer use f until the test ends.
func useFakeDockerClient(t *testing.T, f *fakeDockerClient) {
	t.Helper()
	orig := newDockerClient
	newDockerClient = func() (dockerClient, error) { return f, nil }
	t.Cleanup(func() { newDockerClient = orig })
}

// checkCreated checks that f created and started exactly one container with
// the given name and the unless-stopped restart policy, or none if name is
// empty.
func checkCreated(t *testing.T, f *fakeDockerClient, name string) {
	t.Helper()
	if name == "" {
		if len(f.created) != 0 || len(f.started) != 0 {
			t.Errorf("expected no container to be created or started, got %d created and %d started", len(f.created), len(f.started))
		}
		return
	}
	if len(f.created) != 1 {
		t.Fatalf("expected 1 container to be created, got %d", len(f.created))
	}
	if f.created[0].Name != name {
		t.Errorf("expected container name %s, got %s", name, f.created[0].Name)
	}
	if policy := f.created[0].HostConfig.RestartPolicy.Name; policy != container.RestartPolicyUnlessStopped {
		t.Errorf("expected restart policy %q, got %q", container.RestartPolicyUnlessStopped, policy)
	}
	if len(f.started) != 1 || f.started[0] != fakeContainerID {
		t.Errorf("expected container %s to be started, got %v", fakeContainerID, f.started)
	}
}

func TestCreateImageServerInstance(t *testing.T) {
	listErr := errors.New("cannot connect to the Docker daemon")
	startErr := errors.New("port is already allocated")
	removeErr := errors.New("cannot remove container")

	tests := []struct {
		name        string
		docker      *fakeDockerClient
		wantErr     error
		wantMsg     string
		wantCreated string
		wantRemoved bool
	}{
		{
			name:        "container does not exist",
			docker:      &fakeDockerClient{},
			wantCreated: "vbmctl-image-server",
		},
		{
			name:    "running container exists",
			docker:  &fakeDockerClient{containers: map[string]container.ContainerState{"vbmctl-image-server": container.StateRunning}},
			wantErr: ErrContainerExists,
			wantMsg: `failed to create image server container: image server container vbmctl-image-server already exists (running), delete it first, for example with "docker rm -f vbmctl-image-server"`,
		},
		{
			name:    "stopped container exists",
			docker:  &fakeDockerClient{containers: map[string]container.ContainerState{"vbmctl-image-server": container.StateExited}},
			wantErr: ErrContainerExists,
			wantMsg: `failed to create image server container: image server container vbmctl-image-server already exists (exited), delete it first, for example with "docker rm -f vbmctl-image-server"`,
		},
		{
			name:    "lookup fails",
			docker:  &fakeDockerClient{listErr: listErr},
			wantErr: listErr,
		},
		{
			name:        "container cannot be started",
			docker:      &fakeDockerClient{startErr: startErr},
			wantErr:     startErr,
			wantMsg:     "failed to create image server container: failed to start the image server container: port is already allocated",
			wantCreated: "vbmctl-image-server",
			wantRemoved: true,
		},
		{
			name:        "container cannot be started or removed",
			docker:      &fakeDockerClient{startErr: startErr, removeErr: removeErr},
			wantErr:     removeErr,
			wantMsg:     "failed to create image server container: failed to start the image server container: port is already allocated (removing it also failed: cannot remove container)",
			wantCreated: "vbmctl-image-server",
			wantRemoved: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			useFakeDockerClient(t, tt.docker)
			err := CreateImageServerInstance(t.Context(), &vbmctlapi.ImageServerConfig{
				Image:            "nginxinc/nginx-unprivileged",
				ContainerName:    "image-server",
				DataDir:          t.TempDir(),
				ContainerDataDir: "/usr/share/nginx/html",
				Port:             8080,
				ContainerPort:    8080,
			})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected error wrapping %v, got: %v", tt.wantErr, err)
			}
			if tt.wantMsg != "" && err.Error() != tt.wantMsg {
				t.Errorf("expected error %q, got %q", tt.wantMsg, err.Error())
			}
			checkCreated(t, tt.docker, tt.wantCreated)
			var wantRemoved []string
			if tt.wantRemoved {
				wantRemoved = []string{fakeContainerID}
			}
			if !slices.Equal(tt.docker.removed, wantRemoved) {
				t.Errorf("expected removed containers %v, got %v", wantRemoved, tt.docker.removed)
			}
		})
	}
}

func TestCreateBMCEmulatorInstance(t *testing.T) {
	tests := []struct {
		name         string
		emulatorType string
		docker       *fakeDockerClient
		wantErr      error
		wantCreated  string
	}{
		{
			name:         "vbmc container does not exist",
			emulatorType: vbmctlapi.BMCEmulatorTypeVBMC,
			docker:       &fakeDockerClient{},
			wantCreated:  "vbmctl-vbmc",
		},
		{
			name:         "stopped vbmc container exists",
			emulatorType: vbmctlapi.BMCEmulatorTypeVBMC,
			docker:       &fakeDockerClient{containers: map[string]container.ContainerState{"vbmctl-vbmc": container.StateExited}},
			wantErr:      ErrContainerExists,
		},
		{
			name:         "sushy-tools container does not exist",
			emulatorType: vbmctlapi.BMCEmulatorTypeSushyTools,
			docker:       &fakeDockerClient{},
			wantCreated:  "vbmctl-sushy-tools",
		},
		{
			name:         "running sushy-tools container exists",
			emulatorType: vbmctlapi.BMCEmulatorTypeSushyTools,
			docker:       &fakeDockerClient{containers: map[string]container.ContainerState{"vbmctl-sushy-tools": container.StateRunning}},
			wantErr:      ErrContainerExists,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			useFakeDockerClient(t, tt.docker)
			err := CreateBMCEmulatorInstance(t.Context(), &vbmctlapi.BMCEmulatorConfig{
				Type:  tt.emulatorType,
				Image: "quay.io/metal3-io/" + tt.emulatorType,
			})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected error wrapping %v, got: %v", tt.wantErr, err)
			}
			checkCreated(t, tt.docker, tt.wantCreated)
		})
	}
}
