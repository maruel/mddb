.PHONY: help build dev test coverage lint lint-go lint-frontend lint-fix git-hooks frontend-dev types upgrade

# Variables
DATA_DIR=./data
PORT?=8080
LOG_LEVEL?=info
FRONTEND_STAMP=frontend/node_modules/.stamp
ENV_FILE=$(DATA_DIR)/.env

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
	@echo "  make upgrade        - Upgrade Go and pnpm dependencies"
	@echo ""
	@echo "Environment variables:"
	@echo "  PORT=8080           - Server port (default: 8080)"
	@echo "  LOG_LEVEL=info      - Log level (debug|info|warn|error)"
	@echo ""
	@echo "Note: 'make dev' auto-creates data/.env from .env.example if missing"

# Install frontend dependencies (only when lockfile changes)
$(FRONTEND_STAMP): frontend/pnpm-lock.yaml
	@cd ./frontend && NPM_CONFIG_AUDIT=false NPM_CONFIG_FUND=false pnpm install --frozen-lockfile --silent
	@touch $@

# Build frontend and Go server
build: types
	@go generate ./...
	@go install ./backend/cmd/...

types: $(FRONTEND_STAMP)
	@cd ./backend && go tool tygo generate
	@cd ./frontend && NPM_CONFIG_AUDIT=false NPM_CONFIG_FUND=false pnpm exec prettier --log-level silent --write src/types.gen.ts

# Create data/.env from example if missing (skips interactive onboarding)
# Order-only prerequisite (|) ensures we don't overwrite existing .env
$(ENV_FILE): | .env.example
	@mkdir -p $(DATA_DIR)
	@cp .env.example $@
	@echo "Created $@ from .env.example"

dev: build $(ENV_FILE)
	@mddb -port $(PORT) -data-dir $(DATA_DIR) -log-level $(LOG_LEVEL)

test: $(FRONTEND_STAMP)
	@go test -cover ./...
	@cd ./frontend && NPM_CONFIG_AUDIT=false NPM_CONFIG_FUND=false pnpm test

coverage: $(FRONTEND_STAMP)
	@go test -coverprofile=coverage.out ./...
	@cd ./frontend && NPM_CONFIG_AUDIT=false NPM_CONFIG_FUND=false pnpm coverage

lint: lint-go lint-frontend

lint-go:
	@which golangci-lint > /dev/null || go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
	@golangci-lint run ./...

lint-frontend: $(FRONTEND_STAMP)
	@cd ./frontend && NPM_CONFIG_AUDIT=false NPM_CONFIG_FUND=false pnpm lint

lint-fix: $(FRONTEND_STAMP)
	@cd ./backend && golangci-lint run ./... --fix || true
	@cd ./frontend && NPM_CONFIG_AUDIT=false NPM_CONFIG_FUND=false pnpm lint:fix

git-hooks:
	@echo "Installing git pre-commit hooks..."
	@mkdir -p .git/hooks
	@cp ./scripts/pre-commit .git/hooks/pre-commit
	@chmod +x .git/hooks/pre-commit
	@git config merge.ours.driver true
	@echo "✓ Git hooks installed"

frontend-dev: $(FRONTEND_STAMP)
	@cd ./frontend && NPM_CONFIG_AUDIT=false NPM_CONFIG_FUND=false pnpm dev

upgrade:
	@go get -u ./... && go mod tidy
	@cd ./frontend && pnpm update --latest
