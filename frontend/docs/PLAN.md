# Frontend Implementation Plan

## Overview
Roadmap for the SolidJS frontend, focusing on architecture, performance, and user experience.

## Architecture Improvements

### Reduce Prop Drilling
- [ ] **Refactor Sidebar** - consume workspace context directly instead of 10+ callback props
- [ ] **Refactor WorkspaceMenu** - consume auth/workspace context directly

### Error Handling
- [ ] **Add retry UI** for failed operations

---

## Code Quality

### Accessibility (A11y)
- [ ] Add `aria-label` to icon-only buttons (Sidebar, menus) — currently using `title` attributes

### Type Safety
- [ ] Replace unsafe type assertions (`user() as UserResponse`) with type guards
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

- [ ] Filter Builder: Visual interface for complex AND/OR queries (backend support ready, UI pending).
- [ ] Sort/Column UI: Drag-and-drop interfaces (backend support ready, UI pending).

### Advanced UX
- [ ] Bulk Actions: Multi-record operations

### Future Enhancements
- [ ] Command Palette (Ctrl+K): Navigation and action modal
- [ ] Table Virtualization: 50k+ records with zero lag
- [ ] Relationship Graph: Visualize backlinks and connections
- [ ] Adaptive Themes: Per-organization branding
