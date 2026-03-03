# Table Views Implementation Plan

## Overview
The View System for mddb tables is mostly implemented. A "View" is a saved configuration of a table that includes:
- **Type**: Table, Board, Gallery, List, Calendar.
- **Filtering**: Conditions to include/exclude records.
- **Sorting**: Order of records.
- **Grouping**: Grouping records by property values (for Board/List).
- **Columns**: Visibility and order of columns.

## Completed

- [x] **Backend**: Filter/sort model (`views.go`), query engine (`query.go`), validation in handlers.
- [x] **Backend**: View CRUD endpoints (create, update, delete, reorder).
- [x] **Frontend**: `RecordsContext` with `setFilters()`/`setSorts()`, client-side + server-side execution.
- [x] **Frontend**: `ViewTabs.tsx` — create, switch, delete views with type icons.
- [x] **Frontend**: View type renderers — `TableTable`, `TableBoard`, `TableGallery`, `TableGrid`.
- [x] **SDK**: TypeScript types generated from Go structs (`Filter`, `Sort`, `View`, etc.).

## Remaining Work

### Filter/Sort UI Components

#### `src/components/table/ViewToolbar.tsx`
- Toolbar below view tabs containing:
  - **Filter Button**: Opens `FilterMenu`. Badge shows active filter count.
  - **Sort Button**: Opens `SortMenu`. Badge shows active sort count.
  - **Search Input**: Local or server-side text search.

#### `src/components/table/SortMenu.tsx`
- Dropdown to manage sorts.
- List existing sorts.
- "Add Sort" button.
- Each sort row: [Property] [Direction] [Delete].
- Properties dropdown populated from table schema columns.

#### `src/components/table/FilterMenu.tsx`
- Dropdown to manage filters.
- List existing filters.
- "Add Filter" button.
- Each filter row: [Property] [Operator] [Value] [Delete].
- Operators filtered by property type (text vs number vs date).
- Support for compound AND/OR conditions.

## Notion Import Note
The official Notion API does **not** expose saved views (only database properties and schema). View recreation is a manual step for users after import.
