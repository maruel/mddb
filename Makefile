# Build, verify, test, and development commands.

.DEFAULT_GOAL := help
.PHONY: help build coverage dev fix git-hooks frontend-dev test test-smoke-voice test-e2e test-e2e-go test-e2e-slow types verify tools custom-gcl benchmark

# Ruff is installed separately; Go tool versions are declared in go.mod.
RUFF_VERSION=0.16.8

# Prepend uv's tool directory so a recipe can run Ruff after installing it.
UV_BIN := $(if $(shell command -v uv 2>/dev/null),$(shell uv tool dir --bin 2>/dev/null))
export PATH := $(if $(UV_BIN),$(UV_BIN):)$(PATH)

tools:
	@command -v uv > /dev/null 2>&1 || { echo 'uv is required to install the Python tools; see https://docs.astral.sh/uv/' >&2; exit 1; }
	@ruff --version 2>/dev/null | grep -Fqw "$(RUFF_VERSION)" || uv tool install --force --quiet ruff==$(RUFF_VERSION)

FRONTEND_STAMP=node_modules/.modules.yaml

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
VERIFY_JS = pnpm exec prettier --check --log-level warn --cache --cache-location node_modules/.cache/prettier/.prettier-cache --cache-strategy content . && pnpm exec stylelint "frontend/src/**/*.css" && node scripts/lint_frontend_styles.mjs
VERIFY_TS = pnpm --silent typecheck
VERIFY_ESLINT = pnpm --silent exec eslint --cache --cache-location node_modules/.cache/eslint/ --cache-strategy content frontend/src sdk e2e playwright.config.ts playwright.slow.config.ts
VERIFY_PY = ruff format --check --quiet . && ruff check --quiet .
VERIFY_SH = files=$$(git ls-files "*.sh" "scripts/hooks/*"); [ -z "$$files" ] || go tool shfmt -l $$files
VERIFY_MISC = python3 scripts/lint_binaries.py && python3 scripts/update_agents_file_index.py --check

# The custom-gcl binary is not byte-reproducible (golangci-lint custom builds
# in a random temp directory and stamps VCS metadata), so staleness is tracked
# by hashing the build inputs instead of comparing mtimes: the plugin config
# plus the pinned golangci-lint version and the Go toolchain. A branch switch
# that recreates .custom-gcl.yml with a fresh timestamp must not trigger a
# rebuild; a version or config change must.
.PHONY: custom-gcl
custom-gcl:
	@version=$$(go list -m -f '{{.Version}}' github.com/golangci/golangci-lint/v2) || exit 1; \
	want=$$({ sha256sum .custom-gcl.yml | cut -d" " -f1; echo "$$version"; go env GOVERSION; } | sha256sum | cut -d" " -f1); \
	if [ -x custom-gcl ] && [ "$$want" = "$$(cat .custom-gcl.sha 2>/dev/null)" ]; then exit 0; fi; \
	echo 'Building custom-gcl with the methodfilecheck plugin (one-off; runs when the config, golangci-lint version, or Go toolchain changes)...'; \
	go tool golangci-lint custom --version "$$version" && echo "$$want" > .custom-gcl.sha

help:
	@echo 'mddb - Markdown Document & Table System'
	@echo ''
	@echo 'Available targets:'
	@printf '  %-18s - %s\n' 'make fix' 'Apply every autofix, then refresh the file index'
	@printf '  %-18s - %s\n' 'make verify' 'Fast static gate: lint, formatting, generated docs (pre-push gate)'
	@printf '  %-18s - %s\n' 'make test' 'Run unit tests (Go, frontend)'
	@printf '  %-18s - %s\n' 'make test-smoke-voice' 'Run the live Gemini voice gateway smoke test (slow, needs GEMINI_API_KEY)'
	@printf '  %-18s - %s\n' 'make benchmark' 'Run frontend micro-benchmarks (tinybench)'
	@printf '  %-18s - %s\n' 'make test-e2e' 'Run Playwright e2e tests (slow, fast rate limits, parallel)'
	@printf '  %-18s - %s\n' 'make test-e2e-slow' 'Run e2e tests with normal rate limits (sequential)'
	@printf '  %-18s - %s\n' 'make build' 'Build the Go server (generates types and frontend)'
	@printf '  %-18s - %s\n' 'make dev' 'Run the server in development mode (scripts/run-dev.py)'
	@printf '  %-18s - %s\n' 'make frontend-dev' 'Run frontend dev server (http://localhost:5173)'
	@printf '  %-18s - %s\n' 'make coverage' 'Run tests with coverage'
	@printf '  %-18s - %s\n' 'make types' 'Generate TypeScript DTOs, client, and API reference'
	@printf '  %-18s - %s\n' 'make git-hooks' 'Install git pre-commit hooks'
	@echo ''
	@echo 'make dev runs scripts/run-dev.py; pass --help to see its flags.'

# Install frontend dependencies (only when lockfile changes)
$(FRONTEND_STAMP): pnpm-lock.yaml
	@echo 'Installing frontend dependencies (one-off after a lockfile change)...'
	@pnpm install --frozen-lockfile --silent
	@touch $@

types: $(FRONTEND_STAMP)
	@go generate ./backend/internal/server
	@pnpm --silent exec prettier --log-level silent --write sdk/types.gen.ts

build: types
	@go generate ./...
	@go install -trimpath -ldflags="-s -w -buildid=" ./backend/cmd/...

# Run the server in development mode. scripts/run-dev.py writes a temporary
# config.toml and runs `go run`, passing the e2e-only -fast-rate-limit flag with
# --fake. See the script for the flags.
dev:
	@./scripts/run-dev.py

test: $(FRONTEND_STAMP)
	@go test -cover ./...
	@pnpm --silent test

# Slow and opt-in: needs GEMINI_API_KEY and reachable WebRTC UDP. Runs one live
# Gemini voice turn through the embedded gateway, executes the workspace MCP
# nodes_list tool, and checks that hang-up releases session capacity.
test-smoke-voice:
	@go test -tags=smoke -run TestSmokeVoiceGatewayGemini -v -timeout 5m ./backend/internal/server/

# Frontend micro-benchmarks (tinybench) over frontend/src/**/*.bench.ts. Fast;
# use --save/--compare for JSON baselines when reporting deltas.
benchmark:
	@pnpm --silent benchmark

# End-to-end tests run Playwright, whose webServer starts the e2e server via
# scripts/run-dev.py --fake on the data-e2e directory. Slow. Server log:
# data-e2e/server.log; HTML report: playwright-report/. CI runs these; verify
# and test do not.
test-e2e: test-e2e-go
	@python3 scripts/clean_data_e2e.py
	@pnpm --silent test:e2e; \
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
test-e2e-slow: test-e2e-go
	@python3 scripts/clean_data_e2e.py
	@echo "Running e2e tests with normal rate limits (single worker)..."
	@pnpm --silent exec playwright test --config playwright.slow.config.ts --workers=1; \
	e2e_exit=$$?; \
	cp -f ./data-e2e/server.log playwright-report/server.log 2>/dev/null || true; \
	if [ $$e2e_exit -ne 0 ]; then \
	  echo ""; echo "=== Server Log ==="; cat ./data-e2e/server.log 2>/dev/null || true; \
	  exit $$e2e_exit; \
	fi
	@./scripts/verify_e2e_data.py
	@node e2e/inject-tag-colors.cjs

# The e2e build tag also switches on the Go tests that only exist in that build,
# which assert what a production binary must not do. Running the whole tagged
# suite keeps every package honest about the tag: make test stays untagged.
test-e2e-go:
	@go test -tags e2e ./backend/...

coverage: $(FRONTEND_STAMP)
	@go test -coverprofile=coverage.out ./...
	@pnpm --silent coverage

# The one static gate. Runs every check-only lane concurrently; the read-only
# counterpart of fix and the pre-push gate. Independent of test.
verify: tools custom-gcl $(FRONTEND_STAMP)
	@go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12
	@./scripts/run-concurrently.sh go,js,ts,eslint,python,shell,misc '$(VERIFY_GO)' '$(VERIFY_JS)' '$(VERIFY_TS)' '$(VERIFY_ESLINT)' '$(VERIFY_PY)' '$(VERIFY_SH)' '$(VERIFY_MISC)'

# Apply every autofix, then refresh the generated file index. Order matters:
# the stylelint fixer runs last because its cascade-sensitive rewrites must not
# be undone by another formatter, and the index refresh runs after fixes so the
# index matches the fixed tree. Does not re-check; run verify for that.
fix: tools custom-gcl $(FRONTEND_STAMP)
	@./custom-gcl run --show-stats=false ./... --fix
	@go tool golangci-lint fmt
	@pnpm exec eslint frontend/src sdk e2e playwright.config.ts playwright.slow.config.ts --fix
	@pnpm exec prettier --write --log-level warn .
	@ruff check --quiet --fix .
	@ruff format --quiet .
	@files=$$(git ls-files '*.sh' 'scripts/hooks/*'); [ -z "$$files" ] || go tool shfmt -w $$files
	@pnpm exec stylelint --fix "frontend/src/**/*.css"
	@./scripts/update_agents_file_index.py

git-hooks:
	@./scripts/install-git-hooks.sh

frontend-dev: $(FRONTEND_STAMP)
	@pnpm --silent dev
