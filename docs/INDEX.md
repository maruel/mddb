# mddb Documentation

Welcome to mddb! Here's a guide to all available documentation.

## Getting Started

**New to mddb?** Start here:

1. [QUICKSTART.md](QUICKSTART.md) - Get running in 2 minutes
   - Prerequisites
   - Backend setup
   - Frontend setup
   - Common tasks

2. [README.md](../README.md) - Main project overview
   - Features and capabilities
   - Installation instructions
   - File structure
   - FAQ

## Understanding the Project

**Want to understand the architecture and plan?**

- [PLAN.md](PLAN.md) - Implementation roadmap and technical design
  - Overview and principles
  - Requirements (functional & non-functional)
  - Data model and storage format
  - API architecture
  - Implementation phases
  - Technical decisions

- [AGENTS.md](../AGENTS.md) - Development guidelines (root level)
  - Project overview and storage model
  - Directory structure
  - Go development patterns
  - Frontend development patterns
  - API conventions
  - Testing practices
  - Git workflow

## Development Tools

**Code quality, linting, and standards:**

- [LINTERS.md](LINTERS.md) - Code quality standards
  - Go linting with golangci-lint
  - Frontend linting with ESLint + Prettier
  - Pre-commit hooks setup
  - Code style rules and common fixes

- [MIGRATION_PNPM.md](MIGRATION_PNPM.md) - Package manager migration
  - Why we use pnpm
  - What changed
  - Performance improvements
  - Verification steps

## Quick Commands

```bash
# Development
make dev              # Start backend server
make frontend-dev     # Start frontend dev server
make build-all        # Build everything

# Testing
make test             # Run all tests
make test-backend     # Run Go tests only

# Building
make build            # Build Go binary
make frontend-build   # Build frontend for production

# Code quality
make lint             # Run all linters
make lint-fix         # Auto-fix all linting issues

# Cleanup
make clean            # Remove binaries and data
```

## Storage Model

Every page—document or database—is a directory with a numeric ID (1, 2, 3, etc.):

```
data/
└── pages/
    ├── 1/                    # Document page
    │   ├── index.md          # Content with YAML front matter
    │   └── favicon.ico       # Optional icon
    ├── 2/                    # Database page
    │   ├── index.md
    │   ├── metadata.json     # Schema definition
    │   ├── data.jsonl        # Records (one per line)
    │   └── favicon.png
    └── 3/subfolder/4/        # Nested organization
        ├── index.md
        └── favicon.ico
```

Benefits:
- **Asset namespace**: Each page owns its assets (images, files, etc.)
- **Clarity**: Every page is a directory—no ambiguity
- **Scalability**: Numeric IDs avoid collisions
- **Organization**: Natural hierarchical structure
- **Version control**: Directories are git-friendly

## File Formats

**Documents (index.md)**
```markdown
---
id: uuid
title: Page Title
created: 2024-01-15T10:00:00Z
modified: 2024-01-15T10:00:00Z
---

Content in markdown...
```

**Databases (metadata.json + data.jsonl)**

`metadata.json`:
```json
{
  "id": "uuid",
  "title": "Database Title",
  "columns": [
    { "id": "col_1", "name": "Name", "type": "text" },
    { "id": "col_2", "name": "Status", "type": "select", "options": ["todo", "done"] }
  ],
  "created": "2024-01-15T10:00:00Z",
  "modified": "2024-01-15T10:00:00Z"
}
```

`data.jsonl` (one record per line):
```jsonl
{"id":"rec_1","data":{"Name":"Task A","Status":"todo"},"created":"2024-01-15T10:05:00Z","modified":"2024-01-15T10:05:00Z"}
{"id":"rec_2","data":{"Name":"Task B","Status":"done"},"created":"2024-01-15T10:06:00Z","modified":"2024-01-15T10:06:00Z"}
```

## Project Status

**Phase 3: Databases ✓ Complete**

- [x] Phase 1: Core foundation (routing, page CRUD, error handling)
- [x] Phase 1.5: Code quality (linters, pre-commit hooks)
- [x] Phase 2: Page editor (markdown editor, live preview, auto-save)
- [x] Phase 3: Databases (schema, records, table UI, CRUD)
- [ ] Phase 4: Assets & Media (file uploads, image handling)
- [ ] Phase 5: Polish (search, history, optimization)

See [PLAN.md](PLAN.md) for full details.

## Technology Stack

**Backend**
- Go 1.25.5+
- Standard library (no external dependencies for core features)
- File-based persistence

**Frontend**
- SolidJS 1.9+
- TypeScript 5.9+
- Vite 5.4+
- pnpm package manager

**Tools**
- Make for development automation
- golangci-lint for Go code quality
- ESLint + Prettier for TypeScript/SolidJS quality

## API Overview

```
GET  /api/health              # Health check
GET  /api/pages               # List pages
GET  /api/pages/{id}          # Get page
POST /api/pages               # Create page
PUT  /api/pages/{id}          # Update page
DELETE /api/pages/{id}        # Delete page

GET  /api/databases           # List databases
POST /api/databases           # Create database
GET  /api/databases/{id}      # Get database schema
PUT  /api/databases/{id}      # Update schema
DELETE /api/databases/{id}    # Delete database

GET  /api/databases/{id}/records              # List records
POST /api/databases/{id}/records              # Create record
GET  /api/databases/{id}/records/{rid}        # Get record
PUT  /api/databases/{id}/records/{rid}        # Update record
DELETE /api/databases/{id}/records/{rid}      # Delete record
```

See [README.md](../README.md) for full API documentation.

## Next Steps

1. **New?** → [QUICKSTART.md](QUICKSTART.md)
2. **Understanding?** → [PLAN.md](PLAN.md)
3. **Developing?** → [AGENTS.md](../AGENTS.md)
4. **Need standards?** → [LINTERS.md](LINTERS.md)
5. **Overview?** → [README.md](../README.md)

---

**mddb** is a local-first document and database system. All data stays on your computer, organized in simple directories and markdown files.
