# Code Review Issues

Issues identified during full-stack review. Items marked ~~strikethrough~~ have been fixed.

## High Priority

### Backend

- ~~**Missing Limit/Offset validation** — `ListRecordsRequest`, `SearchRequest`, `ListNodeVersionsRequest` have no bounds checking. Could allow negative offsets or huge limits. (`request.go`)~~ **DONE**
- **Unimplemented search** — `search_service.go` returns "not implemented" for full-text search. Two TODOs remain.
- ~~**Context misuse in goroutines** — `auth.go:439` captures request `ctx` in async email goroutine that outlives the request; `github_webhook.go:91` and `notion_import.go:81` use `context.Background()`.~~ **DONE**

### Frontend

- **Click-outside logic duplicated 3x** — `FieldEditor.tsx` lines 77-94, 263-280, 452-467 have near-identical `createEffect` blocks. `useClickOutside` composable exists but isn't used.
- **Silent error swallowing** — `FieldValue.tsx:46-52` catches all errors (not just missing context). `Sidebar.tsx:88` workspace rename fails silently.
- **17 signals in FieldEditor.tsx** — Three dropdown editors (MultiSelect, SingleSelect, User) duplicate state management, positioning, and keyboard handling. Ripe for a shared hook.

### E2E / CI

- **Playwright trace config is dead** — `trace: 'on-first-retry'` with `retries: 0` means traces are never recorded. Change to `'on-first-failure'`.
- **`waitForTimeout` violations** — `workspace-switch-loop.spec.ts:81` (2000ms) and `block-drag-reorder.spec.ts:149` (100ms) violate the project's own best practices in `e2e/AGENTS.md`.

## Medium Priority

### Backend

- **`nil, nil` return anti-pattern** — `git_remotes.go:33` returns nil response + nil error for "no remote configured". Callers must handle an unusual contract.
- **Dead code** — `convert.go:39` has `orgMembershipToResponse()` marked `//nolint:unused // Reserved for future use`. Remove or implement.
- **No filter depth DoS protection** — `views.go:309` limits nesting to 10 but doesn't cap the number of top-level filters.

### Frontend

- **Comma-split parsing repeated 4x** — `FieldValue.tsx:57`, `FieldEditor.tsx:113`, `SelectOptionsEditor.tsx:150`, `TableBoard.tsx:29` all do `.split(',').map(s => s.trim()).filter(Boolean)`. Extract to `parseMultiSelectValue()`.
- **9 independent UI state signals in TableTable.tsx** — editing, menus, filter panel, options editor, rename, drag, resize all independent. No mutual exclusion. A discriminated union / state machine would prevent impossible states.
- **Prop drilling in view components** — Gallery, Grid, List, Board all pass 5+ CRUD callbacks. A `TableViewContext` would simplify.
- **`setTimeout(() => el?.select(), 0)` repeated 5x** — ViewTabs, SelectOptionsEditor, FieldEditor (2x), TableCell. Extract to utility using `requestAnimationFrame`.

### API

- **`UserQuota` struct defined but never exposed** — Frontend can't show quota limits proactively; only discovers them on failure.
- **Filters/sorts passed as JSON strings** — `ListRecordsRequest.Filters` and `.Sorts` are raw strings with no server-side syntax validation.
- **No batch record operations** — No `BatchUpdate`/`BatchDelete` endpoints for multi-row operations.

### E2E Coverage Gaps

- **No quota/limits tests** — The quota system (caic-13) has zero e2e coverage.
- **No search/filter e2e tests** — Filter combinations, sort+filter interaction untested.
- **No multi-user collaboration tests** — Concurrent editing, conflict resolution not covered.

## Low Priority

- **Accessibility** — `FieldValue.tsx:134` user avatar has `alt=""` instead of the user's name.
- **Editor.module.css is 727 lines** — Could split by feature (toolbar, blocks, etc.).
- **Inconsistent backend error messages** — `DeleteNode` vs `GetNodeVersion` both use `"node"` but `notifications.go` uses `"notification not found"`.
- **CI workflow duplication** — `.github/ci.yml` and `.github/pr.yml` are nearly identical; extract common steps.
- **Large test files** — `editor-toolbar.spec.ts` (943 lines), `page-crud.spec.ts` (998 lines) could be split.
- **Missing e2e helpers** — `waitForSidebar`, `waitForEditorReady`, `clickTableHeader` are repeated patterns that should be in `helpers.ts`.
