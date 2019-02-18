TEST_NAMESPACE = operator-test
GO_TEST_FLAGS = -v
DEBUG = --debug
SETUP = --no-setup

.PHONY: help
help:
	@echo "Targets:"
	@echo "  test      -- run all tests"
	@echo "  e2e-local -- run end-to-end tests locally"
	@echo "  help      -- this help output"
	@echo
	@echo "Variables:"
	@echo "  TEST_NAMESPACE -- project name to use ($(TEST_NAMESPACE))"
	@echo "  SETUP          -- controls the --no-setup flag ($(SETUP))"
	@echo "  GO_TEST_FLAGS  -- flags to pass to --go-test-flags ($(GO_TEST_FLAGS))"
	@echo "  DEBUG          -- debug flag, if any ($(DEBUG))"

.PHONY: test
test: e2e-local

.PHONY: e2e-local
e2e-local:
	operator-sdk test local ./test/e2e \
		--namespace $(TEST_NAMESPACE) \
		--up-local $(SETUP) \
		$(DEBUG) --go-test-flags "$(GO_TEST_FLAGS)"
