# Backend Development Guidelines

## Go Development

### Standard Patterns

**Errors**: Use `errors.NewAPIError(statusCode, code, message)` from internal/errors for HTTP errors. Implement `ErrorWithStatus` interface.

**Logging**: Use context-aware slog methods: `slog.InfoContext()`, `slog.ErrorContext()`, etc. Error fields should use `"err"` not `"error"`.

**Handler Signature**: All HTTP handlers wrapped with `Wrap()` must have signature:
```go
func(context.Context, RequestType) (*ResponseType, error)
```

**Git Integration**: `Git` automatically commits changes to the `data/` directory. Ensure `GIT_CONFIG_GLOBAL` and `GIT_CONFIG_SYSTEM` are ignored (set to `/dev/null`) to prevent user config interference.

**Testing**: Use table-driven tests. Store tests in `*_test.go` files next to implementation. Target 100% coverage for service layers.

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

## Build & Test

Run these commands to verify changes:
- `make lint` - Run linters
- `make build` - Compile backend and frontend
- `make test` - Run all tests

## Code Quality & Linting

**All code must pass linting before commits.**

### Go Backend (golangci-lint)

Configured in `.golangci.yml`. Enforces error handling (`errcheck`, `errorlint`), naming (`errname`), style (`revive`, `gocritic`), and more.

Run with:
```bash
make lint
make lint-fix
```

## Useful Resources

- [Go Effective Go](https://golang.org/doc/effective_go)
