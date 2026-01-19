# mddb Implementation Plan

## Status

**Current Phase: Phase 9 (Advanced Sync & Onboarding) - In Progress**

- [x] Phase 1-4: Core Foundation
- [x] Phase 5: Polish & Features
- [x] Phase 6: Multi-tenant Foundation
- [x] Phase 7: Relational Metadata & Identity
- [x] Phase 8: Experience & Scaling
- [ ] Phase 9: Advanced Sync & Onboarding

## Overview

mddb is a Notion-like document and database system where all data is persisted as markdown files and JSON in a directory-based structure. The frontend (SolidJS) provides a rich user experience while the backend (Go) handles file operations, API endpoints, and business logic.

## Requirements

See [REQUIREMENTS.md](REQUIREMENTS.md) for complete functional and non-functional requirements.

## Design Fundamentals

### API Architecture

Standard RESTful API for Pages, Databases, Records, and Assets. 
**Multi-tenant routing**: Data-acting endpoints follow the pattern `/api/{orgID}/resource/...`. 
The backend validates that the `orgID` in the path matches the user's authenticated session.

See `README.md` and `API.md` for details.

## Implementation Phases

### Phases 1-4: Core Foundation ✓
*Completed.*
- Established directory-based storage, page CRUD, and dual-format databases (Markdown/JSONL).
- Implemented rich text editor with live preview and auto-save.
- Added asset management with directory-level namespaces.
- Set up deterministic builds with frontend assets embedded in a single Go binary.

### Phase 5: Polish & Features ✓
*Completed.*
- Enhanced error handling with structured responses.
- Implemented full-text search across pages and databases.
- Integrated automatic Git versioning for the data directory.
- Developed page history UI and performance optimizations (pagination, caching).
- Unified UI with a hierarchical sidebar and integrated database views.

### Phase 6: Multi-tenant Foundation ✓
*Completed.*
- Implemented JWT-based authentication and workspace isolation.
- Established per-organization Git submodules for data sovereignty.
- Developed RBAC middleware and multi-tenant path resolution.
- Integrated organization lifecycle management and cross-tenant isolation verification.

### Phase 7: Relational Metadata & Identity ✓
*Completed.*
- Implemented many-to-many User/Org membership model.
- Added invitation system and OAuth2 (Google/Microsoft) integration.
- Developed tiered settings for users, memberships, and organizations.

### Phase 8: Experience & Scaling ✓
*Completed.*
- Scoped search results to active organizations.
- Added organization switcher UI and unified API type generation (tygo).
- Implemented quota enforcement and administrative management UIs.

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
    - [x] **CLI Onboarding**: Implement a simple configuration wizard in the CLI.
    - [x] **CLI Versioning**: Implement `mddb -version` to display build metadata (Git commit, dirty state) using Go's build info.
    - [x] **Local Config**: Store CLI configuration in an unversioned `.env` file in `data/` (added to `.gitignore`).
    - [x] **Settings Integration**: Ensure all onboarding options are accessible and editable in Workspace Settings.

### Phase 10: Connectivity & Relations
- [ ] **Backlinks Indexing**: Efficient background indexing of internal markdown links to provide backlink lists.
- [ ] **Relational Data Model**: Update metadata schema to support record-level relations (foreign keys) between databases.
- [ ] **Relational UI**: UI components for selecting related records (popover/search) and displaying Rollups (aggregated data from linked records).
- [ ] **Graph View**: Interactive visualization of page and database connections.
- [ ] **Notion Research**: Document product design insights from Notion documentation in `docs/notion/`.

### Phase 11: AI Intelligence
- [ ] **Advanced Search**:
    - [ ] **BM25 Full-Text Search**: High-quality keyword search implementation.
    - [ ] **Vector Semantic Search**: Embedding generation for documents and records to support semantic search.
    - [ ] **LLM Reranking**: Use LLM to rerank search results for relevance.
    - **Inspiration**: https://github.com/tobi/qmd for search implementation patterns.
- [ ] **MCP Server**: Full implementation of the Model Context Protocol to expose documents/databases to AI agents.
- [ ] **In-Editor AI Actions**: Context-aware AI commands for content generation and summarization.
- [ ] **Data Repository Context**: Automatically add `AGENTS.md` to organization repositories for agent guidance.

### Phase 12: Globalization, Mobile & Theming
- [x] **i18n Infrastructure**: Implemented internationalization framework.
    - [x] Created `frontend/src/i18n/` with `@solid-primitives/i18n` integration.
    - [x] Defined `Dictionary` type interface (`types.ts`) covering all UI sections.
    - [x] Implemented `I18nProvider` context with `useI18n` hook and `translateError` helper.
    - [x] Migrated all user-facing components to use `t()` translator:
        - `App.tsx`, `Auth.tsx`, `WorkspaceSettings.tsx`, `Onboarding.tsx`
        - `DatabaseTable.tsx`, `DatabaseGrid.tsx`, `DatabaseGallery.tsx`, `DatabaseBoard.tsx`
    - [x] Locale persistence via `localStorage` and sync with user settings.
    - [x] English dictionary (`dictionaries/en.ts`) - ready for additional locales (fr, de, es).
- [x] **l10n**: Added French (`fr.ts`), German (`de.ts`), Spanish (`es.ts`) translations.
- [x] **PWA Support**: Configure manifest and service workers for Progressive Web App support.
    - [x] App icons (192x192, 512x512, apple-touch-icon, favicon.png)
    - [x] Web app manifest with standalone display mode
    - [x] Service worker with Workbox for offline caching
    - [x] Runtime caching for API calls (NetworkFirst) and assets (CacheFirst)
    - [x] iOS/Safari meta tags for home screen installation
    - [x] Install prompt UX with `beforeinstallprompt` event and dismissal persistence
- [ ] **Offline Mode**: Implement client-side storage and reconciliation logic for offline use.
- [ ] **Theme System**:
    - [ ] Research Hugo theme engine for inspiration.
    - [ ] Implement a flexible theming system (CSS variables/tokens).
    - [ ] Add support for custom organization themes.

### Phase 13: Advanced Data Architecture
- [x] **JSONLDB Evolution (Part 1)**:
    - [x] Add versioning and column definitions to the first row of JSONL files.
    - [x] Deprecate and remove `metadata.json` for databases.
- [x] **JSONLDB Unification (Part 2)**:
    - [x] Merge `Database` and `Table` into a single `Table[T]` struct.
    - [x] Add `Row[T]` interface: `Cloner[T]` + `GetID() ID`. Table[T] now requires Row[T] constraint.
    - [x] Header (SchemaHeader) required on all tables, unexported field with auto-discovery from JSON.
    - [x] Delete `Database` type and all its methods. `DataRecord` now implements `Row[DataRecord]` for use with `Table[DataRecord]`.
    - [x] Add `GetID() ID` method to: `Organization`, `GitRemote`, `Invitation`, `User`/`userStorage`, `remoteSecret`, `DataRecord`.
    - [x] `Membership` keeps composite key (`UserID`, `OrganizationID`), `GetID()` returns zero - no ID-based lookups, use `All()` with filtering.
    - [x] Updated all callers of `NewDatabase` in `filestore.go` to use `Table[DataRecord]`.
    - [x] Added public `Schema()` and `UpdateSchema()` methods to `Table[T]` for schema access/modification.
    - [x] Database metadata (Title, Created, Modified) now stored in separate `metadata.json` file per page.
    - **Future**: Extend schema discovery to support nested structs (object type) and slices (list type).
- [ ] **JSONLDB Type Coercion (Part 3)**: SQLite-compatible type affinity for consistent storage.
    - [ ] **Storage Classes**: Define 5 storage classes matching SQLite: NULL, INTEGER, REAL, TEXT, BLOB.
    - [ ] **Affinity Mapping**: Map existing column types to affinities:
        - `text` → TEXT (string storage)
        - `number` → NUMERIC (INTEGER if whole, REAL otherwise)
        - `select` → TEXT (string storage)
        - `multi_select` → TEXT (JSON array string)
        - `checkbox` → INTEGER (0/1)
        - `date` → TEXT (ISO8601 format)
    - [ ] **Coercion Logic**: Implement `coerceValue(value any, affinity Affinity) any` function:
        - NUMERIC: Parse text to number; float-equal-to-int becomes INTEGER; non-numeric stays TEXT.
        - INTEGER: Force integer representation (truncate decimals).
        - REAL: Force float64 representation.
        - TEXT: Convert numbers to string.
        - BLOB: Pass through unchanged.
        - NULL: Omitted fields via `omitzero`/`omitempty` struct tags; absent JSON keys = NULL.
    - [ ] **Write Path Integration**: Apply coercion in `Table.Create()` and `Table.Update()`.
    - [ ] **Comparison Semantics**: Apply affinity rules during filtering/comparison operations.
    - [ ] **Migration Tool**: Optional migration script to coerce existing records to new type rules.
- [ ] **JSONLDB Sharding (Part 4)**: Add support for sharding in JSONLDB to handle extremely large datasets.

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

## Future Considerations
- **Notion Integration (via MCP)**: Fetch and sync data from Notion using the Model Context Protocol.
- **Publishing & Extensibility**: Public pages, custom domains, webhooks, and per-organization API keys.
- **Google Drive Integration**: Bi-directional sync/import/export for Google Docs (Markdown) and Google Sheets (JSONL).
- **SQLite Migration**: Migrate `data/db/*.json` to SQLite for better relational integrity and query performance. Type coercion (Phase 13 Part 3) ensures seamless migration by aligning JSONLDB storage with SQLite's type affinity system.
- **Real-time Collaboration**: WebSocket-based sync.
- **Mobile App**: Native mobile clients using the REST API.
