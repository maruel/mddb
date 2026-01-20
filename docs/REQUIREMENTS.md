# mddb Requirements

## Status

**Project State: Production Prototype (Maturing)**
Most core functional requirements for a local-first markdown and database system are met. The architecture follows clean 3-layer separation (API/handlers → business logic/services → persistence/filestore+jsonldb) with no layering violations.

## Functional Requirements

### 1. Document Management
- [x] **CRUD**: Create, read, update, delete pages.
- [x] **Organization**: Logical hierarchical page organization (stored via nested numeric directories).
- [x] **Editor**: Full markdown editing with live preview.
- [x] **Assets**: Attach images and files directly to pages.
- [ ] **Interlinking**: Specialized UI for linking between pages (currently manual markdown links).
- [ ] **Backlinks**: Automatically track and display which pages link to the current page.
- [ ] **Graph View**: Visual representation of the organization's knowledge graph.

### 2. Database/Tables
- [x] **Schema**: Define database schemas (columns with types: text, number, select, multi_select, checkbox, date).
- [x] **Records**: Store and manage database records in JSONL format.
- [x] **Pagination**: Support for `offset` and `limit` to handle large datasets.
- [/] **Type Coercion**: SQLite-compatible type affinity system for consistent storage.
    - **Storage Classes**: Map all values to SQLite's 5 storage classes: NULL, INTEGER, REAL, TEXT, BLOB.
    - **Type Mapping**:
        - `text` → TEXT affinity (stores as TEXT)
        - `number` → NUMERIC affinity (stores as INTEGER if whole number, REAL otherwise)
        - `select` → TEXT affinity (stores as TEXT)
        - `multi_select` → TEXT affinity (stores as JSON array string)
        - `checkbox` → INTEGER affinity (stores as 0 or 1)
        - `date` → TEXT affinity (stores as ISO8601 string "YYYY-MM-DD" or "YYYY-MM-DD HH:MM:SS.SSS")
    - **Coercion Rules** (applied on write):
        - NUMERIC: Well-formed integer text → INTEGER; well-formed real text → REAL; float equal to integer → INTEGER; non-numeric text → TEXT.
        - INTEGER: Same as NUMERIC but forces integer representation.
        - REAL: Like NUMERIC but forces floating point.
        - TEXT: Numeric values converted to string representation.
        - BLOB: No coercion; stored as-is.
    - **NULL Handling**: Absent JSON keys represent NULL; use `omitzero`/`omitempty` struct tags.
    - **Future**: Enables seamless migration to SQLite backend.
- [/] **Advanced Query**: Complex filtering (nested AND/OR logic) and multi-column persistent sorting. Pagination with `offset`/`limit` implemented; complex filtering pending.
- [ ] **Property Editing**: Dynamic UI for schema modifications (adding/deleting columns, renaming, type conversion).
- [ ] **Relations**: Support for "Relation" column type to link records between different databases (Foreign Keys).
- [ ] **Rollups**: Aggregate data from linked records (e.g., sum of related "Cost" fields, count of linked tasks).
- [ ] **Formulas**: Calculated columns using simple expressions (e.g., `Price * Quantity`).

### 3. Media Management
- [x] **Storage**: Upload and store assets in page-specific namespaces.
- [x] **Serving**: Serve assets with proper MIME detection and caching.
- [x] **Formats**: Support common formats (PNG, JPG, GIF, WebP, SVG, PDF, etc.).
- [ ] **Gallery**: Dedicated asset management and gallery view.

### 4. User Experience (Notion-like)

- [ ] **Unified Sidebar**: Single hierarchical tree view for all content (pages and databases).
- [/] **Seamless Databases**: Databases are integrated into pages; every database is a page, and every page can contain database views (supported via `NodeTypeHybrid`).
- [x] **Database Views**: Flexible views (Table, Board, Gallery, Grid) that can be embedded or viewed as full pages.
- [ ] **View Customization**: Toggleable columns, adjustable column widths, and drag-and-drop reordering.
- [ ] **Interaction Polish**: Keyboard shortcuts for navigation, spreadsheet-like cell selection, and context menus for rows/columns.
- [ ] **Undo/Redo**: Support for undoing and redoing actions (document edits, record changes).
- [x] **Auto-save**: Automatic background saving of documents (2s debounce).
- [x] **Search**: Full-text search across all documents and databases with relevance scoring.
- [x] **History**: View and restore previous versions of pages (leveraging Git).
- [ ] **Real-time**: WebSocket-based real-time sync (future consideration).
- [ ] **Theme Support**: Customizable UI themes.
    - **Inspiration**: Investigate Hugo (https://gohugo.io) theme support for potential architectural patterns (optional).
    - **Customization**: Allow users/organizations to customize colors, fonts, and layout elements.

### 5. API & Integration

- [x] **REST API**: Comprehensive API for all operations (Pages, DBs, Records, Assets).
- [x] **Error Handling**: Structured error codes and detailed responses (Centralized `ErrorCode` union with HTTP status mapping).
- [x] **JSON Schema**: Schema generation for API types via `invopop/jsonschema` library.


### 6. Multi-User & Organization

- [x] **Identity**: User accounts and authentication (JWT/Sessions). Supporting the **Linear Model** (one user account, multiple organizations).
- [x] **OAuth2**: Login via Google or Microsoft accounts.
- [x] **Organizations**: Workspace isolation and shared access.
- [x] **Membership Model**: Many-to-many relationship between Users and Organizations to store organization-specific roles and settings.
- [x] **RBAC**: Role-Based Access Control (Admin, Editor, Viewer) at the organization level.
- [x] **Invitations**: Invite users to organizations via email; support assigning roles.
- [x] **Tiered Settings**: Support for Global User settings, Membership-specific settings (per-org), and Organization-wide settings.
- [ ] **Quotas**: Resource limits per organization (storage space, number of pages, max asset size).
- [/] **Org-Controlled Git**: Allow organizations to configure their own Git remote for data persistence.
    - **Custom Remotes**: Support for GitHub (via App/Personal Access Token), GitLab, and generic SSH/HTTPS remotes.
    - **Private Repositories**: Secure handling of credentials for pushing to private organization repositories.
    - **Automated Pushing**: Configurable "push on commit" or scheduled sync to the remote repository.
    - **Status**: API endpoints implemented (`GET/POST/DELETE /api/{orgID}/settings/git/remotes`, `POST .../push`); UI integration pending.
- [x] **Onboarding**: Streamlined setup process for new organizations.
    - **First-Login Onboarding**: Optional guided setup when a user first accesses their default organization.
    - **Configuration Questions**: Ask for organization name, initial members, and optional Git remote configuration.
    - **CLI Onboarding**: Simple configuration onboarding in the CLI, storing settings in an unversioned `.env` file in `data/`.
    - **Deferrable**: Users can skip onboarding and configure settings later via the workspace settings.

### 7. Globalization & Platform
- [ ] **i18n & l10n**: Frontend supports multiple languages and regional formatting; backend remains language and locale agnostic (returns error codes, not localized messages).
- [/] **PWA**: Progressive Web App support for installability and mobile-like experience. Install banner implemented; full offline support pending.
- [ ] **Offline Support**: Ability to work offline with robust data reconciliation upon reconnection.

### 8. Agent Readiness
- [ ] **Agent Context**: Inclusion of `AGENTS.md` in data repositories to provide context for AI agents.

### 9. AI & Intelligence
- [ ] **Semantic Search (RAG)**: Vector-based search allowing for natural language queries across the entire organization.
- [ ] **AI Writing Assistant**: In-editor tool for summarization, expansion, and content generation.
- [ ] **MCP Server**: Implement the Model Context Protocol (MCP) to allow LLMs and AI agents to safely browse, read, and edit the organization's data.

## Non-Functional Requirements

### Performance & Scalability
- [x] **Streaming**: Efficient line-by-line reading of records via `bufio.Scanner`.
- [x] **Scalability**: Designed to handle thousands of pages and large databases via pagination.
- [ ] **JSONLDB Sharding**: Support for sharding large databases in the JSONLDB storage engine.
- [x] **Lightweight**: Fast startup and low memory footprint.
- [x] **Caching**: Internal high-efficiency caching for frequently accessed pages, metadata, and database records to minimize disk I/O.

### Deployment & Architecture
- [x] **Self-Contained**: Single executable binary with embedded frontend (`go:embed`).
- [ ] **CLI Versioning**: Support `-version` flag to output build metadata (Git commit, build time) leveraging Go's `debug.ReadBuildInfo`.
- [ ] **CI/CD Workflows**: Automated GitHub Actions for CI (test/lint) and Release (cross-platform builds).
- [x] **Local-First**: Filesystem-based storage with no external database dependencies.
- [x] **Simplified Storage**: Unified JSONLDB format with versioning and column definitions in the first row, removing the need for separate `metadata.json`.
- [x] **API Contract Centralization**: All Request, Response, and DTO structures are centralized in the `internal/models` package as the single source of truth for the frontend.
- [x] **Encapsulation**: Handlers and storage implementation details are isolated from the API contract.
- [x] **Cross-Platform**: Compatible with Linux, macOS, and Windows.
- [x] **Type Generation**: Automatic TypeScript type generation from Go models via `tygo`, ensuring API contract synchronization between backend and frontend.
- [x] **Concurrent-Safe Storage**: JSONLDB uses RWMutex for concurrent reads, in-memory cache with cloning for mutation safety, and O(1) ID lookups.

### Data & Security
- [x] **Versioning**: Automatic Git commits in the `data/` directory for every change.
- [x] **Audit Trail**: Built-in history via standard Git commits.
- [x] **Path Safety**: Protection against file path traversal.
- [ ] **Sanitization**: Improved markdown sanitization (currently permits HTML).

### Storage Model
- [x] **Sortable IDs**: LUCI-inspired 64-bit IDs with time-sortable, collision-free properties.
    - **Structure**: 48-bit timestamp (10µs intervals) + 11-bit slice counter + 4-bit version.
    - **Encoding**: Custom sortable Base64 alphabet: `-0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ_abcdefghijklmnopqrstuvwxyz`.
    - **Properties**: Lexicographically sortable, monotonically increasing, ~89-year range from epoch (2026-01-01).
- [x] **Reserved Separators**: Uses `-` as first character of Base64 alphabet; separators handled by directory structure.
- [x] **Human Readable**: Transparent storage (Markdown, JSON, JSONL).
- [x] **Portable**: Zero-config "copy and paste" portability.
