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
- Developed page history UI and performance optimizations (pagination).
- Unified UI with a hierarchical sidebar and integrated database views.

**Deferred: Caching Layer**
- In-memory caching was removed to establish a stable storage foundation first.
- Future caching will be added once the storage APIs are finalized:
  - Node tree caching (expensive tree builds for sidebar)
  - Hot records caching (frequently accessed database records)
  - Thread-safe cache with proper invalidation on mutations

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
    - [x] Add `Row[T]` interface: `Clone() T` + `GetID() ID` + `Validate() error`. Table[T] requires Row[T] constraint.
    - [x] Header (SchemaHeader) required on all tables, unexported field with auto-discovery from JSON.
    - [x] Delete `Database` type and all its methods. `DataRecord` now implements `Row[DataRecord]` for use with `Table[DataRecord]`.
    - [x] Add `GetID() ID` and `Validate() error` methods to all Row types: `Organization`, `GitRemote`, `Invitation`, `User`/`userStorage`, `remoteSecret`, `Membership`, `DataRecord`.
    - [x] All Row types must have non-zero IDs. `Membership` and `remoteSecret` now have proper ID fields (no longer composite-key only).
    - [x] Updated all callers of `NewDatabase` in `filestore.go` to use `Table[DataRecord]`.
    - [x] Added public `Schema()` and `UpdateSchema()` methods to `Table[T]` for schema access/modification.
    - [x] Added `byID map[ID]int` index for O(1) lookups via `Get(id)` and duplicate detection on load/append/replace.
    - [x] Added `Delete(id)` and `Update(row)` methods for single-row mutations with persistence.
    - [x] Database metadata (Title, Created, Modified) now stored in separate `metadata.json` file per page.
    - **Future**: Extend schema discovery to support nested structs (object type) and slices (list type).
- [/] **JSONLDB Type Coercion (Part 3)**: SQLite-compatible type affinity for consistent storage.
    - [x] **Storage Classes**: Define 5 storage classes matching SQLite: NULL, INTEGER, REAL, TEXT, BLOB.
    - [x] **Affinity Mapping**: Map column types to affinities (jsonldb only handles primitive types):
        - `text` → TEXT (string storage)
        - `number` → NUMERIC (INTEGER if whole, REAL otherwise)
        - `checkbox` → INTEGER (0/1)
        - `date` → TEXT (ISO8601 format)
    - [x] **Coercion Logic**: Implement `CoerceValue(value any, affinity Affinity) any` function in `jsonldb/affinity.go`:
        - NUMERIC: Parse text to number; float-equal-to-int becomes INTEGER; non-numeric stays TEXT.
        - INTEGER: Force integer representation (truncate decimals).
        - REAL: Force float64 representation.
        - TEXT: Convert numbers to string.
        - BLOB: Pass through unchanged.
        - NULL: Omitted fields via `omitzero`/`omitempty` struct tags; absent JSON keys = NULL.
    - [x] **Write Path Integration**: Apply coercion in `DatabaseService.CreateRecord()` and `DatabaseService.UpdateRecord()`.
    - [ ] **Comparison Semantics**: Apply affinity rules during filtering/comparison operations.
    - [ ] **Migration Tool**: Optional migration script to coerce existing records to new type rules.
    - [x] **Higher-Level Column Types**: Implement `select` and `multi_select` column types in the models/storage layer (not jsonldb). These are UI/application concepts that map to TEXT affinity at storage level.
        - [x] Define `models.ColumnType` with both primitive types (text, number, checkbox, date) and high-level types (select, multi_select)
        - [x] Add `StorageType()` method to map high-level types to jsonldb storage types
        - [x] Add `Options []string` field to `models.Column` for select/multi_select options
        - [x] Update converters in filestore.go to handle type mapping between layers
        - [x] Update frontend components (DatabaseTable, DatabaseBoard) to use options for select dropdowns
        - [ ] Store column options in metadata.json (currently only in-memory via API)
        - [ ] Add validation for select values against defined options
- [ ] **Rich Property System (Part 4)**: Notion-style property types with clear layer separation.

    **Architecture Layers:**

    | Layer | Responsibility | Property Knowledge |
    |-------|---------------|-------------------|
    | `jsonldb` | Raw storage, type coercion | Primitives only: text, number, bool, date, blob, jsonb |
    | `models` | Domain types, API contracts | Full property types with configs |
    | `storage` | Business logic, validation | Computes derived values, validates against configs |

    **Property Type Hierarchy:**
    ```
    Primitive (stored directly in JSONL):
    ├── text      → jsonldb.text
    ├── number    → jsonldb.number
    ├── checkbox  → jsonldb.bool
    └── date      → jsonldb.date

    Enumerated (text storage + options config):
    ├── select       → jsonldb.text + options[]
    └── multi_select → jsonldb.jsonb + options[]

    Reference (ID storage + target config):
    ├── relation → jsonldb.jsonb (array of IDs) + target_database_id
    └── person   → jsonldb.jsonb (array of user IDs)

    Computed (no storage, calculated on read):
    ├── formula  → expression config, evaluated from other properties
    ├── rollup   → relation + target_property + aggregation
    ├── created_time    → auto from record.Created
    ├── last_edited_time → auto from record.Modified
    ├── created_by      → auto from audit log (future)
    └── last_edited_by  → auto from audit log (future)

    Rich Text (jsonb storage):
    ├── title → jsonldb.jsonb (rich text blocks)
    ├── rich_text → jsonldb.jsonb (rich text blocks)
    ├── url   → jsonldb.text + URL validation
    ├── email → jsonldb.text + email validation
    └── phone → jsonldb.text + phone validation

    Media (external storage reference):
    └── files → jsonldb.jsonb (array of asset references)
    ```

    **Storage Structure:**
    ```
    pages/{id}/
    ├── page.md           # Markdown content
    ├── metadata.json     # Title, timestamps, property configs
    │   {
    │     "title": "Tasks",
    │     "properties": {
    │       "Status": {
    │         "type": "select",
    │         "options": [
    │           {"id": "abc", "name": "Todo", "color": "gray"},
    │           {"id": "def", "name": "Done", "color": "green"}
    │         ]
    │       },
    │       "Assignee": {
    │         "type": "person"
    │       },
    │       "Due": {
    │         "type": "date"
    │       },
    │       "Related": {
    │         "type": "relation",
    │         "target_database_id": "xyz"
    │       }
    │     }
    │   }
    └── data.jsonl        # Row data (primitives only in schema header)
        {"version":"1.0","columns":[{"name":"Status","type":"text"},{"name":"Assignee","type":"jsonb"},...]}
        {"id":1,"data":{"Status":"abc","Assignee":["user1"],"Due":"2024-01-15"}}
    ```

    **Key Design Decisions:**
    - Select values stored as option IDs (not display names) for rename safety
    - Property configs in metadata.json, primitive schema in JSONL header
    - Computed properties never stored, always derived on read
    - Relations store target record IDs as jsonb arrays (supports multi-relation)
    - Validation happens in storage layer before write, using property configs
- [ ] **JSONLDB Sharding (Part 5)**: Add support for sharding in JSONLDB to handle extremely large datasets.

### Phase 14: URL Standardization
- [ ] **URL Namespace**: Prefix page URLs with `/p/` to ensure clean routing (e.g., `mddb.app/p/<orgID>/<pageID>`).
- [ ] **Base64 ID Encoding**: Use Base64 URL-encoded characters (without padding) instead of raw numbers for `orgID` and page/note IDs to provide more compact and professional identifiers.
- [ ] **Separator Migration**: Change the ID/path separator from `-` to `+` because `-` is a reserved character in Base64 URL-encoding.

### Phase 15: CI/CD & Automation
- [ ] **CI Workflow**: GitHub Actions workflow for linting (backend/frontend) and testing on every push.
- [ ] **Release Workflow**: GitHub Actions workflow to build and release binaries (Linux, macOS, Windows) on new tags.

### Phase 16: Database Views System

A comprehensive view system allowing multiple configurable views per database, with persistent filters, sorts, and layout customization.

#### Design Overview

**Core Concept**: A View is a saved configuration for displaying database records. Each database can have multiple views, each with its own type, filters, sorts, and column settings.

**Current State Analysis**:
- Frontend has 4 view types implemented: `table`, `grid`, `gallery`, `board`
- View mode is stored only in UI state (`viewMode` signal in App.tsx:47)
- No persistence layer for view configurations
- No filtering or sorting UI implemented
- Board view hardcodes grouping by first select column

#### View Data Model

```go
// View represents a saved view configuration for a database.
type View struct {
    ID          jsonldb.ID       `json:"id"`
    DatabaseID  jsonldb.ID       `json:"database_id"`   // Parent database
    Name        string           `json:"name"`          // User-defined name (e.g., "Active Tasks")
    Type        ViewType         `json:"type"`          // table, board, gallery, grid, calendar, timeline
    IsDefault   bool             `json:"is_default"`    // First view shown when opening database

    // Display Configuration
    Filters     FilterGroup      `json:"filters,omitempty"`      // Root filter group (AND/OR)
    Sorts       []SortConfig     `json:"sorts,omitempty"`        // Multi-column sorting
    Columns     []ColumnConfig   `json:"columns,omitempty"`      // Column visibility, order, width

    // View-type-specific settings
    Settings    ViewSettings     `json:"settings,omitempty"`

    Created     time.Time        `json:"created"`
    Modified    time.Time        `json:"modified"`
}

type ViewType string
const (
    ViewTypeTable    ViewType = "table"
    ViewTypeBoard    ViewType = "board"
    ViewTypeGallery  ViewType = "gallery"
    ViewTypeGrid     ViewType = "grid"
    ViewTypeCalendar ViewType = "calendar"  // Future
    ViewTypeTimeline ViewType = "timeline"  // Future
)
```

#### Filter System

```go
// FilterGroup represents a group of filters combined with AND or OR logic.
// Supports nested groups for complex expressions like: (A AND B) OR (C AND D)
type FilterGroup struct {
    Operator   FilterOperator `json:"operator"`   // "and" or "or"
    Conditions []FilterItem   `json:"conditions"` // Filters or nested groups
}

type FilterOperator string
const (
    FilterOperatorAnd FilterOperator = "and"
    FilterOperatorOr  FilterOperator = "or"
)

// FilterItem is either a Filter or a nested FilterGroup (discriminated union)
type FilterItem struct {
    Type   string       `json:"type"`   // "filter" or "group"
    Filter *Filter      `json:"filter,omitempty"`
    Group  *FilterGroup `json:"group,omitempty"`
}

// Filter represents a single filter condition on a property.
type Filter struct {
    Property string         `json:"property"` // Column name
    Operator FilterCondition `json:"operator"` // Comparison operator
    Value    any            `json:"value"`    // Comparison value (type depends on property)
}

type FilterCondition string
const (
    // Universal
    FilterEquals          FilterCondition = "equals"
    FilterNotEquals       FilterCondition = "not_equals"
    FilterIsEmpty         FilterCondition = "is_empty"
    FilterIsNotEmpty      FilterCondition = "is_not_empty"

    // Text
    FilterContains        FilterCondition = "contains"
    FilterNotContains     FilterCondition = "not_contains"
    FilterStartsWith      FilterCondition = "starts_with"
    FilterEndsWith        FilterCondition = "ends_with"

    // Number
    FilterGreaterThan     FilterCondition = "greater_than"
    FilterLessThan        FilterCondition = "less_than"
    FilterGreaterOrEqual  FilterCondition = "greater_or_equal"
    FilterLessOrEqual     FilterCondition = "less_or_equal"

    // Date
    FilterBefore          FilterCondition = "before"
    FilterAfter           FilterCondition = "after"
    FilterOnOrBefore      FilterCondition = "on_or_before"
    FilterOnOrAfter       FilterCondition = "on_or_after"
    FilterPastWeek        FilterCondition = "past_week"
    FilterPastMonth       FilterCondition = "past_month"
    FilterNextWeek        FilterCondition = "next_week"
    FilterNextMonth       FilterCondition = "next_month"

    // Select/Multi-select
    FilterIn              FilterCondition = "in"       // Value is in list
    FilterNotIn           FilterCondition = "not_in"   // Value not in list
)
```

#### Sort Configuration

```go
// SortConfig defines sorting for a single column.
type SortConfig struct {
    Property  string        `json:"property"`  // Column name
    Direction SortDirection `json:"direction"` // asc or desc
}

type SortDirection string
const (
    SortAsc  SortDirection = "asc"
    SortDesc SortDirection = "desc"
)
```

#### Column Configuration

```go
// ColumnConfig stores per-view column display settings.
type ColumnConfig struct {
    Property string `json:"property"`          // Column name (matches Property.Name)
    Visible  bool   `json:"visible"`           // Whether column is shown
    Width    int    `json:"width,omitempty"`   // Width in pixels (table view)
    Order    int    `json:"order"`             // Display order (0-indexed)
}
```

#### View-Type-Specific Settings

```go
// ViewSettings contains type-specific configuration.
type ViewSettings struct {
    // Board view
    GroupByProperty string   `json:"group_by_property,omitempty"` // Column to group cards by
    HiddenGroups    []string `json:"hidden_groups,omitempty"`     // Option IDs to hide

    // Gallery view
    CardSize        string   `json:"card_size,omitempty"`         // "small", "medium", "large"
    CoverProperty   string   `json:"cover_property,omitempty"`    // Image column for card cover

    // Grid view
    CardPreview     string   `json:"card_preview,omitempty"`      // "none", "content", "page"

    // Calendar view (future)
    DateProperty    string   `json:"date_property,omitempty"`     // Column for calendar placement

    // Timeline view (future)
    StartProperty   string   `json:"start_property,omitempty"`    // Start date column
    EndProperty     string   `json:"end_property,omitempty"`      // End date column
}
```

#### Storage Strategy

**Location**: Views are stored in `metadata.json` alongside database properties.

```
pages/{nodeID}/
├── index.md           # Markdown content (if hybrid)
├── metadata.json      # Extended with views array
│   {
│     "title": "Tasks",
│     "created": "...",
│     "modified": "...",
│     "properties": [...],
│     "views": [
│       {
│         "id": "abc123",
│         "name": "All Tasks",
│         "type": "table",
│         "is_default": true,
│         "filters": { "operator": "and", "conditions": [] },
│         "sorts": [{ "property": "Created", "direction": "desc" }],
│         "columns": [
│           { "property": "Name", "visible": true, "order": 0, "width": 300 },
│           { "property": "Status", "visible": true, "order": 1, "width": 120 }
│         ]
│       },
│       {
│         "id": "def456",
│         "name": "By Status",
│         "type": "board",
│         "settings": { "group_by_property": "Status" }
│       }
│     ]
│   }
└── data.jsonl         # Record data (unchanged)
```

**Rationale**:
- Views are tightly coupled to database schema (reference property names)
- Simplifies atomic updates (single file read/write)
- Views travel with database when exported/moved
- No separate file management complexity

#### API Endpoints

```
# List views for a database
GET /api/{orgID}/databases/{dbID}/views
Response: { "views": [View, ...] }

# Get single view
GET /api/{orgID}/databases/{dbID}/views/{viewID}
Response: View

# Create view
POST /api/{orgID}/databases/{dbID}/views
Body: { "name": "...", "type": "table", ... }
Response: View

# Update view
PUT /api/{orgID}/databases/{dbID}/views/{viewID}
Body: { "name": "...", "filters": {...}, ... }
Response: View

# Delete view
DELETE /api/{orgID}/databases/{dbID}/views/{viewID}
Response: 204 No Content

# Set default view
POST /api/{orgID}/databases/{dbID}/views/{viewID}/set-default
Response: View

# Reorder views
POST /api/{orgID}/databases/{dbID}/views/reorder
Body: { "view_ids": ["id1", "id2", "id3"] }
Response: { "views": [View, ...] }

# Get filtered/sorted records (view applied server-side)
GET /api/{orgID}/databases/{dbID}/records?view={viewID}&offset=0&limit=50
Response: { "records": [...], "total": 150 }
```

#### Implementation Tasks

- [ ] **Backend: View Model & Storage**
    - [ ] Define `View`, `Filter`, `FilterGroup`, `SortConfig`, `ColumnConfig`, `ViewSettings` in `models/view.go`
    - [ ] Update `DatabaseMetadata` struct to include `Views []View`
    - [ ] Add view CRUD methods to `DatabaseService`
    - [ ] Implement default view creation when database is created (table view showing all columns)

- [ ] **Backend: Filter & Sort Engine**
    - [ ] Implement `FilterEngine` in `storage/filter.go` for in-memory filtering
    - [ ] Support all filter conditions per property type
    - [ ] Implement nested FilterGroup evaluation (AND/OR trees)
    - [ ] Implement multi-column sorting with `sort.SliceStable`
    - [ ] Integrate with `GetRecordsPage()` to apply view config

- [ ] **Backend: API Handlers**
    - [ ] Add view CRUD handlers in `handlers/views.go`
    - [ ] Add `?view=` query param support to records endpoint
    - [ ] Return `total` count in filtered results for pagination UI

- [ ] **Frontend: View Management UI**
    - [ ] Add view tabs/dropdown in database header
    - [ ] Implement "Add View" modal with type selection
    - [ ] Add view rename/delete context menu
    - [ ] Persist selected view in URL or localStorage

- [ ] **Frontend: Filter Builder UI**
    - [ ] Create `FilterBuilder` component with add/remove conditions
    - [ ] Support nested groups with visual nesting indication
    - [ ] Property type-aware value inputs (text, number, date picker, select dropdown)
    - [ ] Real-time preview of filtered record count

- [ ] **Frontend: Sort Builder UI**
    - [ ] Create `SortBuilder` component with multi-column support
    - [ ] Drag-and-drop sort priority reordering
    - [ ] Quick-sort by clicking column headers (single-column shortcut)

- [ ] **Frontend: Column Configuration UI**
    - [ ] Column visibility toggle popover
    - [ ] Drag-and-drop column reordering
    - [ ] Column width resizing (table view)
    - [ ] Save column config on change

#### Design Decisions

1. **Server-side vs Client-side Filtering**: Filters are applied server-side to support pagination correctly. Total count is returned for UI pagination controls.

2. **View ID in URL**: Views can be referenced via URL query param (`?view=abc123`) for shareable links. If no view specified, use default view.

3. **Automatic Column Config**: When a new view is created, all columns are visible in schema order. Users customize from there.

4. **Filter Value Serialization**: Filter values are stored as `any` and coerced based on property type at evaluation time (consistent with existing type coercion in Part 3).

5. **View Migration**: When properties are renamed/deleted, views referencing them should gracefully degrade (ignore invalid filters/sorts/columns).

### Phase 17: Advanced Database UX & Power Features
- [ ] **Property Management**: In-app UI for adding, renaming, and changing column types with data migration safety.
- [ ] **Inline Editing & Navigation**: Spreadsheet-like keyboard navigation (Tab/Arrows) and rapid inline cell editing.
- [ ] **Formulas**: Simple calculated properties based on other columns in the same record.
- [ ] **Bulk Actions**: Multi-select records for deletion or property updates.
- [ ] **Undo/Redo**: Global undo/redo support for document edits and database record changes.

### Phase 18: Model Layer Separation ✓
*Completed.*
- Separated structs used for disk serialization (`storage/entity` package) from those used for API responses (`server/dto` package).
- Created API response types: `UserResponse`, `MembershipResponse`, `InvitationResponse`, `OrganizationResponse`, `GitRemoteResponse`, `NodeResponse`, `DataRecordResponse`.
- **dto package is fully self-contained**: Duplicated necessary types (Property, UserRole, NodeType, Settings, etc.) in dto to eliminate dependency on entity. This prevents accidental API contract changes when internal entity types evolve.
- **Package organization**: Entity types live under `internal/storage/entity` (persistence layer), DTO types live under `internal/server/dto` (API layer).
- Moved all entity→dto conversion functions to `handlers/convert.go` where both packages are already imported.
- Storage services return entity types with wrapper structs (`MembershipWithOrgName`, `UserWithMemberships`); handlers perform dto conversion.
- Updated all handlers to use local conversion functions instead of methods on domain models.
- Refactored `PopulateActiveContext()` to work with `UserResponse`.
- Updated frontend TypeScript types via tygo (generating only from dto package).

## Future Considerations
- **Notion Integration (via MCP)**: Fetch and sync data from Notion using the Model Context Protocol.
- **Publishing & Extensibility**: Public pages, custom domains, webhooks, and per-organization API keys.
- **Google Drive Integration**: Bi-directional sync/import/export for Google Docs (Markdown) and Google Sheets (JSONL).
- **SQLite Migration**: Migrate `data/db/*.json` to SQLite for better relational integrity and query performance. Type coercion (Phase 13 Part 3) ensures seamless migration by aligning JSONLDB storage with SQLite's type affinity system.
- **Real-time Collaboration**: WebSocket-based sync.
- **Mobile App**: Native mobile clients using the REST API.
