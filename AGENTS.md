# AGENTS.md

Development guidelines and commands for mddb project.

## Project Overview

mddb is a Notion-like document and database system. Frontend built with SolidJS, backend with Go. Data stored
as markdown files and images.

**Storage Model:**
- All content lives in the `pages/` directory
- Pages: Files ending with `.md` (e.g., `getting-started.md`)
- Databases: Files ending with `.db.md` (e.g., `tasks.db.md`, `contacts.db.md`)
- Assets: Any file that doesn't end with `.md` (e.g., `diagram.png`, `chart.svg`)
- Everything can be organized in subdirectories for hierarchical organization

See README.md for project overview and PLAN.md for implementation roadmap.

## Directory Structure

```
mddb/
├── cmd/mddb/                    # Application entry point
│   └── main.go                  # Server startup and configuration
├── internal/
│   ├── server/                  # HTTP server and routing
│   │   ├── router.go           # Route definitions and SPA handler
│   │   ├── handler_wrapper.go  # Generic handler wrapper with path param extraction
│   │   └── handlers/           # HTTP request handlers by feature
│   │       ├── pages.go        # Page CRUD operations
│   │       ├── databases.go    # Database operations
│   │       ├── assets.go       # Asset management
│   │       └── health.go       # Health check
│   ├── storage/                # File system operations
│   │   ├── filestore.go        # Low-level file operations
│   │   ├── filestore_test.go   # FileStore unit tests
│   │   └── page_service.go     # Page business logic
│   ├── models/                 # Data models
│   │   └── models.go           # Page, Database, Record, Asset structs
│   ├── errors/                 # Error types
│   │   └── errors.go           # ErrorWithStatus interface and APIError
│   └── utils/                  # Utilities
│       ├── uuid.go             # UUID generation
│       └── response.go         # Response formatting (if used)
├── web/                        # SolidJS frontend
│   ├── src/                    # Frontend source code
│   │   ├── index.tsx           # App entry point
│   │   ├── App.tsx             # Main app component
│   │   └── App.module.css      # App styling
│   ├── public/                 # Static files (index.html for SPA)
│   ├── index.html              # Vite HTML template
│   ├── vite.config.ts          # Vite build configuration
│   ├── tsconfig.json           # TypeScript configuration
│   └── package.json            # Frontend dependencies
├── data/                       # Runtime data directory
│   └── pages/                  # All markdown content (created dynamically)
├── Makefile                    # Common development commands
├── PLAN.md                     # Implementation roadmap
├── AGENTS.md                   # This file - Development guidelines
└── README.md                   # Project overview and API documentation
```

## Go Development

### Standard Patterns

**Errors**: Use `errors.NewAPIError(statusCode, message)` from internal/errors for HTTP errors. Implement `ErrorWithStatus` interface.

**Logging**: Use context-aware slog methods: `slog.InfoContext()`, `slog.ErrorContext()`, etc. Error fields should use `"err"` not `"error"`.

**Handler Signature**: All HTTP handlers wrapped with `Wrap()` must have signature:
```go
func(context.Context, RequestType) (*ResponseType, error)
```

**Path Parameters**: Use struct field tags `path:"paramName"` for automatic extraction:
```go
type GetRequest struct {
    ID string `path:"id"`
}
```

**Service Pattern**: Create a service layer (e.g., `PageService`) that uses `FileStore` for business logic.

**Testing**: Use table-driven tests. Store tests in `*_test.go` files next to implementation.

## Frontend Development (SolidJS)

### Code Organization

- Components in `web/src/components/` (reusable, single-responsibility)
- Page layouts in `web/src/pages/`
- Global state in `web/src/stores/`
- Utilities in `web/src/utils/`

### Naming Conventions

- Components: PascalCase (e.g., `PageEditor.tsx`, `DatabaseTable.tsx`)
- Stores: camelCase (e.g., `pageStore.ts`, `editorStore.ts`)
- Props interfaces: `{ComponentName}Props`
- Event handlers: `handleXyz` pattern

### SolidJS Patterns

- Use `createSignal` for reactive state
- Use `createEffect` for side effects
- Use Context API for global state
- Prefer stores over props drilling
- Keep components as functions returning JSX

### Common Commands

Development server:
```bash
cd web
npm run dev
```

Build production:
```bash
cd web
npm run build
```

Type checking:
```bash
cd web
npm run typecheck
```

## API Development

### Endpoint Conventions

- RESTful: Use HTTP verbs (GET, POST, PUT, DELETE) correctly
- Response format: Always JSON
- Error responses: Include `error` field with message
- Success responses: Include `data` field with result
- Status codes: 200 OK, 201 Created, 400 Bad Request, 404 Not Found, 500 Server Error

### Error Response Format

```json
{
  "error": "Descriptive error message",
  "code": "ERROR_CODE"
}
```

### Success Response Format

```json
{
  "data": {
    "id": "...",
    "...": "..."
  }
}
```

## File Operations

### Markdown Handling

- Front matter (YAML) for metadata
- UTF-8 encoding always
- Normalize line endings (LF)

## Testing

### Go Tests

- Use table-driven tests for multiple cases

### Frontend Tests

- Use Vitest
- Filename: `{component}.test.tsx`
- Test component rendering and interactions

## Git Workflow

### Commit Messages

- Format: `{type}: {description}`
- Types: `feat`, `fix`, `docs`, `refactor`, `test`, `chore`
- Keep commits focused and atomic
- Write descriptive messages

## Performance Considerations

### Go Backend

- Use file caching for frequently accessed pages
- Implement pagination for large result sets
- Optimize markdown parsing (consider pre-processing)
- Monitor file I/O operations

### Frontend

- Code splitting for routes
- Lazy load components
- Debounce auto-save operations
- Cache rendered markdown

## Documentation

### Code Comments

- Explain the "why", not the "what"
- Document complex algorithms
- Add examples for tricky functions
- Keep comments up-to-date

### API Documentation

- Document endpoint purpose and parameters
- Include request/response examples
- Document error cases
- Keep in-sync with implementation

## Deployment

### Build Process

1. Ensure tests pass: `go test ./...`
2. Build frontend: `cd web && npm run build`
3. Embed frontend in Go binary (if applicable)
4. Build Go binary: `go build -o mddb ./cmd/mddb`
5. Test binary with sample data

### Configuration

- Use environment variables for configuration
- Support config file (optional)
- Document all config options
- Provide sensible defaults

## Development Checklist

When implementing features:

- [ ] Update PLAN.md if requirements change
- [ ] Write tests first or alongside code
- [ ] Follow naming conventions
- [ ] Run linters and fix issues: `make lint-fix`
- [ ] Run tests: `make test`
- [ ] Document complex logic
- [ ] Test error cases
- [ ] Update relevant README sections
- [ ] Commit with descriptive message (runs pre-commit hooks automatically)

## Code Quality & Linting

**All code must pass linting before commits.** See [docs/LINTERS.md](docs/LINTERS.md) for detailed rules.

Quick commands:
- `make lint` - Run all linters (Go + Frontend)
- `make lint-fix` - Auto-fix all linting issues
- `make git-hooks` - Install pre-commit hooks (required once per repo clone)

## Useful Resources

- [Go Effective Go](https://golang.org/doc/effective_go)
- [SolidJS Docs](https://docs.solidjs.com)
- [mddb PLAN.md](PLAN.md) - Implementation roadmap
- [mddb README.md](README.md) - Project overview
