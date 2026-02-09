# Frontend Implementation Plan

## Overview
Roadmap for the SolidJS frontend, focusing on architecture, performance, and user experience.

## Architecture Improvements (Priority)

### Phase A1: State Management Refactor
App.tsx reduced from 1238 to ~550 lines (~56% reduction) by extracting state into contexts:

- [x] **Create `src/contexts/` directory** for context providers
  - [x] `AuthContext.tsx` - User, token, login/logout, API clients
  - [x] `WorkspaceContext.tsx` - Nodes, navigation, workspace switching, first-login flow
  - [x] `EditorContext.tsx` - Title, content, auto-save, version history
  - [x] `RecordsContext.tsx` - Table records CRUD and pagination
  - [x] `index.ts` - Re-exports for clean imports
- [x] **Create `src/composables/` directory** for reusable logic
  - [x] `useClickOutside.ts` - Click-outside detection (used by WorkspaceMenu, UserMenu)
- [x] **Routing refactor**
  - [x] Migrate to `@solidjs/router`
  - [x] Extract `/w/` into `WorkspaceSection`
  - [x] Extract `/settings/` into `SettingsSection`
  - [x] Replace `window.location` reads with router hooks for reactive UI state

### Phase A2: Reduce Prop Drilling
Components receive 10+ props with 6+ callbacks. Use context instead:

- [ ] **Refactor Sidebar** - consume workspace context directly
- [ ] **Refactor WorkspaceMenu** - consume auth/workspace context
- [x] **Refactor table views** - use shared table utilities

### Phase A3: Extract Shared Utilities
- [x] **Table utilities** (`src/components/table/tableUtils.ts`)
  - [x] Extract duplicate `handleUpdate()` from TableGrid, TableGallery, TableBoard
  - [x] Shared helpers: `updateRecordField`, `handleEnterBlur`, `getFieldValue`, `getRecordTitle`
- [x] **URL builders** (`src/utils/urls.ts`)
  - [x] Centralize URL construction: `workspaceUrl`, `nodeUrl`, `workspaceSettingsUrl`, `orgSettingsUrl`
  - [x] URL parsers: `parseWorkspaceRoot`, `parseNodeUrl`, `parseWorkspaceSettings`, `parseOrgSettings`
- [x] **Fix SidebarNode prefetch cache** (line 31)
  - [x] `createSignal(new Map())` creates fresh Map each render
  - [x] Switch to `createStore()` for cache persistence

### Phase A4: Error Handling
- [x] **Add ErrorBoundary component** - prevent full app crashes
- [ ] **Add retry UI** for failed operations
- [x] **Improve error feedback** - use `aria-live` for screen readers (in App.tsx error display)

---

## Code Quality

### Accessibility (A11y)
- [ ] Add `aria-label` to icon-only buttons (Sidebar, menus)
- [x] Add `role="alert" aria-live="polite"` to error messages
- [x] Replace emoji icons with semantic SVG icons (`@material-symbols/svg-400`)

### Type Safety
- [ ] Replace unsafe type assertions (`user() as UserResponse`) with type guards
- [ ] Add validation logging for missing columns in table lookups

### Performance
- [x] Memoize `groupColumn()` in TableBoard.tsx
- [x] Add `onCleanup()` for debounce flush on unmount (in EditorContext)
- [ ] Event listener cleanup audit

---

## Feature Roadmap

### Phase 1: Onboarding & Experience
- [x] Organization onboarding (guided setup wizard)
- [x] Onboarding state tracking and UI integration
- [ ] Template selection: Propose template Git repositories during onboarding
- [x] Admin dashboards for global admins and organization settings

### Phase 2: Globalization & PWA
- [x] i18n Infrastructure via `@solid-primitives/i18n`
- [x] Localization: English, French, German, Spanish
- [x] PWA Support: Offline caching, install banners, standalone mode
- [ ] Offline Mode: Client-side storage and data reconciliation

### Phase 3: Table Views System
See [PLAN_VIEWS.md](PLAN_VIEWS.md) for detailed implementation plan.

- [x] Backend Support: API endpoints, storage, and server-side query logic
- [x] View Management UI: ViewTabs with create/delete and view type switching.
- [ ] Filter Builder: Visual interface for complex AND/OR queries (backend support ready, UI pending).
- [ ] Sort/Column UI: Drag-and-drop interfaces (backend support ready, UI pending).
- [x] View Persistence: RecordsContext wired to backend view CRUD and filter/sort APIs.

### Phase 4: Advanced UX
- [x] Block-based WYSIWYG Editor (ProseMirror with flat block architecture)
  - [x] High-fidelity Markdown serialization
  - [x] Slash commands and block drag-and-drop
- [x] Property Management UI: Add columns via "+" button with type selection (AddColumnDropdown).
- [x] Inline Editing: Click-to-edit table cells with type-specific inputs (TableCell).
- [x] Undo/Redo: ProseMirror history plugin (Ctrl+Z / Ctrl+Shift+Z).
- [ ] Bulk Actions: Multi-record operations

### Future Enhancements
- [ ] Command Palette (Ctrl+K): Navigation and action modal
- [ ] Table Virtualization: 50k+ records with zero lag
- [ ] Relationship Graph: Visualize backlinks and connections
- [ ] Adaptive Themes: Per-organization branding

---

## Technical Standards
- [x] Type Sync: Automatic TypeScript generation from Go DTOs
- [x] CSS Modules: Scoped styling for all components
- [x] SolidJS: Fine-grained reactivity for performance
