.PHONY: help build run test docker-build docker-run compose-up compose-down clean lint fmt vet swagger init-db

# Default target
help: ## Show this help message
	@echo "Available commands:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

# Build the application
build: ## Build the Go application
	@echo "Building application..."
	@go build -ldflags="-w -s" -o bin/crypto-backend ./cmd/api
	@echo "Build complete: bin/crypto-backend"

# Run the application locally
run: ## Run the application locally
	@echo "Running application..."
	@go run ./cmd/api/main.go

# Run tests
test: ## Run all tests
	@echo "Running tests..."
	@go test -v -race -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Run unit tests only
test-unit: ## Run unit tests only
	@echo "Running unit tests..."
	@go test -v -short ./...

# Run integration tests
test-integration: ## Run integration tests with Docker
	@echo "Running integration tests..."
	@docker-compose -f docker-compose.ci.yml up --build --abort-on-container-exit
	@docker-compose -f docker-compose.ci.yml down --volumes

# Build Docker image
docker-build: ## Build Docker image
	@echo "Building Docker image..."
	@docker build -t crypto-backend:latest .
	@echo "Docker image built: crypto-backend:latest"

# Run Docker container
docker-run: docker-build ## Run Docker container
	@echo "Running Docker container..."
	@docker run --rm -p 8080:8080 crypto-backend:latest

# Start services with Docker Compose
compose-up: ## Start all services with Docker Compose
	@echo "Starting services with Docker Compose..."
	@docker-compose up --build -d
	@echo "Services started. API available at http://localhost:8080"
	@echo "Swagger docs at http://localhost:8080/swagger/index.html"

# Stop services
compose-down: ## Stop all Docker Compose services
	@echo "Stopping services..."
	@docker-compose down
	@echo "Services stopped"

# Stop services and remove volumes
compose-down-volumes: ## Stop services and remove all volumes
	@echo "Stopping services and removing volumes..."
	@docker-compose down --volumes
	@echo "Services stopped and volumes removed"

# View logs
logs: ## View application logs
	@docker-compose logs -f api

# Generate Swagger documentation
swagger: ## Generate Swagger documentation
	@echo "Generating Swagger documentation..."
	@which swag > /dev/null || (echo "Installing swag..." && go install github.com/swaggo/swag/cmd/swag@latest)
	@swag init -g cmd/api/main.go -o ./docs
	@echo "Swagger documentation generated in ./docs"

# Initialize databases
init-db: ## Initialize database schemas
	@echo "Initializing databases..."
	@docker-compose exec clickhouse clickhouse-client --query "CREATE DATABASE IF NOT EXISTS crypto"
	@docker-compose exec postgres psql -U crypto -d crypto -f /docker-entrypoint-initdb.d/init.sql
	@echo "Databases initialized"

# Clean build artifacts
clean: ## Clean build artifacts and Docker resources
	@echo "Cleaning build artifacts..."
	@rm -rf bin/
	@rm -f coverage.out coverage.html
	@docker system prune -f
	@echo "Clean complete"

# Format Go code
fmt: ## Format Go code
	@echo "Formatting Go code..."
	@go fmt ./...
	@echo "Code formatted"

# Vet Go code
vet: ## Vet Go code
	@echo "Vetting Go code..."
	@go vet ./...
	@echo "Code vetted"

# Lint Go code
lint: ## Lint Go code with golangci-lint
	@echo "Linting Go code..."
	@which golangci-lint > /dev/null || (echo "Installing golangci-lint..." && go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)
	@golangci-lint run
	@echo "Code linted"

# Run quality checks
quality: fmt vet lint ## Run all code quality checks

# Install development dependencies
dev-deps: ## Install development dependencies
	@echo "Installing development dependencies..."
	@go install github.com/swaggo/swag/cmd/swag@latest
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@go install github.com/cosmtrek/air@latest
	@echo "Development dependencies installed"

# Watch and reload (requires air)
dev: swagger ## Run in development mode with hot reload
	@echo "Starting development server with hot reload..."
	@which air > /dev/null || (echo "Installing air..." && go install github.com/cosmtrek/air@latest)
	@air -c .air.toml

# Setup project (first time)
setup: dev-deps swagger ## Setup project for first time
	@echo "Project setup complete!"
	@echo "Run 'make compose-up' to start services"
	@echo "Run 'make dev' for development with hot reload"

# Production deployment
deploy: ## Deploy to production (example)
	@echo "Deploying to production..."
	@docker-compose -f docker-compose.prod.yml up -d --build
	@echo "Production deployment complete"

# PostgreSQL migrations
migrate-postgres-up: ## Run PostgreSQL migrations up
	@echo "Running PostgreSQL migrations..."
	@go run cmd/migrate/migrate.go -db=postgres -dir=up

migrate-postgres-down: ## Rollback PostgreSQL migrations
	@echo "Rolling back PostgreSQL migrations..."
	@go run cmd/migrate/migrate.go -db=postgres -dir=down -steps=1

migrate-postgres-version: ## Check PostgreSQL migration version
	@go run cmd/migrate/migrate.go -db=postgres -version

# ClickHouse migrations
migrate-clickhouse-up: ## Run ClickHouse migrations up
	@echo "Running ClickHouse migrations..."
	@go run cmd/migrate/migrate.go -db=clickhouse -dir=up

migrate-clickhouse-down: ## Rollback ClickHouse migrations
	@echo "Rolling back ClickHouse migrations..."
	@go run cmd/migrate/migrate.go -db=clickhouse -dir=down -steps=1

migrate-clickhouse-version: ## Check ClickHouse migration version
	@go run cmd/migrate/migrate.go -db=clickhouse -version

# Run all migrations
migrate-up: migrate-postgres-up migrate-clickhouse-up ## Run all database migrations up

migrate-down: ## Rollback last migration for both databases
	@echo "Rolling back migrations..."
	@go run cmd/migrate/migrate.go -db=postgres -dir=down -steps=1
	@go run cmd/migrate/migrate.go -db=clickhouse -dir=down -steps=1

migrate-reset: ## Reset all migrations (careful!)
	@echo "Resetting all migrations..."
	@go run cmd/migrate/migrate.go -db=postgres -dir=down
	@go run cmd/migrate/migrate.go -db=clickhouse -dir=down
	@$(MAKE) migrate-up

# Seed database with token data
seed-tokens: ## Seed tokens from JSON file
	@echo "Seeding tokens from configs/tokens.json..."
	@go run cmd/seed/main.go configs/tokens.json
	@echo "Token seeding complete"

seed-symbols: ## Seed symbol mappings for exchanges
	@echo "Seeding symbol mappings..."
	@go run cmd/seed-symbols/main.go
	@echo "Symbol mapping seeding complete"

# Run migrations and seed data
db-setup: migrate-up seed-tokens seed-symbols ## Run migrations and seed initial data
	@echo "Database setup complete"

# Performance benchmarks
benchmark: ## Run performance benchmarks
	@echo "Running benchmarks..."
	@go test -bench=. -benchmem ./...

# Security scan
security: ## Run security scan
	@echo "Running security scan..."
	@which gosec > /dev/null || (echo "Installing gosec..." && go install github.com/securecodewarrior/gosec/v2/cmd/gosec@latest)
	@gosec ./...
	@echo "Security scan complete"

# Generate mocks (if using mockgen)
mocks: ## Generate test mocks
	@echo "Generating test mocks..."
	@which mockgen > /dev/null || (echo "Installing mockgen..." && go install github.com/golang/mock/mockgen@latest)
	@echo "Mocks generation not yet configured"

# Local monitoring setup
monitor: ## Start monitoring stack (Grafana, Prometheus)
	@echo "Starting monitoring stack..."
	@docker-compose --profile monitoring up -d
	@echo "Grafana available at http://localhost:3000 (admin/admin123)"