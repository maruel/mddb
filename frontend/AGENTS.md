# Frontend Development Guidelines

## Frontend Development (SolidJS)

### Code Organization

- Components in `frontend/src/components/`
- Global state in `frontend/src/stores/` (if needed) or Context

### Build & Distribution

mddb uses `go:embed` to include the frontend in the mddb binary created from backend/cmd/mddb:

```bash
# Build frontend + Go binary with embedded frontend
make build-all

# Result: ./mddb (single executable, self-contained)
```

The compiled `../backend/frontend/dist/` folder is tracked in git for reproducible builds.

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

## Code Quality & Linting

**All code must pass linting before commits.**

### Frontend (ESLint + Prettier)

Configured in `frontend/.eslintrc.cjs` and `frontend/.prettierrc`. Enforces strict equality, no-unused-vars, and consistent formatting (single quotes, 2 spaces).

## Useful Resources

- [SolidJS Docs](https://docs.solidjs.com)
