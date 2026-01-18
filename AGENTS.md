# AGENTS.md

Development guidelines and commands for mddb project.

## Project Overview

mddb is a Notion-like document and database system. Frontend built with SolidJS, backend with Go. Data stored
as markdown files and images.

**Storage Model:**
- All content lives in the `data/pages/` directory as numbered directories (1, 2, 3, etc.)
- Each directory is a "page" containing:
  - `index.md` - Content with YAML front matter for metadata (title, tags, etc.)
  - `favicon.ico` (optional) - Icon for the page (can be .ico, .avif, or .png)
  - `metadata.json` (databases only) - Database schema and column definitions
  - `data.jsonl` (databases only) - Database records, one per line
  - Other assets (images, files, etc.) - Images, attachments, etc.
- Each page/database is a directory with its own namespace for contained assets
- Hierarchical organization via nested directories (e.g., `1/subfolder/2/index.md`)

See README.md for project overview and docs/PLAN.md for implementation roadmap.

## Directory Structure

```
mddb/
├── backend/                    # Go backend code
│   ├── cmd/mddb/               # Application entry point
│   │   └── main.go             # Server startup and configuration
│   ├── internal/
│   │   ├── models/             # Data models
│   │   ├── server/             # HTTP server and routing
│   │   │   ├── router.go       # Route definitions and SPA handler
│   │   │   ├── embedded.go     # Embedded frontend handler
│   │   │   ├── handler_wrapper.go # Generic handler wrapper with path param extraction
│   │   │   └── handlers/       # HTTP request handlers by feature
│   │   ├── storage/            # File system operations
│   │   └── utils/              # Utilities
│   └── frontend/               # Go embedding for frontend assets
├── frontend/                   # SolidJS frontend
│   ├── src/                    # Frontend source code
│   │   ├── index.tsx           # App entry point
│   │   ├── App.tsx             # Main app component
│   │   └── components/         # UI components
│   ├── dist/                   # Compiled frontend
│   ├── vite.config.ts          # Vite build configuration
│   └── package.json            # Frontend dependencies
├── docs/                       # Project Documentation like PLAN.md and REQUIREMENTS.md
├── Makefile                    # Common development commands
├── AGENTS.md                   # This file - Development guidelines
└── README.md                   # Project overview and API documentation
```

## Go Development

### Standard Patterns

**Errors**: Use `errors.NewAPIError(statusCode, code, message)` from internal/errors for HTTP errors. Implement `ErrorWithStatus` interface.

**Logging**: Use context-aware slog methods: `slog.InfoContext()`, `slog.ErrorContext()`, etc. Error fields should use `"err"` not `"error"`.

**Handler Signature**: All HTTP handlers wrapped with `Wrap()` must have signature:
```go
func(context.Context, RequestType) (*ResponseType, error)
```

**Git Integration**: `GitService` automatically commits changes to the `data/` directory. Ensure `GIT_CONFIG_GLOBAL` and `GIT_CONFIG_SYSTEM` are ignored (set to `/dev/null`) to prevent user config interference.

**Testing**: Use table-driven tests. Store tests in `*_test.go` files next to implementation. Target 100% coverage for service layers.

## Frontend Development (SolidJS)

### Code Organization

- Components in `frontend/src/components/`
- Global state in `frontend/src/stores/` (if needed) or Context

### Build & Distribution

mddb uses `go:embed` to include the frontend in the binary:

```bash
# Build frontend + Go binary with embedded frontend
make build-all

# Result: ./mddb (single executable, self-contained)
```

The compiled `frontend/dist/` folder is tracked in git for reproducible builds.

### Development Workflow

**Frontend development** (live reload):
```bash
make frontend-dev
# Frontend at http://localhost:5173 (proxies API to :8080)
```

**Backend + embedded frontend** (for testing embedded binary):
```bash
make frontend-build   # Build frontend once
make build            # Build Go binary
./mddb                # Run with embedded frontend
```

## API Development

### Type Generation (Single Source of Truth)

mddb uses [tygo](https://github.com/gzuidhof/tygo) to generate TypeScript interfaces from Go structs. This ensures the frontend and backend stay in sync automatically.

- **Source of Truth**: All Request, Response, and DTO structs MUST be defined in `internal/models/` (typically in `api.go`).
- **Encapsulation**: Structs in `internal/storage/` and `internal/server/handlers/` are internal implementation details and are NOT exported to the frontend.
- **Generated File**: `frontend/src/types.ts` (DO NOT EDIT MANUALLY).
- **Command**: `make types` (included in `make build`).

When you add or modify an API endpoint, add the Request/Response structs to `internal/models/api.go`, then run `make types` to update the frontend.

### Endpoint Conventions

- RESTful: Use HTTP verbs (GET, POST, PUT, DELETE) correctly
- Response format: Always JSON
- Error responses: Include `error` field with structured details (code, message)
- Success responses: Include `data` field with result (except for list endpoints which may return array directly under key like `pages`)

## File Operations

### Markdown Handling (Pages)

- Front matter (YAML) for metadata
- UTF-8 encoding always
- Normalize line endings (LF)

### Database Storage Format

- **Schema**: `metadata.json`
- **Records**: `data.jsonl` (one JSON object per line)
- **Column Types**: text, number, select, multi_select, checkbox, date

## Git Workflow

### Commit Messages

- Format: `{type}: {description}`
- Types: `feat`, `fix`, `docs`, `refactor`, `test`, `chore`

### Documentation Changes

- **Do NOT** create a markdown file listing changes you made.
- Update `docs/PLAN.md` to reflect status changes.
- Keep `docs/README.md` up to date.

## Code Quality & Linting

**All code must pass linting before commits.**

### Go Backend (golangci-lint)

Configured in `.golangci.yml`. Enforces error handling (`errcheck`, `errorlint`), naming (`errname`), style (`revive`, `gocritic`), and more.

Run with:
```bash
make lint
make lint-fix
```

### Frontend (ESLint + Prettier)

Configured in `frontend/.eslintrc.cjs` and `frontend/.prettierrc`. Enforces strict equality, no-unused-vars, and consistent formatting (single quotes, 2 spaces).

### Pre-Commit Hooks

Install with `make git-hooks`. Runs linters and tests before every commit.

## Useful Resources

- [Go Effective Go](https://golang.org/doc/effective_go)
- [SolidJS Docs](https://docs.solidjs.com)
- [mddb PLAN.md](docs/PLAN.md)
- [mddb README.md](README.md)
