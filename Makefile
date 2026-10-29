# Build, verify, test, and development commands.

.DEFAULT_GOAL := help
.PHONY: help build coverage dev fix git-hooks frontend-dev test test-smoke-voice test-e2e test-e2e-go types verify custom-gcl benchmark

FRONTEND_STAMP=node_modules/.modules.yaml

FRONTEND_TEST = pnpm exec node --test scripts/css_module_types.test.mjs && NODE_OPTIONS=--no-experimental-webstorage pnpm exec tsx --test --conditions=browser --test-force-exit --test-reporter=./scripts/quiet-test-reporter.mjs --import ./frontend/tests/setup-dom.ts
FRONTEND_TEST_FILES = "frontend/src/**/*.test.ts" "frontend/src/**/*.test.tsx"

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
	@printf '  %-18s - %s\n' 'make test-e2e' 'Run Playwright e2e tests (slow, parallel)'
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
# config.toml and runs `go run`. See the script for the flags.
dev:
	@uv run --locked python scripts/run-dev.py

test: $(FRONTEND_STAMP)
	@./scripts/run-concurrently.sh go,frontend \
		'go test -cover ./...' \
		'$(FRONTEND_TEST) $(FRONTEND_TEST_FILES)'

# Slow and opt-in: needs GEMINI_API_KEY and reachable WebRTC UDP. Runs one live
# Gemini voice turn through the embedded gateway, executes the workspace MCP
# nodes_list tool, and checks that hang-up releases session capacity.
test-smoke-voice:
	@go test -tags=smoke -run TestSmokeVoiceGatewayGemini -v -timeout 5m ./backend/internal/server/

# Frontend micro-benchmarks (tinybench) over frontend/src/**/*.bench.ts. Fast;
# use --save/--compare for JSON baselines when reporting deltas.
benchmark:
	@pnpm exec tsx scripts/bench.ts 'frontend/src/**/*.bench.ts'

# Run the complete browser suite and verify its saved data with normal pairing limits.
# Focused diagnostics use pnpm exec playwright test e2e/file.spec.ts directly.
test-e2e: $(FRONTEND_STAMP) test-e2e-go
	@pnpm exec playwright install --no-shell chromium
	@uv run --locked python scripts/clean_data_e2e.py
	@pnpm exec playwright test $(PLAYWRIGHT_ARGS); \
	e2e_exit=$$?; \
	cp -f ./data-e2e/server.log playwright-report/server.log 2>/dev/null || true; \
	if [ $$e2e_exit -ne 0 ]; then echo 'Server Log: ./data-e2e/server.log'; exit $$e2e_exit; fi
	@uv run --locked python scripts/verify_e2e_data.py
	@pnpm exec node e2e/inject-tag-colors.cjs

# The e2e build tag also switches on the Go tests that only exist in that build,
# which assert what a production binary must not do. Running the whole tagged
# suite keeps every package honest about the tag: make test stays untagged.
test-e2e-go:
	@go test -tags e2e ./backend/...

coverage: $(FRONTEND_STAMP)
	@go test -coverprofile=coverage.out ./...
	@$(FRONTEND_TEST) --experimental-test-coverage $(FRONTEND_TEST_FILES)

verify: custom-gcl $(FRONTEND_STAMP)
	@./scripts/run-concurrently.sh sdk,workflows,go,js,ts,eslint,python,shell,misc \
		'cd ./backend/internal/server && go run ../cmd/gen-api-sdk -check' \
		'go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12' \
		'./custom-gcl run --show-stats=false ./... && go mod tidy -diff' \
		'pnpm exec prettier --check --log-level warn --cache --cache-location node_modules/.cache/prettier/.prettier-cache --cache-strategy content . && pnpm exec stylelint "frontend/src/**/*.css" && pnpm exec node scripts/lint_frontend_styles.mjs' \
		'pnpm exec node scripts/css_module_types.mjs --source-path frontend/src && pnpm exec tsc --noEmit --incremental --tsBuildInfoFile node_modules/.cache/tsc/root.tsbuildinfo && pnpm exec tsc --noEmit --incremental --tsBuildInfoFile node_modules/.cache/tsc/e2e.tsbuildinfo -p tsconfig.e2e.json' \
		'pnpm --silent exec eslint --cache --cache-location node_modules/.cache/eslint/ --cache-strategy content frontend/src sdk e2e playwright.config.ts scripts/css_module_types.mjs scripts/css_module_types.test.mjs' \
		'uv run --locked ruff format --check --quiet . && uv run --locked ruff check --quiet .' \
		'files=$$(git ls-files "*.sh" "scripts/hooks/*"); [ -z "$$files" ] || go tool shfmt -d $$files' \
		'uv run --locked python scripts/lint_binaries.py && uv run --locked python scripts/update_agents_file_index.py --check'

# Apply every autofix, then refresh the generated file index. Order matters:
# the stylelint fixer runs last because its cascade-sensitive rewrites must not
# be undone by another formatter, and the index refresh runs after fixes so the
# index matches the fixed tree. Does not re-check; run verify for that.
fix: custom-gcl $(FRONTEND_STAMP)
	@./custom-gcl run --show-stats=false ./... --fix
	@go tool golangci-lint fmt
	@pnpm exec eslint frontend/src sdk e2e playwright.config.ts scripts/css_module_types.mjs scripts/css_module_types.test.mjs --fix
	@pnpm exec prettier --write --log-level warn .
	@uv run --locked ruff check --quiet --fix .
	@uv run --locked ruff format . >/dev/null
	@files=$$(git ls-files '*.sh' 'scripts/hooks/*'); [ -z "$$files" ] || go tool shfmt -w $$files
	@pnpm exec stylelint --fix "frontend/src/**/*.css"
	@pnpm exec node scripts/css_module_types.mjs --source-path frontend/src --write
	@uv run --locked python scripts/update_agents_file_index.py

git-hooks:
	@./scripts/install-git-hooks.sh

frontend-dev: $(FRONTEND_STAMP)
	@pnpm --silent dev
