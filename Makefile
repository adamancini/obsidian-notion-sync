# obsidian-notion-sync Makefile

# Binary name
BINARY_NAME := obsidian-notion

# Build directories
BUILD_DIR := build
CMD_DIR := cmd/obsidian-notion

# Go parameters
GOCMD := go
GOBUILD := $(GOCMD) build
GOTEST := $(GOCMD) test
GOMOD := $(GOCMD) mod
GOFMT := gofmt
GOVET := $(GOCMD) vet

# Version information
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

# Build flags
LDFLAGS := -ldflags "-s -w \
	-X main.version=$(VERSION) \
	-X main.commit=$(COMMIT) \
	-X main.date=$(DATE)"

# CGO is required for SQLite
CGO_ENABLED := 1

# Platforms for cross-compilation
PLATFORMS := darwin/amd64 darwin/arm64 linux/amd64 linux/arm64

.PHONY: all
all: lint test build

## Build targets

.PHONY: build
build: ## Build the binary for the current platform
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=$(CGO_ENABLED) $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./$(CMD_DIR)

.PHONY: build-all
build-all: ## Build for all platforms
	@echo "Building for all platforms..."
	@for platform in $(PLATFORMS); do \
		os=$${platform%/*}; \
		arch=$${platform#*/}; \
		output=$(BUILD_DIR)/$(BINARY_NAME)-$$os-$$arch; \
		if [ "$$os" = "windows" ]; then output="$$output.exe"; fi; \
		echo "Building $$os/$$arch..."; \
		GOOS=$$os GOARCH=$$arch CGO_ENABLED=$(CGO_ENABLED) $(GOBUILD) $(LDFLAGS) -o $$output ./$(CMD_DIR) || exit 1; \
	done

.PHONY: install
install: build ## Install the binary to $GOPATH/bin
	@echo "Installing $(BINARY_NAME)..."
	@cp $(BUILD_DIR)/$(BINARY_NAME) $(GOPATH)/bin/$(BINARY_NAME)

.PHONY: uninstall
uninstall: ## Remove the installed binary
	@echo "Removing $(BINARY_NAME) from $(GOPATH)/bin..."
	@rm -f $(GOPATH)/bin/$(BINARY_NAME)

## Development targets

.PHONY: run
run: ## Run the application
	@$(GOCMD) run ./$(CMD_DIR) $(ARGS)

.PHONY: test
test: ## Run tests
	@echo "Running tests..."
	$(GOTEST) -v -race -coverprofile=coverage.out ./...

.PHONY: test-short
test-short: ## Run short tests only
	@echo "Running short tests..."
	$(GOTEST) -v -short ./...

.PHONY: coverage
coverage: test ## Generate coverage report
	@echo "Generating coverage report..."
	@$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

.PHONY: bench
bench: ## Run benchmarks
	@echo "Running benchmarks..."
	$(GOTEST) -bench=. -benchmem ./...

## Code quality targets

.PHONY: lint
lint: ## Run linters
	@echo "Running linters..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed, running basic checks..."; \
		$(GOVET) ./...; \
		$(GOFMT) -d .; \
	fi

.PHONY: fmt
fmt: ## Format code
	@echo "Formatting code..."
	$(GOFMT) -s -w .

.PHONY: vet
vet: ## Run go vet
	@echo "Running go vet..."
	$(GOVET) ./...

.PHONY: tidy
tidy: ## Tidy and verify go modules
	@echo "Tidying modules..."
	$(GOMOD) tidy
	$(GOMOD) verify

.PHONY: deps
deps: ## Download dependencies
	@echo "Downloading dependencies..."
	$(GOMOD) download

## Cleanup targets

.PHONY: clean
clean: ## Remove build artifacts
	@echo "Cleaning..."
	@rm -rf $(BUILD_DIR)
	@rm -f coverage.out coverage.html

.PHONY: clean-all
clean-all: clean ## Remove all generated files including cache
	@echo "Cleaning all..."
	@$(GOCMD) clean -cache -testcache

## Documentation targets

.PHONY: docs
docs: ## Generate documentation
	@echo "Generating documentation..."
	@$(GOCMD) doc -all ./...

.PHONY: godoc
godoc: ## Start godoc server
	@echo "Starting godoc server at http://localhost:6060..."
	@godoc -http=:6060

## Help target

.PHONY: help
help: ## Show this help
	@echo "obsidian-notion-sync - Bidirectional sync between Obsidian and Notion"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'

# Default target
.DEFAULT_GOAL := help
