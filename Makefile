.PHONY: help build build-all dev test lint lint-fix git-hooks frontend-dev frontend-build

# Variables
DATA_DIR=./data
PORT?=8080
LOG_LEVEL?=info

help:
	@echo "mddb - Markdown Document & Database System"
	@echo ""
	@echo "Available targets:"
	@echo "  make build          - Build Go server binary (requires frontend built)"
	@echo "  make build-all      - Build frontend + Go server with embedded assets + tests"
	@echo "  make dev            - Run the server in development mode"
	@echo "  make test           - Run backend tests"
	@echo "  make lint           - Run linters (Go + frontend)"
	@echo "  make lint-fix       - Fix all linting issues automatically"
	@echo "  make git-hooks      - Install git pre-commit hooks"
	@echo "  make frontend-dev   - Run frontend dev server (http://localhost:5173)"
	@echo "  make frontend-build - Build frontend for production"
	@echo ""
	@echo "Environment variables:"
	@echo "  PORT=8080           - Server port (default: 8080)"
	@echo "  LOG_LEVEL=info      - Log level (debug|info|warn|error)"

# Build Go server (assumes frontend is already built)
build:
	go install ./cmd/mddb

dev: build
	mddb -port $(PORT) -data-dir $(DATA_DIR) -log-level $(LOG_LEVEL)

test:
	go test ./...

lint:
	@which golangci-lint > /dev/null || (echo "Installing golangci-lint..." && go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)
	golangci-lint run ./...
	cd frontend && pnpm lint

lint-fix:
	golangci-lint run ./... --fix || true
	cd frontend && pnpm lint:fix

git-hooks:
	@echo "Installing git pre-commit hooks..."
	@mkdir -p .git/hooks
	@cp ./scripts/pre-commit .git/hooks/pre-commit
	@chmod +x .git/hooks/pre-commit
	@echo "✓ Git hooks installed"

frontend-dev:
	cd frontend && pnpm install && pnpm dev

frontend-build:
	cd frontend && pnpm install && pnpm build

# Build complete system with embedded frontend (frontend + Go server with embedded assets)
build-all: frontend-build test build
	@echo "✓ Build complete"
