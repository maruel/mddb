# Technical Documentation

## Storage Model

mddb uses a directory-per-page structure. Every page (document or database) is assigned a monotonically increasing numeric ID.

```
data/pages/
├── 1/                    # Page ID 1
│   ├── index.md          # Content + YAML Front Matter
│   └── photo.jpg         # Asset local to this page
└── 2/                    # Page ID 2 (Database)
    ├── index.md
    ├── metadata.json     # Schema
    └── data.jsonl        # Records (JSON Lines)
```

### Automatic Versioning
The `data/` directory is an initialized Git repository. Every change (create/update/delete) triggers an automatic commit via the `GitService`. This provides a built-in audit trail and history/restore capability.

## Embedded Build Process

mddb is distributed as a single self-contained binary. This is achieved using Go's `embed` package.

### Workflow
1. **Frontend Build**: The SolidJS frontend is compiled into static assets in `frontend/dist/`.
2. **Go Generate**: The `frontend/frontend.go` file uses `//go:embed dist/*` to include these assets.
3. **Compilation**: The Go compiler includes the static assets in the final binary.

### Reproducibility
The `frontend/dist/` directory is tracked in Git to ensure that the Go binary can be built deterministically without requiring a Node.js environment in the build pipeline (though `make build` will attempt to rebuild it if tools are available).

## Performance Optimizations

### Streaming Reads
Database records are stored in JSONL format, allowing for line-by-line streaming using `bufio.Scanner`. This keeps memory usage low even for large databases.

### API Pagination
Record retrieval supports `offset` and `limit` parameters to handle large datasets efficiently.
