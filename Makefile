.DEFAULT_GOAL := help

# Build settings
VERSION := $(shell git describe --tags 2>/dev/null || echo "dev")
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

# Other variables
DOCKER_COMPOSE_LOCAL_FILE := "./deployments/local/docker-compose.yml"

# Build flags
LDFLAGS := -w -s \
	-X 'repos.defensepoint.com/iam/oss-pmt/kmt/internal/version.Version=$(VERSION)' \
	-X 'repos.defensepoint.com/iam/oss-pmt/kmt/internal/version.GitCommit=$(GIT_COMMIT)' \
	-X 'repos.defensepoint.com/iam/oss-pmt/kmt/internal/version.BuildDate=$(BUILD_DATE)'

.PHONY: help
help: ## Show this help
	@echo "Monitoring Dashboard - Available Commands:"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

# -----------------------------------------------------------------
# BUILD TARGETS
# -----------------------------------------------------------------

.PHONY: build-server
build-server: ## Build the API server binary
	@echo "Building server..."
	@mkdir -p bin
	@go build -ldflags "$(LDFLAGS)" -o bin/server ./cmd/server

.PHONY: build-web
build-web: build-frontend ## Build the web frontend server binary
	@echo "Building web server..."
	@mkdir -p bin
	@go build -ldflags "$(LDFLAGS)" -o bin/web ./cmd/web

.PHONY: build-all
build-all: build-server build-web ## Build all binaries

# -----------------------------------------------------------------
# FRONTEND TARGETS
# -----------------------------------------------------------------

.PHONY: install-frontend
install-frontend: ## Install frontend dependencies
	@echo "Installing frontend dependencies..."
	@cd web && npm install

.PHONY: build-frontend
build-frontend: ## Build the web frontend
	@echo "Building frontend..."
	@cd web && npm run build

.PHONY: dev-frontend
dev-frontend: ## Run the web frontend in dev mode
	@cd web && npm run dev

.PHONY: clean-frontend
clean-frontend: ## Clean frontend build artifacts
	@rm -rf web/dist/
	@rm -rf web/node_modules/

# -----------------------------------------------------------------
# RUN TARGETS
# -----------------------------------------------------------------

.PHONY: run
run: ## Run the API server locally
	@go run ./cmd/server

.PHONY: run-web-server
run-web-server: ## Run the web frontend server locally (requires built frontend)
	@./bin/web

# -----------------------------------------------------------------
# QUALITY CHECKS
# -----------------------------------------------------------------

.PHONY: test-backend
test-backend: ## Run Go backend tests (skips integration tests)
	@echo "Running backend tests..."
	@go test -short -v -race -coverprofile=coverage.out ./...

.PHONY: test-frontend
test-frontend: ## Run frontend tests
	@echo "Running frontend tests..."
	@cd web && npm test -- --run

.PHONY: test-all
test-all: test-backend test-frontend ## Run all tests (backend and frontend)
	@echo "All tests passed"

.PHONY: fmt
fmt: ## Format Go code
	@echo "Formatting code..."
	@go fmt ./...

.PHONY: vet
vet: ## Run go vet
	@echo "Running go vet..."
	@go vet ./...

.PHONY: lint
lint: ## Run golangci-lint
	@echo "Running golangci-lint..."
	@golangci-lint run --timeout 5m

.PHONY: tidy
tidy: ## Tidy go modules
	@echo "Tidying go.mod..."
	@go mod tidy

.PHONY: deps
deps: ## Download dependencies
	@echo "Downloading dependencies..."
	@go mod download

.PHONY: check
check: tidy fmt vet lint test-all ## Run all quality checks (tidy, fmt, vet, test)
	@echo "✓ All quality checks passed"

# -----------------------------------------------------------------
# DOCKER TARGETS
# -----------------------------------------------------------------

.PHONY: docker-build-server
docker-build-server: ## Build server Docker image
	@echo "Building server Docker image..."
	@docker build \
		--build-arg SERVICE=server \
		-t kmt-server:$(VERSION) \
		-t kmt-server:latest \
		-f build/Dockerfile .
	@echo "Server image built: kmt-server:$(VERSION)"

.PHONY: docker-build-web
docker-build-web: ## Build web Docker image
	@echo "Building web Docker image..."
	@docker build \
		--build-arg SERVICE=web \
		-t kmt-web:$(VERSION) \
		-t kmt-web:latest \
		-f build/Dockerfile .
	@echo "Web image built: kmt-web:$(VERSION)"

.PHONY: docker-build
docker-build: docker-build-server docker-build-web ## Build all Docker images

.PHONY: docker-up
docker-up: ## Start services with docker-compose
	@docker compose -f $(DOCKER_COMPOSE_LOCAL_FILE) up -d

.PHONY: docker-down
docker-down: ## Stop all services
	@docker compose -f $(DOCKER_COMPOSE_LOCAL_FILE) down

.PHONY: docker-restart
docker-restart: ## Restart all services
	@docker compose -f $(DOCKER_COMPOSE_LOCAL_FILE) restart

.PHONY: docker-logs
docker-logs: ## Show docker-compose logs
	@docker compose -f $(DOCKER_COMPOSE_LOCAL_FILE) logs -f

.PHONY: docker-clean
docker-clean: ## Stop services and remove volumes
	@docker compose -f $(DOCKER_COMPOSE_LOCAL_FILE) down -v

.PHONY: docker-rebuild
docker-rebuild: docker-build docker-down docker-up ## Rebuild images and restart services

# -----------------------------------------------------------------
# SWAGGER TARGETS
# -----------------------------------------------------------------

.PHONY: swagger
swagger: ## Generate Swagger documentation
	@echo "Generating Swagger documentation..."
	@swag init -g cmd/server/main.go -o docs/swagger --parseDependency --parseInternal
	@echo "Swagger docs generated at docs/swagger/"

.PHONY: swagger-fmt
swagger-fmt: ## Format Swagger annotations
	@echo "Formatting Swagger annotations..."
	@swag fmt

# -----------------------------------------------------------------
# CLEANUP TARGETS
# -----------------------------------------------------------------

.PHONY: clean
clean: ## Clean build artifacts
	@echo "Cleaning build artifacts..."
	@rm -rf bin/
	@rm -f coverage.out

.PHONY: clean-all
clean-all: clean clean-frontend ## Clean all artifacts (including frontend)
