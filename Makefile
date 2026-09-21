# Build, verify, test, and development commands.

.DEFAULT_GOAL := help
.PHONY: help build dev coverage fix git-hooks frontend-dev test test-e2e test-e2e-slow types verify tools custom-gcl benchmark

# Tool versions. The tools target installs a tool that is missing or at another version, so
# these are the only places the versions are written down.
GOLANGCI_LINT_VERSION=v2.13.2
SHFMT_VERSION=v3.14.1
RUFF_VERSION=0.16.8

# The tools target installs into the Go and uv tool directories. Prepend them so a recipe
# that just installed a tool can run it, whatever the caller's PATH holds.
GO_BIN := $(if $(shell command -v go 2>/dev/null),$(shell go env GOPATH 2>/dev/null)/bin)
UV_BIN := $(if $(shell command -v uv 2>/dev/null),$(shell uv tool dir --bin 2>/dev/null))
export PATH := $(if $(GO_BIN),$(GO_BIN):)$(if $(UV_BIN),$(UV_BIN):)$(PATH)

tools:
	@command -v golangci-lint > /dev/null 2>&1 && golangci-lint --version 2>/dev/null | grep -Fqw "$(GOLANGCI_LINT_VERSION:v%=%)" || go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	@command -v shfmt > /dev/null 2>&1 && shfmt --version 2>/dev/null | grep -Fqw "$(SHFMT_VERSION)" || go install mvdan.cc/sh/v3/cmd/shfmt@$(SHFMT_VERSION)
	@command -v uv > /dev/null 2>&1 || { echo 'uv is required to install the Python tools; see https://docs.astral.sh/uv/' >&2; exit 1; }
	@ruff --version 2>/dev/null | grep -Fqw "$(RUFF_VERSION)" || uv tool install --force --quiet ruff==$(RUFF_VERSION)

# Variables
DATA_DIR?=./data
HTTP?=:8080
LOG_LEVEL?=info
FRONTEND_STAMP=node_modules/.stamp
ENV_FILE=$(DATA_DIR)/.env

# Static checks for verify, grouped into independent lanes run concurrently by
# scripts/run-concurrently.sh. Each is read-only; fix applies their autofixes.
#
# The verify recipe passes these single-quoted through two shell layers, so a
# lane variable must not contain a single quote; use double quotes inside.
#
# The gofmt and goimports formatters are checked by custom-gcl run itself
# (formatters section of .golangci.yml) with its warm analysis cache; a separate
# `golangci-lint fmt --diff` pass would re-typecheck the whole tree without that
# cache. Caveat: when another linter fails on the same file, run reports the
# lint error only, so a formatting problem there surfaces on the next verify
# after the lint fix. fix applies the formatters through `golangci-lint fmt`.
# methodfilecheck (see .golangci.yml) is a golangci-lint module plugin, so Go
# linting must run through the custom binary built from the published plugin
# module; the plain binary is fine for `fmt`.
VERIFY_GO = ./custom-gcl run --show-stats=false ./...
VERIFY_JS = pnpm --silent format:check && pnpm --silent lint:style && node scripts/lint_frontend_styles.mjs
VERIFY_TS = pnpm --silent typecheck
VERIFY_ESLINT = pnpm --silent exec eslint --cache --cache-location node_modules/.cache/eslint/ --cache-strategy content frontend/src sdk e2e playwright.config.ts
VERIFY_PY = ruff format --check --quiet . && ruff check --quiet .
VERIFY_SH = files=$$(git ls-files "*.sh" "scripts/hooks/*"); [ -z "$$files" ] || { out=$$(shfmt -l $$files); [ -z "$$out" ] || { echo "Shell files need shfmt:" >&2; echo "$$out" >&2; exit 1; }; }
VERIFY_MISC = python3 scripts/lint_binaries.py && python3 scripts/update_agents_file_index.py --check

# The custom-gcl binary is not byte-reproducible (golangci-lint custom builds
# in a random temp directory and stamps VCS metadata), so staleness is tracked
# by hashing the build inputs instead of comparing mtimes: the plugin config
# plus the pinned golangci-lint version and the Go toolchain. A branch switch
# that recreates .custom-gcl.yml with a fresh timestamp must not trigger a
# rebuild; a version or config change must.
.PHONY: custom-gcl
custom-gcl:
	@want=$$({ sha256sum .custom-gcl.yml | cut -d" " -f1; echo "$(GOLANGCI_LINT_VERSION)"; go env GOVERSION; } | sha256sum | cut -d" " -f1); \
	if [ -x custom-gcl ] && [ "$$want" = "$$(cat .custom-gcl.sha 2>/dev/null)" ]; then exit 0; fi; \
	echo 'Building custom-gcl with the methodfilecheck plugin (one-off; runs when the config, golangci-lint version, or Go toolchain changes)...'; \
	golangci-lint custom --version $(GOLANGCI_LINT_VERSION) && echo "$$want" > .custom-gcl.sha

help:
	@echo 'mddb - Markdown Document & Table System'
	@echo ''
	@echo 'Available targets:'
	@printf '  %-18s - %s\n' 'make fix' 'Apply every autofix, then refresh the file index'
	@printf '  %-18s - %s\n' 'make verify' 'Fast static gate: lint, formatting, generated docs (pre-push gate)'
	@printf '  %-18s - %s\n' 'make test' 'Run unit tests (Go, frontend)'
	@printf '  %-18s - %s\n' 'make benchmark' 'Run frontend micro-benchmarks (tinybench)'
	@printf '  %-18s - %s\n' 'make test-e2e' 'Run Playwright e2e tests (slow, fast rate limits, parallel)'
	@printf '  %-18s - %s\n' 'make test-e2e-slow' 'Run e2e tests with normal rate limits (sequential)'
	@printf '  %-18s - %s\n' 'make build' 'Build the Go server (generates types and frontend)'
	@printf '  %-18s - %s\n' 'make dev' 'Run the server in development mode'
	@printf '  %-18s - %s\n' 'make frontend-dev' 'Run frontend dev server (http://localhost:5173)'
	@printf '  %-18s - %s\n' 'make coverage' 'Run tests with coverage'
	@printf '  %-18s - %s\n' 'make types' 'Generate TypeScript types from Go structs'
	@printf '  %-18s - %s\n' 'make git-hooks' 'Install git pre-commit hooks'
	@echo ''
	@echo 'Environment variables:'
	@echo '  HTTP=:8080          - Server address (default: :8080)'
	@echo '  LOG_LEVEL=info      - Log level (debug|info|warn|error)'
	@echo ''
	@echo "Note: 'make dev' auto-creates data/.env from .env.example if missing"

# Install frontend dependencies (only when lockfile changes)
$(FRONTEND_STAMP): pnpm-lock.yaml
	@echo 'Installing frontend dependencies (one-off after a lockfile change)...'
	@pnpm install --frozen-lockfile --silent
	@touch $@

types: $(FRONTEND_STAMP)
	@cd ./backend && go tool tygo generate
	@pnpm --silent exec prettier --log-level silent --write sdk/types.gen.ts

build: types
	@go generate ./...
	@go install -trimpath -ldflags="-s -w -buildid=" ./backend/cmd/...

# Create data/.env from example if missing (skips interactive onboarding)
# Order-only prerequisite (|) ensures we don't overwrite existing .env
$(ENV_FILE): | .env.example
	@mkdir -p $(DATA_DIR)
	@cp .env.example $@
	@echo "Created $@ from .env.example"

dev: build $(ENV_FILE)
	@TEST_OAUTH=1 TEST_FAST_RATE_LIMIT=1 mddb -http $(HTTP) -data-dir $(DATA_DIR) -log-level $(LOG_LEVEL)

test: $(FRONTEND_STAMP)
	@go test -cover ./...
	@pnpm --silent test

# Frontend micro-benchmarks (tinybench) over frontend/src/**/*.bench.ts. Fast;
# use --save/--compare for JSON baselines when reporting deltas.
benchmark:
	@pnpm --silent benchmark

# End-to-end tests build the server, start a test instance on scraped data, and
# run Playwright. Slow, and it shares the frontend build with build targets, so
# never run it concurrently with them. Server log: data-e2e/server.log; HTML
# report: playwright-report/. CI runs these; verify and test do not.
test-e2e: build
	@python3 scripts/clean_data_e2e.py
	@TEST_OAUTH=1 TEST_FAST_RATE_LIMIT=1 pnpm --silent test:e2e; \
	e2e_exit=$$?; \
	cp -f ./data-e2e/server.log playwright-report/server.log 2>/dev/null || true; \
	if [ $$e2e_exit -ne 0 ]; then \
	  echo ""; echo "Server Log: ./data-e2e/server.log"; \
	  exit $$e2e_exit; \
	fi
	@./scripts/verify_e2e_data.py
	@node e2e/inject-tag-colors.cjs

# Same as test-e2e but with production rate limits and a single worker, for
# verifying behavior that the 10000x rate-limit multiplier hides.
test-e2e-slow: build
	@python3 scripts/clean_data_e2e.py
	@echo "Running e2e tests with normal rate limits (single worker)..."
	@TEST_OAUTH=1 TEST_FAST_RATE_LIMIT=0 pnpm --silent exec playwright test --workers=1; \
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
	@pnpm --silent coverage

# The one static gate. Runs every check-only lane concurrently; the read-only
# counterpart of fix and the pre-push gate. Independent of test.
verify: tools custom-gcl $(FRONTEND_STAMP)
	@./scripts/run-concurrently.sh go,js,ts,eslint,python,shell,misc '$(VERIFY_GO)' '$(VERIFY_JS)' '$(VERIFY_TS)' '$(VERIFY_ESLINT)' '$(VERIFY_PY)' '$(VERIFY_SH)' '$(VERIFY_MISC)'

# Apply every autofix, then refresh the generated file index. Order matters:
# the stylelint fixer runs last because its cascade-sensitive rewrites must not
# be undone by another formatter, and the index refresh runs after fixes so the
# index matches the fixed tree. Does not re-check; run verify for that.
fix: tools custom-gcl $(FRONTEND_STAMP)
	@./custom-gcl run --show-stats=false ./... --fix
	@golangci-lint fmt
	@pnpm --silent lint:fix
	@pnpm --silent format
	@ruff check --quiet --fix .
	@ruff format --quiet .
	@files=$$(git ls-files '*.sh' 'scripts/hooks/*'); [ -z "$$files" ] || shfmt -w $$files
	@pnpm --silent lint:style:fix
	@./scripts/update_agents_file_index.py

git-hooks:
	@./scripts/install-git-hooks.sh

frontend-dev: $(FRONTEND_STAMP)
	@pnpm --silent dev
