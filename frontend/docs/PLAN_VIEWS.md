# Table Views Implementation Plan

## Overview
This document outlines the plan to implement a robust View System for mddb tables. A "View" is a saved configuration of a table that includes:
- **Type**: Table, Board, Gallery, List, Calendar.
- **Filtering**: Conditions to include/exclude records.
- **Sorting**: Order of records.
- **Grouping**: Grouping records by property values (for Board/List).
- **Columns**: Visibility and order of columns.

## Backend Implementation

### 1. DTO Updates (`backend/internal/server/dto/types.go`)
Expose internal storage types to the API by copying/aliasing them in DTOs.
- `View`, `ViewType`, `ViewColumn`
- `Filter`, `FilterOp`
- `Sort`, `SortDir`
- `Group`

### 2. API Response Updates (`backend/internal/server/dto/response.go`)
- Update `NodeResponse` to include `Views []View`.
- Update `GetTableSchemaResponse` to include `Views []View`.

### 3. API Endpoints (`backend/internal/server/handlers/views.go`)
Create a new handler file for view-specific operations.
- `POST /api/nodes/{id}/views`: Create a new view.
- `PUT /api/nodes/{id}/views/{viewID}`: Update an existing view.
- `DELETE /api/nodes/{id}/views/{viewID}`: Delete a view.

### 4. Record Querying (`backend/internal/server/handlers/nodes.go`)
Update `ListRecords` to support server-side filtering and sorting.
- Update `ListRecordsRequest` to accept:
  - `ViewID`: To apply a saved view's configuration.
  - `Filters`: Ad-hoc filters (JSON).
  - `Sorts`: Ad-hoc sorts (JSON).
- Update logic to use `content.QueryRecords` before pagination.

## Frontend Implementation

### 1. SDK Generation
- Run `make types` to generate new TypeScript interfaces from updated DTOs.

### 2. State Management (`src/contexts/RecordsContext.tsx`)
- Add state for `views` (list of saved views).
- Add state for `activeViewId`.
- Add state for `activeFilters` and `activeSorts` (derived from view or ad-hoc).
- Add methods:
  - `createView(name, type)`
  - `updateView(id, config)`
  - `deleteView(id)`
  - `setFilters(filters)`
  - `setSorts(sorts)`

### 3. UI Components

#### `src/components/table/ViewTabs.tsx`
- Horizontal list of views above the table.
- "Pro" style tabs (Notion-like).
- "New View" button (+).
- Context menu on tabs (Rename, Delete, Duplicate).

#### `src/components/table/ViewToolbar.tsx`
- Toolbar below tabs containing:
  - **Filter Button**: Opens `FilterMenu`.
  - **Sort Button**: Opens `SortMenu`.
  - **Search Input**: Local or server-side text search.
  - **New Record Button**: Existing functionality.

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

### 4. Component Integration
- Update `TableGrid`, `TableBoard`, `TableGallery` to:
  - Accept `views` and `activeViewId` props (or consume context).
  - Render data based on the *server-returned* records (which are now filtered/sorted).
  - (Optional) Client-side fallback for small datasets if we want immediate reactivity without reloading.

## Notion Import
- Notion's `Query` endpoint returns results based on filters/sorts.
- During import, we should try to extract view configurations from Notion if the API allows (Notion API is limited here; might only get current query results, not saved views).
- *Constraint*: The official Notion API does **not** expose saved views (only database properties and schema). We might have to recreate views manually or infer them if possible, but likely this is a "manual recreation" step for users.

## Phases

### Phase 1: Backend Foundation
- [ ] DTOs and Types.
- [ ] Handler implementation.
- [ ] Storage layer updates (ensure views are saved to `metadata.json`).

### Phase 2: Frontend State & SDK
- [ ] SDK generation.
- [ ] `RecordsContext` updates.
- [ ] Basic "View Tabs" UI (read-only list of views).

### Phase 3: View Management UI
- [ ] Create/Edit/Delete View UI.
- [ ] View Toolbar (Filter/Sort UI).
- [ ] Wiring up `ListRecords` with filters.
