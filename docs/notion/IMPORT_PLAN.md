# Notion Import Plan

Plan for extracting Notion workspace data into mddb.

## Current Status

**Working**: Basic import of pages and databases with primitive property types.
Run via `go run ./backend/cmd/notion-import -token $NOTION_TOKEN -workspace "My Workspace" -output ./data`

**Issues Fixed**:
- ~~Relations stored as Notion IDs~~ → Now resolved to mddb IDs via two-pass extraction
- ~~Unique ID formatting bug~~ → Fixed with `fmt.Sprintf`
- ~~Nested blocks not rendered~~ → Block.Children field + recursive markdown converter

**Remaining Issues**:
- ~~No hierarchy (ParentID not set on nodes)~~ → Now tracked via nodes.jsonl manifest
- Child pages/databases referenced but not imported as separate nodes
- Assets not downloaded (Notion file URLs expire)

## Architecture Decision: Layered Approach

### Why Not Pure Frontend?

| Constraint | Impact |
|------------|--------|
| **CORS** | Notion API blocks browser-origin requests → must proxy through backend |
| **Rate limiting** | Precise timing easier in Go than async JS |
| **File system** | Browser can't write directly to mddb storage |
| **Long-running** | Browser tab must stay open; crashes lose progress |

**Reality**: Any frontend approach requires `Browser → Backend → Notion API`. The question is where the orchestration logic lives.

### Layered Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                        Interfaces                           │
├────────────────────────┬────────────────────────────────────┤
│   CLI (cmd/notion-import)   │   Web UI (frontend/src/...)   │
│   - Power users             │   - Non-technical users       │
│   - Automation/CI           │   - Interactive selection     │
│   - Headless operation      │   - OAuth flow                │
└────────────────────────┴────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                   API Layer (optional)                       │
│            internal/server/handlers/import.go                │
│   - POST /api/import/notion/connect     (OAuth initiate)    │
│   - GET  /api/import/notion/callback    (OAuth callback)    │
│   - POST /api/import/notion/start       (begin import)      │
│   - GET  /api/import/notion/status      (progress/results)  │
│   - POST /api/import/notion/cancel      (abort)             │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                    Core Logic (shared)                       │
│                   internal/notion/                           │
│   - client.go      (API client + rate limiting)             │
│   - extractor.go   (orchestration + progress reporting)     │
│   - mapper.go      (Notion → mddb type conversion)          │
│   - writer.go      (mddb storage format output)             │
│   - types.go       (Notion API response types)              │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                    mddb Storage                              │
│   data/{wsID}/{nodeID}/                                      │
│   - index.md, metadata.json, data.jsonl                     │
└─────────────────────────────────────────────────────────────┘
```

### Trade-off Summary

| Aspect | CLI | Web UI |
|--------|-----|--------|
| **Setup** | Token via flag/env | OAuth redirect flow |
| **Selection** | All or filter by ID | Visual tree picker |
| **Progress** | Stdout/stderr | Real-time UI updates |
| **Automation** | Yes (cron, CI) | No |
| **Target users** | Developers, admins | End users |
| **Implementation** | Phase 1 | Phase 4 |

**Build order**: Core logic → CLI → API endpoints → Web UI

---

## Part 1: Core Extraction Logic (`internal/notion/`)

### 1.1 Package Structure

```
internal/notion/
├── client.go       # HTTP client, auth, rate limiting
├── types.go        # Notion API response structs
├── extractor.go    # High-level extraction orchestration
├── mapper.go       # Notion → mddb type conversion
├── writer.go       # Write to mddb storage format
├── progress.go     # Progress reporting interface
└── errors.go       # Domain-specific errors
```

### 1.2 Client Implementation

```go
// client.go
package notion

import (
    "context"
    "net/http"
    "sync"
    "time"
)

const (
    BaseURL       = "https://api.notion.com/v1"
    APIVersion    = "2022-06-28"  // Pin version for stability
    RateLimit     = 3             // requests per second
    MinInterval   = 334 * time.Millisecond
)

type Client struct {
    token       string
    httpClient  *http.Client
    lastRequest time.Time
    mu          sync.Mutex
}

func NewClient(token string) *Client {
    return &Client{
        token:      token,
        httpClient: &http.Client{Timeout: 30 * time.Second},
    }
}

func (c *Client) do(ctx context.Context, method, path string, body any) (*http.Response, error) {
    c.throttle()
    // Build request with:
    // - Authorization: Bearer {token}
    // - Notion-Version: {APIVersion}
    // - Content-Type: application/json
}

func (c *Client) throttle() {
    c.mu.Lock()
    defer c.mu.Unlock()

    elapsed := time.Since(c.lastRequest)
    if elapsed < MinInterval {
        time.Sleep(MinInterval - elapsed)
    }
    c.lastRequest = time.Now()
}

// API methods
func (c *Client) Search(ctx context.Context, query string, filter *SearchFilter) (*SearchResponse, error)
func (c *Client) GetDatabase(ctx context.Context, id string) (*Database, error)
func (c *Client) QueryDatabase(ctx context.Context, id string, opts *QueryOptions) (*QueryResponse, error)
func (c *Client) GetPage(ctx context.Context, id string) (*Page, error)
func (c *Client) GetBlockChildren(ctx context.Context, id string, cursor string) (*BlocksResponse, error)
func (c *Client) GetPageProperty(ctx context.Context, pageID, propID string) (*PropertyResponse, error)
```

### 1.3 Extractor Implementation

```go
// extractor.go
package notion

import (
    "context"
)

type Extractor struct {
    client   *Client
    writer   *Writer
    progress ProgressReporter
    opts     ExtractOptions
}

type ExtractOptions struct {
    // Filter what to extract
    DatabaseIDs []string  // specific databases, empty = all
    PageIDs     []string  // specific pages, empty = all

    // Behavior
    IncludeContent   bool  // fetch page content (blocks)
    IncludeRelations bool  // resolve relation properties
    MaxDepth         int   // max nesting depth (0 = unlimited)

    // View manifest (optional)
    ViewManifestPath string
}

type ProgressReporter interface {
    OnStart(total int)
    OnProgress(current int, item string)
    OnWarning(msg string)
    OnError(err error)
    OnComplete(stats ExtractStats)
}

type ExtractStats struct {
    Pages      int
    Databases  int
    Records    int
    Assets     int
    Errors     int
    Duration   time.Duration
}

func NewExtractor(client *Client, writer *Writer, progress ProgressReporter) *Extractor

func (e *Extractor) Extract(ctx context.Context, opts ExtractOptions) (*ExtractStats, error) {
    // 1. Search for all accessible content
    // 2. Build dependency graph (for relations)
    // 3. Extract databases first (need schemas for records)
    // 4. Extract pages
    // 5. Extract nested content (blocks)
    // 6. Apply view manifest if provided
}

func (e *Extractor) ExtractDatabase(ctx context.Context, id string) error
func (e *Extractor) ExtractPage(ctx context.Context, id string) error
```

### 1.4 Progress Reporting

```go
// progress.go
package notion

import (
    "fmt"
    "io"
)

// CLIProgress writes to stdout/stderr
type CLIProgress struct {
    out io.Writer
    err io.Writer
}

func (p *CLIProgress) OnProgress(current int, item string) {
    fmt.Fprintf(p.out, "[%d] %s\n", current, item)
}

// ChannelProgress sends updates via channel (for API/WebSocket)
type ChannelProgress struct {
    updates chan<- ProgressUpdate
}

type ProgressUpdate struct {
    Type    string `json:"type"`    // "progress", "warning", "error", "complete"
    Current int    `json:"current,omitempty"`
    Total   int    `json:"total,omitempty"`
    Message string `json:"message,omitempty"`
}
```

### 1.5 Data Model Mapping

| Notion Concept | mddb Equivalent | Mapping Notes |
|----------------|-----------------|---------------|
| Page | Node (document) | `index.md` with content from blocks |
| Database | Node (table) | `metadata.json` with properties |
| Database Page | DataRecord | Row in `data.jsonl` |
| Property | Property | Direct type mapping (see below) |
| Relation | `PropertyTypeRelation` | Array of target node/record IDs |
| Rollup | `PropertyTypeRollup` | Cached computed value + config |
| Formula | `PropertyTypeFormula` | Expression string + cached value |
| Block | Markdown | Convert block tree to markdown |

### 1.6 Property Type Mapping

| Notion Type | mddb Type | Notes |
|-------------|-----------|-------|
| `title` | `text` | Primary title field |
| `rich_text` | `text` | Strip to plain text or preserve formatting |
| `number` | `number` | Direct |
| `select` | `select` | Map options |
| `multi_select` | `multi_select` | Map options |
| `date` | `date` | ISO8601 |
| `checkbox` | `checkbox` | Direct |
| `url` | `url` | Direct |
| `email` | `email` | Direct |
| `phone_number` | `phone` | Direct |
| `relation` | `relation` | **New type** - array of IDs |
| `rollup` | `rollup` | **New type** - config + cached value |
| `formula` | `formula` | **New type** - expression + cached value |
| `files` | (special) | Extract as assets |
| `people` | `text` | Convert to names (no user sync) |
| `created_time` | (metadata) | Map to record.Created |
| `last_edited_time` | (metadata) | Map to record.Modified |
| `created_by` | (skip) | No user mapping |
| `last_edited_by` | (skip) | No user mapping |

---

## Part 2: CLI Interface (`cmd/notion-import/`)

### 2.1 Command Structure

```
cmd/notion-import/
├── main.go         # Entry point, flag parsing
└── commands.go     # Subcommand implementations (if needed)
```

### 2.2 CLI Usage

```bash
# Basic: extract entire workspace
notion-import \
  --token=secret_xxx \
  --workspace=ws_abc123 \
  --output=./data

# Selective: specific database only
notion-import \
  --token=secret_xxx \
  --workspace=ws_abc123 \
  --database=db_xyz789 \
  --output=./data

# With view manifest
notion-import \
  --token=secret_xxx \
  --workspace=ws_abc123 \
  --views=views.yaml \
  --output=./data

# Dry run: show what would be imported
notion-import \
  --token=secret_xxx \
  --dry-run

# Resume interrupted import
notion-import \
  --token=secret_xxx \
  --workspace=ws_abc123 \
  --resume=./import-state.json \
  --output=./data
```

### 2.3 Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--token` | string | `$NOTION_TOKEN` | Integration token (required) |
| `--workspace` | string | (required) | Target mddb workspace ID |
| `--output` | string | `./data` | Output directory |
| `--database` | string[] | (all) | Specific database IDs to import |
| `--page` | string[] | (all) | Specific page IDs to import |
| `--views` | string | | View manifest YAML path |
| `--include-content` | bool | true | Fetch page content (blocks) |
| `--max-depth` | int | 0 | Max nesting depth (0=unlimited) |
| `--dry-run` | bool | false | Show what would be imported |
| `--resume` | string | | Resume from state file |
| `--verbose` | bool | false | Verbose output |

### 2.4 Output

```
$ notion-import --token=secret_xxx --workspace=ws_notion

Notion Import
=============
Connecting to Notion API...
Found 3 databases, 47 pages

[1/50] Database: Project Tracker (12 records)
[2/50] Database: Meeting Notes (8 records)
[3/50] Database: Team Directory (5 records)
[4/50] Page: Welcome
[5/50] Page: Getting Started
...
[50/50] Page: Archive/Old Notes

Complete!
---------
Databases: 3
Pages:     47
Records:   25
Assets:    12
Errors:    0
Duration:  45s

Output: ./data/ws_notion/
```

---

## Part 3: API Endpoints (`internal/server/handlers/import.go`)

### 3.1 Endpoints

```
# OAuth flow (if using OAuth instead of integration token)
GET  /api/import/notion/connect
     → Redirects to Notion OAuth authorization
     → Query: redirect_uri (where to return after auth)

GET  /api/import/notion/callback
     → Handles OAuth callback
     → Exchanges code for token, stores in session
     → Redirects to frontend import UI

# Import operations
POST /api/import/notion/preview
     → Lists available databases/pages from Notion
     → Request: { token?: string } (or uses session token)
     → Response: { databases: [...], pages: [...] }

POST /api/import/notion/start
     → Begins async import job
     → Request: {
         workspace_id: string,
         database_ids?: string[],
         page_ids?: string[],
         views_manifest?: string
       }
     → Response: { job_id: string }

GET  /api/import/notion/status/{jobID}
     → Returns import job status
     → Response: {
         status: "running" | "complete" | "failed" | "cancelled",
         progress: { current: 10, total: 50, message: "Importing..." },
         stats?: { databases: 3, pages: 47, ... },
         error?: string
       }

POST /api/import/notion/cancel/{jobID}
     → Cancels running import
     → Response: { cancelled: true }

# WebSocket for real-time updates (optional)
WS   /api/import/notion/stream/{jobID}
     → Streams progress updates
```

### 3.2 Job Management

```go
// internal/server/handlers/import.go

type ImportJob struct {
    ID         string
    WorkspaceID string
    Status     JobStatus
    Progress   ProgressUpdate
    Stats      *notion.ExtractStats
    Error      error
    Cancel     context.CancelFunc
    StartedAt  time.Time
}

type ImportHandler struct {
    jobs     map[string]*ImportJob
    mu       sync.RWMutex
    notion   *notion.Client
    store    *content.FileStore
}

func (h *ImportHandler) StartImport(w http.ResponseWriter, r *http.Request) {
    // 1. Parse request
    // 2. Create job with unique ID
    // 3. Start goroutine with extractor
    // 4. Return job ID immediately
}

func (h *ImportHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
    // Return current job status
}
```

---

## Part 4: Web UI (`frontend/src/components/import/`)

### 4.1 Component Structure

```
frontend/src/components/import/
├── NotionImport.tsx        # Main import wizard
├── NotionConnect.tsx       # OAuth connection step
├── NotionPreview.tsx       # Database/page selection tree
├── NotionProgress.tsx      # Import progress display
├── NotionComplete.tsx      # Import results summary
└── ViewManifestEditor.tsx  # Optional: edit view manifest in-browser
```

### 4.2 Import Wizard Flow

```
┌─────────────────────────────────────────────────────────────┐
│  Step 1: Connect                                            │
│  ┌─────────────────────────────────────────────────────┐   │
│  │  [Connect to Notion]                                 │   │
│  │                                                      │   │
│  │  Or paste integration token:                         │   │
│  │  ┌────────────────────────────────────────────────┐ │   │
│  │  │ secret_xxx...                                  │ │   │
│  │  └────────────────────────────────────────────────┘ │   │
│  └─────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
                              ▼
┌─────────────────────────────────────────────────────────────┐
│  Step 2: Select Content                                     │
│  ┌─────────────────────────────────────────────────────┐   │
│  │  ☑ Project Tracker (database, 12 records)           │   │
│  │  ☑ Meeting Notes (database, 8 records)              │   │
│  │  ☐ Team Directory (database, 5 records)             │   │
│  │  ☑ Welcome (page)                                   │   │
│  │  ☑ Getting Started (page)                           │   │
│  │    ☑ Quick Start Guide (child page)                 │   │
│  │    ☐ Advanced Topics (child page)                   │   │
│  └─────────────────────────────────────────────────────┘   │
│                                                             │
│  Target workspace: [ws_notion ▼]                            │
│                                                             │
│  [Start Import]                                             │
└─────────────────────────────────────────────────────────────┘
                              ▼
┌─────────────────────────────────────────────────────────────┐
│  Step 3: Progress                                           │
│  ┌─────────────────────────────────────────────────────┐   │
│  │  Importing... 23/50                                  │   │
│  │  ████████████████░░░░░░░░░░░░░░  46%                │   │
│  │                                                      │   │
│  │  Current: Page "Meeting Notes/2024-01-15"           │   │
│  └─────────────────────────────────────────────────────┘   │
│                                                             │
│  [Cancel]                                                   │
└─────────────────────────────────────────────────────────────┘
                              ▼
┌─────────────────────────────────────────────────────────────┐
│  Step 4: Complete                                           │
│  ┌─────────────────────────────────────────────────────┐   │
│  │  ✓ Import Complete                                   │   │
│  │                                                      │   │
│  │  Databases:  3                                       │   │
│  │  Pages:      47                                      │   │
│  │  Records:    25                                      │   │
│  │  Assets:     12                                      │   │
│  │  Duration:   45s                                     │   │
│  │                                                      │   │
│  │  ⚠ 2 warnings (click to view)                       │   │
│  └─────────────────────────────────────────────────────┘   │
│                                                             │
│  [View Imported Content]  [Import More]                     │
└─────────────────────────────────────────────────────────────┘
```

### 4.3 State Management

```typescript
// frontend/src/stores/importStore.ts

interface ImportState {
  step: 'connect' | 'select' | 'progress' | 'complete';
  token: string | null;
  preview: NotionPreview | null;
  selected: Set<string>;
  jobId: string | null;
  progress: ProgressUpdate | null;
  result: ImportResult | null;
  error: string | null;
}

const [importState, setImportState] = createStore<ImportState>({
  step: 'connect',
  token: null,
  preview: null,
  selected: new Set(),
  jobId: null,
  progress: null,
  result: null,
  error: null,
});
```

---

## Part 5: mddb View System

### 5.1 New Types (`internal/storage/content/views.go`)

```go
package content

// View represents a saved table view configuration.
type View struct {
    ID      string   `json:"id"`
    Name    string   `json:"name"`
    Type    ViewType `json:"type"`
    Default bool     `json:"default,omitempty"`

    // Display configuration
    Columns []ViewColumn `json:"columns,omitempty"`

    // Data shaping
    Filters []Filter `json:"filters,omitempty"`
    Sorts   []Sort   `json:"sorts,omitempty"`
    Groups  []Group  `json:"groups,omitempty"`
}

type ViewType string

const (
    ViewTypeTable    ViewType = "table"
    ViewTypeBoard    ViewType = "board"
    ViewTypeGallery  ViewType = "gallery"
    ViewTypeList     ViewType = "list"
    ViewTypeCalendar ViewType = "calendar"
)

type ViewColumn struct {
    Property string `json:"property"`
    Width    int    `json:"width,omitempty"`
    Visible  bool   `json:"visible"`
}

type Filter struct {
    Property string   `json:"property,omitempty"`
    Operator FilterOp `json:"operator,omitempty"`
    Value    any      `json:"value,omitempty"`
    And      []Filter `json:"and,omitempty"`
    Or       []Filter `json:"or,omitempty"`
}

type FilterOp string

const (
    FilterOpEquals       FilterOp = "equals"
    FilterOpNotEquals    FilterOp = "not_equals"
    FilterOpContains     FilterOp = "contains"
    FilterOpNotContains  FilterOp = "not_contains"
    FilterOpStartsWith   FilterOp = "starts_with"
    FilterOpEndsWith     FilterOp = "ends_with"
    FilterOpGreaterThan  FilterOp = "gt"
    FilterOpLessThan     FilterOp = "lt"
    FilterOpGreaterEqual FilterOp = "gte"
    FilterOpLessEqual    FilterOp = "lte"
    FilterOpIsEmpty      FilterOp = "is_empty"
    FilterOpIsNotEmpty   FilterOp = "is_not_empty"
)

type Sort struct {
    Property  string  `json:"property"`
    Direction SortDir `json:"direction"`
}

type SortDir string

const (
    SortAsc  SortDir = "asc"
    SortDesc SortDir = "desc"
)

type Group struct {
    Property string `json:"property"`
    Hidden   []any  `json:"hidden,omitempty"`
}
```

### 5.2 New Property Types

```go
// Add to internal/storage/content/types.go

const (
    // ... existing types ...

    // PropertyTypeRelation links to records in another table.
    PropertyTypeRelation PropertyType = "relation"

    // PropertyTypeRollup aggregates values from related records.
    PropertyTypeRollup PropertyType = "rollup"

    // PropertyTypeFormula computes a value from other properties.
    PropertyTypeFormula PropertyType = "formula"
)

// RelationConfig defines a relation property's target.
type RelationConfig struct {
    TargetNodeID jsonldb.ID `json:"target_node_id"`
    IsDualLink   bool       `json:"is_dual_link,omitempty"`
    DualProperty string     `json:"dual_property,omitempty"`
}

// RollupConfig defines how to aggregate related values.
type RollupConfig struct {
    RelationProperty string       `json:"relation_property"`
    TargetProperty   string       `json:"target_property"`
    Aggregation      RollupAgg    `json:"aggregation"`
}

type RollupAgg string

const (
    RollupCount       RollupAgg = "count"
    RollupCountValues RollupAgg = "count_values"
    RollupSum         RollupAgg = "sum"
    RollupAvg         RollupAgg = "average"
    RollupMin         RollupAgg = "min"
    RollupMax         RollupAgg = "max"
    RollupShowAll     RollupAgg = "show_all"
)

// FormulaConfig defines a computed property.
type FormulaConfig struct {
    Expression string `json:"expression"`
}

// Extended Property struct
type Property struct {
    Name     string       `json:"name"`
    Type     PropertyType `json:"type"`
    Required bool         `json:"required,omitempty"`
    Options  []SelectOption `json:"options,omitempty"`

    // Type-specific configuration
    RelationConfig *RelationConfig `json:"relation_config,omitempty"`
    RollupConfig   *RollupConfig   `json:"rollup_config,omitempty"`
    FormulaConfig  *FormulaConfig  `json:"formula_config,omitempty"`
}
```

### 5.3 Updated metadata.json Schema

```json
{
  "properties": [
    { "name": "Title", "type": "text" },
    { "name": "Status", "type": "select", "options": [...] },
    { "name": "Due Date", "type": "date" },
    {
      "name": "Assignee",
      "type": "relation",
      "relation_config": {
        "target_node_id": "01JWAB...",
        "is_dual_link": true,
        "dual_property": "Tasks"
      }
    },
    {
      "name": "Task Count",
      "type": "rollup",
      "rollup_config": {
        "relation_property": "Tasks",
        "target_property": "Title",
        "aggregation": "count"
      }
    }
  ],
  "views": [
    {
      "id": "default",
      "name": "All Items",
      "type": "table",
      "default": true,
      "columns": [
        { "property": "Title", "visible": true, "width": 300 },
        { "property": "Status", "visible": true },
        { "property": "Due Date", "visible": true }
      ],
      "sorts": [
        { "property": "Due Date", "direction": "asc" }
      ]
    },
    {
      "id": "by-status",
      "name": "By Status",
      "type": "board",
      "groups": [
        { "property": "Status" }
      ],
      "filters": [
        { "property": "Archived", "operator": "equals", "value": false }
      ]
    }
  ]
}
```

### 5.4 View API Endpoints

```
# View CRUD
GET    /api/ws/{wsID}/nodes/{nodeID}/views
POST   /api/ws/{wsID}/nodes/{nodeID}/views
GET    /api/ws/{wsID}/nodes/{nodeID}/views/{viewID}
PUT    /api/ws/{wsID}/nodes/{nodeID}/views/{viewID}
DELETE /api/ws/{wsID}/nodes/{nodeID}/views/{viewID}

# Query with view applied (server-side filtering/sorting)
GET    /api/ws/{wsID}/nodes/{nodeID}/records?view={viewID}

# Alternative: client-side filtering (return view config + all records)
GET    /api/ws/{wsID}/nodes/{nodeID}/records?include_view={viewID}
```

---

## Part 6: View Manifest for Import

### 6.1 Manifest Format

Since Notion API doesn't expose view definitions, users document views manually:

```yaml
# views.yaml
version: 1

databases:
  # Match by Notion database ID
  - notion_id: "abc123def456"
    views:
      - name: "All Tasks"
        type: table
        default: true
        columns:
          - property: "Title"
            width: 300
          - property: "Status"
          - property: "Due Date"
          - property: "Assignee"
        sorts:
          - property: "Due Date"
            direction: asc

      - name: "By Status"
        type: board
        group_by: "Status"
        hidden_groups:
          - "Archived"
        filters:
          - property: "Archived"
            operator: not_equals
            value: true

      - name: "Calendar"
        type: calendar
        date_property: "Due Date"

      - name: "Overdue"
        type: table
        filters:
          - and:
              - property: "Due Date"
                operator: lt
                value: "@today"
              - property: "Status"
                operator: not_equals
                value: "Done"

  - notion_id: "xyz789"
    views:
      - name: "Gallery"
        type: gallery
        cover_property: "Image"
        title_property: "Name"
```

### 6.2 Manifest Parser

```go
// internal/notion/manifest.go

type ViewManifest struct {
    Version   int                  `yaml:"version"`
    Databases []DatabaseViewConfig `yaml:"databases"`
}

type DatabaseViewConfig struct {
    NotionID string       `yaml:"notion_id"`
    Views    []ViewConfig `yaml:"views"`
}

type ViewConfig struct {
    Name         string         `yaml:"name"`
    Type         string         `yaml:"type"`
    Default      bool           `yaml:"default"`
    Columns      []ColumnConfig `yaml:"columns,omitempty"`
    Sorts        []SortConfig   `yaml:"sorts,omitempty"`
    Filters      []FilterConfig `yaml:"filters,omitempty"`
    GroupBy      string         `yaml:"group_by,omitempty"`
    HiddenGroups []string       `yaml:"hidden_groups,omitempty"`
    DateProperty string         `yaml:"date_property,omitempty"`
    // Gallery-specific
    CoverProperty string `yaml:"cover_property,omitempty"`
    TitleProperty string `yaml:"title_property,omitempty"`
}

func ParseManifest(path string) (*ViewManifest, error)
func (m *ViewManifest) ViewsForDatabase(notionID string) []ViewConfig
```

---

## Implementation Phases

### Phase 1: Core Extraction (MVP) ✅

Complete. Basic CLI imports pages/databases with primitive properties.

Files: `client.go`, `types.go`, `extractor.go`, `mapper.go`, `writer.go`, `markdown.go`, `progress.go`, `cmd/notion-import/main.go`

### Phase 2: Bug Fixes & Data Fidelity

#### 2.1 Bug Fixes ✅

| Bug | Location | Status |
|-----|----------|--------|
| Unique ID formatting | `mapper.go` | ✅ Fixed: `fmt.Sprintf("%s-%d", ...)` |
| Relations not resolved | `extractor.go` | ✅ Fixed: Two-pass extraction with `AssignRecordID()` |
| Relation values as Notion IDs | `mapper.go` | ✅ Fixed: Resolves to mddb IDs, prefixes unresolved with `notion:` |

#### 2.2 Hierarchy & Structure (Complete)

- [x] Set `ParentID` on nodes based on Notion's `Parent` field
- [x] Pre-assign node IDs before mapping for parent resolution
- [x] Export hierarchy to `nodes.jsonl` manifest file
- [x] Import child pages/databases discovered in block content
- [x] Resolve child_page/child_database block links to mddb node IDs
- [x] Deduplicate imports (items found in both search and block content)
- [ ] Track Notion→mddb ID mapping persistently for incremental imports

#### 2.3 Nested Content ✅

- [x] Block.Children field added to store nested blocks
- [x] `GetBlockChildrenRecursive` populates Children instead of flattening
- [x] `blocksToMarkdownRecursive` renders nested content with proper indentation
- [x] Table header separator row added for markdown tables

#### 2.4 Assets (Partial)

- [x] Download images/videos/files from page blocks before URLs expire
- [x] Store in `{nodeDir}/assets/` directory with hash-prefixed filenames
- [x] Update markdown references to use local paths
- [x] Track download stats in extraction summary
- [ ] Download files from Files property in database records
- [ ] Download page covers and icons

### Phase 3: Relational Properties ✅

Types exist (`relation`, `rollup`, `formula` in `content/types.go`). Relation resolution implemented.

- [x] `AssignRecordID()` pre-assigns mddb IDs before mapping
- [x] Two-pass extraction: first collect all row IDs, then map with resolution
- [x] Relation values stored as mddb IDs (or `notion:ID` for unresolved cross-database refs)
- [x] Rollup/formula values preserved from Notion's computed results

### Phase 4: View System ✅

Backend complete (`views.go`, `query.go`). View manifest import works via `--views` flag.

### Phase 5: Import API & Web UI (Future)

Not started. Requires Phase 2 completion first.

### Phase 6: Advanced Views (Future)

Frontend view components (Board, Gallery, Calendar). Deferred until basic import is solid

---

## Testing

```bash
# Full import test
./tmp/test_import.sh

# Check output structure
find tmp/import -type f

# View a database's records
cat tmp/import/*/XXXX/data.jsonl | head -5

# View a page's content
cat tmp/import/*/XXXX/index.md
```

## Next Steps (Priority Order)

1. ~~**Fix unique ID bug**~~ ✅ Fixed: `fmt.Sprintf("%s-%d", prefix, n)`
2. ~~**Wire up relation resolution**~~ ✅ Fixed: Two-pass extraction pre-assigns record IDs
3. ~~**Fix nested block rendering**~~ ✅ Fixed: Block.Children field + recursive markdown
4. ~~**Set ParentID**~~ ✅ Fixed: Pre-assign node IDs + `nodes.jsonl` manifest
5. ~~**Add asset downloading**~~ ✅ Partial: Page blocks (image/video/file/pdf) downloaded to `assets/`
6. ~~**Import child pages/databases**~~ ✅ Fixed: Recursively imports children with deduplication
7. **Files property assets** - Download files from database record Files properties
8. **Incremental imports** - Track Notion→mddb ID mapping persistently

## Dependencies

| Dependency | Status |
|------------|--------|
| Notion integration token | ✅ Working |
| `gopkg.in/yaml.v3` | ✅ Working (manifest parsing) |

## Success Criteria

1. **Data fidelity**: All pages, databases, records imported with correct property values
2. **Relations work**: Relation properties link to correct mddb records
3. **Assets local**: Files/images downloaded and accessible offline
4. **Hierarchy preserved**: Parent-child relationships match Notion structure
