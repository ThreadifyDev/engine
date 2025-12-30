.PHONY: help build up down restart logs clean test

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build all Docker images
	docker-compose build

up: ## Start all services
	docker-compose up -d

down: ## Stop all services
	docker-compose down

restart: ## Restart all services
	docker-compose restart

logs: ## View logs from all services
	docker-compose logs -f

logs-engine: ## View logs from Threadify Engine only
	docker-compose logs -f threadify-engine

logs-db: ## View logs from PostgreSQL only
	docker-compose logs -f postgres

logs-valkey: ## View logs from Valkey only
	docker-compose logs -f valkey

logs-archiver: ## View logs from Archiver only
	docker-compose logs -f threadify-archiver

ps: ## Show running services
	docker-compose ps

clean: ## Stop services and remove volumes (⚠️  deletes all data)
	docker-compose down -v

rebuild-engine: ## Rebuild and restart Threadify Engine
	docker-compose build threadify-engine
	docker-compose up -d threadify-engine

rebuild-archiver: ## Rebuild and restart Threadify Archiver
	docker-compose build threadify-archiver
	docker-compose up -d threadify-archiver

shell-engine: ## Open shell in Threadify Engine container
	docker-compose exec threadify-engine sh

shell-archiver: ## Open shell in Archiver container
	docker-compose exec threadify-archiver sh

shell-db: ## Open PostgreSQL shell
	docker-compose exec postgres psql -U td_engine -d threadify

shell-valkey: ## Open Valkey CLI
	docker-compose exec valkey valkey-cli -a threadify_secure_password

test: ## Run E2E tests
	cd threadify-sdk && node test/e2e-validation.test.js

health: ## Check health of all services
	@echo "Checking Threadify Engine..."
	@curl -s http://localhost:8081/health || echo "❌ Engine not responding"
	@echo "\nChecking PostgreSQL..."
	@docker-compose exec -T postgres pg_isready -U td_engine || echo "❌ PostgreSQL not ready"
	@echo "\nChecking Valkey..."
	@docker-compose exec -T valkey valkey-cli -a threadify_secure_password ping || echo "❌ Valkey not responding"

dev-db: ## Start only databases (for local development)
	docker-compose up -d postgres valkey

dev-server: ## Run Go server locally (requires dev-db)
	cd threadify-go && go run cmd/server/main.go

backup-db: ## Backup PostgreSQL data
	docker run --rm -v threadify-persist:/data -v $$(pwd):/backup alpine tar czf /backup/postgres-backup-$$(date +%Y%m%d-%H%M%S).tar.gz /data

restore-db: ## Restore PostgreSQL data from backup (set BACKUP_FILE=filename)
	@if [ -z "$(BACKUP_FILE)" ]; then echo "Error: Set BACKUP_FILE=filename"; exit 1; fi
	docker-compose down
	docker volume rm threadify-persist || true
	docker volume create threadify-persist
	docker run --rm -v threadify-persist:/data -v $$(pwd):/backup alpine tar xzf /backup/$(BACKUP_FILE) -C /
	docker-compose up -d

init: ## Initialize environment (first time setup)
	@if [ ! -f .env ]; then cp .env.example .env; echo "✅ Created .env file"; else echo "⚠️  .env already exists"; fi
	@echo "✅ Ready to run 'make up'"
