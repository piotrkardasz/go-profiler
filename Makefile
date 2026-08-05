.PHONY: build build-dev test vet ui-build ui-dist ui-dev clean help example-basic example-otel example-gorm-mysql example-gorm-postgres

# Default target
help: ## Show this help
	@echo "Go Profiler - Available targets:"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'

build: ui-dist ## Build with embedded UI (requires Node.js)
	@echo "Building Go packages with profiler_ui tag..."
	@go build -tags profiler_ui ./...
	@echo "Done."

build-dev: ## Build without embedded UI (no Node.js required)
	@go build ./...

test: ## Run all Go tests
	@go test ./...

test-ui: ## Run all Go tests with embedded UI
	@go test -tags profiler_ui ./...

vet: ## Run go vet on all packages
	@go vet ./...

lint: vet ## Run linting (go vet)

ui-build: ## Build the Vue UI for production
	@echo "Building Vue UI..."
	@cd ui && npm install --silent && npm run build
	@echo "UI built at ui/dist/"

ui-dist: ui-build ## Build UI and copy to handler/ui_dist
	@rm -rf handler/ui_dist
	@cp -r ui/dist handler/ui_dist
	@echo "handler/ui_dist/ updated"

ui-dev: ## Start Vue UI dev server (with hot reload)
	@echo "Starting Vite dev server..."
	@echo "Set GO_PROFILER_UI_DEV=true on the Go server to proxy to this."
	@cd ui && npm run dev

clean: ## Clean build artifacts
	@rm -rf ui/dist ui/node_modules
	@rm -rf var/profiler
	@echo "Cleaned."

example-basic: build ## Run the basic example
	@echo "Starting basic example server..."
	@cd examples/basic && PROFILER_LOGGER_BACKTRACE=1 go run -tags profiler_ui .

example-otel: build ## Run the OpenTelemetry example
	@echo "Starting OTel example server..."
	@go run -tags profiler_ui ./examples/otel/

example-gorm-mysql: build ## Run the GORM MySQL example
	@echo "Starting MySQL via Docker Compose..."
	@cd examples/gorm-mysql && docker compose up -d --wait mysql
	@sleep 2
	@echo "Starting GORM MySQL example server..."
	@cd examples/gorm-mysql && GORM_PROFILER_BACKTRACE=1 go run -tags profiler_ui .

example-gorm-postgres: build ## Run the GORM PostgreSQL example
	@echo "Starting PostgreSQL via Docker Compose..."
	@cd examples/gorm-postgres && docker compose up -d --wait postgres
	@echo "Starting GORM PostgreSQL example server..."
	@cd examples/gorm-postgres && GORM_PROFILER_BACKTRACE=1 go run -tags profiler_ui .
