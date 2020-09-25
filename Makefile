GO_TEST_FLAGS = $(VERBOSE)
RUN_NAMESPACE = metal3

# Image URL to use all building/pushing image targets
IMG ?= controller:latest
# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
# trivialVersions=false means generate the CRD with multiple versions,
#     eventually allowing conversion webhooks
# allowDangerousTypes=true lets use the float64 field for clock speeds
# crdVersions=v1 generates the v1 version of the CRD type
# preserveUnknownFields=false causes the API server to discard "extra" data instead of storing it
CRD_OPTIONS ?= "crd:trivialVersions=false,allowDangerousTypes=true,crdVersions=v1,preserveUnknownFields=false"
CONTROLLER_TOOLS_VERSION=v0.4.0

# See pkg/version.go for details
SOURCE_GIT_COMMIT ?= $(shell git rev-parse --verify 'HEAD^{commit}')
BUILD_VERSION ?= $(shell git describe --always --abbrev=40 --dirty)
export LDFLAGS="-X github.com/metal3-io/baremetal-operator/pkg/version.Raw=${BUILD_VERSION} -X github.com/metal3-io/baremetal-operator/pkg/version.Commit=${SOURCE_GIT_COMMIT}"

TOOLS_DIR := tools
TOOLS_BIN_DIR := $(TOOLS_DIR)/bin
KUSTOMIZE := $(TOOLS_BIN_DIR)/kustomize

# Set some variables the operator expects to have in order to work
# Those need to be the same as in deploy/ironic_ci.env
export OPERATOR_NAME=baremetal-operator
export DEPLOY_KERNEL_URL=http://172.22.0.1:6180/images/ironic-python-agent.kernel
export DEPLOY_RAMDISK_URL=http://172.22.0.1:6180/images/ironic-python-agent.initramfs
export IRONIC_ENDPOINT=http://localhost:6385/v1/
export IRONIC_INSPECTOR_ENDPOINT=http://localhost:5050/v1/
export GO111MODULE=on
export GOFLAGS=

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

.PHONY: help
help:  ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)
	@echo
	@echo "Variables:"
	@echo "  RUN_NAMESPACE    -- project name to use for run target ($(RUN_NAMESPACE))"
	@echo "  GO_TEST_FLAGS    -- flags to pass to --go-test-flags ($(GO_TEST_FLAGS))"
	@echo "  DEBUG            -- debug flag, if any ($(DEBUG))"

.PHONY: test
test: generate fmt vet manifests unit ## Run common developer tests

.PHONY: unit
unit: ## Run the unit tests
	go test $(GO_TEST_FLAGS) ./... -coverprofile cover.out

# Compatibility alias from older version of this file
.PHONY: unit-cover
unit-cover: unit

.PHONY: unit-verbose
unit-verbose: ## Run unit tests with verbose output
	VERBOSE=-v make unit

.PHONY: manager
manager: generate fmt vet ## Build the primary controller binary
	go build -ldflags $(LDFLAGS) -o bin/manager main.go

# Compatibility alias from older version of this file
.PHONY: build
build: manager

.PHONY: run
run: generate fmt vet manifests ## Run the controller against the configured Kubernetes cluster in ~/.kube/config
	go run -ldflags $(LDFLAGS) ./main.go -dev -namespace $(RUN_NAMESPACE)

.PHONY: demo
demo: generate fmt vet manifests ## Run the controller against the configured Kubernetes cluster in ~/.kube/config
	go run -ldflags $(LDFLAGS) ./main.go -dev -demo-mode -namespace $(RUN_NAMESPACE)

.PHONY: install
install: manifests $(KUSTOMIZE) ## # Install CRDs into a cluster
	kustomize build config/crd | kubectl apply -f -

$(KUSTOMIZE):
	cd $(TOOLS_DIR); ./install_kustomize.sh

.PHONY: uninstall
uninstall: manifests ## Uninstall CRDs from a cluster
	kustomize build config/crd | kubectl delete -f -

.PHONY: deploy
deploy: manifests ## Deploy controller in the configured Kubernetes cluster in ~/.kube/config
	cd config/manager && kustomize edit set image controller=${IMG}
	kustomize build config/default | kubectl apply -f -

.PHONY: manifests
manifests: controller-gen ## Generate manifests e.g. CRD, RBAC etc.
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role webhook paths="./api/..." output:crd:artifacts:config=config/crd/bases

.PHONY: mod
mod: ## Update go modules
	go mod tidy
	go mod verify

.PHONY: fmt
fmt: ## Run go fmt against code
	go fmt ./...

.PHONY: fmt-check
fmt-check: ## Run gofmt and report an error if any changes are made
	./hack/gofmt.sh

.PHONY: vet
vet: ## Run go vet against code
	go vet ./...

.PHONY: lint
lint: golint-binary ## Run golint
	find $(CODE_DIRS) -type f -name \*.go  |grep -v zz_ | xargs -L1 golint -set_exit_status

.PHONY: golint-binary
golint-binary:
	which golint 2>&1 >/dev/null || $(MAKE) $(GOPATH)/bin/golint
$(GOPATH)/bin/golint:
	go get -u golang.org/x/lint/golint

.PHONY: generate
generate: controller-gen ## Update generated source code
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./api/..."

.PHONY: generate-check
generate-check: ## Verify that generated files are checked in
	./hack/generate.sh

.PHONY: generate-check-local
generate-check-local: ## Verify that generated files are checked in without using a container
	IS_CONTAINER=local ./hack/generate.sh

.PHONY: sec
sec: $(GOPATH)/bin/gosec ## Run gosec
	gosec -severity medium --confidence medium -quiet ./...

$(GOPATH)/bin/gosec:
	go get -u github.com/securego/gosec/cmd/gosec

.PHONY: docker-build
docker-build: test ## Build the docker image
	docker build . -t ${IMG}

# Compatibility alias from older version of this file
.PHONY: docker
docker: docker-build

.PHONY: docker-push
docker-push: ## Push the docker image
	docker push ${IMG}

# find or download controller-gen
# download controller-gen if necessary
.PHONY: controller-gen
controller-gen:
ifeq (, $(shell which controller-gen))
	@{ \
	set -e ;\
	CONTROLLER_GEN_TMP_DIR=$$(mktemp -d) ;\
	cd $$CONTROLLER_GEN_TMP_DIR ;\
	go mod init tmp ;\
	go get sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION) ;\
	rm -rf $$CONTROLLER_GEN_TMP_DIR ;\
	}
CONTROLLER_GEN=$(GOBIN)/controller-gen
else
CONTROLLER_GEN=$(shell which controller-gen)
endif

.PHONY: tools
tools: bin/get-hardware-details bin/make-bm-worker bin/make-virt-host ## Build programs in ./cmds/

bin/get-hardware-details:
	go build -o bin/get-hardware-details cmd/get-hardware-details/main.go

bin/make-bm-worker:
	go build -o bin/make-bm-worker cmd/make-bm-worker/main.go

bin/make-virt-host:
	go build -o bin/make-virt-host cmd/make-virt-host/main.go

.PHONY: docs
docs: $(patsubst %.dot,%.png,$(wildcard docs/*.dot))

%.png: %.dot
	dot -Tpng $< >$@

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
