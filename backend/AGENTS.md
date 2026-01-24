# Backend Development Guidelines

## Go Development

### Standard Patterns

**Reflection**:
- Always use reflect.Pointer instead of reflect.Ptr
- Always use reflect.TypeFor[T]() instead of reflect.TypeOf() when the type is known at compile time.

**Logging**:
- Use context-aware slog methods: `slog.InfoContext()`, `slog.ErrorContext()`, etc.
- Error fields should use `"err"` not `"error"`.

**Testing**:
- Use table-driven tests.
- Store tests in `*_test.go` files next to implementation.
- Target 95% coverage.
- Use subtest. Create TestFoo then for each method create a subtest.
- In unit tests, use t.Context(), never context.Background().

### HTTP handlers

**Errors**: Use `errors.NewAPIError(statusCode, code, message)` from internal/errors for HTTP errors. Implement `ErrorWithStatus` interface.

**Handler Signature**: All HTTP handlers wrapped with `Wrap()` must have signature:
```go
func(context.Context, RequestType) (*ResponseType, error)
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

### Database Storage Format (jsonldb)

The `internal/jsonldb` package provides a generic, concurrent-safe, JSONL-backed data store.

#### JSONL Table Format

Tables are stored as `.jsonl` files with:
- **Line 1**: Schema header (JSON object with `version` and `columns`)
- **Lines 2+**: Data rows (one JSON object per line)

Example `data.jsonl`:
```jsonl
{"version":"1","columns":[{"name":"id","type":"id"},{"name":"title","type":"string"}]}
{"id":"01JWAB...","title":"First row"}
{"id":"01JWAC...","title":"Second row"}
```

**Row Requirements:**
- Must implement `Row[T]` interface: `Clone()`, `GetID()`, `Validate()`
- IDs are 64-bit integers encoded as base32 strings (ULID-like, time-sortable)
- Rows are kept sorted by ID on disk

#### Blob Storage Format

Large binary data is stored separately from JSONL rows:
- **Location**: Sibling directory with `.blobs` suffix (e.g., `data.jsonl` → `data.blobs/`)
- **Structure**: 256-way fan-out by first 2 chars of hash (e.g., `data.blobs/4O/YMIQUY7Q...`)
- **Reference Format**: `sha256:<BASE32>-<size>` (52 uppercase base32 chars + decimal size)
- **Content-Addressed**: Identical content shares the same file (deduplication)
- **Garbage Collection**: Orphaned blobs are removed on table load

Example blob ref: `sha256:4OYMIQUY7QOBJGX36TEJS35ZEQT24QPEMSNZGTFESWMRW6CSXBKQ-0`

**Using Blobs in Rows:**
```go
type MyRow struct {
    ID      jsonldb.ID
    Content jsonldb.Blob  // Automatically discovered via reflection
}

// Creating a blob:
writer, _ := table.NewBlob()
writer.Write(data)
blob, _ := writer.Close()
row.Content = blob
table.Append(&row)

// Reading a blob:
reader, _ := row.Content.Reader()
io.Copy(dst, reader)
reader.Close()
```

Blob fields are discovered automatically via reflection, including nested structs and slices.

#### Column Types

- `id` - Row identifier (required, unique)
- `string` - Text
- `int`, `float` - Numbers
- `bool` - Boolean
- `time` - Timestamp
- `blob_ref` - Reference to external blob file

## Build & Test

Run these commands to verify changes:
- `make lint` - Run linters
- `make build` - Compile backend and frontend
- `make test` - Run all tests

## Code Quality & Linting

**All code must pass `make lint` before commits.**

### Go Backend (golangci-lint)

Configured in `.golangci.yml`. Enforces error handling (`errcheck`, `errorlint`), naming (`errname`), style (`revive`, `gocritic`), and more.

Run with:
```bash
make lint
make lint-fix
```
