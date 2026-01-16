# mddb Implementation Plan

## Status

**Current Phase: Phase 5 (Polish & Features) - Git Versioning ✓ + Full-Text Search ✓ + Page History ✓ + Performance ✓**

- [x] Phase 1: Core foundation with routing, error handling, page CRUD
- [x] Phase 1.5: Code quality & linters (golangci-lint, ESLint, Prettier)
- [x] Phase 2: Page editor with live preview and auto-save
- [x] Phase 3: Databases with dual-format storage and table UI
- [x] Phase 3.5: Storage migration to directory-based numeric IDs
- [x] Phase 4: Assets & Media (file uploads, asset management, serving)
- [x] Embedded Build: go:embed frontend in binary, deterministic builds, tracked in git
- [x] Phase 5.1: Better Error Handling (error codes, detailed responses, validation)
- [x] Phase 5.2: Full-Text Search (pages, databases, records, relevance scoring)
- [x] Phase 5.3: Automatic Git Versioning (data directory tracking)
- [x] Phase 5.4: Page history/versioning UI
- [x] Phase 5.5: Performance Optimization (pagination, streaming reads)
- [ ] Phase 5.6: In-Memory Caching (metadata, hot pages, and records)
- [/] Phase 5.7: Seamless UI Overhaul (Unified Sidebar & Integrated Views)

**Key Stats:**
- Backend: ~40 comprehensive tests (100% passing), zero linting errors
- Frontend: Production-grade TypeScript/SolidJS, zero linting errors
- Storage: Directory-based numeric IDs (1, 2, 3, etc.) with asset namespaces
- Distribution: Single binary with embedded frontend (go:embed), deterministic builds

## Overview

mddb is a Notion-like document and database system where all data is persisted as markdown files and JSON in a directory-based structure. The frontend (SolidJS) provides a rich user experience while the backend (Go) handles file operations, API endpoints, and business logic.

**Key Principle**: Every page—whether a document or database—is a directory with a monotonically increasing numeric ID. This provides a clean namespace for assets and supports unlimited scaling.

## Requirements

See [REQUIREMENTS.md](REQUIREMENTS.md) for complete functional and non-functional requirements.

## Design Fundamentals

### Data Model

```
Project Root/
└── pages/
    ├── 1/                    # First page (document)
    │   ├── index.md
    │   ├── favicon.ico
    │   └── image-1.png       # Page assets
    ├── 2/                    # Second page (database)
    │   ├── index.md
    │   ├── metadata.json
    │   ├── data.jsonl
    │   └── favicon.ico
    └── ...
```

### Storage Format

**Pages (Markdown with YAML Front Matter)**
Stored as `index.md` with YAML front matter for metadata (title, created, modified, tags).

**Databases (Directory with metadata.json + data.jsonl)**
- `metadata.json`: Schema definition (columns, types).
- `data.jsonl`: Records, one per line.

**Assets**
Stored directly in the page directory.

### API Architecture

Standard RESTful API for Pages, Databases, Records, and Assets.
See `README.md` and `ASSET_API.md` for details.

## Implementation Phases

### Phase 1: Core Foundation ✓
*Completed.*

### Phase 1.5: Code Quality & Linters ✓
*Completed.*

### Phase 2: Page Editor ✓
*Completed.*

### Phase 3: Databases ✓
*Completed.*

### Phase 3.5: Storage Migration ✓
*Completed.*

### Phase 4: Assets & Media ✓
*Completed.*
- Asset storage in page directories
- Multipart form upload
- Binary serving with MIME detection

### Embedded Build ✓
*Completed.*
- Single binary distribution
- Frontend embedded via `go:embed`

### Phase 5: Polish & Features ✓
*Completed.*
- [x] **Better Error Handling**: Structured error responses, validation.
- [x] **Full-Text Search**: Search service for pages and databases.
- [x] **Git Versioning**: Automatic commit of data changes.
- [x] **Page History UI**: View and restore previous versions (leveraging Git).
- [x] **Performance**: Optimization of large lists/databases (pagination, streaming).
- [ ] **In-Memory Caching**: High-efficiency caching of metadata and frequently accessed content.
- [x] Phase 5.7: Seamless UI Overhaul (Unified Sidebar & Integrated Views)
    - [x] Unify Pages and Databases into a single "Node" concept.
    - [x] Hierarchical sidebar with recursive nesting.
    - [x] Breadcrumbs navigation.
    - [x] Integrated view: Show database records within the page context.
    - [x] Database "Views" as a first-class UI concept (Table, Grid).
    - [ ] Advanced Database Views (Gallery, Board).

### Phase 6: Multi-user & RBAC (Planned)

- [ ] **Auth System**: User registration, login, and JWT-based authentication.
- [ ] **OAuth2 Integration**: Support for Google and Microsoft login flows.
- [ ] **Organizations**: Workspace isolation and shared access.
- [ ] **RBAC Implementation**: Enforce roles (Admin, Editor, Viewer) in middleware.
- [ ] **Quota Enforcement**: Track and limit resource usage (storage, pages) per organization.
- [ ] **User UI**: Management of users, permissions, and quotas in the frontend.

## Future Considerations
- **Real-time Collaboration**: WebSocket-based sync.
- **Mobile App**: Native mobile clients using the REST API.
