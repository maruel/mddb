# Frontend Implementation Plan

## Overview
Roadmap for the SolidJS frontend, focusing on architecture, performance, and user experience.

## Architecture Improvements (Priority)

### Phase A1: State Management Refactor
App.tsx is 1242 lines with 15+ concerns mixed together. Extract into proper modules:

- [ ] **Create `src/stores/` directory** for global state
  - [ ] `authStore.ts` - User authentication, token management
  - [ ] `nodeStore.ts` - Node/page state, breadcrumb tracking
  - [ ] `editorStore.ts` - Title, content, unsaved changes
  - [ ] `recordStore.ts` - Table records state
  - [ ] `workspaceStore.ts` - Workspace switching logic
- [ ] **Create `src/contexts/` directory** for React-style context providers
  - [ ] `AuthContext.tsx` - Auth state provider
  - [ ] `WorkspaceContext.tsx` - Workspace/org context
- [x] **Create `src/composables/` directory** for reusable logic
  - [ ] `useAutoSave.ts` - Debounced auto-save logic
  - [x] `useClickOutside.ts` - Click-outside detection (used by WorkspaceMenu, UserMenu)
  - [ ] `useRouting.ts` - URL parsing and navigation helpers
- [ ] **Fix state management anti-patterns**
  - [ ] Replace direct variable mutations (lines 139, 387, 545, 589) with signals
  - [ ] Standardize on `createStore()` for complex objects

### Phase A2: Reduce Prop Drilling
Components receive 10+ props with 6+ callbacks. Use context instead:

- [ ] **Refactor Sidebar** - consume workspace context directly
- [ ] **Refactor WorkspaceMenu** - consume auth/workspace context
- [ ] **Refactor table views** - share context for record operations

### Phase A3: Extract Shared Utilities
- [ ] **Table utilities** (`src/components/table/tableUtils.ts`)
  - [ ] Extract duplicate `handleUpdate()` from TableGrid, TableGallery, TableBoard
  - [ ] Shared cell rendering logic
- [ ] **URL builders** (`src/utils/urls.ts`)
  - [ ] Centralize URL construction currently scattered in App.tsx
- [x] **Fix SidebarNode prefetch cache** (line 31)
  - [x] `createSignal(new Map())` creates fresh Map each render
  - [x] Switch to `createStore()` for cache persistence

### Phase A4: Error Handling
- [ ] **Add ErrorBoundary component** - prevent full app crashes
- [ ] **Add retry UI** for failed operations
- [ ] **Improve error feedback** - use `aria-live` for screen readers

---

## Code Quality

### Accessibility (A11y)
- [ ] Add `aria-label` to icon-only buttons (Sidebar, menus)
- [ ] Add `role="alert" aria-live="polite"` to error messages
- [ ] Replace emoji icons with semantic SVG icons

### Type Safety
- [ ] Replace unsafe type assertions (`user() as UserResponse`) with type guards
- [ ] Add validation logging for missing columns in table lookups

### Performance
- [ ] Memoize `groupColumn()` in TableBoard.tsx
- [ ] Add `onCleanup()` for debounce flush on unmount
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
- [ ] View Management UI: Tabs and dropdowns for switching/creating views
- [ ] Filter Builder: Visual interface for complex AND/OR queries
- [ ] Sort/Column UI: Drag-and-drop interfaces
- [ ] View Persistence: Link UI state to backend view configurations

### Phase 4: Advanced UX
- [ ] Block-based WYSIWYG Editor (TipTap or SolidJS implementation)
  - [ ] High-fidelity Markdown serialization
  - [ ] Slash commands and block drag-and-drop
- [ ] Property Management UI: Adding/renaming table columns
- [ ] Inline Editing: Spreadsheet-like keyboard navigation
- [ ] Undo/Redo: See `docs/PLAN_UNDO.md`
- [ ] Bulk Actions: Multi-record operations

### Future Enhancements
- [ ] Command Palette (Ctrl+K): Navigation and action modal
- [ ] Slash Commands: Inline editor commands
- [ ] Table Virtualization: 50k+ records with zero lag
- [ ] Relationship Graph: Visualize backlinks and connections
- [ ] Adaptive Themes: Per-organization branding

---

## Technical Standards
- [x] Type Sync: Automatic TypeScript generation from Go DTOs
- [x] CSS Modules: Scoped styling for all components
- [x] SolidJS: Fine-grained reactivity for performance
