# Select Column UX Improvement Plan

## Overview

The `select` and `multi_select` column types have basic functionality but are missing several key UX
features. This plan covers all work needed across frontend components, i18n, and e2e tests.

No backend changes are required: the existing `updateTable` endpoint already accepts a full
`Property[]` with `options[]`; option management is purely a frontend concern.

---

## Current State

| Feature | Status | Notes |
|---|---|---|
| Display chips (read mode) | ✓ | `FieldValue.tsx` |
| Select value in cell | ✓ | `SingleSelectEditor` / `MultiSelectEditor` |
| Select value in card views | ✓ | `FieldEditor.tsx` |
| `multi_select` in AddColumnDropdown | ✗ | Missing from `COLUMN_TYPES` list |
| Add/rename/recolor/delete options | ✗ | No UI exists; API-only |
| Define options at column creation | ✗ | `AddColumnDropdown` creates empty options list |
| MultiSelectEditor when all selected | ✗ | Dropdown hides; user can only deselect via chips |
| Active option highlighted in dropdown | ✗ | `SingleSelectEditor` doesn't mark current value |
| Keyboard nav in dropdown | ✗ | No arrow-key / Enter support |
| Search/filter in dropdown | ✗ | No type-to-filter |
| Filter panel for select: option picker | ✗ | Plain text input; user must type option ID |
| Chip styling consistency | ✗ | `FieldValue` (radius 4px) ≠ `FieldEditor` (radius 10px) |
| Text contrast on colored chips | ✗ | Always `color: #fff`; unreadable on light colors |
| Board view: multi-select grouping | ✗ | Only works for single-select; multi-select broken |

---

## Files Affected

### Frontend
- `src/components/table/FieldEditor.tsx` + `FieldEditor.module.css` — editors, chips, dropdowns
- `src/components/table/FieldValue.tsx` + `FieldValue.module.css` — read-mode chips
- `src/components/table/FilterPanel.tsx` + `FilterPanel.module.css` — filter value input
- `src/components/table/AddColumnDropdown.tsx` + `AddColumnDropdown.module.css` — column creation
- `src/components/table/SelectOptionsEditor.tsx` + `.module.css` — **new** option management panel
- `src/components/TableTable.tsx` — column header context menu entry point
- `src/components/TableBoard.tsx` — multi-select grouping fix
- `src/i18n/types.ts` + all 4 dictionaries — new strings

### E2E
- `e2e/table-operations.spec.ts` — extend existing tests
- `e2e/select-options.spec.ts` — **new** option management tests

---

## Steps

### Step 1 — Quick fixes and chip polish (pure frontend, ~2h)

**Self-contained. Can be done in parallel with Step 4.**

**Problem A — `multi_select` missing from column creation.**
`AddColumnDropdown.tsx` `COLUMN_TYPES` array omits `multi_select`. Add it between `select` and `url`.
No other changes needed; the column is created with an empty options list (same as `select`).

**Problem B — MultiSelectEditor hides when all options selected.**
In `FieldEditor.tsx` line 144:
```tsx
<Show when={open() && unselectedOptions().length > 0 && dropPos()}>
```
Change to show all options, marking selected ones with a checkmark icon. Replace `unselectedOptions()`
with `props.column.options ?? []` and render each item with a checked/unchecked state. Use a tick
mark (`✓`) before the option name when `selectedIds().includes(opt.id)`.

**Problem C — SingleSelectEditor doesn't highlight current value.**
In the portal dropdown, add a class `optionItemActive` to the item whose `opt.id === props.value`.
Style with a subtle background and a checkmark prefix.

**Problem D — Chip style inconsistency.**
`FieldValue.module.css` `.selectChip` uses `border-radius: 4px` and `font-size: 0.8em`.
`FieldEditor.module.css` `.chip` uses `border-radius: 10px` and `font-size: 0.75rem`.
Unify: move a shared `.selectChip` style to a new `src/components/table/selectChip.module.css`
(or inline as a CSS variable strategy). Both files import it. Agree on pill shape (10px radius)
and consistent font size (0.78rem).

**Problem E — Text contrast on colored chips.**
Currently `style={color ? { background: color, color: '#fff' } : {}}`.
Add a `chipTextColor(hexBg: string): string` utility in `tableUtils.ts` that computes relative
luminance (W3C formula) and returns `'#fff'` or `'#111'`. Use it in `FieldValue.tsx` and
`FieldEditor.tsx` wherever chips are rendered.

**i18n:** No new strings needed for this step.

**Tests:** Verify in `table-operations.spec.ts` that chips render in both read and edit mode.

---

### Step 2 — Keyboard navigation and search in select dropdowns (~2h)

**Depends on Step 1 (shares `FieldEditor.tsx`). Can be done in parallel with Step 5.**

Both `SingleSelectEditor` and `MultiSelectEditor` get:

1. **Search input** — a text `<input>` at the top of the portal dropdown with placeholder
   `t('table.searchOptions')`. Filters displayed options by `opt.name.toLowerCase().includes(query)`.
   Auto-focused when dropdown opens. Does NOT affect stored values; purely a display filter.
   Reset to `''` when dropdown closes.

2. **Keyboard navigation** — `createSignal<number>(-1)` tracks the focused index within the
   filtered list. On `ArrowDown` / `ArrowUp` from the search input (or anywhere in the dropdown):
   move focus index, wrap at boundaries. On `Enter`: select the focused option (same as click).
   On `Escape`: close dropdown (already implemented).

3. **Visual focus ring** — add `.optionItemFocused` CSS class applied when index matches.

Implementation notes:
- Extract a shared `useSelectDropdown` composable (or inline per-component — keep it simple) that
  manages `open`, `dropPos`, `query`, `focusedIndex`, click-outside, and Escape.
- The search input `onKeyDown` must intercept Arrow keys and Enter before they bubble.
- For `MultiSelectEditor`, pressing Enter on a focused option toggles it (does not close the
  dropdown), so the user can pick multiple items by keyboard.

**New i18n keys:**
```
table.searchOptions: "Search options…"   // en
```
Add to all 4 dictionaries.

**Tests:** Add keyboard nav tests to `e2e/table-operations.spec.ts`.

---

### Step 3 — Option management UI (biggest session, ~4h)

**Depends on Step 1. Front-end only (uses existing `updateTable` API).**

#### 3a — `RecordsContext` — expose `updateColumns`

Add to `RecordsContext`:
```ts
updateColumns: (columns: Property[]) => Promise<void>
```
Calls `updateTable(wsId, nodeId, { properties: columns })` and refreshes the local `columns`
signal. The full updated `Property[]` (including all non-modified columns) must be sent because
the API replaces the entire schema.

#### 3b — New component: `SelectOptionsEditor`

File: `src/components/table/SelectOptionsEditor.tsx`
CSS: `src/components/table/SelectOptionsEditor.module.css`

A portal panel (similar in structure to `FilterPanel`) that manages the options of one select or
multi-select column. Opened by the column header context menu. Receives:
```ts
interface SelectOptionsEditorProps {
  column: Property;                         // current column with options
  allColumns: Property[];                   // full schema (needed for updateColumns)
  position: { x: number; y: number };      // viewport position for portal
  onUpdateColumns: (cols: Property[]) => Promise<void>;
  onClose: () => void;
}
```

UI layout (top to bottom):
1. **Header**: column name + close button.
2. **Option list** (`<For each={localOptions()}`):
   - Drag handle (visual only in this step — drag-to-reorder is Step 3c).
   - **Color swatch** (filled circle, 14px): click opens an inline color picker with ~12 preset
     swatches. No free-form hex input needed.
   - **Name input** (`<input type="text">`): inline rename, saves on blur or Enter.
   - **Delete button** (× icon): removes option and saves. Show a warning tooltip if any records
     use this option (client-side check: scan `records` from context).
3. **"+ Add option" button** at the bottom: appends a new option with auto-generated ID
   (`crypto.randomUUID()` trimmed to 8 chars), empty name focused, no color.

State: `createSignal<SelectOption[]>` initialised from `props.column.options ?? []`.
Every mutation (add, rename, delete, recolor) updates local state immediately and calls
`onUpdateColumns` with the full updated schema (debounced 300ms for renames).

**ID generation:** Use `crypto.randomUUID().slice(0, 8)` for new options. IDs must not collide
with existing ones (check and retry on collision).

**Color presets** (12 swatches, accessible palette):
```ts
const OPTION_COLORS = [
  '#e03e3e', '#d9730d', '#dfab01', '#0f7b6c',
  '#0b6e99', '#6940a5', '#ad1a72', '#64473a',
  '#9b9a97', '#37352f', '#787774', '#ffffff',
];
```
`#ffffff` = no color (renders as default `--c-bg-hover`).

#### 3c — Drag-to-reorder options (optional sub-task, can be deferred)

Use the HTML5 drag-and-drop API (`draggable`, `ondragover`, `ondrop`) to reorder the option list
within `SelectOptionsEditor`. On drop, update local state and call `onUpdateColumns`.

#### 3d — Entry point: column header context menu

In `TableTable.tsx`, the column header context menu already has: Rename, Sort Asc/Desc, Remove sort,
Filter by, Insert Left/Right, Delete. Add a new action **"Edit options"** for `select` and
`multi_select` columns only, positioned after "Rename". Clicking it opens
`SelectOptionsEditor` as a portal panel anchored to the column header.

#### 3e — Entry point: `AddColumnDropdown`

When the user selects `select` or `multi_select` as type, expand the dropdown form to show an
initial option list (same UI as in `SelectOptionsEditor` but inlined, not a portal). The user can
add/name options before confirming. The `Property` passed to `onAddColumn` will include the
options array. This removes the awkward "create a select column then immediately edit options"
workflow.

**New i18n keys:**
```
table.editOptions: "Edit options"
table.addOption: "Add an option"
table.optionPlaceholder: "Option name"
table.deleteOption: "Delete option"
table.optionUsedWarning: "Used by {n} record(s)"
```
Add to all 4 dictionaries.

**Tests:** New `e2e/select-options.spec.ts`:
- Add options when creating a select column.
- Edit option name and color from column header menu.
- Delete an unused option.
- Delete an option used by records (verify warning shown; records updated after deletion).

---

### Step 4 — Filter panel option picker (~1.5h)

**Fully independent. Can be done in parallel with Steps 1, 2, or 3.**

In `FilterPanel.tsx`, when `column.type === PropertyTypeSelect || column.type === PropertyTypeMultiSelect`
and `needsValue()`, replace the plain `<input type="text">` with a custom option picker.

The picker renders as a scrollable list of chips (one per `column.options`), each toggleable.
- For `select` columns with `equals` / `not_equals` operators: single selection (radio-like).
- For `multi_select` or `contains` operator: multi-selection (checkbox-like), stored as comma-
  separated option IDs — the same format used in record data.

The `value` signal stores the selected option ID(s) as a CSV string, which is what the filter
engine already expects.

Add a small search input above the list (reuse the same pattern as Step 2).

`FilterPanel.module.css`: add `.optionPicker`, `.optionPickerItem`, `.optionPickerItemSelected`.

**New i18n keys:** None (search input reuses `table.searchOptions` from Step 2).

**Tests:** Extend `e2e/views-api.spec.ts` to filter a select column by clicking an option chip.

---

### Step 5 — Board view: multi-select grouping (~2h)

**Fully independent. Can be done in parallel with Step 2.**

**Current bug:** `TableBoard.tsx` groups records by treating the cell value as a single option ID.
A multi-select record value `"bug,feature"` is treated as one unknown group key, so the record
appears in an "(no value)" group instead of both "bug" and "feature" groups.

**Fix:**

In `TableBoard.tsx`, refactor `groupedRecords()` to support multi-select:

```ts
// For each record, get the list of matching group IDs.
function getRecordGroupIds(record: DataRecordResponse, col: Property): string[] {
  const raw = String(record.data[col.name] ?? '');
  if (!raw) return ['__none__'];
  if (col.type === PropertyTypeMultiSelect) {
    return raw.split(',').map(s => s.trim()).filter(Boolean);
  }
  return [raw]; // single select
}
```

When building group buckets: for each record, push it into ALL groups returned by
`getRecordGroupIds`. A record can now appear in multiple columns of the board.

**Drag-and-drop** on the board currently calls `updateRecord` with a single value. For
multi-select columns, dragging a card from "bug" to "feature" should:
- Remove `"bug"` from the record's tags.
- Add `"feature"` to the record's tags.

This is a semantic question: current implementation replaces the entire value. Decide on
behaviour: replace-all (simpler) or add-to-existing (Notion-like). Document the choice in the
component file header. Suggest: **replace-all** for simplicity; revisit if needed.

**Tests:** Add a board-view multi-select test to `e2e/table-operations.spec.ts`.

---

### Step 6 — E2E test coverage (~2h)

**Depends on Steps 1–5 being complete.**

Extend `e2e/table-operations.spec.ts`:
- Chips render consistently in table, gallery, grid, and board views.
- Keyboard nav: arrow-key through options and press Enter to select.
- All-selected multi-select: verify the dropdown still opens and shows checked items.

New `e2e/select-options.spec.ts`:
- Create a select column with 3 options defined inline (Step 3e).
- Rename an option from the column header context menu (Step 3d).
- Recolor an option by clicking a swatch (Step 3b).
- Delete an option not used by any record.
- Delete an option used by a record and verify the record's value is cleared.
- Verify option order is preserved across page reload.

Extend `e2e/views-api.spec.ts`:
- Filter a select column using the option-picker UI (Step 4).
- Filter a multi-select column with the `contains` operator.

---

## Parallelism Guide

```
Session 1 (Step 1)  ──────────────────┐
                                       ├── merge ──> Session 3 (Step 3)
Session 4 (FilterPanel) ──────────────┘            │
                                                     │
Session 2 (Step 2)  ──────────────────┐             │
                                       ├── merge ──> Session 6 (Step 6)
Session 5 (Board)   ──────────────────┘             │
                                                     │
                        Session 3 (Step 3) ─────────┘
```

- **Steps 1 and 4** touch completely different files — fully parallel.
- **Steps 2 and 5** touch completely different files — fully parallel.
- **Step 3** touches `FieldEditor.tsx` and `TableTable.tsx`; start after Step 1 lands to avoid
  conflicts on `FieldEditor.tsx`.
- **Step 6** should be the final session once all features are in place.

---

## Non-goals (explicitly out of scope)

- Option-level permissions or visibility rules.
- Free-form color hex input (presets only).
- Server-side validation of option IDs (backend already stores whatever is sent).
- Relation / rollup / formula column types (defined in backend, not exposed yet).
- Option usage count in the editor (beyond the simple client-side warning in Step 3b).
