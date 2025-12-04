.PHONY: build run test clean migrate-up migrate-down docker-build docker-up docker-down lint

# Binary names
SERVER_BINARY=alexander-server
ADMIN_BINARY=alexander-admin
MIGRATE_BINARY=alexander-migrate

# Build directories
BUILD_DIR=./bin

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=$(GOCMD) fmt
GOVET=$(GOCMD) vet

# Database
DB_URL ?= postgres://alexander:alexander@localhost:5432/alexander?sslmode=disable

# Build all binaries
build: build-server build-admin build-migrate

build-server:
	@echo "Building server..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) -o $(BUILD_DIR)/$(SERVER_BINARY) ./cmd/alexander-server

build-admin:
	@echo "Building admin CLI..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) -o $(BUILD_DIR)/$(ADMIN_BINARY) ./cmd/alexander-admin

build-migrate:
	@echo "Building migration tool..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) -o $(BUILD_DIR)/$(MIGRATE_BINARY) ./cmd/alexander-migrate

# Run the server
run: build-server
	@echo "Running server..."
	$(BUILD_DIR)/$(SERVER_BINARY)

# Run tests
test:
	@echo "Running tests..."
	$(GOTEST) -v -race -cover ./...

# Run tests with coverage report
test-coverage:
	@echo "Running tests with coverage..."
	$(GOTEST) -v -race -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html

# Clean build artifacts
clean:
	@echo "Cleaning..."
	@rm -rf $(BUILD_DIR)
	@rm -f coverage.out coverage.html

# Database migrations
migrate-up:
	@echo "Running migrations up..."
	migrate -path migrations/postgres -database "$(DB_URL)" up

migrate-down:
	@echo "Running migrations down..."
	migrate -path migrations/postgres -database "$(DB_URL)" down 1

migrate-create:
	@echo "Creating new migration..."
	@read -p "Enter migration name: " name; \
	migrate create -ext sql -dir migrations/postgres -seq $$name

# Docker commands
docker-build:
	@echo "Building Docker image..."
	docker build -t alexander-storage:latest .

docker-up:
	@echo "Starting Docker Compose services..."
	docker-compose -f configs/docker-compose.yaml up -d

docker-down:
	@echo "Stopping Docker Compose services..."
	docker-compose -f configs/docker-compose.yaml down

docker-logs:
	docker-compose -f configs/docker-compose.yaml logs -f

# Development tools
lint:
	@echo "Running linter..."
	golangci-lint run ./...

fmt:
	@echo "Formatting code..."
	$(GOFMT) ./...

vet:
	@echo "Running go vet..."
	$(GOVET) ./...

# Download dependencies
deps:
	@echo "Downloading dependencies..."
	$(GOMOD) download
	$(GOMOD) tidy

# Generate mocks (if using mockgen)
generate:
	@echo "Generating mocks..."
	$(GOCMD) generate ./...

# Help
help:
	@echo "Available targets:"
	@echo "  build          - Build all binaries"
	@echo "  build-server   - Build the server binary"
	@echo "  build-admin    - Build the admin CLI binary"
	@echo "  build-migrate  - Build the migration tool binary"
	@echo "  run            - Build and run the server"
	@echo "  test           - Run tests"
	@echo "  test-coverage  - Run tests with coverage report"
	@echo "  clean          - Clean build artifacts"
	@echo "  migrate-up     - Run database migrations"
	@echo "  migrate-down   - Rollback last migration"
	@echo "  migrate-create - Create a new migration"
	@echo "  docker-build   - Build Docker image"
	@echo "  docker-up      - Start Docker Compose services"
	@echo "  docker-down    - Stop Docker Compose services"
	@echo "  lint           - Run linter"
	@echo "  fmt            - Format code"
	@echo "  deps           - Download dependencies"
