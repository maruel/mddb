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
- [x] **JSONLDB Evolution (Part 1)**:
    - [x] Add versioning and column definitions to the first row of JSONL files.
    - [x] Deprecate and remove `metadata.json` for databases.
- [ ] **JSONLDB Sharding (Part 2)**: Add support for sharding in JSONLDB to handle extremely large datasets.

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
