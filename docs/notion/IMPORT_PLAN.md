# Notion Import Plan

Plan for extracting Notion workspace data and building mddb's view system.

## Problem Statement

1. **Data extraction**: Pull pages, databases, records, relations, rollups, formulas from Notion
2. **View definitions**: Notion API does not expose saved views (filters, sorts, groups, layout)
3. **View system**: mddb needs a view abstraction to match Notion's capabilities
4. **User experience**: Support both technical (CLI) and non-technical (Web UI) users

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
- [x] `internal/notion/client.go` - API client with rate limiting
- [x] `internal/notion/types.go` - Notion API response types
- [x] `internal/notion/extractor.go` - Basic extraction (pages, databases)
- [x] `internal/notion/mapper.go` - Type mapping for primitive properties
- [x] `internal/notion/writer.go` - Write to mddb storage format
- [x] `internal/notion/markdown.go` - Block to markdown conversion
- [x] `internal/notion/progress.go` - Progress reporting
- [x] `cmd/notion-import/main.go` - CLI entry point
- [x] Unit tests for mapper and markdown

**Deliverable**: CLI that imports pages and databases with primitive properties

### Phase 2: Relational Properties ✅
- [x] Add `PropertyTypeRelation` to mddb types
- [x] Add `RelationConfig` struct
- [x] Map Notion relations to mddb
- [x] Add `PropertyTypeRollup` with `RollupConfig`
- [x] Add `PropertyTypeFormula` with `FormulaConfig`
- [x] Store computed values (rollup/formula results from Notion)
- [x] Add `RollupAggregation` type and mapRollupAggregation function
- [x] Add relation resolution (PendingRelations map + ResolveRelations method)
- [x] Unit tests for relational property mapping
- [ ] Update frontend to display relation/rollup/formula fields (deferred to Phase 5/6)

**Deliverable**: CLI imports all property types including relations

### Phase 3: View System Foundation (Backend Complete)
- [x] Add `View` type and related structs to mddb (`views.go`)
- [x] Add `Filter`, `Sort`, `Group`, `ViewColumn` types
- [x] Add `FilterOp` and `SortDir` constants
- [x] Update `Node` struct to include `Views` field
- [x] Backend filter/sort application to records query (`query.go`)
- [x] Unit tests for query/filter/sort logic
- [ ] View CRUD API endpoints (deferred to Phase 5)
- [ ] Frontend `ViewSwitcher` component (deferred to Phase 5)
- [ ] Frontend `TableView` with column visibility/ordering (deferred to Phase 5)

**Deliverable**: Backend view types and query logic ready for use

### Phase 4: View Import ✅
- [x] `internal/notion/manifest.go` - YAML manifest parser
- [x] Validate manifest (version, required fields, valid view types)
- [x] Apply manifest views during import (`extractDatabase` adds views to node)
- [x] CLI `--views` flag
- [x] Error reporting for invalid view configs
- [x] Unit tests for manifest parsing and conversion

**Deliverable**: CLI can import views from manifest file

### Phase 5: Import API & Web UI
- [ ] `internal/server/handlers/import.go` - Import job management
- [ ] Import preview endpoint
- [ ] Async import with progress reporting
- [ ] `frontend/src/components/import/NotionImport.tsx` - Wizard
- [ ] `NotionConnect.tsx` - Token input / OAuth
- [ ] `NotionPreview.tsx` - Content selection tree
- [ ] `NotionProgress.tsx` - Real-time progress
- [ ] `NotionComplete.tsx` - Results summary

**Deliverable**: Non-technical users can import via web UI

### Phase 6: Advanced Views
- [ ] `BoardView.tsx` - Kanban board layout
- [ ] `GalleryView.tsx` - Card grid layout
- [ ] `CalendarView.tsx` - Calendar layout
- [ ] `FilterBuilder.tsx` - Visual filter editor
- [ ] `SortBuilder.tsx` - Sort configuration UI
- [ ] `GroupBuilder.tsx` - Grouping configuration

**Deliverable**: Full Notion-like view system

---

## Dependencies

| Dependency | Purpose | Required By |
|------------|---------|-------------|
| Notion integration token | API authentication | Phase 1 |
| `gopkg.in/yaml.v3` | Manifest parsing | Phase 4 |
| OAuth setup (optional) | Browser-based auth | Phase 5 |

## Risks & Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| Notion API changes | Extraction breaks | Pin API version, monitor changelog |
| Large workspace | Slow extraction, rate limits | Progress reporting, resumable state |
| Complex formulas | Can't evaluate in mddb | Store expression + Notion's computed value |
| Circular relations | Infinite loops | Track visited IDs, max depth |
| View manifest errors | Silent import failures | Strict validation, clear error messages |

## Success Criteria

1. **Data fidelity**: 100% of accessible Notion data extracted (pages, databases, records, all property types)
2. **Computed values**: Rollups and formulas preserve their Notion-computed values
3. **View accuracy**: Manifest-defined views render correctly in mddb
4. **UX options**: Both CLI (power users) and Web UI (end users) available
5. **Performance**: Small workspace (<100 items) imports in <2 minutes
