SHELL:=/usr/bin/env bash
RUN_NAMESPACE = metal3
GO_TEST_FLAGS = $(TEST_FLAGS)
DEBUG = --debug
COVER_PROFILE = cover.out
GO := $(shell type -P go)
GO_VERSION ?= 1.25.7

ROOT_DIR := $(shell dirname $(realpath $(firstword $(MAKEFILE_LIST))))

# CRD Generation Options
#
# allowDangerousTypes=true lets use the float64 field for clock speeds
# crdVersions=v1 generates the v1 version of the CRD type
#
# NOTE: Assumes the default is preserveUnknownFields=false with
#       crdVersions=v1, so that the API server discards "extra" data
#       instead of storing it
#
#
BIN_DIR := bin
TOOLS_DIR := hack/tools
TOOLS_BIN_DIR := $(abspath $(TOOLS_DIR)/$(BIN_DIR))

CRD_OPTIONS ?= "crd:allowDangerousTypes=true,crdVersions=v1"
KUSTOMIZE = tools/bin/kustomize
CONTROLLER_GEN = tools/bin/controller-gen
GINKGO = tools/bin/ginkgo
DEPLOY_CLI = tools/bin/deploy-cli

# See pkg/version.go for details
SOURCE_GIT_COMMIT ?= $(shell git rev-parse --short HEAD)
BUILD_VERSION ?= $(shell git describe --always --abbrev=40 --dirty)
VERSION_URI = "github.com/metal3-io/baremetal-operator/pkg/version"
export LDFLAGS="-X $(VERSION_URI).Raw=${BUILD_VERSION} \
                -X $(VERSION_URI).Commit=${SOURCE_GIT_COMMIT} \
                -X $(VERSION_URI).BuildTime=$(shell date +%Y-%m-%dT%H:%M:%S%z)"

# Set some variables the operator expects to have in order to work
# Those need to be the same as in config/default/ironic.env
export OPERATOR_NAME=baremetal-operator
export DEPLOY_KERNEL_URL=http://172.22.0.1:6180/images/ironic-python-agent.kernel
export DEPLOY_RAMDISK_URL=http://172.22.0.1:6180/images/ironic-python-agent.initramfs
export IRONIC_ENDPOINT=http://localhost:6385/v1/
export GO111MODULE=on
export GOFLAGS=

#
# Ginkgo configuration.
#
GINKGO_FOCUS ?=
GINKGO_SKIP ?=
GINKGO_SKIP_LABELS ?=
GINKGO_NODES ?= 2
GINKGO_TIMEOUT ?= 3h
GINKGO_POLL_PROGRESS_AFTER ?= 60m
GINKGO_POLL_PROGRESS_INTERVAL ?= 5m
E2E_CONF_FILE ?= $(ROOT_DIR)/test/e2e/config/fixture.yaml
E2E_BMCS_CONF_FILE ?= $(ROOT_DIR)/test/e2e/config/bmcs-fixture.yaml
USE_EXISTING_CLUSTER ?= false
SKIP_RESOURCE_CLEANUP ?= false
GINKGO_NOCOLOR ?= false

GOLANGCI_LINT_BIN := golangci-lint
GOLANGCI_LINT_VER := v2.10.1
GOLANGCI_LINT := $(abspath $(TOOLS_BIN_DIR)/$(GOLANGCI_LINT_BIN))
GOLANGCI_LINT_PKG := github.com/golangci/golangci-lint/v2/cmd/golangci-lint

# to set multiple ginkgo skip flags, if any
ifneq ($(strip $(GINKGO_SKIP)),)
_SKIP_ARGS := $(foreach arg,$(strip $(GINKGO_SKIP)),-skip="$(arg)")
endif

# to set multiple ginkgo skip labels, if any
ifneq ($(strip $(GINKGO_SKIP_LABELS)),)
_SKIP_LABELS_ARGS := --label-filter="!$(GINKGO_SKIP_LABELS)"
endif

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
REGISTRY ?= quay.io/metal3-io
IMG_NAME ?= baremetal-operator
IMG_TAG ?= latest
IMG ?= $(REGISTRY)/$(IMG_NAME)

# Which configuration to use when deploying (from the config directory)
DEPLOY_CONFIG ?= default

## --------------------------------------
## Test Targets
## --------------------------------------

# Run tests
.PHONY: test
test: generate lint manifests unit ## Run common developer tests

.PHONY: unit
unit: ## Run unit tests
	go test ./... $(GO_TEST_FLAGS) -coverprofile $(COVER_PROFILE)
	cd apis/ && go test ./... $(GO_TEST_FLAGS) -coverprofile $(COVER_PROFILE)
	cd pkg/hardwareutils && go test ./... $(GO_TEST_FLAGS) -coverprofile $(COVER_PROFILE)

.PHONY: unit-cover
unit-cover: ## Run unit tests with code coverage
	go test -coverprofile=$(COVER_PROFILE) $(GO_TEST_FLAGS) ./...
	go tool cover -func=$(COVER_PROFILE)
	cd apis/ && go test -coverprofile=$(COVER_PROFILE) $(GO_TEST_FLAGS) ./...
	cd apis/ && go tool cover -func=$(COVER_PROFILE)
	cd pkg/hardwareutils/ && go test -coverprofile=$(COVER_PROFILE) $(GO_TEST_FLAGS) ./...
	cd pkg/hardwareutils/ && go tool cover -func=$(COVER_PROFILE)

.PHONY: unit-verbose
unit-verbose: ## Run unit tests with verbose output
	TEST_FLAGS=-v make unit

ARTIFACTS ?= ${ROOT_DIR}/test/e2e/_artifacts

.PHONY: test-e2e
test-e2e: $(GINKGO) ## Run the end-to-end tests
	$(GINKGO) -v --trace -poll-progress-after=$(GINKGO_POLL_PROGRESS_AFTER) \
		-poll-progress-interval=$(GINKGO_POLL_PROGRESS_INTERVAL) --tags=e2e,vbmctl --focus="$(GINKGO_FOCUS)" \
		$(_SKIP_ARGS)  $(_SKIP_LABELS_ARGS) --nodes=$(GINKGO_NODES) --timeout=$(GINKGO_TIMEOUT) --no-color=$(GINKGO_NOCOLOR) \
		--output-dir="$(ARTIFACTS)" --junit-report="junit.e2e_suite.1.xml" $(GINKGO_ARGS) test/e2e -- \
		-e2e.config="$(E2E_CONF_FILE)" -e2e.bmcsConfig="$(E2E_BMCS_CONF_FILE)" \
		-e2e.use-existing-cluster=$(USE_EXISTING_CLUSTER) \
		-e2e.skip-resource-cleanup=$(SKIP_RESOURCE_CLEANUP) -e2e.artifacts-folder="$(ARTIFACTS)"

## --------------------------------------
## Linter Targets
## --------------------------------------

.PHONY: linters
linters: lint generate-check

$(GOLANGCI_LINT):
	GOBIN=$(TOOLS_BIN_DIR) go install $(GOLANGCI_LINT_PKG)@$(GOLANGCI_LINT_VER)

.PHONY: $(GOLANGCI_LINT_BIN)
$(GOLANGCI_LINT_BIN): $(GOLANGCI_LINT) ## Build a local copy of golangci-lint.

.PHONY: lint
lint: $(GOLANGCI_LINT)
	$(GOLANGCI_LINT) run -v $(GOLANGCI_LINT_EXTRA_ARGS) ./... --timeout=10m
	cd apis; $(GOLANGCI_LINT) run -v $(GOLANGCI_LINT_EXTRA_ARGS) ./... --timeout=10m
	cd test; $(GOLANGCI_LINT) run -v $(GOLANGCI_LINT_EXTRA_ARGS) ./... --timeout=10m
	cd pkg/hardwareutils; $(GOLANGCI_LINT) run -v $(GOLANGCI_LINT_EXTRA_ARGS) ./... --timeout=10m
	cd hack/tools; $(GOLANGCI_LINT) run -v $(GOLANGCI_LINT_EXTRA_ARGS) ./... --timeout=10m

.PHONY: lint-fix
lint-fix: $(GOLANGCI_LINT) ## Lint the codebase and run auto-fixers if supported by the linter
	GOLANGCI_LINT_EXTRA_ARGS=--fix $(MAKE) lint

.PHONY: manifest-lint
manifest-lint: ## Run manifest validation
	./hack/manifestlint.sh

## --------------------------------------
## Build/Run Targets
## --------------------------------------

.PHONY: build
build: generate manifests manager tools build-e2e ## Build everything

.PHONY: manager
manager: generate lint ## Build manager binary
	go build -ldflags $(LDFLAGS) -o bin/$(OPERATOR_NAME) main.go

.PHONY: run
run: generate lint manifests ## Run against the configured Kubernetes cluster in ~/.kube/config
	go run -ldflags $(LDFLAGS) ./main.go -namespace=$(RUN_NAMESPACE) -dev -webhook-port=0 $(RUN_FLAGS)

.PHONY: demo
demo: generate lint manifests ## Run in demo mode
	go run -ldflags $(LDFLAGS) ./main.go -namespace=$(RUN_NAMESPACE) -dev -demo-mode -webhook-port=0 $(RUN_FLAGS)

.PHONY: run-test-mode
run-test-mode: generate lint manifests ## Run against the configured Kubernetes cluster in ~/.kube/config
	go run -ldflags $(LDFLAGS) ./main.go -namespace=$(RUN_NAMESPACE) -dev -test-mode -webhook-port=0 $(RUN_FLAGS)

.PHONY: install
install: $(KUSTOMIZE) manifests ## Install CRDs into a cluster
	$< build config/base/crds | kubectl apply -f -

.PHONY: uninstall
uninstall: $(KUSTOMIZE) manifests ## Uninstall CRDs from a cluster
	$< build config/base/crds | kubectl delete -f -

.PHONY: deploy
deploy: $(KUSTOMIZE) manifests  ## Deploy controller in the configured Kubernetes cluster in ~/.kube/config
	make set-manifest-image-bmo MANIFEST_IMG=$(IMG_NAME) MANIFEST_TAG=$(IMG_TAG)
	$< build config/$(DEPLOY_CONFIG) | kubectl apply -f -

$(CONTROLLER_GEN): hack/tools/go.mod
	cd hack/tools; go build -o $(abspath $@) sigs.k8s.io/controller-tools/cmd/controller-gen

$(KUSTOMIZE): hack/tools/go.mod
	cd hack/tools; go build -o $(abspath $@) sigs.k8s.io/kustomize/kustomize/v5

.PHONY: deploy-cli
deploy-cli:
	cd hack/tools/deploy-cli; go build -o $(abspath $(DEPLOY_CLI))

.PHONY: build-e2e
build-e2e:
	cd test; go build --tags=e2e ./...

.PHONY: build-vbmctl
build-vbmctl:
	cd test; go build --tags=e2e,vbmctl -ldflags $(LDFLAGS) -o $(abspath $(BIN_DIR)/vbmctl) ./vbmctl/main.go

.PHONY: manifests
manifests: manifests-generate manifests-kustomize ## Generate manifests e.g. CRD, RBAC etc.

.PHONY: manifests-generate
manifests-generate: $(CONTROLLER_GEN)
	cd apis; $(abspath $<) $(CRD_OPTIONS) rbac:roleName=manager-role webhook paths="./..." output:webhook:dir=../config/base/webhook/ output:crd:artifacts:config=../config/base/crds/bases
	$< rbac:roleName=manager-role paths="./..." output:rbac:artifacts:config=config/base/rbac

.PHONY: manifests-kustomize
manifests-kustomize: $(KUSTOMIZE)
	$< build config/default > config/render/capm3.yaml

.PHONY: set-manifest-pull-policy
set-manifest-pull-policy:
	$(info Updating kustomize pull policy file for manager resource)
	sed -i'' -e 's@imagePullPolicy: .*@imagePullPolicy: '"$(PULL_POLICY)"'@' ./config/base/manager.yaml

.PHONY: set-manifest-image-bmo
set-manifest-image-bmo: $(KUSTOMIZE) manifests
	$(info Updating container image for BMO to use ${MANIFEST_IMG}:${MANIFEST_TAG})
	sed -i'' -e 's@image: .*@image: \"'"${MANIFEST_IMG}:$(MANIFEST_TAG)"'\"@' ./config/base/manager.yaml

.PHONY: set-manifest-image-ironic
set-manifest-image-ironic: $(KUSTOMIZE) manifests
	$(info Updating container image for Ironic to use ${MANIFEST_IMG}:${MANIFEST_TAG})
	cd ironic-deployment/base && $(abspath $(KUSTOMIZE)) edit set image quay.io/metal3-io/ironic=${MANIFEST_IMG}:${MANIFEST_TAG}

.PHONY: set-manifest-image-mariadb
set-manifest-image-mariadb: $(KUSTOMIZE) manifests
	$(info Updating container image for Mariadb to use ${MANIFEST_IMG}:${MANIFEST_TAG})
	cd ironic-deployment/components/mariadb && $(abspath $(KUSTOMIZE)) edit set image quay.io/metal3-io/mariadb=${MANIFEST_IMG}:${MANIFEST_TAG}

.PHONY: set-manifest-image-keepalived
set-manifest-image-keepalived: $(KUSTOMIZE) manifests
	$(info Updating container image for keepalived to use ${MANIFEST_IMG}:${MANIFEST_TAG})
	cd ironic-deployment/components/keepalived && $(abspath $(KUSTOMIZE)) edit set image quay.io/metal3-io/keepalived=${MANIFEST_IMG}:${MANIFEST_TAG}

.PHONY: set-manifest-image-ipa-downloader
set-manifest-image-ipa-downloader: $(KUSTOMIZE) manifests
	$(info Updating container image for IPA downloader to use ${MANIFEST_IMG}:${MANIFEST_TAG})
	cd ironic-deployment/base && $(abspath $(KUSTOMIZE)) edit set image quay.io/metal3-io/ironic-ipa-downloader=${MANIFEST_IMG}:${MANIFEST_TAG}

.PHONY: generate
generate: $(CONTROLLER_GEN) ## Generate code
	cd apis; $(abspath $<) object:headerFile="../hack/boilerplate.go.txt" paths="./..."
	$< object:headerFile="hack/boilerplate.go.txt" paths="./..."

## --------------------------------------
## Docker Targets
## --------------------------------------

.PHONY: docker
docker: generate manifests ## Build the docker image
	docker build . -t ${IMG}:${IMG_TAG} \
	--build-arg http_proxy=$(http_proxy) \
	--build-arg https_proxy=$(https_proxy)

.PHONY: docker-debug
docker-debug: generate manifests ## Build the docker image with debug info
	docker build . -t ${IMG}:${IMG_TAG} \
	--build-arg http_proxy=$(http_proxy) \
	--build-arg https_proxy=$(https_proxy) \
	--build-arg LDFLAGS="-extldflags=-static"

# Push the docker image
.PHONY: docker-push
docker-push:
	docker push ${IMG}:${IMG_TAG}

## --------------------------------------
## CI Targets
## --------------------------------------

.PHONY: generate-check
generate-check:
	./hack/generate.sh

.PHONY: generate-check-local
generate-check-local:
	IS_CONTAINER=local ./hack/generate.sh

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

$(GINKGO): ## Install ginkgo in tools/bin. Take ginkgo version from `test/go.mod`.
	cd test; GOBIN=$(abspath tools/bin) go install github.com/onsi/ginkgo/v2/ginkgo

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
	cd apis; go mod tidy
	cd apis; go mod verify
	cd pkg/hardwareutils; go mod tidy
	cd pkg/hardwareutils; go mod verify
	cd hack/tools; go mod tidy
	cd hack/tools; go mod verify
	cd test; go mod tidy
	cd test; go mod verify

## --------------------------------------
## Release
## --------------------------------------
RELEASE_TAG ?= $(shell git describe --abbrev=0 2>/dev/null)
ifneq (,$(findstring -,$(RELEASE_TAG)))
    PRE_RELEASE=true
endif
# the previous release tag, e.g., v1.7.0, excluding pre-release tags
PREVIOUS_TAG ?= $(shell git tag -l | grep -E "^v[0-9]+\.[0-9]+\.[0-9]+" | sort -V | grep -B1 $(RELEASE_TAG) | grep -E "^v[0-9]+\.[0-9]+\.[0-9]+$$" | head -n 1 2>/dev/null)
RELEASE_DIR := out
RELEASE_NOTES_DIR := releasenotes

$(RELEASE_DIR):
	mkdir -p $(RELEASE_DIR)/

$(RELEASE_NOTES_DIR):
	mkdir -p $(RELEASE_NOTES_DIR)/

.PHONY: release-notes
release-notes: $(RELEASE_NOTES_DIR) $(RELEASE_NOTES)
	cd hack/tools && $(GO) run release/notes.go  --releaseTag=$(RELEASE_TAG) > $(realpath $(RELEASE_NOTES_DIR))/$(RELEASE_TAG).md

.PHONY: release-manifests
release-manifests: $(KUSTOMIZE) $(RELEASE_DIR) ## Builds the manifests to publish with a release
	$(KUSTOMIZE) build config > $(RELEASE_DIR)/baremetal-operator.yaml

.PHONY: release
release:
	@if [ -z "${RELEASE_TAG}" ]; then echo "RELEASE_TAG is not set"; exit 1; fi
	@if ! [ -z "$$(git status --porcelain)" ]; then echo "You have uncommitted changes"; exit 1; fi
	git checkout "${RELEASE_TAG}"
	MANIFEST_IMG=$(IMG) MANIFEST_TAG=$(RELEASE_TAG) $(MAKE) set-manifest-image-bmo
	PULL_POLICY=IfNotPresent $(MAKE) set-manifest-pull-policy
	$(MAKE) release-manifests
	$(MAKE) release-notes

go-version: ## Print the go version we use to compile our binaries and images
	@echo $(GO_VERSION)

## --------------------------------------
## Clean
## --------------------------------------

.PHONY: clean
clean: ## Remove all temporary files, directories and tools
	rm -rf ironic-deployment/overlays/temp
	rm -rf config/overlays/temp
	rm -rf $(TOOLS_BIN_DIR)

.PHONY: clean-e2e
clean-e2e: ## Remove everything related to e2e tests
	./hack/clean-e2e.sh
