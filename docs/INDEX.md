# mddb Documentation Index

Welcome to mddb! Here's a guide to all available documentation.

## Getting Started

**New to mddb?** Start here:

1. [QUICKSTART.md](QUICKSTART.md) - Get running in 2 minutes
   - Prerequisites
   - Backend setup
   - Frontend setup
   - Common tasks

2. [DEMO.md](DEMO.md) - Live working examples
   - Running the demo
   - Testing with curl
   - Checking data storage
   - Architecture in action

## Understanding the Project

**Want to understand the project better?**

- [../README.md](../README.md) - Main project overview
  - Architecture
  - API endpoints
  - Project structure
  - Deployment info

- [PLAN.md](PLAN.md) - Implementation roadmap
  - Requirements (functional & non-functional)
  - Design fundamentals
  - Data model and storage format
  - Implementation phases
  - Technical decisions

- [PROGRESS.md](PROGRESS.md) - Phase 1 completion details
  - What's been implemented
  - Testing results
  - Code metrics
  - Architecture highlights
  - Known limitations

## Development

**Contributing or extending mddb?**

- [AGENTS.md](../AGENTS.md) - Development guidelines (root level for agent discovery)
  - Go development patterns
  - Frontend development patterns
  - API conventions
  - Testing practices
  - Git workflow
  - Performance tips

- [MIGRATION_PNPM.md](MIGRATION_PNPM.md) - pnpm migration details
  - Why we switched from npm
  - What changed
  - Performance improvements
  - Verification results

## Project Structure

```
mddb/
├── README.md                  # Start here for overview
├── AGENTS.md                  # Development guidelines (root for agent discovery)
├── docs/                      # Documentation directory
│   ├── INDEX.md              # You are here
│   ├── QUICKSTART.md         # 2-minute quick start
│   ├── DEMO.md               # Live examples
│   ├── PLAN.md               # Full roadmap
│   ├── PROGRESS.md           # Phase 1 summary
│   └── MIGRATION_PNPM.md     # pnpm details
├── cmd/mddb/                 # Backend entry point
├── internal/                 # Backend code
│   ├── server/              # HTTP server
│   ├── storage/             # File operations
│   ├── models/              # Data models
│   ├── errors/              # Error types
│   └── utils/               # Utilities
├── web/                      # Frontend (SolidJS)
├── data/                     # Runtime data (created dynamically)
├── Makefile                  # Development commands
└── go.mod                    # Go dependencies
```

## Quick Commands

```bash
# Development
make dev              # Start backend server
make frontend-dev     # Start frontend dev server
make build-all        # Build everything

# Testing
make test-backend     # Run Go tests
pnpm test            # Run frontend tests (from web/)

# Building
make build            # Build Go binary
make frontend-build   # Build frontend for production

# Cleanup
make clean            # Remove binaries and data
```

## Features by Phase

### Phase 1: ✓ Core Foundation (Complete)
- [x] Page CRUD operations
- [x] File storage with markdown
- [x] HTTP API with 6 endpoints
- [x] SolidJS frontend UI
- [x] Unit tests
- [x] TypeScript support

### Phase 2: Page Editor (Planned)
- [ ] Markdown editor with syntax highlighting
- [ ] Live preview
- [ ] Auto-save
- [ ] Page linking

### Phase 3: Databases (Planned)
- [ ] Database schema definition
- [ ] Record CRUD
- [ ] Table view

### Phase 4: Assets (Planned)
- [ ] File upload
- [ ] Image serving
- [ ] Asset gallery

### Phase 5: Polish (Planned)
- [ ] Full-text search
- [ ] Performance optimization
- [ ] Documentation

See [PLAN.md](PLAN.md) for full details.

## API Endpoints

Quick reference:

```
GET  /api/health              # Health check
GET  /api/pages               # List pages
GET  /api/pages/{id}          # Get page
POST /api/pages               # Create page
PUT  /api/pages/{id}          # Update page
DELETE /api/pages/{id}        # Delete page
```

See [README.md](../README.md#api-endpoints) for complete API documentation.

## Technology Stack

**Backend**
- Go 1.25.5+
- Standard library only
- Markdown + YAML front matter

**Frontend**
- SolidJS 1.9+
- TypeScript 5.9+
- Vite 5.4+
- pnpm package manager

**Tools**
- Make (development)
- Makefile (automation)

## Useful Resources

- **Getting Help**: Check AGENTS.md for development patterns
- **Understanding Architecture**: See PLAN.md technical decisions
- **Seeing Progress**: Check PROGRESS.md for Phase 1 details
- **Examples**: Run DEMO.md examples

## Next Steps

1. **First time?** → [QUICKSTART.md](QUICKSTART.md)
2. **Want examples?** → [DEMO.md](DEMO.md)
3. **Want to understand?** → [PLAN.md](PLAN.md)
4. **Want to develop?** → [AGENTS.md](AGENTS.md)
5. **Want the overview?** → [../README.md](../README.md)

## Files at a Glance

| File | Purpose | Audience |
|------|---------|----------|
| ../AGENTS.md | Dev guidelines | Developers |
| QUICKSTART.md | 2-minute setup guide | Everyone |
| DEMO.md | Working examples | Users |
| PLAN.md | Full roadmap | Architects |
| PROGRESS.md | Phase 1 summary | Project managers |
| MIGRATION_PNPM.md | Package manager switch | Maintainers |
| INDEX.md | This file | Everyone |

---

**mddb** is ready for Phase 2. Happy developing! 🚀
