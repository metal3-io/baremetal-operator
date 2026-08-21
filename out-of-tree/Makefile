# Build the out of tree Starlark provisioner plugin.
# It targets a pinned upstream BMO release.

# BMO_VERSION and the toolchain image live in the Dockerfile and stay in sync
# via the retarget script. Change them there, not by overriding this.
BMO_VERSION ?= main
BMO_REPO ?= https://github.com/metal3-io/baremetal-operator.git
ARCH ?= amd64
CONTAINER_RUNTIME ?= docker

REGISTRY ?= localhost
IMAGE_NAME ?= baremetal-operator-oot
IMG_TAG ?= latest
IMG ?= $(REGISTRY)/$(IMAGE_NAME)-$(ARCH):$(IMG_TAG)

PLUGIN_SO ?= bin/starlark-provisioner.so

.PHONY: all
all: image

# Point go.mod at BMO_VERSION and build a loadable .so. This mutates go.mod.
# Run "git checkout go.mod go.sum" to restore the committed form.
.PHONY: plugin
plugin:
	./hack/retarget-bmo.sh $(BMO_VERSION) $(BMO_REPO)
	CGO_ENABLED=1 go build -buildmode=plugin -o $(PLUGIN_SO) ./plugin

# Compile the plugin against the committed module tags without fetching source.
.PHONY: compile-check
compile-check:
	CGO_ENABLED=1 go build -buildmode=plugin -o $(PLUGIN_SO) ./plugin

# Build the operator image with the layered plugin. The build load tests the .so.
# BMO_VERSION must match the Dockerfile so the toolchain image stays consistent.
.PHONY: image
image:
	@test "$(BMO_VERSION)" = "$$(sed -n 's/^ARG BMO_VERSION=//p' Dockerfile)" \
		|| { echo "BMO_VERSION $(BMO_VERSION) does not match the Dockerfile, run hack/retarget-bmo.sh $(BMO_VERSION)" >&2; exit 1; }
	$(CONTAINER_RUNTIME) build --platform=linux/$(ARCH) \
		--build-arg ARCH=$(ARCH) \
		--build-arg BMO_VERSION=$(BMO_VERSION) \
		--build-arg BMO_REPO=$(BMO_REPO) \
		-f Dockerfile . -t $(IMG)

# Push the image built by the image target.
.PHONY: push
push:
	$(CONTAINER_RUNTIME) push $(IMG)

.PHONY: clean
clean:
	rm -rf bin .bmo-src
