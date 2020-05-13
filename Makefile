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
export GO111MODULE=on
export GOFLAGS=

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
test: generate unit lint

.PHONY: generate
generate: bin/operator-sdk
	./bin/operator-sdk generate $(VERBOSE) k8s
	./bin/operator-sdk generate $(VERBOSE) crds
	openapi-gen \
		--input-dirs ./pkg/apis/metal3/v1alpha1 \
		--output-package ./pkg/apis/metal3/v1alpha1 \
		--output-base "" \
		--output-file-base zz_generated.openapi \
		--report-filename "-" \
		--go-header-file /dev/null

bin/operator-sdk: bin
	make -C tools/operator-sdk install

bin:
	mkdir -p bin

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

.PHONY: lint
lint: test-sec $GOPATH/bin/golint generate-check gofmt-check
	find ./pkg ./cmd -type f -name \*.go  |grep -v zz_ | xargs -L1 golint -set_exit_status
	go vet ./pkg/... ./cmd/...

.PHONY: generate-check
generate-check:
	./hack/generate.sh

.PHONY: generate-check-local
generate-check-local:
	IS_CONTAINER=local ./hack/generate.sh

.PHONY: test-sec
test-sec: $GOPATH/bin/gosec
	gosec -severity medium --confidence medium -quiet ./...

$GOPATH/bin/gosec:
	go get -u github.com/securego/gosec/cmd/gosec

$GOPATH/bin/golint:
	go get -u golang.org/x/lint/golint

.PHONY: gofmt
gofmt:
	gofmt -l -w ./pkg ./cmd

.PHONY: gofmt-check
gofmt-check:
	./hack/gofmt.sh

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

.PHONY: run
run:
	operator-sdk run --local \
		--go-ldflags=$(LDFLAGS) \
		--watch-namespace=$(RUN_NAMESPACE) \
		--operator-flags="-dev"

.PHONY: demo
demo:
	operator-sdk run --local \
		--go-ldflags=$(LDFLAGS) \
		--watch-namespace=$(RUN_NAMESPACE) \
		--operator-flags="-dev -demo-mode"

.PHONY: docker
docker: docker-operator docker-sdk

.PHONY: docker-operator
docker-operator:
	docker build . -f build/Dockerfile

.PHONY: docker-sdk
docker-sdk:
	docker build . -f hack/Dockerfile.operator-sdk

.PHONY: build
build:
	@echo LDFLAGS=$(LDFLAGS)
	go build -o build/_output/bin/baremetal-operator cmd/manager/main.go

.PHONY: tools
tools:
	@echo LDFLAGS=$(LDFLAGS)
	go build -o build/_output/bin/get-hardware-details cmd/get-hardware-details/main.go

.PHONY: deploy
deploy:
	cd deploy && kustomize edit set namespace $(RUN_NAMESPACE) && cd ..
	kustomize build deploy | kubectl apply -f -
