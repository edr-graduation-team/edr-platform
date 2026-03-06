.PHONY: all build run test test-unit test-integration test-coverage proto clean lint help

# Variables
BINARY_NAME=connection-manager
PROTO_DIR=proto/v1
GO_FILES=$(shell find . -name '*.go' -not -path './vendor/*')

# Build
all: proto build

build:
	@echo "Building $(BINARY_NAME)..."
	go build -o bin/$(BINARY_NAME) ./cmd/server

run: build
	@echo "Running $(BINARY_NAME)..."
	./bin/$(BINARY_NAME)

# Protocol Buffers
proto:
	@echo "Generating protobuf files..."
	protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		$(PROTO_DIR)/edr.proto

proto-install:
	@echo "Installing protoc plugins..."
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Testing
test: test-unit

test-unit:
	@echo "Running unit tests..."
	go test -v -race -short ./...

test-integration:
	@echo "Running integration tests..."
	go test -v -race -run Integration ./...

test-coverage:
	@echo "Running tests with coverage..."
	go test -v -race -coverprofile=coverage.out -covermode=atomic ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

test-load:
	@echo "Running load tests..."
	go test -v -run Load ./tests/load/...

# Code Quality
lint:
	@echo "Running linter..."
	golangci-lint run ./...

fmt:
	@echo "Formatting code..."
	gofmt -s -w $(GO_FILES)

vet:
	@echo "Running go vet..."
	go vet ./...

# Database
migrate-up:
	@echo "Running migrations up..."
	migrate -path internal/database/migrations -database "$(DATABASE_URL)" up

migrate-down:
	@echo "Running migrations down..."
	migrate -path internal/database/migrations -database "$(DATABASE_URL)" down

migrate-create:
	@echo "Creating new migration: $(name)"
	migrate create -ext sql -dir internal/database/migrations -seq $(name)

# Certificates (for development)
certs-generate:
	@echo "Generating development certificates..."
	./scripts/generate_certs.sh

# Docker
docker-build:
	@echo "Building Docker image..."
	docker build -t $(BINARY_NAME):latest .

docker-run:
	@echo "Running in Docker..."
	docker-compose -f deployments/docker-compose.yml up -d

docker-stop:
	@echo "Stopping Docker containers..."
	docker-compose -f deployments/docker-compose.yml down

# Cleanup
clean:
	@echo "Cleaning..."
	rm -rf bin/
	rm -f coverage.out coverage.html
	rm -f $(PROTO_DIR)/*.pb.go

# Dependencies
deps:
	@echo "Downloading dependencies..."
	go mod download
	go mod tidy

# Help
help:
	@echo "Available targets:"
	@echo "  make build          - Build the binary"
	@echo "  make run            - Build and run the server"
	@echo "  make proto          - Generate protobuf files"
	@echo "  make test           - Run unit tests"
	@echo "  make test-coverage  - Run tests with coverage report"
	@echo "  make lint           - Run linter"
	@echo "  make migrate-up     - Run database migrations"
	@echo "  make clean          - Clean build artifacts"
	@echo "  make help           - Show this help"
