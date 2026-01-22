.PHONY: help build dev test coverage lint lint-go lint-frontend lint-fix git-hooks frontend-dev types upgrade

# Variables
DATA_DIR=./data
PORT?=8080
LOG_LEVEL?=info
FRONTEND_STAMP=frontend/node_modules/.stamp

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

# Install frontend dependencies (only when lockfile changes)
$(FRONTEND_STAMP): frontend/pnpm-lock.yaml
	@cd ./frontend && NPM_CONFIG_AUDIT=false NPM_CONFIG_FUND=false pnpm install --frozen-lockfile --silent
	@touch $@

# Build frontend and Go server
build: types
	@cd ./backend && go generate ./...
	@cd ./backend && go install ./cmd/...

types: $(FRONTEND_STAMP)
	@cd ./backend && go tool tygo generate
	@cd ./frontend && NPM_CONFIG_AUDIT=false NPM_CONFIG_FUND=false pnpm exec prettier --log-level silent --write src/types.gen.ts

dev: build
	@mddb -port $(PORT) -data-dir $(DATA_DIR) -log-level $(LOG_LEVEL)

test: $(FRONTEND_STAMP)
	@cd ./backend && go test -cover ./...
	@cd ./frontend && NPM_CONFIG_AUDIT=false NPM_CONFIG_FUND=false pnpm test

coverage: $(FRONTEND_STAMP)
	@cd ./backend && go test -coverprofile=coverage.out ./...
	@cd ./frontend && NPM_CONFIG_AUDIT=false NPM_CONFIG_FUND=false pnpm coverage

lint: lint-go lint-frontend

lint-go:
	@which golangci-lint > /dev/null || go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
	@cd ./backend && golangci-lint run ./...

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
	@cd ./backend && go get -u ./... && go mod tidy
	@cd ./frontend && pnpm update --latest
