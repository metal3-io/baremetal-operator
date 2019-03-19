TEST_NAMESPACE = operator-test
GO_TEST_FLAGS = $(VERBOSE)
DEBUG = --debug
SETUP = --no-setup

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
test: unit lint

.PHONY: travis
travis: test-verbose lint

.PHONY: unit
unit:
	go test $(GO_TEST_FLAGS) ./pkg/...

.PHONY: unit-verbose
test-verbose:
	VERBOSE=-v make unit

.PHONY: lint
lint:
	golint -set_exit_status pkg/... cmd/...

.PHONY: e2e-local
e2e-local:
	operator-sdk test local ./test/e2e \
		--namespace $(TEST_NAMESPACE) \
		--up-local $(SETUP) \
		$(DEBUG) --go-test-flags "$(GO_TEST_FLAGS)"

.PHONY: dep
dep:
	dep ensure
