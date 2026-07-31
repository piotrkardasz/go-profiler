.PHONY: build test vet ui-build ui-dev clean help example-basic example-otel example-gorm-mysql example-gorm-postgres

# Default target
help: ## Show this help
	@echo "Go Profiler - Available targets:"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'

build: ui-build ## Build the Go package (including embedded UI)
	@echo "Copying UI dist to handler/ui_dist..."
	@rm -rf handler/ui_dist
	@cp -r ui/dist handler/ui_dist
	@echo "Building Go packages..."
	@go build ./...
	@echo "Done."

test: ## Run all Go tests
	@go test ./...

vet: ## Run go vet on all packages
	@go vet ./...

lint: vet ## Run linting (go vet)

ui-build: ## Build the Vue UI for production
	@echo "Building Vue UI..."
	@cd ui && npm install --silent && npm run build
	@echo "UI built at ui/dist/"

ui-dev: ## Start Vue UI dev server (with hot reload)
	@echo "Starting Vite dev server..."
	@echo "Set GO_PROFILER_UI_DEV=true on the Go server to proxy to this."
	@cd ui && npm run dev

clean: ## Clean build artifacts
	@rm -rf ui/dist ui/node_modules handler/ui_dist
	@rm -rf var/profiler
	@echo "Cleaned."

example-basic: build ## Run the basic example
	@echo "Starting basic example server..."
	@go run ./examples/basic/

example-otel: build ## Run the OpenTelemetry example
	@echo "Starting OTel example server..."
	@go run ./examples/otel/

example-gorm-mysql: build ## Run the GORM MySQL example
	@echo "Starting GORM MySQL example server..."
	@go run ./examples/gorm-mysql/

example-gorm-postgres: build ## Run the GORM PostgreSQL example
	@echo "Starting GORM PostgreSQL example server..."
	@go run ./examples/gorm-postgres/
