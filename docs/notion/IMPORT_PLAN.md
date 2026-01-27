# Notion Import

Import Notion workspace data into mddb.

## Quick Start

```bash
# Import entire workspace
go run ./backend/cmd/notion-import \
  -token $NOTION_TOKEN \
  -workspace "My Workspace" \
  -output ./data

# With view manifest
go run ./backend/cmd/notion-import \
  -token $NOTION_TOKEN \
  -workspace "My Workspace" \
  -views views.yaml \
  -output ./data

# Dry run
go run ./backend/cmd/notion-import \
  -token $NOTION_TOKEN \
  -dry-run
```

## Features

| Feature | Status |
|---------|--------|
| Pages and databases | Done |
| All property types (text, number, select, date, etc.) | Done |
| Relation properties (resolved to mddb IDs) | Done |
| Rollup/formula values (cached from Notion) | Done |
| Nested blocks (lists, toggles, columns) | Done |
| Parent-child hierarchy | Done |
| Asset downloading (images, files, videos) | Done |
| Files property in database records | Done |
| Page/database icons and covers | Done |
| Incremental imports (ID mapping persistence) | Done |
| View manifest import | Done |
| Web UI | Future |
| OAuth flow | Future |

## Output Structure

```
{output}/{workspace}/
├── nodes.jsonl           # Manifest of all nodes with hierarchy
├── notion_id_mapping.json       # Notion ID → mddb ID mapping (for incremental imports)
└── {nodeID}/
    ├── index.md          # Page content (documents)
    ├── data.jsonl        # Records with schema header (tables)
    ├── metadata.json     # Views configuration (if any)
    └── assets/           # Downloaded images/files
        └── {hash}-{filename}
```

## CLI Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-token` | `$NOTION_TOKEN` | Integration token (required) |
| `-workspace` | (required) | Target workspace name |
| `-output` | `./data` | Output directory |
| `-database` | (all) | Specific database IDs |
| `-page` | (all) | Specific page IDs |
| `-views` | | View manifest YAML path |
| `-include-content` | true | Fetch page blocks |
| `-max-depth` | 0 | Max nesting depth (0=unlimited) |
| `-dry-run` | false | Show what would be imported |
| `-verbose` | false | Verbose output |

## Incremental Imports

Re-running the import on an existing workspace:

1. Loads `notion_id_mapping.json` to reuse existing mddb IDs
2. Clears and rewrites `nodes.jsonl` and `data.jsonl` files
3. Preserves IDs so external references remain valid

## Property Type Mapping

| Notion Type | mddb Type | Notes |
|-------------|-----------|-------|
| `title` | `text` | Plain text |
| `rich_text` | `markdown` | Inline markdown (bold, italic, links) |
| `number` | `number` | Direct |
| `select`, `status` | `select` | With options |
| `multi_select` | `multi_select` | With options |
| `date` | `date` | Unix timestamp |
| `checkbox` | `checkbox` | Direct |
| `url` | `url` | Direct |
| `email` | `email` | Direct |
| `phone_number` | `phone` | Direct |
| `relation` | `relation` | Array of mddb IDs |
| `rollup` | `rollup` | Cached value + config |
| `formula` | `formula` | Cached value + expression |
| `files` | `text` | Local asset paths |
| `people` | `text` | Comma-separated names |
| `unique_id` | `text` | `PREFIX-123` format |

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                        Interfaces                           │
├────────────────────────┬────────────────────────────────────┤
│   CLI (cmd/notion-import)   │   Web UI (future)            │
└────────────────────────┴────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                    Core Logic                               │
│                   internal/notion/                          │
│   client.go      - API client + rate limiting               │
│   extractor.go   - Orchestration + progress                 │
│   mapper.go      - Notion → mddb type conversion            │
│   writer.go      - mddb storage format output               │
│   markdown.go    - Block → markdown conversion              │
│   assets.go      - Asset downloading                        │
│   types.go       - Notion API response types                │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                    mddb Storage                             │
│   {workspace}/{nodeID}/                                     │
│   index.md, metadata.json, data.jsonl, assets/              │
└─────────────────────────────────────────────────────────────┘
```

## View Manifest

Since Notion API doesn't expose view definitions, configure views via YAML:

```yaml
version: 1
databases:
  - notion_id: "abc123def456"
    views:
      - name: "All Tasks"
        type: table
        default: true
        columns:
          - property: "Title"
            width: 300
          - property: "Status"
        sorts:
          - property: "Due Date"
            direction: asc

      - name: "By Status"
        type: board
        group_by: "Status"
        filters:
          - property: "Archived"
            operator: not_equals
            value: true
```

## Future Work

- **Web UI**: Visual import wizard with OAuth flow
- **Selective import**: Tree picker for choosing specific pages/databases
- **Real-time progress**: WebSocket-based progress updates
- **Advanced views**: Board, Gallery, Calendar frontend components
