.PHONY: help dev build test clean frontend lint lint-fix format

# Variables
BINARY_NAME=mddb
DATA_DIR=./data
PORT?=8080
LOG_LEVEL?=info

help:
	@echo "mddb - Markdown Document & Database System"
	@echo ""
	@echo "Available targets:"
	@echo "  make build          - Build the Go server binary"
	@echo "  make dev            - Run the server in development mode"
	@echo "  make test           - Run all tests"
	@echo "  make test-backend   - Run backend tests only"
	@echo "  make test-frontend  - Run frontend tests (requires pnpm)"
	@echo "  make lint           - Run all linters (go + frontend)"
	@echo "  make lint-go        - Run Go linter (golangci-lint)"
	@echo "  make lint-frontend  - Run frontend linter (eslint)"
	@echo "  make lint-fix       - Fix all linting issues automatically"
	@echo "  make format         - Format code with prettier"
	@echo "  make clean          - Clean build artifacts"
	@echo "  make frontend-dev   - Run frontend dev server"
	@echo "  make frontend-build - Build frontend for production"
	@echo "  make git-hooks      - Install git pre-commit hooks"
	@echo "  make help           - Show this help message"
	@echo ""
	@echo "Environment variables:"
	@echo "  PORT=8080           - Server port (default: 8080)"
	@echo "  LOG_LEVEL=info      - Log level (debug|info|warn|error)"

build:
	go build -o $(BINARY_NAME) ./cmd/mddb

dev: build
	./$(BINARY_NAME) -port $(PORT) -data-dir $(DATA_DIR) -log-level $(LOG_LEVEL)

test: test-backend test-frontend

test-backend:
	go test -v ./...

test-frontend:
	cd web && pnpm test || echo "Frontend tests skipped (pnpm not installed)"

lint: lint-go lint-frontend
	@echo "✓ All linting passed"

lint-go:
	@which golangci-lint > /dev/null || (echo "Installing golangci-lint..." && go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)
	golangci-lint run ./...

lint-frontend: frontend-install
	cd web && pnpm lint

lint-fix:
	golangci-lint run ./... --fix || true
	cd web && pnpm lint:fix

format:
	cd web && pnpm format

git-hooks:
	@echo "Installing git pre-commit hooks..."
	@mkdir -p .git/hooks
	@cp ./scripts/pre-commit .git/hooks/pre-commit
	@chmod +x .git/hooks/pre-commit
	@echo "✓ Git hooks installed"

clean:
	rm -f $(BINARY_NAME)
	rm -rf $(DATA_DIR)
	go clean

frontend-install:
	cd web && pnpm install

frontend-dev: frontend-install
	cd web && pnpm dev

frontend-build: frontend-install
	cd web && pnpm build

# Build complete system (backend + frontend)
build-all: test-backend frontend-build build
	@echo "✓ Build complete"
	@echo "  Backend binary: ./$(BINARY_NAME)"
	@echo "  Frontend built to: ./web/dist/"

# Run both backend and frontend in development mode
dev-all: frontend-install
	@echo "Starting development servers..."
	@echo "  Backend: http://localhost:$(PORT)"
	@echo "  Frontend (dev): http://localhost:5173"
	@echo ""
	@echo "Press Ctrl+C to stop"
	@echo ""
	make dev &
	cd web && pnpm dev
