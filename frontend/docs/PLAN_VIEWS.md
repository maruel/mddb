# Table Views Implementation Plan

## Overview
The View System for mddb tables is substantially implemented. A "View" is a saved configuration
of a table that includes:
- **Type**: Table, Board, Gallery, Grid (List), Calendar.
- **Filtering**: Per-column conditions with type-aware operators.
- **Sorting**: Multi-column sorting with direction per column.
- **Grouping**: Grouping records by property values (Board view).
- **Columns**: Visibility, order, and width — all persisted per view.

## Completed

- [x] **Backend**: Filter/sort model (`views.go`), query engine (`query.go`), validation in handlers.
- [x] **Backend**: View CRUD endpoints (create, update, delete, reorder).
- [x] **Frontend**: `RecordsContext` with `setFilters()`/`setSorts()`, client-side + server-side execution.
- [x] **Frontend**: `ViewTabs.tsx` — create, switch, delete, rename, duplicate views with type icons.
- [x] **Frontend**: View type renderers — `TableTable`, `TableBoard`, `TableGallery`, `TableGrid`.
- [x] **Frontend**: Sort UI — left-click column header cycles direction; right-click context menu
      with ascending / descending / remove-sort options. Additive (preserves sorts on other columns).
- [x] **Frontend**: Filter UI — per-column `FilterPanel.tsx` (portal-rendered, type-aware
      operators). Active filters displayed as chip bar above table with quick-remove buttons.
- [x] **Frontend**: Column visibility — hide/show via context menu, persisted per view.
- [x] **Frontend**: Column resizing — drag resize handle, widths persisted per view.
- [x] **Frontend**: Column reordering — drag-to-reorder, order persisted per view.
- [x] **SDK**: TypeScript types generated from Go structs (`Filter`, `Sort`, `View`, etc.).

## Remaining Work

### Calendar view — not implemented
Backend DTO defines `ViewTypeCalendar = "calendar"` and the view-type picker in `ViewTabs.tsx`
shows the option, but no `TableCalendar` component exists. Selecting it will not render anything.

**Scope:** Calendar grid grouped by a date property column; week/month toggle.

### List view — mapped to Grid, semantics undefined
Backend defines `ViewTypeList = "list"`. The frontend currently falls through to `TableGrid` for
this type. No distinct `TableList` component exists.

**Decision needed:** Either implement a distinct list layout or consolidate "list" and "grid" into
one renderer and remove the duplicate view-type option from the picker.

### Potential enhancements

- **Compound AND/OR filters** — currently only one filter per column; no multi-condition UI.
- **Global search input** — text search across all columns.
- **Drag-drop record reordering** — manual ordering within sort/filter results (needs backend
  sort-order field; see `PLAN_BLOCK_EDITOR.md` — "Table row reorder persistence").

## Notion Import Note
The official Notion API does **not** expose saved views (only database properties and schema).
View recreation is a manual step for users after import.
