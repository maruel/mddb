# Code Quality & Linters

mddb uses automated linting to enforce code quality standards across both backend and frontend code.

## Go Backend (golangci-lint)

Configured in `.golangci.yml` at the project root.

### Enabled Linters

- **errcheck** - Detects unchecked error returns
- **errname** - Enforces error naming conventions (`Err` or `Error` prefix)
- **errorlint** - Detects improper error wrapping/formatting (prefers `errors.Is()` over `==`)
- **exhaustive** - Ensures all enum cases are handled
- **gocritic** - Style and performance checks
- **gofmt** - Code formatting
- **goimports** - Organizes imports
- **govet** - Catches suspicious constructs
- **ineffassign** - Detects unused assignments
- **lll** - Enforces max line length (120 characters)
- **misspell** - Finds common spelling mistakes
- **nolintlint** - Validates `//nolint` directives
- **revive** - Style enforcement
- **staticcheck** - Static analysis bugs
- **typecheck** - Type checking
- **unconvert** - Removes redundant type conversions
- **unparam** - Finds unused function parameters
- **unused** - Detects unused variables and constants

### Running Go Linters

```bash
make lint-go          # Run linter
make lint-fix         # Auto-fix linting issues
```

### Code Style Rules (Go)

- Use octal literals with `0o` prefix (e.g., `0o755` not `0755`)
- Use `errors.Is()` instead of `==` for error comparison
- No unchecked error returns (suppress with `_ = err` or `if err != nil { ... }`)
- Max line length: 120 characters
- Proper export comments on all public symbols

## Frontend (ESLint + Prettier)

Configured in `web/.eslintrc.cjs` and `web/.prettierrc`.

### ESLint Rules

- **@typescript-eslint/no-unused-vars** - Detects unused variables (prefix with `_` to suppress)
- **@typescript-eslint/no-explicit-any** - Warns on `any` types
- **no-console** - Allows only `console.warn()` and `console.error()`
- **no-debugger** - Detects leftover debuggers
- **eqeqeq** - Enforces strict equality (`===` and `!==`)
- **no-var** - Requires `const`/`let` instead of `var`
- **prefer-const** - Prefers `const` over `let` when variable isn't reassigned
- **prefer-arrow-callback** - Uses arrow functions in callbacks
- **object-shorthand** - Requires `{ foo }` instead of `{ foo: foo }`

### Prettier Code Formatting

- 100 character print width
- 2 space indentation
- Single quotes for strings
- Trailing commas (ES5 style)
- Always parentheses around arrow function parameters

### Running Frontend Linters

```bash
cd web
pnpm lint              # Run ESLint
pnpm lint:fix          # Auto-fix issues with ESLint
pnpm format            # Format code with Prettier
```

Or from root:

```bash
make lint-frontend     # Run ESLint
make format            # Run Prettier
make lint-fix          # Auto-fix all issues
```

## Pre-Commit Hooks

Install with:

```bash
make git-hooks
```

Pre-commit hooks automatically run:
1. Go linting (`golangci-lint`)
2. Frontend linting (`eslint`)
3. All tests

Commits are blocked if any check fails.

## Continuous Integration

All linters run as part of:
- `make lint` - All linters
- `make test` - Tests (prerequisite for commits)
- `make build` - Builds (requires passing linters first)

## Common Fixes

### Go

```go
// ❌ Unchecked error
json.NewEncoder(w).Encode(data)

// ✅ Handle error
if err := json.NewEncoder(w).Encode(data); err != nil {
    _ = err  // Suppress if intentional
}

// ❌ Old octal style
os.MkdirAll(dir, 0755)

// ✅ New octal style
os.MkdirAll(dir, 0o755)

// ❌ Wrong error comparison
if err != http.ErrServerClosed { ... }

// ✅ Correct error comparison
if !errors.Is(err, http.ErrServerClosed) { ... }
```

### TypeScript/SolidJS

```typescript
// ❌ Unused variable
const data = await res.json();

// ✅ Use variable or prefix with _
const pageData = await res.json();
// or
const _data = await res.json();

// ❌ Non-strict equality
if (x == 5) { ... }

// ✅ Strict equality
if (x === 5) { ... }

// ❌ var declaration
var name = 'test';

// ✅ const/let
const name = 'test';
```

## Disabling Linters

### Go

Use `//nolint:ruleName` comment:

```go
json.NewEncoder(w).Encode(data) //nolint:errcheck
```

### TypeScript

Use ESLint disable comments:

```typescript
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const data: any = {};
```

## Integration with Editors

### VS Code

Recommended extensions:
- Go: `golang.go`
- ESLint: `dbaeumer.vscode-eslint`
- Prettier: `esbenp.prettier-vscode`

Configure `.vscode/settings.json`:

```json
{
  "[go]": {
    "editor.defaultFormatter": "golang.go",
    "editor.formatOnSave": true
  },
  "[typescript]": {
    "editor.defaultFormatter": "esbenp.prettier-vscode",
    "editor.formatOnSave": true,
    "editor.codeActionsOnSave": {
      "source.fixAll.eslint": true
    }
  }
}
```

## Lockdown Strategy

Linters are enforced at multiple levels:

1. **Pre-commit hooks** - Block commits that fail linting
2. **Makefile targets** - Each build step checks linters first
3. **CI/CD** - Tests won't run if linting fails
4. **Code review** - All PRs must pass linting before merge

This ensures consistent code quality across the entire project.
