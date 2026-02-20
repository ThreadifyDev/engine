.PHONY: help build run test clean docker-up docker-down install lint fmt generate-graphql build-web-api build-web-ui build-docs push-web-api push-web-ui push-docs

VERSION := $(shell git describe --tags --always)

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

install: ## Install dependencies
	go mod download
	go mod tidy
	go install github.com/99designs/gqlgen@v0.17.85

generate-graphql: ## Generate GraphQL code from schema
	@echo "Generating GraphQL code..."
	cd internal/graphql && $(HOME)/go/bin/gqlgen generate
	@echo "✓ GraphQL code generated"

build: generate-graphql ## Build the server binary
	@echo "Building server..."
	@mkdir -p bin
	go build -o bin/server cmd/server/main.go
	@echo "✓ Binary created at bin/server"

run: ## Run the server
	@echo "Killing any process on port 8081..."
	@lsof -ti:8081 | xargs kill -9 2>/dev/null || true
	go build -o bin/server cmd/server/main.go
	./bin/server

run-archiver: ## Build and run the archiver binary
	@echo "Killing any process on port 8082..."
	@lsof -ti:8082 | xargs kill -9 2>/dev/null || true
	@echo "Building archiver..."
	go build -o bin/archiver cmd/archiver/main.go
	@echo "✓ Binary created at bin/archiver"
	./bin/archiver

build-api: ## Build the Web API binary
	@echo "Building Web API..."
	@mkdir -p bin
	cd api && go build -o ../bin/api cmd/server/main.go
	@echo "✓ Binary created at bin/api"

run-api: ## Run the Web API server
	@echo "Killing any process on port 3001..."
	@lsof -ti:3001 | xargs kill -9 2>/dev/null || true
	make build-api
	cd api && go run cmd/server/main.go --config=../config/config.yaml

test: ## Run all tests
	go test ./tests/... -v -count=1

test-coverage: ## Run tests with coverage
	go test ./tests/... -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html
	@echo "✓ Coverage report: coverage.html"

lint: ## Run linter
	golangci-lint run

fmt: ## Format code
	go fmt ./...
	gofmt -s -w .

clean: ## Clean build artifacts
	rm -rf bin/
	rm -f coverage.out coverage.html

docker-up: ## Start PostgreSQL and Redis
	docker-compose up -d postgres redis

docker-down: ## Stop all containers
	docker-compose down

docker-logs: ## Show container logs
	docker-compose logs -f

dev: docker-up ## Start dev environment and run server
	@sleep 2
	@make run

build-image: ## Build Docker image
	docker build -t intelijence/th-engine:${VERSION} .

build-archiver: ## Build archiver Docker image
	docker build $(DOCKER_BUILD_ARGS) -f Dockerfile.archiver -t intelijence/th-archiver:${VERSION} .

build-web-api: ## Build Web API Docker image
	docker build -f Dockerfile.api -t intelijence/th_web_api:${VERSION} .

build-web-ui: ## Build Web UI Docker image
	cd .. && docker build -f threadify-go/Dockerfile.web_ui -t intelijence/th_web_ui:${VERSION} .

build-docs: ## Build Documentation Docker image
	cd ../documentation && docker build -t intelijence/th-docs:${VERSION} .

push: ## Push Threadify Engine server
	docker push intelijence/th-engine:${VERSION}

push-archiver: ## Push archiver Docker image
	docker push intelijence/th-archiver:${VERSION}

push-web-api: ## Push Web API Docker image
	docker push intelijence/th_web_api:${VERSION}

push-web-ui: ## Push Web UI Docker image
	docker push intelijence/th_web_ui:${VERSION}

push-docs: ## Push Documentation Docker image
	docker push intelijence/th-docs:${VERSION}

run-docker: ## Run server in Docker
	docker run -p 8080:8080 --env-file .env intelijence/th-engine:latest

.DEFAULT_GOAL := help
