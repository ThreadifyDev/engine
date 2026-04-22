.PHONY: help install generate-graphql \
        build build-api build-archiver \
        build-image build-archiver-image build-web-api build-web-ui build-docs \
        run run-api run-archiver run-docker \
        push push-archiver push-web-api push-web-ui push-docs \
        test-engine-unit test-engine-coverage \
        test-archiver test-archiver-coverage \
        test-api test-api-unit \
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

build-archiver-binary: ## Build the archiver binary
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

test-engine-unit: ## Run engine unit tests
	@echo "Running engine unit tests..."
	@$(GO) test $(ENGINE_PKGS) -count=1

test-engine-integration: ## Run engine integration tests
	@echo "Running engine integration tests..."
	@cd tests && $(GO) test ./engine/... -count=1

test-api-integration: ## Run API integration tests
	@echo "Running API integration tests..."
	@cd tests && $(GO) test ./api/... -count=1

test-engine-coverage: ## Run engine tests with HTML coverage
	@echo "Running engine coverage..."
	@$(GO) test $(ENGINE_PKGS) -coverprofile=coverage.out
	@$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "✓ Coverage report: coverage.html"

test-archiver: ## Run archiver tests
	@echo "Running archiver tests..."
	@$(GO) test ./internal/archiver/... -count=1

test-archiver-coverage: ## Run archiver tests with HTML coverage
	@echo "Running archiver coverage..."
	@$(GO) test ./internal/archiver/... -coverprofile=coverage.out
	@$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "✓ Coverage report: coverage.html"

test-api: ## Run all API module tests
	@echo "Running all API tests..."
	@cd api && $(GO) test ./...

test-api-unit: ## Run fast API unit tests only
	@echo "Running API unit tests..."
	@cd api && $(GO) test ./internal/service/tests/... ./internal/handlers/tests/...

test-shared: ## Run shared module tests
	@echo "Running shared tests..."
	@cd shared && $(GO) test ./...

test-all: ## Run all tests (engine + archiver + api + shared)
	$(MAKE) test-engine-unit
	$(MAKE) test-archiver
	$(MAKE) test-api
	$(MAKE) test-shared

test-coverage-api: ## Run API tests with HTML coverage
	@echo "Running API coverage..."
	@cd api && $(GO) test \
	  ./internal/service/tests/... \
	  ./internal/handlers/tests/... \
	  -coverpkg=./internal/service/...,./internal/handlers/... \
	  -coverprofile=coverage.out \
	  -count=1
	@cd api && $(GO) tool cover -html=coverage.out -o coverage.html
	@echo "✓ Coverage report: api/coverage.html"

test-coverage-shared: ## Run shared tests with HTML coverage
	@echo "Running shared coverage..."
	@cd shared && $(GO) test ./billing/... ./repository/... \
	  -coverprofile=coverage.out \
	  -count=1
	@cd shared && $(GO) tool cover -html=coverage.out -o coverage.html
	@echo "✓ Coverage report: shared/coverage.html"

mocks-engine: ## Generate engine mocks
	@echo "Generating engine mocks..."
	@$(GO) generate ./...
	@echo "✓ Engine mocks generated"

mocks-api: ## Generate API mocks
	@echo "Generating API mocks..."
	@cd api && $(GO) generate ./...
	@echo "✓ API mocks generated"

mocks-shared: ## Generate shared mocks
	@echo "Generating shared mocks..."
	@cd shared && $(GO) generate ./...
	@echo "✓ Shared mocks generated"

mocks-all: ## Generate all mocks (engine + api + shared)
	$(MAKE) mocks-engine
	$(MAKE) mocks-shared
	$(MAKE) mocks-api

lint: ## Run linter
	golangci-lint run

fmt: ## Format all code
	@echo "Formatting root module..."
	@$(GO) fmt ./...
	@echo "Formatting api module..."
	@cd api && $(GO) fmt ./...
	@echo "Formatting shared module..."
	@cd shared && $(GO) fmt ./...
	@echo "Formatting tests module..."
	@cd tests && $(GO) fmt ./...
	@$(GOFMT) -s -w .

docker-up: ## Start PostgreSQL and Redis
	docker-compose up -d

docker-down: ## Stop all containers
	docker-compose down

docker-logs: ## Follow container logs
	docker-compose logs -f

dev: docker-up ## Start dev environment and run server
	@sleep 2
	$(MAKE) run

build-image: ## Build engine Docker image
	docker build -t intelijence/th-engine:$(VERSION) .

build-archiver: ## Build archiver Docker image
	docker build -f Dockerfile.archiver -t intelijence/th-archiver:$(VERSION) .

build-web-api: ## Build Web API Docker image
	docker build -f Dockerfile.api -t intelijence/th_web_api:$(VERSION) .

build-web-ui: ## Build Web UI Docker image
	cd .. && docker build -f threadify-go/Dockerfile.web_ui -t intelijence/th_web_ui:$(VERSION) .

build-docs: ## Build Documentation Docker image
	cd ../documentation && docker build -t intelijence/th-docs:$(VERSION) .

push: ## Push engine Docker image
	docker push intelijence/th-engine:$(VERSION)

push-archiver: ## Push archiver Docker image
	docker push intelijence/th-archiver:$(VERSION)

push-web-api: ## Push Web API Docker image
	docker push intelijence/th_web_api:$(VERSION)

push-web-ui: ## Push Web UI Docker image
	docker push intelijence/th_web_ui:$(VERSION)

push-docs: ## Push Documentation Docker image
	docker push intelijence/th-docs:$(VERSION)

run-docker: ## Run engine in Docker
	docker run -p 8080:8080 --env-file .env intelijence/th-engine:latest

clean: ## Remove build artifacts and coverage reports
	rm -rf bin/
	rm -f coverage.out coverage.html
	rm -f api/coverage.out api/coverage.html
	rm -f shared/coverage.out shared/coverage.html
