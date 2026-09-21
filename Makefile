SHELL := /bin/bash
.DEFAULT_GOAL := build
.PHONY: build build-mcp help fmt fmt-check lint test ci tools

BIN_DIR := $(CURDIR)/bin
BIN := $(BIN_DIR)/gog-mcp
TOOLS_DIR := $(CURDIR)/.tools
GOFUMPT := $(TOOLS_DIR)/gofumpt
GOIMPORTS := $(TOOLS_DIR)/goimports
GOLANGCI_LINT := $(TOOLS_DIR)/golangci-lint

build:
	@mkdir -p $(BIN_DIR)
	@go build -trimpath -o $(BIN) ./cmd/gog-mcp

build-mcp: build

help: build
	@$(BIN) --help

tools:
	@mkdir -p $(TOOLS_DIR)
	@GOBIN=$(TOOLS_DIR) go install mvdan.cc/gofumpt@v0.9.2
	@GOBIN=$(TOOLS_DIR) go install golang.org/x/tools/cmd/goimports@v0.50.0
	@GOBIN=$(TOOLS_DIR) go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.2

fmt: tools
	@$(GOIMPORTS) -local github.com/steipete/gogcli -w .
	@$(GOFUMPT) -w .

fmt-check: tools
	@files="$$($(GOIMPORTS) -local github.com/steipete/gogcli -l .)"; \
	if [ -n "$$files" ]; then \
		echo "goimports needs changes:" >&2; \
		echo "$$files" >&2; \
		exit 1; \
	fi
	@files="$$($(GOFUMPT) -l .)"; \
	if [ -n "$$files" ]; then \
		echo "gofumpt needs changes:" >&2; \
		echo "$$files" >&2; \
		exit 1; \
	fi

lint: tools
	@$(GOLANGCI_LINT) run

test:
	@go test ./...

ci: fmt-check lint test
