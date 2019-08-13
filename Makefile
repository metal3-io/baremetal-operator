TEST_NAMESPACE = operator-test
RUN_NAMESPACE = metal3
GO_TEST_FLAGS = $(VERBOSE)
DEBUG = --debug
SETUP = --no-setup

# See pkg/version.go for details
GIT_COMMIT="$(shell git rev-parse --verify 'HEAD^{commit}')"
export LDFLAGS="-X github.com/metal3-io/baremetal-operator/pkg/version.Raw=$(shell git describe --always --abbrev=40 --dirty) -X github.com/metal3-io/baremetal-operator/pkg/version.Commit=${GIT_COMMIT}"

# Set some variables the operator expects to have in order to work
export OPERATOR_NAME=baremetal-operator
export DEPLOY_KERNEL_URL=http://172.22.0.1/images/ironic-python-agent.kernel
export DEPLOY_RAMDISK_URL=http://172.22.0.1/images/ironic-python-agent.initramfs
export IRONIC_ENDPOINT=http://localhost:6385/v1/
export IRONIC_INSPECTOR_ENDPOINT=http://localhost:5050/v1/

.PHONY: help
help:
	@echo "Targets:"
	@echo "  test         -- run unit tests and linter"
	@echo "  unit         -- run the unit tests"
	@echo "  unit-verbose -- run unit tests with verbose flag enabled"
	@echo "  lint         -- run the linter"
	@echo "  e2e-local    -- run end-to-end tests locally"
	@echo "  help         -- this help output"
	@echo
	@echo "Variables:"
	@echo "  TEST_NAMESPACE -- project name to use ($(TEST_NAMESPACE))"
	@echo "  SETUP          -- controls the --no-setup flag ($(SETUP))"
	@echo "  GO_TEST_FLAGS  -- flags to pass to --go-test-flags ($(GO_TEST_FLAGS))"
	@echo "  DEBUG          -- debug flag, if any ($(DEBUG))"

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

.PHONY: unit-verbose
unit-verbose:
	VERBOSE=-v make unit

crd_file=deploy/crds/metal3_v1alpha1_baremetalhost_crd.yaml
crd_tmp=.crd.yaml.tmp

.PHONY: lint
lint: test-sec
	golint -set_exit_status pkg/... cmd/...
	go vet ./pkg/... ./cmd/...
	cp $(crd_file) $(crd_tmp); make generate; if ! diff -q $(crd_file) $(crd_tmp); then mv $(crd_tmp) $(crd_file); exit 1; else rm $(crd_tmp); fi

.PHONY: test-sec
test-sec:
	@which gosec 2> /dev/null >&1 || { echo "gosec must be installed to lint code";  exit 1; }
	gosec -severity medium --confidence medium -quiet ./...

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
	dep ensure

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
	echo "{ \"kind\": \"Namespace\", \"apiVersion\": \"v1\", \"metadata\": { \"name\": \"$(RUN_NAMESPACE)\", \"labels\": { \"name\": \"$(RUN_NAMESPACE)\" } } }" | kubectl apply -f -
	kubectl apply -f deploy/service_account.yaml -n $(RUN_NAMESPACE)
	kubectl apply -f deploy/role.yaml -n $(RUN_NAMESPACE)
	kubectl apply -f deploy/role_binding.yaml
	kubectl apply -f deploy/crds/metal3_v1alpha1_baremetalhost_crd.yaml
	kubectl apply -f deploy/operator.yaml -n $(RUN_NAMESPACE)

.PHONY: dep-check
dep-check:
	dep check
