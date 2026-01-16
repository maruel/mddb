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
	@echo "  make build          - Build Go server binary (requires frontend built)"
	@echo "  make build-all      - Build frontend + Go server with embedded assets"
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

# Build Go server (assumes frontend is already built)
build:
	go build -o $(BINARY_NAME) ./cmd/mddb

dev: build
	./$(BINARY_NAME) -port $(PORT) -data-dir $(DATA_DIR) -log-level $(LOG_LEVEL)

test: test-backend test-frontend

test-backend:
	go test ./...

test-frontend:
	cd frontend && pnpm test

lint: lint-go lint-frontend
	@echo "✓ All linting passed"

lint-go:
	@which golangci-lint > /dev/null || (echo "Installing golangci-lint..." && go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)
	golangci-lint run ./...

lint-frontend: frontend-install
	cd frontend && pnpm lint

lint-fix:
	golangci-lint run ./... --fix || true
	cd frontend && pnpm lint:fix

format:
	cd frontend && pnpm format

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
	cd frontend && pnpm install

frontend-dev: frontend-install
	cd frontend && pnpm dev

frontend-build: frontend-install
	cd frontend && pnpm build

# Build complete system with embedded frontend (frontend + Go server with embedded assets)
build-all: frontend-build test-backend build
	@echo "✓ Build complete - embedded binary"
	@echo "  Binary: ./$(BINARY_NAME)"
	@echo "  Includes: Frontend from ./frontend/dist/ (go:embed)"
	@echo "  Build is deterministic and reproducible"

# Run both backend and frontend in development mode
dev-all: frontend-install
	@echo "Starting development servers..."
	@echo "  Backend: http://localhost:$(PORT)"
	@echo "  Frontend (dev): http://localhost:5173"
	@echo ""
	@echo "Press Ctrl+C to stop"
	@echo ""
	make dev &
	cd frontend && pnpm dev
