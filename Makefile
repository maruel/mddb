.PHONY: help dev build test clean frontend

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
	@echo "  make test-frontend  - Run frontend tests (requires npm)"
	@echo "  make clean          - Clean build artifacts"
	@echo "  make frontend-dev   - Run frontend dev server"
	@echo "  make frontend-build - Build frontend for production"
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
