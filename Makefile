.PHONY: help build dev test e2e e2e-slow coverage lint lint-go lint-frontend lint-binaries lint-fix git-hooks frontend-dev types upgrade docs

# Variables
DATA_DIR?=./data
PORT?=8080
LOG_LEVEL?=info
FRONTEND_STAMP=node_modules/.stamp
ENV_FILE=$(DATA_DIR)/.env

help:
	@echo "mddb - Markdown Document & Database System"
	@echo ""
	@echo "Available targets:"
	@echo "  make build          - Build Go server (auto-generates frontend)"
	@echo "  make dev            - Run the server in development mode"
	@echo "  make test           - Run unit tests"
	@echo "  make e2e            - Run end-to-end browser tests (fast, no rate limits)"
	@echo "  make e2e-slow       - Run e2e tests with normal rate limits (sequential)"
	@echo "  make types          - Generate TypeScript types from Go structs"
	@echo "  make docs           - Update AGENTS.md file index"
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
$(FRONTEND_STAMP): pnpm-lock.yaml
	@NPM_CONFIG_AUDIT=false NPM_CONFIG_FUND=false pnpm install --frozen-lockfile --silent
	@touch $@

# Build frontend and Go server
build: types docs
	@go generate ./...
	@go install ./backend/cmd/...

types: $(FRONTEND_STAMP)
	@cd ./backend && go tool tygo generate
	@NPM_CONFIG_AUDIT=false NPM_CONFIG_FUND=false pnpm exec prettier --log-level silent --write sdk/types.gen.ts

docs:
	@./scripts/update_agents_file_index.py

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
	@NPM_CONFIG_AUDIT=false NPM_CONFIG_FUND=false pnpm test

e2e: build
	@TEST_OAUTH=1 TEST_FAST_RATE_LIMIT=1 NPM_CONFIG_AUDIT=false NPM_CONFIG_FUND=false pnpm test:e2e; \
	e2e_exit=$$?; \
	cp -f ./data-e2e/server.log playwright-report/server.log 2>/dev/null || true; \
	if [ $$e2e_exit -ne 0 ]; then \
	  echo ""; echo "=== Server Log ==="; cat ./data-e2e/server.log 2>/dev/null || true; \
	  exit $$e2e_exit; \
	fi
	@./scripts/verify_e2e_data.py
	@node e2e/inject-tag-colors.cjs

e2e-slow: build
	@echo "Running e2e tests with normal rate limits (single worker)..."
	@TEST_OAUTH=1 TEST_FAST_RATE_LIMIT=0 NPM_CONFIG_AUDIT=false NPM_CONFIG_FUND=false pnpm exec playwright test --workers=1; \
	e2e_exit=$$?; \
	cp -f ./data-e2e/server.log playwright-report/server.log 2>/dev/null || true; \
	if [ $$e2e_exit -ne 0 ]; then \
	  echo ""; echo "=== Server Log ==="; cat ./data-e2e/server.log 2>/dev/null || true; \
	  exit $$e2e_exit; \
	fi
	@./scripts/verify_e2e_data.py
	@node e2e/inject-tag-colors.cjs

coverage: $(FRONTEND_STAMP)
	@go test -coverprofile=coverage.out ./...
	@NPM_CONFIG_AUDIT=false NPM_CONFIG_FUND=false pnpm coverage

lint: lint-go lint-frontend lint-python lint-binaries

lint-go:
	@which golangci-lint > /dev/null || go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
	@golangci-lint run ./...

lint-frontend: $(FRONTEND_STAMP)
	@NPM_CONFIG_AUDIT=false NPM_CONFIG_FUND=false pnpm lint

lint-python:
	@ruff check scripts/

lint-binaries:
	@binaries=$$(git ls-files -z | xargs -0 -r file --mime-type | grep -E 'application/(x-executable|x-mach-binary|x-dosexec|x-pie-executable|x-sharedlib)' | cut -d: -f1); \
	if [ -n "$$binaries" ]; then \
		echo "Error: Binary executables found in repository:"; \
		echo "$$binaries"; \
		exit 1; \
	fi

lint-fix: $(FRONTEND_STAMP)
	@cd ./backend && golangci-lint run ./... --fix || true
	@NPM_CONFIG_AUDIT=false NPM_CONFIG_FUND=false pnpm lint:fix
	@ruff check scripts/ --fix
	@ruff format scripts/

format-python:
	@ruff format scripts/
	@ruff check scripts/ --fix

git-hooks:
	@echo "Installing git pre-commit hooks..."
	@mkdir -p .git/hooks
	@cp ./scripts/pre-commit .git/hooks/pre-commit
	@chmod +x .git/hooks/pre-commit
	@git config merge.ours.driver true
	@echo "✓ Git hooks installed"

frontend-dev: $(FRONTEND_STAMP)
	@NPM_CONFIG_AUDIT=false NPM_CONFIG_FUND=false pnpm dev

upgrade:
	@go get -u ./... && go mod tidy
	@pnpm update --latest
