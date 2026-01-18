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

**Key Principle**: Every page—whether a document or database—is a directory with a monotonically increasing Base64 URL-encoded ID (without padding). This provides a clean namespace for assets and supports unlimited scaling.

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
- [x] Schema definition and storage
- [x] Record creation and JSONL storage
- [x] Record listing with pagination
- [x] Record update and deletion

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
*Completed.*

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
- [x] **Isolation Verification**: Integration tests for cross-tenant access prevention.
- [x] **Linear Membership Model**: Support many-to-many User/Org relationship.
- [x] **Pending Memberships**: Resource assignment for invited users.

### Phase 7: Relational Metadata & Identity ✓
*Completed.*

- [x] **Membership Service**: Implement the "Linear" access model (many-to-many User/Org) via the `Membership` bridge.
- [x] **Invitation Flow**: Email-based invites for pending memberships and resource assignment.
- [x] **OAuth2 Integration**: Support for Google login flow.
- [x] **OAuth2 Integration (Microsoft)**: Support for Microsoft login flows.
- [x] **Tiered Settings**: Global User, Membership-specific, and Organization-wide settings.

### Phase 8: Experience & Scaling ✓
*Completed.*

- [x] **Org-Aware Search**: Scope search results strictly to the active organization. (Implemented in SearchService)
- [x] **Frontend Multi-tenancy UX**:
    - [x] **Organization Switcher**: UI component for switching between active memberships.
- [x] **Unified API Type Generation (tygo)**: Go as single source of truth for frontend types. Centralized all Request, Response, and DTO structures in `internal/models`, including structured `ErrorCode` unions for consistent error handling.
- [x] **Quota Enforcement**: Track and limit resource usage (storage, pages) per organization.
- [x] **User UI**: Management of users, permissions, and quotas in the frontend.

### Phase 9: Advanced Sync & Onboarding
*In Progress.*

- [x] **Advanced Git Remotes**:
    - [x] **Remote Configuration UI**: Interface for organization admins to set up Git remotes.
    - [ ] **GitHub App Integration**: Support for GitHub App installation to manage organization repository access.
    - [x] **Remote Push Service**: Background worker to push organization submodule changes to their configured remotes. (Implemented manual push)
    - [x] **Secret Management**: Secure storage of Git credentials/tokens per organization.
- [x] **Organization Onboarding**:
    - [x] **Onboarding State Tracking**: Track if an organization has completed the onboarding process.
    - [x] **Onboarding UI**: Guided multi-step modal or wizard for initial configuration (Org name, members, Git remote).
    - [ ] **CLI Onboarding**: Implement a simple configuration wizard in the CLI.
    - [ ] **CLI Versioning**: Implement `mddb -version` to display build metadata (Git commit, dirty state) using Go's build info.
    - [ ] **Local Config**: Store CLI configuration in an unversioned `.env` file in `data/` (added to `.gitignore`).
    - [x] **Settings Integration**: Ensure all onboarding options are accessible and editable in Workspace Settings.

### Phase 10: Connectivity & Relations
- [ ] **Backlinks Indexing**: Efficient background indexing of internal markdown links to provide backlink lists.
- [ ] **Relational Data Model**: Update metadata schema to support record-level relations (foreign keys) between databases.
- [ ] **Relational UI**: UI components for selecting related records (popover/search) and displaying Rollups (aggregated data from linked records).
- [ ] **Graph View**: Interactive visualization of page and database connections.
- [ ] **Notion Research**: Document product design insights from Notion documentation in `docs/notion/`.

### Phase 11: AI Intelligence
- [ ] **MCP Server**: Full implementation of the Model Context Protocol to expose documents/databases to AI agents.
- [ ] **Vector Search Integration**: Embedding generation for documents and records to support semantic search.
- [ ] **In-Editor AI Actions**: Context-aware AI commands for content generation and summarization.
- [ ] **Data Repository Context**: Automatically add `AGENTS.md` to organization repositories for agent guidance.

### Phase 12: Globalization, Mobile & Theming
- [ ] **i18n and l10n**: Implement internationalization and localization framework.
- [ ] **PWA Support**: Configure manifest and service workers for Progressive Web App support.
- [ ] **Offline Mode**: Implement client-side storage and reconciliation logic for offline use.
- [ ] **Theme System**:
    - [ ] Research Hugo theme engine for inspiration.
    - [ ] Implement a flexible theming system (CSS variables/tokens).
    - [ ] Add support for custom organization themes.

### Phase 13: Advanced Data Architecture
- [ ] **JSONLDB Evolution**:
    - [ ] Add versioning and column definitions to the first row of JSONL files.
    - [ ] Deprecate and remove `metadata.json` for databases.
- [ ] **Sharding**: Add support for sharding in JSONLDB to handle extremely large datasets.

### Phase 14: URL Standardization
- [ ] **URL Namespace**: Prefix page URLs with `/p/` to ensure clean routing (e.g., `mddb.app/p/<orgID>/<pageID>`).
- [ ] **Base64 ID Encoding**: Use Base64 URL-encoded characters (without padding) instead of raw numbers for `orgID` and page/note IDs to provide more compact and professional identifiers.
- [ ] **Separator Migration**: Change the ID/path separator from `-` to `+` because `-` is a reserved character in Base64 URL-encoding.

### Phase 15: CI/CD & Automation
- [ ] **CI Workflow**: GitHub Actions workflow for linting (backend/frontend) and testing on every push.
- [ ] **Release Workflow**: GitHub Actions workflow to build and release binaries (Linux, macOS, Windows) on new tags.

### Phase 16: Advanced Database UX & Power Features
- [ ] **Advanced Filtering & Sorting**: UI for multi-condition filters (AND/OR) and multi-column persistent sorting.
- [ ] **Property Management**: In-app UI for adding, renaming, and changing column types with data migration safety.
- [ ] **Inline Editing & Navigation**: Spreadsheet-like keyboard navigation (Tab/Arrows) and rapid inline cell editing.
- [ ] **View Customization**: Per-view column visibility toggle, column resizing, and drag-and-drop reordering.
- [ ] **Formulas**: Simple calculated properties based on other columns in the same record.
- [ ] **Bulk Actions**: Multi-select records for deletion or property updates.
- [ ] **Undo/Redo**: Global undo/redo support for document edits and database record changes.

## Future Considerations- **Notion Integration (via MCP)**: Fetch and sync data from Notion using the Model Context Protocol.
- **Publishing & Extensibility**: Public pages, custom domains, webhooks, and per-organization API keys.
- **Google Drive Integration**: Bi-directional sync/import/export for Google Docs (Markdown) and Google Sheets (JSONL).
- **SQLite Migration**: Migrate `data/db/*.json` to SQLite for better relational integrity and query performance.
- **Real-time Collaboration**: WebSocket-based sync.
- **Mobile App**: Native mobile clients using the REST API.
