# Quick Start Guide

Get mddb up and running in 2 minutes.

## Prerequisites

- Go 1.25.5+
- (Optional) Node.js 18+ for frontend development
- (Optional) pnpm for frontend (install with: `npm install -g pnpm`)
- (Optional) Make for convenience commands

## Quick Start (Backend Only)

### 1. Build
```bash
make build
```

### 2. Run
```bash
make dev
```

Server will be available at `http://localhost:8080/api/health`

### 3. Test It
```bash
# Create a page
curl -X POST http://localhost:8080/api/pages \
  -H "Content-Type: application/json" \
  -d '{"title":"My First Page","content":"# Hello\n\nWelcome to mddb!"}'

# List pages
curl http://localhost:8080/api/pages

# Delete (optional)
make clean
```

## Full Development Setup (Backend + Frontend)

### 1. Build Both
```bash
make build-all
```

This builds:
- Backend binary: `./mddb`
- Frontend: `./web/dist/`

### 2. Run Backend
```bash
make dev
```

Starts server at `http://localhost:8080`

### 3. Run Frontend (Separate Terminal)
```bash
make frontend-dev
```

Frontend dev server at `http://localhost:5173`
(Uses pnpm; automatically proxies API calls to backend)

### 4. Open Browser
Visit `http://localhost:5173` and start managing pages

## Available Commands

```bash
make              # Show help
make build        # Build backend binary
make dev          # Run backend server
make test         # Run all tests
make clean        # Clean up binaries and data
make frontend-dev # Run frontend dev server
make frontend-build # Build frontend for production
```

## API Endpoints

### Pages
```bash
# List all pages
GET /api/pages

# Get a specific page
GET /api/pages/{id}

# Create a new page
POST /api/pages
Body: {"title":"...", "content":"..."}

# Update a page
PUT /api/pages/{id}
Body: {"title":"...", "content":"..."}

# Delete a page
DELETE /api/pages/{id}

# Health check
GET /api/health
```

## Data Storage

All data is stored in `./data/pages/` as markdown files:

```
data/
└── pages/
    ├── a1f86c50-b91b-4c70-888a-6e82f83d7dc0.md
    └── nested/
        └── another-page.md
```

Each file has YAML front matter with metadata:
```markdown
---
id: a1f86c50-b91b-4c70-888a-6e82f83d7dc0
title: Page Title
created: 2026-01-15T20:09:42Z
modified: 2026-01-15T20:09:42Z
---

# Your content here
```

## Common Tasks

### Create a Page via API
```bash
curl -X POST http://localhost:8080/api/pages \
  -H "Content-Type: application/json" \
  -d '{
    "title": "Introduction",
    "content": "# Welcome\n\nStart editing!"
  }'
```

### Get a Page
```bash
# Replace {id} with actual page ID
curl http://localhost:8080/api/pages/{id}
```

### Update a Page
```bash
curl -X PUT http://localhost:8080/api/pages/{id} \
  -H "Content-Type: application/json" \
  -d '{
    "title": "Updated Title",
    "content": "Updated content..."
  }'
```

### Run Tests
```bash
make test-backend
```

### Change Server Port
```bash
make dev PORT=3000
```

### Change Log Level
```bash
make dev LOG_LEVEL=debug
```

## Troubleshooting

### Server won't start
- Check if port 8080 is already in use: `lsof -i :8080`
- Try a different port: `make dev PORT=9000`

### Data directory issues
- Remove old data: `make clean`
- Ensure write permissions on current directory

### Frontend not updating
- Make sure backend is running on port 8080
- Check browser console for API errors
- Restart frontend dev server if needed

## Next Steps

- Create a page in the UI
- Read [PLAN.md](PLAN.md) for upcoming features
- Check [PROGRESS.md](PROGRESS.md) for what's been completed
- Review [README.md](../README.md) for full documentation

## Need Help?

- Check `make help` for all available commands
- Review the test files in `internal/storage/filestore_test.go`
- Check the API structure in `internal/server/handlers/`
