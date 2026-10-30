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
// in containers. It has every image unless imageMissing is set.
type fakeDockerClient struct {
	// containers maps the names of existing containers to their state.
	containers map[string]container.ContainerState
	// listErr is returned by ContainerList if set.
	listErr error
	// imageMissing makes ImageInspect fail, so that the image is pulled.
	imageMissing bool
	// pullErr is returned by ImagePull if set. Otherwise ImagePull returns
	// a pull whose Wait returns pullWaitErr.
	pullErr     error
	pullWaitErr error
	// startErr is returned by ContainerStart if set.
	startErr error
	// cancelStart, if set, is called by ContainerStart, which then fails
	// with the error of its context, as if the user pressed Ctrl-C while the
	// container was starting.
	cancelStart context.CancelFunc
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

func (f *fakeDockerClient) ImageInspect(_ context.Context, imageID string, _ ...client.ImageInspectOption) (client.ImageInspectResult, error) {
	if f.imageMissing {
		return client.ImageInspectResult{}, errors.New("no such image: " + imageID)
	}
	return client.ImageInspectResult{}, nil
}

func (f *fakeDockerClient) ImagePull(_ context.Context, _ string, _ client.ImagePullOptions) (client.ImagePullResponse, error) {
	if f.pullErr != nil {
		return nil, f.pullErr
	}
	return fakeImagePull{waitErr: f.pullWaitErr}, nil
}

func (f *fakeDockerClient) ContainerCreate(_ context.Context, options client.ContainerCreateOptions) (client.ContainerCreateResult, error) {
	f.created = append(f.created, options)
	return client.ContainerCreateResult{ID: fakeContainerID}, nil
}

func (f *fakeDockerClient) ContainerStart(ctx context.Context, containerID string, _ client.ContainerStartOptions) (client.ContainerStartResult, error) {
	f.started = append(f.started, containerID)
	if f.cancelStart != nil {
		f.cancelStart()
		return client.ContainerStartResult{}, ctx.Err()
	}
	return client.ContainerStartResult{}, f.startErr
}

// ContainerRemove fails without removing anything if ctx is done, as the real
// client does not send the request then.
func (f *fakeDockerClient) ContainerRemove(ctx context.Context, containerID string, _ client.ContainerRemoveOptions) (client.ContainerRemoveResult, error) {
	if err := ctx.Err(); err != nil {
		return client.ContainerRemoveResult{}, err
	}
	f.removed = append(f.removed, containerID)
	return client.ContainerRemoveResult{}, f.removeErr
}

func (f *fakeDockerClient) Close() error {
	return nil
}

// fakeImagePull is an image pull that ends with waitErr. Only its Wait method
// is implemented.
type fakeImagePull struct {
	client.ImagePullResponse

	waitErr error
}

func (p fakeImagePull) Wait(_ context.Context) error {
	return p.waitErr
}

// useFakeDockerClient makes CreateRunningContainer use f until the test ends.
func useFakeDockerClient(t *testing.T, f *fakeDockerClient) {
	t.Helper()
	orig := newDockerClient
	newDockerClient = func() (dockerClient, error) { return f, nil }
	t.Cleanup(func() { newDockerClient = orig })
}

// checkCreated checks that f created and started exactly one container with
// the given name, or none if name is empty.
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
	if len(f.started) != 1 || f.started[0] != fakeContainerID {
		t.Errorf("expected container %s to be started, got %v", fakeContainerID, f.started)
	}
}

func TestCreateImageServerInstance(t *testing.T) {
	listErr := errors.New("cannot connect to the Docker daemon")
	pullErr := errors.New("cannot connect to the Docker daemon")
	pullWaitErr := errors.New("manifest unknown")
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
			name:        "creates and starts the container",
			docker:      &fakeDockerClient{},
			wantCreated: "vbmctl-image-server",
		},
		{
			name:    "fails if a running container exists",
			docker:  &fakeDockerClient{containers: map[string]container.ContainerState{"vbmctl-image-server": container.StateRunning}},
			wantErr: ErrContainerExists,
			wantMsg: `failed to create image server container: failed to create image server container vbmctl-image-server: already exists (running), delete it first, for example with "docker rm -f vbmctl-image-server"`,
		},
		{
			name:    "fails if a stopped container exists",
			docker:  &fakeDockerClient{containers: map[string]container.ContainerState{"vbmctl-image-server": container.StateExited}},
			wantErr: ErrContainerExists,
			wantMsg: `failed to create image server container: failed to create image server container vbmctl-image-server: already exists (exited), delete it first, for example with "docker rm -f vbmctl-image-server"`,
		},
		{
			name:    "fails if the container lookup fails",
			docker:  &fakeDockerClient{listErr: listErr},
			wantErr: listErr,
		},
		{
			name:    "fails if the image pull cannot be started",
			docker:  &fakeDockerClient{imageMissing: true, pullErr: pullErr},
			wantErr: pullErr,
			wantMsg: `failed to create image server container: failed to start pulling image "nginxinc/nginx-unprivileged" for image server container: cannot connect to the Docker daemon`,
		},
		{
			name:    "fails if the image download fails",
			docker:  &fakeDockerClient{imageMissing: true, pullWaitErr: pullWaitErr},
			wantErr: pullWaitErr,
			wantMsg: `failed to create image server container: failed to download image "nginxinc/nginx-unprivileged" for image server container: manifest unknown`,
		},
		{
			name:        "removes the container if it fails to start",
			docker:      &fakeDockerClient{startErr: startErr},
			wantErr:     startErr,
			wantMsg:     "failed to create image server container: failed to start the image server container: port is already allocated",
			wantCreated: "vbmctl-image-server",
			wantRemoved: true,
		},
		{
			name:        "returns both errors if start and removal fail",
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

// TestCreateRunningContainerRemovesContainerAfterCancel checks that a container
// that failed to start because the context was cancelled is still removed.
func TestCreateRunningContainerRemovesContainerAfterCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	docker := &fakeDockerClient{cancelStart: cancel}
	useFakeDockerClient(t, docker)

	err := CreateRunningContainer(ctx, "image server", &client.ContainerCreateOptions{
		Name:   "vbmctl-image-server",
		Config: &container.Config{Image: "nginxinc/nginx-unprivileged"},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected error wrapping %v, got: %v", context.Canceled, err)
	}
	checkCreated(t, docker, "vbmctl-image-server")
	if want := []string{fakeContainerID}; !slices.Equal(docker.removed, want) {
		t.Errorf("expected removed containers %v, got %v", want, docker.removed)
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
			name:         "creates and starts the vbmc container",
			emulatorType: vbmctlapi.BMCEmulatorTypeVBMC,
			docker:       &fakeDockerClient{},
			wantCreated:  "vbmctl-vbmc",
		},
		{
			name:         "fails if a stopped vbmc container exists",
			emulatorType: vbmctlapi.BMCEmulatorTypeVBMC,
			docker:       &fakeDockerClient{containers: map[string]container.ContainerState{"vbmctl-vbmc": container.StateExited}},
			wantErr:      ErrContainerExists,
		},
		{
			name:         "creates and starts the sushy-tools container",
			emulatorType: vbmctlapi.BMCEmulatorTypeSushyTools,
			docker:       &fakeDockerClient{},
			wantCreated:  "vbmctl-sushy-tools",
		},
		{
			name:         "fails if a running sushy-tools container exists",
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
