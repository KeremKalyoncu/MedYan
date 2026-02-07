.PHONY: help build run test clean docker-up docker-down deps install-tools

help: ## Show this help message
	@echo "Media Extraction SaaS - Make Commands"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

deps: ## Download Go dependencies
	@echo "Downloading dependencies..."
	go mod download
	go mod tidy

install-tools: ## Install required external tools (yt-dlp, ffmpeg)
	@echo "Installing external tools..."
	@echo "Please install manually:"
	@echo "  - yt-dlp: pip install -U yt-dlp OR download from https://github.com/yt-dlp/yt-dlp/releases"
	@echo "  - ffmpeg: Download from https://ffmpeg.org/download.html"

build: ## Build API and Worker binaries
	@echo "Building binaries..."
	go build -o bin/api.exe cmd/api/main.go
	go build -o bin/worker.exe cmd/worker/main.go
	@echo "Binaries built: bin/api.exe, bin/worker.exe"

run-api: ## Run API server
	@echo "Starting API server..."
	go run cmd/api/main.go

run-worker: ## Run Worker
	@echo "Starting Worker..."
	go run cmd/worker/main.go

test: ## Run tests
	@echo "Running tests..."
	go test -v -race ./...

test-coverage: ## Run tests with coverage
	@echo "Running tests with coverage..."
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

test-unit: ## Run unit tests only
	@echo "Running unit tests..."
	go test -v -short ./...

test-bench: ## Run benchmarks
	@echo "Running benchmarks..."
	go test -bench=. -benchmem ./...

docker-up: ## Start Docker services (Redis, Asynq Monitor)
	@echo "Starting Docker services..."
	docker-compose up -d
	@echo "Services started:"
	@echo "  - Redis: localhost:6379"
	@echo "  - MinIO: http://localhost:9000 (admin:admin)"
	@echo "  - MinIO Console: http://localhost:9001"

docker-logs: ## Show Docker logs
	@echo "Docker logs..."
	docker-compose logs -f

docker-down: ## Stop Docker services
	@echo "Stopping Docker services..."
	docker-compose down

docker-build: ## Build Docker images
	@echo "Building Docker images..."
	docker build -t media-extraction-api:latest -f deployments/Dockerfile.api .
	docker build -t media-extraction-worker:latest -f deployments/Dockerfile.worker .

clean: ## Clean build artifacts
	@echo "Cleaning..."
	rm -rf bin/
	rm -f coverage.out coverage.html
	rm -f *.log
	go clean

fmt: ## Format Go code
	@echo "Formatting code..."
	go fmt ./...

lint: ## Run linter (requires golangci-lint)
	@echo "Running linter..."
	@command -v golangci-lint >/dev/null 2>&1 && golangci-lint run ./... || echo "⚠️  golangci-lint not installed"

vet: ## Run go vet
	@echo "Running go vet..."
	go vet ./...

dev: docker-up ## Start development environment
	@echo ""
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@echo " 🚀 Development Environment Ready!"
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@echo ""
	@echo " Services running:"
	@echo "   - Redis: localhost:6379"
	@echo "   - MinIO: http://localhost:9000"
	@echo "   - MinIO Console: http://localhost:9001"
	@echo ""
	@echo " Next steps (run in separate terminals):"
	@echo "   make run-api    # Start API server"
	@echo "   make run-worker # Start worker"
	@echo ""
	@echo " Frontend:"
	@echo "   ./START-WEB.bat # Start web server (Windows)"
	@echo "   bash START-WEB.sh # Start web server (Unix)"
	@echo ""

check-deps: ## Check if external tools are installed
	@echo "Checking dependencies..."
	@command -v yt-dlp >/dev/null 2>&1 && echo "✓ yt-dlp" || echo "✗ yt-dlp not found"
	@command -v ffmpeg >/dev/null 2>&1 && echo "✓ ffmpeg" || echo "✗ ffmpeg not found"
	@command -v redis-cli >/dev/null 2>&1 && echo "✓ redis-cli" || echo "✗ redis-cli not found"
	@command -v python >/dev/null 2>&1 && echo "✓ python" || echo "✗ python not found"
	@echo ""

setup: ## Setup development environment
	@echo "Setting up development environment..."
	cp .env.example .env
	go mod download
	go mod tidy
	@echo "✓ Setup complete. Update .env with your configuration."

all: clean deps test build ## Build everything and run tests
