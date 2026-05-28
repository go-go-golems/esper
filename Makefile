.PHONY: logcopter-generate logcopter-check glazed-lint-build glazed-lint

GLAZED_LINT_BIN ?= /tmp/glazed-lint
GLAZED_LINT_PKG ?= github.com/go-go-golems/glazed/cmd/tools/glazed-lint
GLAZED_VERSION ?= $(shell GOWORK=off go list -m -f '{{.Version}}' github.com/go-go-golems/glazed 2>/dev/null)
GLAZED_LINT_FLAGS ?=

logcopter-generate:
	GOWORK=off go generate ./...

logcopter-check:
	GOWORK=off go tool logcopter-gen -area-prefix go-go-golems.esper -strip-prefix github.com/go-go-golems/esper -check ./cmd/... ./pkg/...

glazed-lint-build:
	@echo "Building glazed-lint from Glazed module..."
	@if [ -n "$(GLAZED_VERSION)" ] && [ "$(GLAZED_VERSION)" != "(devel)" ]; then 		echo "Installing $(GLAZED_LINT_PKG)@$(GLAZED_VERSION)"; 		GOBIN=$(dir $(GLAZED_LINT_BIN)) GOWORK=off go install $(GLAZED_LINT_PKG)@$(GLAZED_VERSION); 	else 		echo "Installing $(GLAZED_LINT_PKG) from workspace/module"; 		GOBIN=$(dir $(GLAZED_LINT_BIN)) go install $(GLAZED_LINT_PKG); 	fi

glazed-lint: glazed-lint-build
	go vet -vettool=$(GLAZED_LINT_BIN) $(GLAZED_LINT_FLAGS) ./pkg/...
