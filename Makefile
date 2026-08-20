GO      ?= go
BIN     := agbala
PKG     := ./...
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

# Packages whose tests write golden files. Add new ones here as they appear.
GOLDEN_PKGS ?= ./internal/ui
LDFLAGS := -s -w -X main.version=$(VERSION)

.DEFAULT_GOAL := help

.PHONY: help
help: ## List the available targets
	@grep -hE '^[a-z-]+:.*?## ' $(MAKEFILE_LIST) | awk -F':.*?## ' '{printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2}'

.PHONY: build
build: ## Build the binary into ./agbala
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags '$(LDFLAGS)' -o $(BIN) ./cmd/agbala

.PHONY: test
test: ## Run the test suite with the race detector
	$(GO) test -race -count=1 $(PKG)

.PHONY: bench
bench: ## Run the render benchmarks
	$(GO) test -run '^$$' -bench . -benchmem $(PKG)

.PHONY: golden
# The packages must precede -update: go test stops parsing package patterns at
# the first flag it does not recognise.
golden: ## Regenerate the golden files, then review the diff before committing
	$(GO) test -run TestGolden $(GOLDEN_PKGS) -update=true

.PHONY: lint
lint: ## Run golangci-lint
	golangci-lint run

.PHONY: fmt
fmt: ## Format the tree
	gofmt -w .

.PHONY: check-fmt
check-fmt: ## Fail if anything is unformatted
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "unformatted files:"; echo "$$unformatted"; exit 1; \
	fi

.PHONY: tidy
tidy: ## Tidy go.mod and go.sum
	$(GO) mod tidy

.PHONY: clean
clean: ## Remove build artefacts
	rm -rf $(BIN) dist/
