# k8s-gsidecar Makefile

# Variables
APP_NAME := k8s-gsidecar
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S')
DOCKER_REGISTRY ?= docker.io
DOCKER_IMAGE := $(DOCKER_REGISTRY)/$(APP_NAME)
GO_VERSION := 1.25

# Build flags
LDFLAGS := -ldflags "-X main.Version=$(VERSION) -X main.Commit=$(COMMIT) -X main.BuildTime=$(BUILD_TIME) -w -s"

.PHONY: help
help: ## Display this help message
	@echo "Available targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-20s %s\n", $$1, $$2}'

.PHONY: build
build: ## Build the binary
	@echo "Building $(APP_NAME)..."
	go build $(LDFLAGS) -o $(APP_NAME) .
	@echo "Build complete: ./$(APP_NAME)"

.PHONY: build-linux
build-linux: ## Build the binary for Linux
	@echo "Building $(APP_NAME) for Linux..."
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(APP_NAME)-linux-amd64 .
	@echo "Build complete: ./$(APP_NAME)-linux-amd64"

.PHONY: build-darwin
build-darwin: ## Build the binary for macOS
	@echo "Building $(APP_NAME) for macOS..."
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $(APP_NAME)-darwin-amd64 .
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(APP_NAME)-darwin-arm64 .
	@echo "Build complete: ./$(APP_NAME)-darwin-*"

.PHONY: build-all
build-all: build-linux build-darwin ## Build binaries for all platforms

.PHONY: test
test: ## Run tests
	@echo "Running tests..."
	go test -v -coverprofile=coverage.out ./...
	@echo "Tests complete"

.PHONY: test-coverage
test-coverage: test ## Run tests with coverage report
	@echo "Generating coverage report..."
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

.PHONY: clean
clean: ## Clean build artifacts
	@echo "Cleaning..."
	rm -f $(APP_NAME)
	rm -f $(APP_NAME)-*
	rm -f coverage.out coverage.html
	@echo "Clean complete"

.PHONY: fmt
fmt: ## Format Go code
	@echo "Formatting code..."
	go fmt ./...
	@echo "Format complete"

.PHONY: lint
lint: ## Run linter
	@echo "Running linter..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not found, skipping..."; \
		echo "Install: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
	fi

.PHONY: vet
vet: ## Run go vet
	@echo "Running go vet..."
	go vet ./...
	@echo "Vet complete"

.PHONY: tidy
tidy: ## Tidy Go modules
	@echo "Tidying modules..."
	go mod tidy
	@echo "Tidy complete"

.PHONY: docker-build
docker-build: ## Build Docker image
	@echo "Building Docker image: $(DOCKER_IMAGE):$(VERSION)..."
	docker build -f docker/Dockerfile -t $(DOCKER_IMAGE):$(VERSION) .
	docker tag $(DOCKER_IMAGE):$(VERSION) $(DOCKER_IMAGE):latest
	@echo "Docker build complete"

.PHONY: docker-build-no-cache
docker-build-no-cache: ## Build Docker image without cache
	@echo "Building Docker image (no cache): $(DOCKER_IMAGE):$(VERSION)..."
	docker build --no-cache -f docker/Dockerfile -t $(DOCKER_IMAGE):$(VERSION) .
	docker tag $(DOCKER_IMAGE):$(VERSION) $(DOCKER_IMAGE):latest
	@echo "Docker build complete"

.PHONY: docker-push
docker-push: ## Push Docker image to registry
	@echo "Pushing Docker image: $(DOCKER_IMAGE):$(VERSION)..."
	docker push $(DOCKER_IMAGE):$(VERSION)
	docker push $(DOCKER_IMAGE):latest
	@echo "Docker push complete"

.PHONY: docker-run
docker-run: ## Run Docker container locally
	@echo "Running Docker container..."
	docker run --rm -it \
		-e METHOD=watch \
		-e NAMESPACE=default \
		-e LABEL=app \
		-e RESOURCE=configmap \
		-e FOLDER=/config \
		-v $(PWD)/config:/config \
		$(DOCKER_IMAGE):latest

.PHONY: run
run: ## Run the application locally
	@echo "Running $(APP_NAME)..."
	go run .

.PHONY: install
install: ## Install the binary to GOPATH/bin
	@echo "Installing $(APP_NAME)..."
	go install $(LDFLAGS) .
	@echo "Install complete"

.PHONY: deps
deps: ## Download dependencies
	@echo "Downloading dependencies..."
	go mod download
	@echo "Dependencies downloaded"

.PHONY: check
check: fmt vet lint test ## Run all checks (fmt, vet, lint, test)
	@echo "All checks passed"

.PHONY: release
release: clean check build-all docker-build docker-push ## Build and release (all platforms + Docker)
	@echo "Release $(VERSION) complete"

.PHONY: version
version: ## Display version information
	@echo "Version:    $(VERSION)"
	@echo "Commit:     $(COMMIT)"
	@echo "Build Time: $(BUILD_TIME)"
	@echo "Go Version: $(GO_VERSION)"

.PHONY: dev
dev: ## Run in development mode with hot reload (requires air)
	@if command -v air >/dev/null 2>&1; then \
		air; \
	else \
		echo "air not found. Install: go install github.com/cosmtrek/air@latest"; \
		echo "Running without hot reload..."; \
		$(MAKE) run; \
	fi

.PHONY: k8s-deploy
k8s-deploy: ## Deploy to Kubernetes (requires kubectl)
	@echo "Deploying to Kubernetes..."
	@if [ -f k8s/deployment.yaml ]; then \
		kubectl apply -f k8s/; \
	else \
		echo "k8s/deployment.yaml not found"; \
	fi

.PHONY: k8s-delete
k8s-delete: ## Delete from Kubernetes
	@echo "Deleting from Kubernetes..."
	@if [ -f k8s/deployment.yaml ]; then \
		kubectl delete -f k8s/; \
	else \
		echo "k8s/deployment.yaml not found"; \
	fi

.PHONY: k8s-logs
k8s-logs: ## Show Kubernetes logs
	@echo "Showing logs..."
	kubectl logs -l app=$(APP_NAME) -f

.DEFAULT_GOAL := help

