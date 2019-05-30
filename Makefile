TEST_NAMESPACE = operator-test
RUN_NAMESPACE = metal3
GO_TEST_FLAGS = $(VERBOSE)
DEBUG = --debug
SETUP = --no-setup

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
test: unit lint dep-check

.PHONY: travis
travis: test-verbose lint

.PHONY: unit
unit:
	go test $(GO_TEST_FLAGS) ./cmd/... ./pkg/...

.PHONY: unit-verbose
test-verbose:
	VERBOSE=-v make unit

.PHONY: lint
lint:
	golint -set_exit_status pkg/... cmd/...
	go vet ./pkg/... ./cmd/...

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
		--namespace=$(RUN_NAMESPACE) \
		--operator-flags="-dev"

.PHONY: demo
demo:
	operator-sdk up local \
		--namespace=$(RUN_NAMESPACE) \
		--operator-flags="-dev -demo-mode"

.PHONY: docker
docker:
	docker build . -f build/Dockerfile

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
