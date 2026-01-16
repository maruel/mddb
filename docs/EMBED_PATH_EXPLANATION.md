# Go Embed Path Explanation

## The Issue

When using `go:embed`, the path is **relative to the Go source file's directory**, not the project root.

## Our Setup

**File Location:**
```
internal/server/embedded.go
```

**Target to Embed:**
```
web/dist/
```

## The Solution

From `internal/server/embedded.go`, the relative path to `web/dist/` is:

```go
//go:embed ../../web/dist/*
var WebFS embed.FS
```

### Path Breakdown

```
internal/server/embedded.go
   ↑         ↑
   |         └─ "server" directory
   └───────────────────── "../" (up to internal/)
                           "../" (up to project root)
                           "web/dist/*" (target)

Total: ../../web/dist/*
```

## How Paths Work

From `internal/server/embedded.go`:

| Target | Relative Path |
|--------|---------------|
| `web/dist/` | `../../web/dist/*` |
| `internal/models/` | `../models/*` |
| `cmd/mddb/` | `../../cmd/mddb/*` |

## Embedded FS Structure

Once embedded, `web/dist/*` becomes the **root** of the filesystem:

```
Embedded FS Root
├── index.html       ← Access as "index.html"
├── assets/
│   ├── index.js     ← Access as "assets/index.js"
│   └── index.css    ← Access as "assets/index.css"
└── favicon.ico      ← Access as "favicon.ico"
```

Not as:
```
❌ "web/dist/index.html"
❌ "web/dist/assets/index.js"
```

## Code Examples

### Correct ✓
```go
//go:embed ../../web/dist/*
var WebFS embed.FS

f, _ := WebFS.Open("index.html")
f, _ := WebFS.Open("assets/index.js")
```

### Wrong ✗
```go
//go:embed ../../web/dist/*
var WebFS embed.FS

f, _ := WebFS.Open("web/dist/index.html")  // ❌ Path too deep
```

## Key Points

1. **Path is relative to source file** - Not project root
2. **Only direct contents embedded** - The `/*` means files/dirs directly in `web/dist/`
3. **Becomes root of FS** - Access files by their path within `web/dist/`
4. **No directory prefix** - Don't include the directory name in access path

## Reference

- [Go Embed Documentation](https://pkg.go.dev/embed)
- [Go 1.16 Release Notes](https://golang.org/doc/go1.16)
