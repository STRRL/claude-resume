.PHONY: help build build-all build-linux build-linux-amd64 build-linux-arm64 build-darwin install clean test test-verbose test-integration test-coverage test-bench run release release-snapshot

# Default target is help
.DEFAULT_GOAL := help

# Help command - displays all available targets with descriptions
help: ## Show this help message
	@echo 'Quick Start:'
	@echo '  make build         # Build the application'
	@echo '  make run           # Build and run'
	@echo '  make test          # Run tests'
	@echo ''
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*##"; printf "\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)
	@echo ''

##@ Building

build: ## Build for current platform
	go build -o claude-resume ./cmd/claude-resume

build-all: build-linux build-darwin ## Build for all supported platforms

build-linux: build-linux-amd64 build-linux-arm64 ## Build for Linux (amd64 and arm64)

build-linux-amd64: ## Build for Linux amd64/x86_64
	GOOS=linux GOARCH=amd64 CGO_ENABLED=1 go build -ldflags="-s -w" -o dist/claude-resume-linux-amd64 ./cmd/claude-resume

build-linux-arm64: ## Build for Linux arm64
	GOOS=linux GOARCH=arm64 CGO_ENABLED=1 CC=aarch64-linux-gnu-gcc CXX=aarch64-linux-gnu-g++ go build -ldflags="-s -w" -o dist/claude-resume-linux-arm64 ./cmd/claude-resume

build-darwin: ## Build for macOS (Intel and Apple Silicon)
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=1 go build -ldflags="-s -w" -o dist/claude-resume-darwin-amd64 ./cmd/claude-resume
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=1 go build -ldflags="-s -w" -o dist/claude-resume-darwin-arm64 ./cmd/claude-resume

##@ Installation & Cleanup

install: ## Install to Go bin directory
	go install ./cmd/claude-resume

clean: ## Remove build artifacts
	rm -f claude-resume
	rm -rf dist/

##@ Testing

test: ## Run all tests
	go test ./...

test-verbose: ## Run tests with verbose output
	go test -v ./...

test-integration: ## Run integration tests
	go test -v ./internal/sessions -run TestAsync
	go test -v ./internal/tui

test-coverage: ## Run tests with coverage report
	go test -cover ./...

test-bench: ## Run benchmark tests
	go test -bench=. -benchmem ./...

##@ Development

run: build ## Build and run the application
	./claude-resume

##@ Release

release: ## Create a release using GoReleaser
	goreleaser release --clean

release-snapshot: ## Test release build locally (without publishing)
	goreleaser release --snapshot --clean