TEST_NAMESPACE = operator-test
RUN_NAMESPACE = metal3
GO_TEST_FLAGS = $(VERBOSE)
DEBUG = --debug
SETUP = --no-setup

# See pkg/version.go for details
GIT_COMMIT="$(shell git rev-parse --verify 'HEAD^{commit}')"
export LDFLAGS="-X github.com/metal3-io/baremetal-operator/pkg/version.Raw=$(shell git describe --always --abbrev=40 --dirty) -X github.com/metal3-io/baremetal-operator/pkg/version.Commit=${GIT_COMMIT}"

# Set some variables the operator expects to have in order to work
# Those need to be the same as in deploy/ironic_ci.env
export OPERATOR_NAME=baremetal-operator
export DEPLOY_KERNEL_URL=http://172.22.0.1:6180/images/ironic-python-agent.kernel
export DEPLOY_RAMDISK_URL=http://172.22.0.1:6180/images/ironic-python-agent.initramfs
export IRONIC_ENDPOINT=http://localhost:6385/v1/
export IRONIC_INSPECTOR_ENDPOINT=http://localhost:5050/v1/

# Define Docker related variables. Releases should modify and double check these vars.
# REGISTRY ?= gcr.io/$(shell gcloud config get-value project)
REGISTRY ?= quay.io/metal3-io
STAGING_REGISTRY := quay.io/metal3-io
PROD_REGISTRY := quay.io/metal3-io
IMAGE_NAME = $(OPERATOR_NAME)
CONTROLLER_IMG ?= $(REGISTRY)/$(IMAGE_NAME)
TAG ?= dev
ARCH ?= amd64
ALL_ARCH = amd64 arm arm64 ppc64le s390x
# Allow overriding the imagePullPolicy
PULL_POLICY ?= IfNotPresent

.PHONY: help
help:
	@echo "Targets:"
	@echo "  test             -- run unit tests and linter"
	@echo "  unit             -- run the unit tests"
	@echo "  unit-cover       -- run the unit tests and write code coverage statistics to console"
	@echo "  unit-cover-html  -- run the unit tests and open code coverage statistics in a browser"
	@echo "  unit-verbose     -- run unit tests with verbose flag enabled"
	@echo "  lint             -- run the linter"
	@echo "  e2e-local        -- run end-to-end tests locally"
	@echo "  help             -- this help output"
	@echo
	@echo "Variables:"
	@echo "  TEST_NAMESPACE   -- project name to use ($(TEST_NAMESPACE))"
	@echo "  SETUP            -- controls the --no-setup flag ($(SETUP))"
	@echo "  GO_TEST_FLAGS    -- flags to pass to --go-test-flags ($(GO_TEST_FLAGS))"
	@echo "  DEBUG            -- debug flag, if any ($(DEBUG))"

.PHONY: test
test: generate unit lint dep-check

.PHONY: generate
generate:
	operator-sdk generate k8s
	operator-sdk generate openapi

.PHONY: travis
travis: unit-verbose lint

.PHONY: unit
unit:
	go test $(GO_TEST_FLAGS) ./cmd/... ./pkg/...

.PHONY: unit-cover
unit-cover:
	go test -coverprofile=cover.out $(GO_TEST_FLAGS) ./cmd/... ./pkg/...
	go tool cover -func=cover.out

.PHONY: unit-cover-html
unit-cover-html:
	go test -coverprofile=cover.out $(GO_TEST_FLAGS) ./cmd/... ./pkg/...
	go tool cover -html=cover.out

.PHONY: unit-verbose
unit-verbose:
	VERBOSE=-v make unit

crd_file=deploy/crds/metal3.io_baremetalhosts_crd.yaml
crd_tmp=.crd.yaml.tmp

.PHONY: lint
lint: test-sec $GOPATH/bin/golint
	find ./pkg ./cmd -type f -name \*.go  |grep -v zz_ | xargs -L1 golint -set_exit_status
	go vet ./pkg/... ./cmd/...
	cp $(crd_file) $(crd_tmp); make generate; if ! diff -q $(crd_file) $(crd_tmp); then mv $(crd_tmp) $(crd_file); exit 1; else rm $(crd_tmp); fi

.PHONY: test-sec
test-sec: $GOPATH/bin/gosec
	gosec -severity medium --confidence medium -quiet ./...

$GOPATH/bin/gosec:
	go get -u github.com/securego/gosec/cmd/gosec

$GOPATH/bin/golint:
	go get -u golang.org/x/lint/golint

.PHONY: docs
docs: $(patsubst %.dot,%.png,$(wildcard docs/*.dot))

%.png: %.dot
	dot -Tpng $< >$@

.PHONY: e2e-local
e2e-local:
	operator-sdk test local ./test/e2e \
		--namespace $(TEST_NAMESPACE) \
		--up-local $(SETUP) \
		$(DEBUG) --go-test-flags "$(GO_TEST_FLAGS)"

.PHONY: dep
dep:
	dep ensure -v

.PHONY: run
run:
	operator-sdk up local \
		--go-ldflags=$(LDFLAGS) \
		--namespace=$(RUN_NAMESPACE) \
		--operator-flags="-dev"

.PHONY: demo
demo:
	operator-sdk up local \
		--go-ldflags=$(LDFLAGS) \
		--namespace=$(RUN_NAMESPACE) \
		--operator-flags="-dev -demo-mode"

.PHONY: docker
docker:
	docker build . -f build/Dockerfile

.PHONY: build
build:
	@echo LDFLAGS=$(LDFLAGS)
	go build -o build/_output/bin/baremetal-operator cmd/manager/main.go

.PHONY: deploy
deploy:
	cd deploy && kustomize edit set namespace $(RUN_NAMESPACE) && cd ..
	kustomize build deploy | kubectl apply -f -

.PHONY: dep-status
dep-status:
	dep status

.PHONY: dep-prune
dep-prune:
	dep prune -v

## --------------------------------------
## Docker
## --------------------------------------

.PHONY: docker-build
docker-build: ## Build the docker image for controller-manager
	docker build --pull --build-arg ARCH=$(ARCH) -f build/Dockerfile -t $(CONTROLLER_IMG)-$(ARCH):$(TAG) .
	MANIFEST_IMG=$(CONTROLLER_IMG)-$(ARCH) MANIFEST_TAG=$(TAG) $(MAKE) set-manifest-image
	$(MAKE) set-manifest-pull-policy

.PHONY: docker-push
docker-push: ## Push the docker image
	docker push $(CONTROLLER_IMG)-$(ARCH):$(TAG)

.PHONY: set-manifest-image
set-manifest-image:
	$(info Updating kustomize image patch file for manager resource)
	sed -i'' -e 's@image: .*@image: '"${MANIFEST_IMG}:$(MANIFEST_TAG)"'@' ./deploy/manager_image_patch.yaml

.PHONY: set-manifest-pull-policy
set-manifest-pull-policy:
	$(info Updating kustomize pull policy file for manager resource)
	sed -i'' -e 's@imagePullPolicy: .*@imagePullPolicy: '"$(PULL_POLICY)"'@' ./deploy/manager_pull_policy.yaml


## --------------------------------------
## Release
## --------------------------------------

RELEASE_TAG := $(shell git describe --abbrev=0 2>/dev/null)
RELEASE_DIR := out

$(RELEASE_DIR):
	mkdir -p $(RELEASE_DIR)/

.PHONY: release
release: clean-release  ## Builds and push container images using the latest git tag for the commit.
	@if [ -z "${RELEASE_TAG}" ]; then echo "RELEASE_TAG is not set"; exit 1; fi
	git checkout "${RELEASE_TAG}"
	
	# Set the manifest image to the production bucket.
	MANIFEST_IMG=$(PROD_REGISTRY)/$(IMAGE_NAME) MANIFEST_TAG=$(RELEASE_TAG) \
		$(MAKE) set-manifest-image
	PULL_POLICY=IfNotPresent $(MAKE) set-manifest-pull-policy

	$(MAKE) release-manifests

.PHONY: release-manifests
release-manifests: $(RELEASE_DIR) ## Builds the manifests to publish with a release
	cd deploy && kustomize edit set namespace $(RUN_NAMESPACE) && cd ..
	kustomize build deploy > $(RELEASE_DIR)/infrastructure-components.yaml

## --------------------------------------
## Cleanup / Verification
## --------------------------------------

.PHONY: clean-release
clean-release: ## Remove the release folder
	rm -rf $(RELEASE_DIR)