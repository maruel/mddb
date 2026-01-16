# Quick Start: Building mddb

## One Command Build

Build a single, self-contained mddb binary with embedded frontend:

```bash
make build-all
```

Done. You now have `./mddb` executable that contains everything.

## Run It

```bash
./mddb
# Open http://localhost:8080
```

That's it.

## What Happens

```
make build-all
├── Runs Go tests
├── Builds SolidJS frontend to web/dist/
└── Builds Go binary with embedded frontend
    └── Result: ./mddb (single executable)
```

## Options

Run with custom settings:

```bash
./mddb -port 9000 -data-dir /path/to/data -log-level debug
```

- `-port` - Server port (default: 8080)
- `-data-dir` - Data directory (default: ./data)
- `-log-level` - Log level: debug, info, warn, error (default: info)

## Development

### Live Frontend Development

```bash
make frontend-dev
# Frontend on http://localhost:5173
# Proxies API to http://localhost:8080
```

### Backend Only

```bash
make build
./mddb
```

### Full Development

```bash
# Terminal 1: Backend
make dev

# Terminal 2: Frontend
make frontend-dev
```

## File Structure

After `make build-all`:

```
.
├── mddb                   # ← Single executable (your binary)
├── web/
│   ├── dist/             # Compiled frontend (embedded in binary)
│   └── src/              # Frontend source code
├── internal/             # Backend source code
└── data/                 # Your data (created at runtime)
```

## What's Embedded

The binary contains:
- Entire SolidJS frontend (HTML, CSS, JavaScript)
- All assets (images, icons, etc.)
- No external dependencies needed

You can delete `web/dist/` after building if you want—the binary is completely self-contained.

## Reproducibility

Build twice with same code = same binary (or nearly identical except for timestamps).

This is important for security and distribution—anyone can verify the binary was built from the source.

## Distribution

Ship `./mddb` to users. That's all they need.

No npm, no Node.js, no git—just one executable.

## Troubleshooting

**"web/dist doesn't exist"**
```bash
make frontend-build
make build
```

**"Binary is huge"**
Normal—includes Go runtime + frontend. Usually 15-25 MB.

**"Changes not reflected in binary"**
```bash
make clean
make build-all
```

**"Port already in use"**
```bash
./mddb -port 9000
```

## Next Steps

- See [EMBEDDED_BUILD.md](EMBEDDED_BUILD.md) for detailed information
- Check [README.md](../README.md) for usage guide
- Review [AGENTS.md](../AGENTS.md) for development

## Summary

```bash
git clone <repo>
cd mddb
make build-all
./mddb
# Open http://localhost:8080
```

Done!
