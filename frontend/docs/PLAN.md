# Frontend Implementation Plan

## Overview
Roadmap for the SolidJS frontend, focusing on architecture, performance, and user experience.

## Architecture Improvements

### Reduce Prop Drilling
- [ ] **Refactor Sidebar** — still receives ~18 callback props; should consume workspace context directly
- [x] **Refactor WorkspaceMenu** — now consumes auth/workspace context directly

### Error Handling
- [ ] **Add retry UI** for failed operations

---

## Code Quality

### Accessibility (A11y)
- [ ] Add `aria-label` to icon-only buttons (Sidebar, menus) — currently using `title` attributes

### Type Safety
- [x] Replace unsafe type assertions (`user() as UserResponse`) with type guards
- [ ] Add validation logging for missing columns in table lookups

### Performance
- [ ] Event listener cleanup audit

---

## Feature Roadmap

### Onboarding
- [ ] Template selection: Propose template Git repositories during onboarding

### Globalization
- [ ] Offline Mode: Client-side storage and data reconciliation

### Table Views System
See [PLAN_VIEWS.md](PLAN_VIEWS.md) for detailed implementation plan.

- [x] Backend filter/sort model, query engine, view persistence
- [x] RecordsContext state management with client-side and server-side execution
- [x] View tabs (create, switch, delete, rename, duplicate)
- [x] Sort via column header (left-click cycles direction; right-click context menu; additive)
- [x] Filter via per-column FilterPanel (type-aware operators; active filter chip bar)
- [x] Column visibility, resizing, and reordering — persisted per view
- [x] Record detail slide-over panel
- [ ] ~~ViewToolbar with Filter and Sort buttons~~ — REMOVED (moved to column header)
- [ ] ~~SortMenu / FilterMenu as global dropdowns~~ — REMOVED (replaced by per-column UI)
- [ ] Advanced filter UI: compound AND/OR conditions (currently single filter per column)

### Select Column UX

- [x] `multi_select` in AddColumnDropdown with inline option creation
- [x] Search + keyboard nav in select/multi-select dropdowns
- [x] `SelectOptionsEditor` panel (rename, recolor, delete options)
- [x] "Edit options" in column header context menu
- [x] Filter panel option picker for select/multi-select columns
- [x] Board view multi-select grouping fix
- [x] Chip style consistency and text contrast (`chipTextColor`)
- [x] Drag-to-reorder options in SelectOptionsEditor

### Advanced UX
- [ ] Bulk Actions: Multi-record operations

### Future Enhancements
- [ ] Command Palette (Ctrl+K): Navigation and action modal
- [ ] Table Virtualization: 50k+ records with zero lag
- [ ] Relationship Graph: Visualize backlinks and connections
- [ ] Adaptive Themes: Per-organization branding
