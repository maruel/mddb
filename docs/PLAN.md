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
- [x] Phase 5.6: In-Memory Caching (metadata, hot pages, and records)
- [x] Phase 5.7: Seamless UI Overhaul (Unified Sidebar & Integrated Views)

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

### API Architecture

Standard RESTful API for Pages, Databases, Records, and Assets. 
**Multi-tenant routing**: Data-acting endpoints follow the pattern `/api/{orgID}/resource/...`. 
The backend validates that the `orgID` in the path matches the user's authenticated session.

See `README.md` and `API.md` for details.

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
- [x] **In-Memory Caching**: High-efficiency caching of metadata and frequently accessed content.
- [x] Phase 5.7: Seamless UI Overhaul (Unified Sidebar & Integrated Views)
    - [x] Unify Pages and Databases into a single "Node" concept.
    - [x] Hierarchical sidebar with recursive nesting.
    - [x] Breadcrumbs navigation.
    - [x] Integrated view: Show database records within the page context.
    - [x] Database "Views" as a first-class UI concept (Table, Grid).
    - [x] Advanced Database Views (Gallery, Board).

### Phase 6: Multi-tenant Foundation ✓
*In Progress.*

- [x] **Auth System**: User registration, login, and JWT-based authentication.
- [x] **Initial Organizations**: Workspace isolation and shared access.
- [x] **Initial RBAC**: Enforce roles (Admin, Editor, Viewer) in middleware.
- [x] **Storage Restructuring**: Moved page data to `data/{orgID}/pages/` and system metadata to `data/db/`.
- [x] **Path Resolution Refactor**: Update services (`PageService`, `DatabaseService`, etc.) to derive paths from `orgID`.
- [x] **Context-Propagated OrgID**: Standardize extraction of `orgID` from `context.Context` across service layers.
- [x] **Org ID Validation Middleware**: Ensure `{orgID}` in path matches user's `OrganizationID` (or they are a global admin).
- [x] **GitService Refactoring**:
    - [x] Support multiple repository roots (root `data/` and submodule `data/{orgID}/`).
    - [x] Implement submodule management (init, add, update).
    - [x] Update `CommitChange` to route commits to the correct repository based on `orgID`.
    - [x] Implement automatic "Root Sync" for submodule pointer updates in the root repository.
- [x] **Per-Organization Git**: Initialize/manage separate Git repositories for each `data/{orgID}/` as submodules.
- [x] **Organization Lifecycle**: Bootstrapping logic for new organizations (dirs, git, welcome content).
- [ ] **Isolation Verification**: Integration tests for cross-tenant access prevention.
- [ ] **Linear Membership Model**: Support many-to-many User/Org relationship.
- [ ] **Pending Memberships**: Resource assignment for invited users.

### Phase 7: Relational Metadata & Identity

- [ ] **Membership Service**: Implement the "Linear" access model (many-to-many User/Org) via the `Membership` bridge.
- [ ] **Invitation Flow**: Email-based invites for pending memberships and resource assignment.
- [ ] **OAuth2 Integration**: Support for Google and Microsoft login flows.
- [ ] **Tiered Settings**: Global User, Membership-specific, and Organization-wide settings.

### Phase 8: Experience & Scaling

- [ ] **Frontend Multi-tenancy UX**:
    - [ ] **Organization Switcher**: UI component for switching between active memberships.
    - [ ] **Org-Aware Search**: Scope search results strictly to the active organization.
- [ ] **Quota Enforcement**: Track and limit resource usage (storage, pages) per organization.
- [x] **User UI**: Management of users, permissions, and quotas in the frontend. (Initial version implemented)

## Future Considerations
- **SQLite Migration**: Migrate `data/db/*.json` to SQLite for better relational integrity and query performance.
- **Real-time Collaboration**: WebSocket-based sync.
- **Mobile App**: Native mobile clients using the REST API.
