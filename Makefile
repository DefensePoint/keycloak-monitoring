.DEFAULT_GOAL := help

# Build settings
VERSION := $(shell git describe --tags 2>/dev/null || echo "dev")
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
SWAG_VERSION ?= v1.16.6
SWAG = go run github.com/swaggo/swag/cmd/swag@$(SWAG_VERSION)
BUILD_DATE := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

# Other variables
DOCKER_COMPOSE_LOCAL_FILE := "./deployments/local/docker-compose.yml"
DOCKER_DEMO_PROJECT ?= kmt-demo
DOCKER_DEMO_COMPOSE = docker compose --env-file .env -p $(DOCKER_DEMO_PROJECT) -f $(DOCKER_COMPOSE_LOCAL_FILE) -f deployments/local/docker-compose.bundled.yml --profile bundled-kc
# A restarted setup container keeps the logs of its earlier runs, so a failure prints only this run's.
DOCKER_DEMO_UP = since=$$(date -u +%Y-%m-%dT%H:%M:%SZ); \
	$(DOCKER_DEMO_COMPOSE) up -d $(1) --wait --wait-timeout 600 || \
	{ status=$$?; $(DOCKER_DEMO_COMPOSE) logs --no-log-prefix --since "$$since" keycloak-setup; exit $$status; }

# Build flags
LDFLAGS := -w -s \
	-X 'github.com/DefensePoint/keycloak-monitoring/internal/version.Version=$(VERSION)' \
	-X 'github.com/DefensePoint/keycloak-monitoring/internal/version.GitCommit=$(GIT_COMMIT)' \
	-X 'github.com/DefensePoint/keycloak-monitoring/internal/version.BuildDate=$(BUILD_DATE)'

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

.PHONY: build-mcp-server
build-mcp-server: ## Build the MCP server binary
	@echo "Building MCP server..."
	@mkdir -p bin
	@go build -ldflags "$(LDFLAGS)" -o bin/mcp-server ./cmd/mcp-server

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

.PHONY: docker-build-mcp-server
docker-build-mcp-server: ## Build MCP server Docker image
	@echo "Building MCP server Docker image..."
	@docker build \
		--build-arg SERVICE=mcp-server \
		-t kmt-mcp-server:$(VERSION) \
		-t kmt-mcp-server:latest \
		-f build/Dockerfile .
	@echo "MCP server image built: kmt-mcp-server:$(VERSION)"

.PHONY: docker-build
docker-build: docker-build-server docker-build-web docker-build-mcp-server ## Build all Docker images

.PHONY: docker-up
docker-up: docker-env-check docker-config-check ## Start services with docker-compose
	@docker compose -f $(DOCKER_COMPOSE_LOCAL_FILE) up -d

# The local compose file loads .env into the containers and mounts
# config.yaml; without them Compose fails with a bare "env file not found",
# or Docker mounts an empty directory in place of config.yaml.
.PHONY: docker-env-check
docker-env-check:
	@test -f .env || { echo "Missing .env. Create it from the example and set MONITORING_AUTH_SIMPLE_DEFAULT_PASS:"; echo "  cp .env.example .env"; exit 1; }

.PHONY: docker-config-check
docker-config-check:
	@test -f config.yaml || { echo "Missing config.yaml. Create it from the example and add your Keycloak tenants:"; echo "  cp config.yaml.example config.yaml"; exit 1; }

.PHONY: docker-down
.PHONY: docker-demo-up docker-demo-down docker-demo-restart docker-demo-clean docker-demo-logs
docker-demo-up: docker-env-check ## Build and start the self-contained two-Keycloak demo
	@$(call DOCKER_DEMO_UP,--build)

docker-demo-down: docker-env-check ## Stop the bundled demo, retaining its data
	@$(DOCKER_DEMO_COMPOSE) down

docker-demo-restart: docker-env-check ## Restart the bundled demo with retained data
	@$(DOCKER_DEMO_COMPOSE) stop
	@$(call DOCKER_DEMO_UP,--no-build)

docker-demo-clean: docker-env-check ## Stop the bundled demo and remove only its volumes
	@$(DOCKER_DEMO_COMPOSE) down -v

docker-demo-logs: docker-env-check ## Follow bundled demo logs
	@$(DOCKER_DEMO_COMPOSE) logs -f

docker-down: docker-env-check ## Stop all services
	@docker compose -f $(DOCKER_COMPOSE_LOCAL_FILE) down

.PHONY: docker-restart
docker-restart: docker-env-check docker-config-check ## Restart all services
	@docker compose -f $(DOCKER_COMPOSE_LOCAL_FILE) restart

.PHONY: docker-logs
docker-logs: docker-env-check ## Show docker-compose logs
	@docker compose -f $(DOCKER_COMPOSE_LOCAL_FILE) logs -f

.PHONY: docker-clean
docker-clean: docker-env-check ## Stop services and remove volumes
	@docker compose -f $(DOCKER_COMPOSE_LOCAL_FILE) down -v

.PHONY: docker-rebuild
docker-rebuild: docker-build docker-down docker-up ## Rebuild images and restart services

# -----------------------------------------------------------------
# SWAGGER TARGETS
# -----------------------------------------------------------------

# Without a local Go toolchain, run swag in a Go container instead. The named
# volume caches the module downloads between runs.
ifeq ($(shell command -v go 2>/dev/null),)
SWAG = docker run --rm -v $(CURDIR):/app -v kmt-swag-cache:/go -w /app \
	golang:1.27.2 go run github.com/swaggo/swag/cmd/swag@$(SWAG_VERSION)
endif

# --parseDependency is deliberately absent: it makes swag name every schema after
# its full import path instead of alerts.Statistics, and produces the same
# document otherwise. Add it back only if a handler exposes a type swag cannot
# resolve, and expect every schema name to change if you do.
.PHONY: swagger
swagger: ## Generate Swagger documentation
	@echo "Generating Swagger documentation with swag $(SWAG_VERSION)..."
	@$(SWAG) init -g cmd/server/main.go -o docs/swagger --parseInternal
	@echo "Swagger docs generated at docs/swagger/"

.PHONY: swagger-check
swagger-check: ## Fail if docs/swagger differs from what make swagger generates
	@$(MAKE) --no-print-directory swagger || { echo "make swagger failed: fix the annotation it reports above, then run make swagger and commit docs/swagger."; exit 1; }
	@if [ -n "$$(git status --porcelain -- docs/swagger)" ]; then \
		git status --short -- docs/swagger; git diff --stat -- docs/swagger; \
		echo "docs/swagger is out of date: run make swagger (swag $(SWAG_VERSION)) and commit docs/swagger."; \
		exit 1; \
	fi
	@echo "docs/swagger is up to date"

.PHONY: swagger-fmt
swagger-fmt: ## Format Swagger annotations
	@echo "Formatting Swagger annotations..."
	@$(SWAG) fmt

# -----------------------------------------------------------------
# SECURITY SCAN TARGETS
# -----------------------------------------------------------------
# Mirrors the CI security stage locally. Every sub-target is callable on its
# own, and every scan writes a JSON report to reports/security/ so results
# survive between runs. Findings are informational: none of these targets
# fails the make invocation on findings, matching the warn-only CI rollout.

SECURITY_REPORTS := reports/security

.PHONY: sec-preflight
sec-preflight: ## Check that security scan tools are on PATH
	@missing=""; \
	for tool in hadolint trivy gitleaks semgrep docker; do \
		if ! command -v $$tool >/dev/null 2>&1; then \
			missing="$$missing $$tool"; \
		fi; \
	done; \
	if [ -n "$$missing" ]; then \
		echo "Missing tools:$$missing"; \
		echo ""; \
		echo "Install hints (macOS):"; \
		echo "  brew install hadolint trivy gitleaks semgrep"; \
		echo "  Docker Desktop or colima for 'docker'"; \
		exit 1; \
	fi; \
	echo "All security tools present."
	@mkdir -p $(SECURITY_REPORTS)

.PHONY: sec-hadolint
sec-hadolint: sec-preflight ## Lint build/Dockerfile with hadolint
	@echo "Running hadolint on build/Dockerfile..."
	@hadolint --format json build/Dockerfile | tee $(SECURITY_REPORTS)/hadolint.json || true

.PHONY: sec-trivy-fs
sec-trivy-fs: sec-preflight ## Trivy filesystem scan for Go and npm CVEs
	@echo "Running Trivy filesystem scan..."
	@trivy fs \
		--scanners vuln \
		--severity CRITICAL,HIGH \
		--ignore-unfixed \
		--format json \
		--output $(SECURITY_REPORTS)/trivy-fs.json \
		.
	@trivy fs \
		--scanners vuln \
		--severity CRITICAL,HIGH \
		--ignore-unfixed \
		.

.PHONY: sec-gitleaks
sec-gitleaks: sec-preflight ## Gitleaks scan of full git history
	@echo "Running gitleaks on full git history..."
	@gitleaks detect \
		--source . \
		--redact \
		--report-format json \
		--report-path $(SECURITY_REPORTS)/gitleaks.json \
		--exit-code 0

.PHONY: sec-semgrep
sec-semgrep: sec-preflight ## Semgrep SAST scan (auto ruleset)
	@echo "Running semgrep with the auto ruleset..."
	@semgrep scan \
		--config auto \
		--severity ERROR \
		--exclude node_modules \
		--exclude vendor \
		--exclude .terraform \
		--exclude .git \
		--json \
		--output $(SECURITY_REPORTS)/semgrep.json \
		--quiet || true
	@semgrep scan \
		--config auto \
		--severity ERROR \
		--exclude node_modules \
		--exclude vendor \
		--exclude .terraform \
		--exclude .git || true

.PHONY: sec-build-images
sec-build-images: sec-preflight ## Build every container image with tag :scan
	@echo "Building kmt-server:scan..."
	@docker build --build-arg SERVICE=server -t kmt-server:scan -f build/Dockerfile .
	@echo "Building kmt-web:scan..."
	@docker build --build-arg SERVICE=web -t kmt-web:scan -f build/Dockerfile .
	@echo "Building kmt-mcp-server:scan..."
	@docker build --build-arg SERVICE=mcp-server -t kmt-mcp-server:scan -f build/Dockerfile .

.PHONY: sec-trivy-images
sec-trivy-images: sec-preflight ## Trivy image scan for every service image
	@for svc in server web mcp-server; do \
		echo "Scanning kmt-$$svc:scan..."; \
		trivy image \
			--severity CRITICAL,HIGH \
			--ignore-unfixed \
			--format json \
			--output $(SECURITY_REPORTS)/trivy-image-$$svc.json \
			kmt-$$svc:scan; \
		trivy image \
			--severity CRITICAL,HIGH \
			--ignore-unfixed \
			kmt-$$svc:scan; \
	done

.PHONY: sec-container
sec-container: sec-build-images sec-trivy-images ## Build and scan both container images

.PHONY: security-scan
security-scan: sec-hadolint sec-trivy-fs sec-gitleaks sec-semgrep sec-container ## Run every security scan and write reports to reports/security/
	@echo ""
	@echo "Security scan complete. Reports in $(SECURITY_REPORTS)/"

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
