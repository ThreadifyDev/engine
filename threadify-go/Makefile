.PHONY: help install generate-graphql \
        build build-api build-archiver \
        build-image build-archiver-image build-web-api build-web-ui build-docs \
        run run-api run-archiver run-docker \
        push push-archiver push-web-api push-web-ui push-docs \
        test-engine-integration test-engine-coverage \
        test-archiver test-archiver-coverage \
        test-api test-api-integration \
        test-shared test-all \
        test-coverage-api test-coverage-shared \
        mocks-engine mocks-api mocks-shared mocks-all \
        lint fmt clean \
        docker-up docker-down docker-logs dev

VERSION := $(shell git describe --tags --always --match "v*" 2>/dev/null || git rev-parse --short HEAD)

GO ?= go
GOFMT ?= gofmt
GOBIN ?= $(HOME)/go/bin
MOCKGEN ?= $(GOBIN)/mockgen

ENGINE_PKGS := $(shell $(GO) list ./... | grep -v archiver)

.DEFAULT_GOAL := help

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-25s\033[0m %s\n", $$1, $$2}'

install: ## Install dependencies
	go mod download
	go mod tidy
	go install github.com/99designs/gqlgen@v0.17.85
	go install github.com/golang/mock/mockgen@v1.6.0

generate-graphql: ## Generate GraphQL code from schema
	@echo "Generating GraphQL code..."
	cd internal/graphql && $(GO) run github.com/99designs/gqlgen generate
	@echo "✓ GraphQL code generated"

build: generate-graphql ## Build the server binary
	@echo "Building server..."
	@mkdir -p bin
	$(GO) build -o bin/server ./cmd/server
	@echo "✓ Binary created at bin/server"

build-api: ## Build the Web API binary
	@echo "Building Web API..."
	@mkdir -p bin
	cd api && $(GO) build -o ../bin/api cmd/server/main.go
	@echo "✓ Binary created at bin/api"

build-archiver: ## Build the archiver binary
	@echo "Building archiver..."
	@mkdir -p bin
	$(GO) build -o bin/archiver cmd/archiver/main.go
	@echo "✓ Binary created at bin/archiver"

run: build ## Build and run the server
	@lsof -ti:8081 | xargs kill -9 2>/dev/null || true
	./bin/server

run-api: build-api ## Build and run the Web API server
	@lsof -ti:3001 | xargs kill -9 2>/dev/null || true
	cd api && $(GO) run cmd/server/main.go --config=../config/config.yaml

run-archiver: build-archiver ## Build and run the archiver
	@lsof -ti:8082 | xargs kill -9 2>/dev/null || true
	./bin/archiver

test-engine-integration: ## Run engine integration tests
	@echo "Running engine integration tests..."
	@cd tests && $(GO) test -v ./engine/... -count=1

test-api-integration: ## Run API integration tests
	@echo "Running API integration tests..."
	@cd tests && $(GO) test ./api/... -count=1

test-archiver: ## Run archiver tests
	@echo "Running archiver tests..."
	@$(GO) test ./internal/archiver/... -count=1

test-api: ## Run all API module tests
	@echo "Running all API tests..."
	@cd api && $(GO) test ./...

test-shared: ## Run shared module tests
	@echo "Running shared tests..."
	@cd shared && $(GO) test ./...

test-all: ## Run all tests (engine + archiver + api + shared)
	$(MAKE) test-engine-integration
	$(MAKE) test-archiver
	$(MAKE) test-api
	$(MAKE) test-shared

test-archiver-coverage: ## Run archiver tests with HTML coverage
	@echo "Running archiver coverage..."
	@rm -f coverage.out coverage.html
	@./coverage_archiver.sh

test-engine-coverage: ## Run engine tests with HTML coverage
	@echo "Running engine coverage..."
	@rm -f coverage.out coverage.html
	@./coverage.sh

test-coverage-api: ## Run API tests with HTML coverage
	@echo "Running API coverage..."
	@rm -f api/coverage.out api/coverage.html
	@./coverage_api.sh

test-coverage-shared: ## Run shared module tests with HTML coverage
	@echo "Running shared coverage..."
	@rm -f shared/coverage.out shared/coverage.html
	@./coverage_shared.sh
