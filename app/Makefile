# Makefile for the reflector Go controller

GO = go
GOFLAGS = -v

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GIT_COMMIT ?= $(shell git rev-parse HEAD 2>/dev/null || echo "unknown")
BUILD_TIME ?= $(shell date -u '+%Y-%m-%d %H:%M:%S UTC')
LDFLAGS = -ldflags "-s -w -X 'github.com/mqtt-home/reflector/version.Version=$(VERSION)' -X 'github.com/mqtt-home/reflector/version.GitCommit=$(GIT_COMMIT)' -X 'github.com/mqtt-home/reflector/version.BuildTime=$(BUILD_TIME)'"

BUILD_DIR = build
BINARY_NAME = reflector
DOCKER_IMAGE_NAME = pharndt/reflector:latest

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help
	@echo "reflector — available targets:"
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

.PHONY: build
build: ## Build the Go binary
	@echo "Building the backend..."
	@$(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) .

.PHONY: version
version: ## Print version, commit and build time
	@echo "Version: $(VERSION)"
	@echo "Git Commit: $(GIT_COMMIT)"
	@echo "Build Time: $(BUILD_TIME)"

.PHONY: test
test: ## Run the Go tests
	@echo "Running tests..."
	@$(GO) test ./...

.PHONY: vet
vet: ## Run go vet
	@$(GO) vet ./...

.PHONY: run
run: build ## Build and run against the current kubeconfig (out-of-cluster)
	@echo "Running the reflector..."
	@$(BUILD_DIR)/$(BINARY_NAME)

.PHONY: docker
docker: ## Build the Docker image
	@echo "Building Docker image..."
	@docker build -t $(DOCKER_IMAGE_NAME) .

.PHONY: clean
clean: ## Remove build artifacts
	@echo "Cleaning up..."
	@rm -rf $(BUILD_DIR)

.PHONY: deps
deps: ## Download and tidy Go dependencies
	@echo "Downloading Go dependencies..."
	@$(GO) mod download
	@$(GO) mod tidy
