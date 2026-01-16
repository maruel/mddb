# Embedded Build & Distribution

## Overview

mddb uses Go's `go:embed` to include the compiled SolidJS frontend directly in the binary. This creates a single, self-contained executable with no external dependencies.

## Build Process

### One-Step Build

```bash
make build-all
```

This command:
1. Runs backend tests
2. Builds the SolidJS frontend to `web/dist/`
3. Builds the Go binary with embedded frontend via `go:embed`

Result: Single executable at `./mddb` that includes the entire frontend.

### Manual Steps

If you need to build step-by-step:

```bash
# Build frontend only
make frontend-build

# Build Go binary (with embedded frontend)
make build
```

## How It Works

### Frontend Build (Deterministic)

The Vite config in `web/vite.config.ts` ensures deterministic builds:
- Terser minification (consistent output)
- No source maps
- Deterministic chunk naming (vendor chunk)
- No timestamp/hash variations

Result: `web/dist/` directory with:
```
web/dist/
├── index.html
├── assets/
│   ├── index-*.js      (hashed, but deterministic)
│   └── index-*.css     (hashed, but deterministic)
```

### Go Embedding

In `internal/server/embedded.go`:

```go
//go:embed ../../web/dist/*
var WebFS embed.FS
```

This embeds `web/dist/` and all its contents at compile time. The relative path is from the source file's directory. The binary includes the entire frontend—no separate files needed.

### Runtime Serving

The `EmbeddedSPAHandler` in `internal/server/embedded.go`:
- Serves files from the embedded FS
- Falls back to `index.html` for SPA routing
- Sets proper cache headers for assets

### Git Tracking

The `web/dist/` directory is now tracked in git (removed from `.gitignore`). This:
- Makes builds reproducible (no need to rebuild frontend)
- Enables quick CI/CD workflows
- Documents exactly what's in each release
- Allows rollback without rebuilding

## Reproducible Builds

To verify your build is reproducible:

```bash
# Clean and build
make clean
make build-all

# Check the binary exists
ls -lh ./mddb

# You can rebuild and the binary should have identical size
# (exact byte-for-byte match requires stripping timestamps from Go build)
```

For byte-for-byte reproducibility, use:

```bash
go build -trimpath -ldflags="-s -w" ./cmd/mddb
```

## Distribution

### Single Binary Distribution

The compiled `./mddb` binary:
- Contains the entire frontend
- Requires no external assets
- Works on any system with the same OS/architecture

### Docker Distribution

```dockerfile
FROM golang:1.21
WORKDIR /build
COPY . .
RUN make build-all

FROM alpine:latest
COPY --from=builder /build/mddb /usr/local/bin/
EXPOSE 8080
ENTRYPOINT ["mddb"]
```

The binary works out-of-the-box in Docker.

## Development Workflow

### Frontend Changes

```bash
# If you change frontend code:
make frontend-build  # Rebuild frontend to web/dist/
git add web/dist/    # Commit the dist folder

# Backend automatically picks up the changes on next build
make build
```

### Backend Changes Only

```bash
make build
./mddb -data-dir ./data -port 8080
```

### Live Frontend Development

For development, use the Vite dev server:

```bash
make frontend-dev
# Frontend runs on http://localhost:5173
# Proxies API calls to http://localhost:8080
```

## Git Integration

### .gitignore Changes

Removed `web/dist/` from `.gitignore` so the build artifacts are tracked:

```diff
# Before
/web/node_modules/
/web/dist/        <- REMOVED

# After  
/web/node_modules/
```

This means:
- `web/dist/` files are committed to git
- Each commit has an exact snapshot of the frontend
- CI/CD doesn't need to rebuild the frontend on every check

### Commit Pattern

```bash
# Make frontend changes
# Test locally with: make frontend-dev

# When ready to commit:
make frontend-build  # Rebuild to web/dist/
git add web/dist/
git commit -m "feat: Update frontend components"

# Or update both frontend and backend in one commit:
# Make changes to web/src/ and internal/
make frontend-build
git add web/dist/ internal/
git commit -m "feat: Add feature (frontend + backend)"
```

## Troubleshooting

### Binary Doesn't Embed Frontend

Ensure `web/dist/` exists:
```bash
ls -la web/dist/
# Should show index.html and assets/
```

If not, rebuild:
```bash
make frontend-build
```

### 404 on Root Path

Verify the embedded FS is working:
- Check `internal/server/embedded.go` has the `//go:embed` directive
- Ensure `web/dist/` is not empty
- Clean and rebuild: `make clean && make build-all`

### Embedded FS Not Updating

Go caches embed directives at compile time. To force update:
```bash
make clean
make build-all
```

## Performance

- **Binary Size**: ~15-25 MB (frontend + backend + Go runtime)
- **Startup**: < 100ms (assets already in memory)
- **Serving**: Direct memory serving, no disk I/O for frontend

## Future Enhancements

- [ ] Build script with version stamping
- [ ] Release automation (goreleaser)
- [ ] Multi-platform builds (Linux, macOS, Windows)
- [ ] Asset optimization (gzip pre-compression)
- [ ] Security scanning in CI/CD
