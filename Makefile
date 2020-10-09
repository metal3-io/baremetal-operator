RUN_NAMESPACE = metal3
GO_TEST_FLAGS = $(VERBOSE)
DEBUG = --debug
COVER_PROFILE = cover.out

# CRD Generation Options
#
# trivialVersions=false means generate the CRD with multiple versions,
#     eventually allowing conversion webhooks
# allowDangerousTypes=true lets use the float64 field for clock speeds
# crdVersions=v1 generates the v1 version of the CRD type
#
# NOTE: Assumes the default is preserveUnknownFields=false with
#       crdVersions=v1, so that the API server discards "extra" data
#       instead of storing it
#
CRD_OPTIONS ?= "crd:trivialVersions=false,allowDangerousTypes=true,crdVersions=v1"
CONTROLLER_TOOLS_VERSION=v0.4.0
CONTROLLER_GEN=./bin/controller-gen

# Directories.
TOOLS_DIR := tools
TOOLS_BIN_DIR := $(TOOLS_DIR)/bin
KUSTOMIZE := $(TOOLS_BIN_DIR)/kustomize
BIN_DIR := bin

# See pkg/version.go for details
SOURCE_GIT_COMMIT ?= $(shell git rev-parse --verify 'HEAD^{commit}')
BUILD_VERSION ?= $(shell git describe --always --abbrev=40 --dirty)
export LDFLAGS="-X github.com/metal3-io/baremetal-operator/pkg/version.Raw=${BUILD_VERSION} -X github.com/metal3-io/baremetal-operator/pkg/version.Commit=${SOURCE_GIT_COMMIT}"

# Set some variables the operator expects to have in order to work
# Those need to be the same as in config/default/ironic.env
export OPERATOR_NAME=baremetal-operator
export DEPLOY_KERNEL_URL=http://172.22.0.1:6180/images/ironic-python-agent.kernel
export DEPLOY_RAMDISK_URL=http://172.22.0.1:6180/images/ironic-python-agent.initramfs
export IRONIC_ENDPOINT=http://localhost:6385/v1/
export IRONIC_INSPECTOR_ENDPOINT=http://localhost:5050/v1/
export GO111MODULE=on
export GOFLAGS=

.PHONY: help
help:  ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)
	@echo
	@echo "Variables:"
	@echo "  TEST_NAMESPACE   -- project name to use ($(TEST_NAMESPACE))"
	@echo "  SETUP            -- controls the --no-setup flag ($(SETUP))"
	@echo "  GO_TEST_FLAGS    -- flags to pass to --go-test-flags ($(GO_TEST_FLAGS))"
	@echo "  DEBUG            -- debug flag, if any ($(DEBUG))"

# Image URL to use all building/pushing image targets
IMG ?= baremetal-operator:latest
# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= "crd:trivialVersions=true"

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

## --------------------------------------
## Test Targets
## --------------------------------------

# Run tests
.PHONY: test
test: generate lint manifests unit ## Run common developer tests

.PHONY: unit
unit: ## Run unit tests
	go test ./... $(VERBOSE) -coverprofile $(COVER_PROFILE)

.PHONY: unit-cover
unit-cover: ## Run unit tests with code coverage
	go test -coverprofile=$(COVER_PROFILE) $(GO_TEST_FLAGS) ./...
	go tool cover -func=$(COVER_PROFILE)

.PHONY: unit-verbose
unit-verbose: ## Run unit tests with verbose output
	VERBOSE=-v make unit

## --------------------------------------
## Linter Targets
## --------------------------------------

.PHONY: linters
linters: lint generate-check fmt-check

.PHONY: sec
sec: lint

.PHONY: fmt
fmt: lint

.PHONY: vet
vet: lint

.PHONY: lint
lint: $(GOBIN)/golangci-lint
	$(GOBIN)/golangci-lint run

$(GOBIN)/golangci-lint:
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(GOBIN) v1.31.0

## --------------------------------------
## Build/Run Targets
## --------------------------------------

.PHONY: build
build: generate manifests manager tools ## Build everything

.PHONY: manager
manager: generate lint ## Build manager binary
	go build -ldflags $(LDFLAGS) -o bin/manager main.go

.PHONY: run
run: generate lint manifests ## Run against the configured Kubernetes cluster in ~/.kube/config
	go run -ldflags $(LDFLAGS) ./main.go -namespace=$(RUN_NAMESPACE) -dev

.PHONY: demo
demo: generate lint manifests ## Run in demo mode
	go run -ldflags $(LDFLAGS) ./main.go -namespace=$(RUN_NAMESPACE) -dev -demo-mode

.PHONY: run-test-mode
run-test-mode: generate fmt vet manifests ## Run against the configured Kubernetes cluster in ~/.kube/config
	go run -ldflags $(LDFLAGS) ./main.go -namespace=$(RUN_NAMESPACE) -dev -test-mode

.PHONY: install
install: $(KUSTOMIZE) manifests ## Install CRDs into a cluster
	$(KUSTOMIZE) build config/crd | kubectl apply -f -

.PHONY: uninstall
uninstall: $(KUSTOMIZE) manifests ## Uninstall CRDs from a cluster
	$(KUSTOMIZE) build config/crd | kubectl delete -f -

.PHONY: deploy
deploy: $(KUSTOMIZE) manifests ## Deploy controller in the configured Kubernetes cluster in ~/.kube/config
	cd config/manager && kustomize edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default | kubectl apply -f -

.PHONY: manifests
manifests: $(CONTROLLER_GEN) $(KUSTOMIZE) ## Generate manifests e.g. CRD, RBAC etc.
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role webhook paths="./..." output:crd:artifacts:config=config/crd/bases 
	$(KUSTOMIZE) build config/default > config/render/capm3.yaml

.PHONY: generate
generate: $(CONTROLLER_GEN) ## Generate code
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: $(KUSTOMIZE)
$(KUSTOMIZE):
	cd $(TOOLS_DIR); ./install_kustomize.sh

# Build the version of controller-gen that we use
$(CONTROLLER_GEN):
	./hack/install-controller-gen.sh $(CONTROLLER_TOOLS_VERSION) $$(pwd)/$(CONTROLLER_GEN)

## --------------------------------------
## Docker Targets
## --------------------------------------

.PHONY: docker
docker: test ## Build the docker image
	docker build . -t ${IMG}

# Push the docker image
.PHONY: docker-push
docker-push:
	docker push ${IMG}

## --------------------------------------
## CI Targets
## --------------------------------------

.PHONY: generate-check
generate-check:
	./hack/generate.sh

.PHONY: generate-check-local
generate-check-local:
	IS_CONTAINER=local ./hack/generate.sh

.PHONY: fmt-check
fmt-check: ## Run gofmt and report an error if any changes are made
	./hack/gofmt.sh

## --------------------------------------
## Documentation
## --------------------------------------

.PHONY: docs
docs: $(patsubst %.dot,%.png,$(wildcard docs/*.dot))

%.png: %.dot
	dot -Tpng $< >$@

## --------------------------------------
## Tool apps
## --------------------------------------

.PHONY: tools
tools:
	go build -o bin/get-hardware-details cmd/get-hardware-details/main.go
	go build -o bin/make-bm-worker cmd/make-bm-worker/main.go
	go build -o bin/make-virt-host cmd/make-virt-host/main.go

## --------------------------------------
## Tilt / Kind
## --------------------------------------

.PHONY: kind-create
kind-create: ## create bmo kind cluster if needed
	./hack/kind_with_registry.sh

.PHONY: tilt-up
tilt-up: $(KUSTOMIZE) kind-create ## start tilt and build kind cluster if needed
	tilt up

.PHONY: kind-reset
kind-reset: ## Destroys the "bmo" kind cluster.
	kind delete cluster --name=bmo || true

## --------------------------------------
## Go module Targets
## --------------------------------------

.PHONY:
mod: ## Clean up go module settings
	go mod tidy
	go mod verify
