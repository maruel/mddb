# Table Views Implementation Plan

## Overview
The View System for mddb tables is mostly implemented. A "View" is a saved configuration of a table that includes:
- **Type**: Table, Board, Gallery, List, Calendar.
- **Filtering**: Conditions to include/exclude records.
- **Sorting**: Order of records.
- **Grouping**: Grouping records by property values (for Board/List).
- **Columns**: Visibility and order of columns.

## Remaining Work

### Filter/Sort UI Components

#### `src/components/table/ViewToolbar.tsx`
- Toolbar below tabs containing:
  - **Filter Button**: Opens `FilterMenu`.
  - **Sort Button**: Opens `SortMenu`.
  - **Search Input**: Local or server-side text search.

#### `src/components/table/FilterMenu.tsx`
- Dropdown to manage filters.
- List existing filters.
- "Add Filter" button.
- Each filter row: [Property] [Operator] [Value] [Delete].

#### `src/components/table/SortMenu.tsx`
- Dropdown to manage sorts.
- List existing sorts.
- "Add Sort" button.
- Each sort row: [Property] [Direction] [Delete].

## Notion Import Note
The official Notion API does **not** expose saved views (only database properties and schema). View recreation is a manual step for users after import.
