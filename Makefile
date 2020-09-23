
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


# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

all: manager

# Run tests
test: generate fmt vet manifests
	go test ./... -coverprofile cover.out

# Build manager binary
manager: generate fmt vet
	go build -ldflags $(LDFLAGS) -o bin/manager main.go

# Run against the configured Kubernetes cluster in ~/.kube/config
run: generate fmt vet manifests
	go run -ldflags $(LDFLAGS) ./main.go

# Install CRDs into a cluster
install: manifests
	kustomize build config/crd | kubectl apply -f -

# Uninstall CRDs from a cluster
uninstall: manifests
	kustomize build config/crd | kubectl delete -f -

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy: manifests
	cd config/manager && kustomize edit set image controller=${IMG}
	kustomize build config/default | kubectl apply -f -

# Generate manifests e.g. CRD, RBAC etc.
manifests: controller-gen
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role webhook paths="./api/..." output:crd:artifacts:config=config/crd/bases

# Run go fmt against code
fmt:
	go fmt ./...

# Run go vet against code
vet:
	go vet ./...

# Generate code
generate: controller-gen
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./api/..."

# Build the docker image
docker-build: test
	docker build . -t ${IMG}

# Push the docker image
docker-push:
	docker push ${IMG}

# find or download controller-gen
# download controller-gen if necessary
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
