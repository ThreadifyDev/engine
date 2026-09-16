.PHONY: help install generate-graphql \
	build build-api build-archiver \
	build-image build-web-api build-docs \
	run run-api run-archiver run-docker \
	push publish-engine push-archiver publish-archiver \
	push-web-api publish-web-api push-docs \
	test-engine-integration test-engine-coverage \
	test-archiver test-archiver-coverage \
	test-api test-api-integration \
	test-shared test-all \
	test-coverage-api test-coverage-shared \
	mocks-engine mocks-api mocks-shared mocks-all \
	test-engine \
	lint fmt clean \
	docker-up docker-down docker-logs dev

VERSION := $(shell git describe --tags --always --match "v*" 2>/dev/null || git rev-parse --short HEAD)
DOCKER_NAMESPACE ?= intelijence
DOCKER_PLATFORMS ?= linux/amd64,linux/arm64
ENGINE_IMAGE ?= $(DOCKER_NAMESPACE)/th-engine
ARCHIVER_IMAGE ?= $(DOCKER_NAMESPACE)/th-archiver
WEB_API_IMAGE ?= $(DOCKER_NAMESPACE)/th_web_api

define publish-multiarch
	docker buildx build --platform $(DOCKER_PLATFORMS) -f $(1) \
		-t $(2):$(VERSION) \
		-t $(2):latest \
		--push $(3)
endef

GO ?= go
# Bound compiler concurrency on developer machines; override on larger builders.
BUILD_JOBS ?= 2
BUILD_PROCS ?= 2
GOFMT ?= gofmt
GOBIN ?= $(HOME)/go/bin
MOCKGEN ?= $(GOBIN)/mockgen

ENGINE_PKGS = $(shell $(GO) list ./... | grep -v -E "/api|/archiver|/shared|/tests")

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

build: ## Build the combined Threadify binary (engine, broker, persistence)
	@echo "Building Threadify..."
	@mkdir -p bin
	GOMAXPROCS=$(BUILD_PROCS) CGO_ENABLED=0 $(GO) build -p $(BUILD_JOBS) -trimpath -ldflags="-s -w" -o bin/threadify ./cmd/server
	@echo "✓ Binary created at bin/threadify"

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
	./bin/threadify

run-api: build-api ## Build and run the Web API server
	@lsof -ti:3001 | xargs kill -9 2>/dev/null || true
	cd api && $(GO) run cmd/server/main.go --config=../config/config.yaml

run-archiver: build-archiver-binary ## Build and run the archiver
	@lsof -ti:8082 | xargs kill -9 2>/dev/null || true
	./bin/archiver --config=config/config.yaml

test-engine-integration: ## Run engine integration tests
	@echo "Running engine integration tests..."
	@cd tests && $(GO) test -v ./engine/... -count=1

.PHONY: test-e2e test-e2e-live check-dashboard
check-dashboard: ## Verify CI-built dashboard assets are staged
	@python3 scripts/check-dashboard.py

test-e2e: check-dashboard build ## Run compiled-binary E2E with disposable PostgreSQL and Valkey (Docker required)
	@cd tests && THREADIFY_LIVE_DIR= THREADIFY_E2E_BINARY="$(CURDIR)/bin/threadify" $(GO) test -v ./e2e/... -count=1 -timeout=5m

test-e2e-live: ## Run E2E against the explicit local config directory in THREADIFY_LIVE_DIR
	@test -n "$$THREADIFY_LIVE_DIR" || (echo "Set THREADIFY_LIVE_DIR to the local instance's config directory"; exit 1)
	@cd tests && $(GO) test -v ./e2e/... -count=1 -timeout=5m

test-engine: ## Run all engine tests (unit)
	@echo "Running engine unit tests..."
	@$(GO) test $(ENGINE_PKGS) -count=1

test-api-integration: ## Run API integration tests
	@echo "Running API integration tests..."
	@cd tests && $(GO) test -v ./api/... -count=1

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
	$(MAKE) test-e2e
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
	docker build -t $(ENGINE_IMAGE):$(VERSION) .

build-archiver: ## Build archiver Docker image
	docker build -f Dockerfile.archiver -t $(ARCHIVER_IMAGE):$(VERSION) .

build-web-api: ## Build Web API Docker image
	docker build -f Dockerfile.api -t $(WEB_API_IMAGE):$(VERSION) .

build-docs: ## Build Documentation Docker image
	cd ../documentation && docker build -t intelijence/th-docs:$(VERSION) .

push: ## Push engine Docker image
	docker push $(ENGINE_IMAGE):$(VERSION)

publish-engine: ## Build and push multi-architecture engine images
	$(call publish-multiarch,Dockerfile,$(ENGINE_IMAGE),.)

push-archiver: ## Push archiver Docker image
	docker push $(ARCHIVER_IMAGE):$(VERSION)

publish-archiver: ## Build and push multi-architecture archiver images
	$(call publish-multiarch,Dockerfile.archiver,$(ARCHIVER_IMAGE),.)

push-web-api: ## Push Web API Docker image
	docker push $(WEB_API_IMAGE):$(VERSION)

publish-web-api: ## Build and push multi-architecture Web API images
	$(call publish-multiarch,Dockerfile.api,$(WEB_API_IMAGE),.)

push-docs: ## Push Documentation Docker image
	docker push intelijence/th-docs:$(VERSION)

run-docker: ## Run engine in Docker
	docker run -p 8080:8080 --env-file .env intelijence/th-engine:latest

clean: ## Remove build artifacts and coverage reports
	rm -rf bin/
	rm -f coverage.out coverage.html
	rm -f api/coverage.out api/coverage.html
	rm -f shared/coverage.out shared/coverage.html
	@rm -f shared/coverage.out shared/coverage.html
	@./coverage_shared.sh
