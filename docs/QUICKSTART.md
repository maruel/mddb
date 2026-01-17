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
# Create a page (replace {orgID} with actual organization ID, e.g., 1)
curl -X POST http://localhost:8080/api/1/nodes \
  -H "Content-Type: application/json" \
  -d '{"title":"My First Page","content":"# Hello\n\nWelcome to mddb!","type":"page"}'

# List nodes
curl http://localhost:8080/api/1/nodes

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

### Nodes (Pages & Databases)
```bash
# List all nodes
GET /api/{orgID}/nodes

# Get a specific node
GET /api/{orgID}/nodes/{id}

# Create a new node
POST /api/{orgID}/nodes
Body: {"title":"...", "content":"...", "type":"page|database"}

# Update a node
PUT /api/{orgID}/nodes/{id}
Body: {"title":"...", "content":"..."}

# Delete a node
DELETE /api/{orgID}/nodes/{id}

# Health check
GET /api/health
```

## Data Storage

All data is stored in `./data/{orgID}/pages/` as numbered directories:

```
data/
└── {orgID}/
    └── pages/
        ├── 1/
        │   └── index.md
        ├── 2/
        │   ├── index.md
        │   ├── metadata.json
        │   └── data.jsonl
        └── 3/
            └── index.md
```

Each page directory contains `index.md` with YAML front matter:
```markdown
---
id: uuid-here
title: Page Title
created: 2026-01-15T20:09:42Z
modified: 2026-01-15T20:09:42Z
---

# Your content here
```

Databases (in any page directory) include:
- `metadata.json` - Column schema
- `data.jsonl` - Records, one per line
- `index.md` - Database metadata (if needed)

## Common Tasks

### Create a Page via API
```bash
curl -X POST http://localhost:8080/api/{orgID}/nodes \
  -H "Content-Type: application/json" \
  -d '{
    "title": "Introduction",
    "content": "# Welcome\n\nStart editing!",
    "type": "page"
  }'
```

### Get a Page
```bash
# Replace {id} with actual page ID
curl http://localhost:8080/api/{orgID}/nodes/{id}
```

### Update a Page
```bash
curl -X PUT http://localhost:8080/api/{orgID}/nodes/{id} \
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
