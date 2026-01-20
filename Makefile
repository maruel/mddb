.PHONY: help build dev test lint lint-go lint-frontend lint-fix git-hooks frontend-dev types

# Variables
DATA_DIR=./data
PORT?=8080
LOG_LEVEL?=info

help:
	@echo "mddb - Markdown Document & Database System"
	@echo ""
	@echo "Available targets:"
	@echo "  make build          - Build Go server (auto-generates frontend)"
	@echo "  make dev            - Run the server in development mode"
	@echo "  make test           - Run backend tests"
	@echo "  make types          - Generate TypeScript types from Go structs"
	@echo "  make lint           - Run linters (Go + frontend)"
	@echo "  make lint-fix       - Fix all linting issues automatically"
	@echo "  make git-hooks      - Install git pre-commit hooks"
	@echo "  make frontend-dev   - Run frontend dev server (http://localhost:5173)"
	@echo ""
	@echo "Environment variables:"
	@echo "  PORT=8080           - Server port (default: 8080)"
	@echo "  LOG_LEVEL=info      - Log level (debug|info|warn|error)"

# Build frontend and Go server
build: types
	cd backend && go generate ./...
	cd backend && go install ./cmd/...

types:
	cd backend && go tool tygo generate
	@mv frontend/src/types.gen.ts frontend/src/types.ts
	cd frontend && pnpm exec prettier --write src/types.ts

dev: build
	mddb -port $(PORT) -data-dir $(DATA_DIR) -log-level $(LOG_LEVEL)

test:
	cd backend && go test -cover ./...

lint: lint-go lint-frontend

lint-go:
	@which golangci-lint > /dev/null || (echo "Installing golangci-lint..." && go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest)
	cd backend && golangci-lint run ./...

lint-frontend:
	cd frontend && pnpm lint

lint-fix:
	cd backend && golangci-lint run ./... --fix || true
	cd frontend && pnpm lint:fix

git-hooks:
	@echo "Installing git pre-commit hooks..."
	@mkdir -p .git/hooks
	@cp ./scripts/pre-commit .git/hooks/pre-commit
	@chmod +x .git/hooks/pre-commit
	@git config merge.ours.driver true
	@echo "✓ Git hooks installed"

frontend-dev:
	cd frontend && pnpm install && pnpm dev
