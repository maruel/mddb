# mddb frontend handles compound queries and Markdown tables

Table views express compound filters, and the editor edits Markdown tables
instead of disabling them.

## Phase 1 — compound-filters: Combine filters with AND and OR

- **Scope:** Filter UI for nested `and` and `or` groups across columns. The
  backend `Filter` model and `RecordsContext` already evaluate compound
  filters; the per-column `FilterPanel` holds one condition per column.
- **Preserve:** Existing per-column filters and chips keep working, and views
  saved with them load unchanged.
- **Verify:** A saved view with an OR group across two columns returns the same
  records from client-side and server-side execution after reload. An e2e test
  covers creating and editing the group.

## Phase 2 — markdown-tables: Edit Markdown tables in the block editor

- **Scope:** A `table` node in the ProseMirror schema, parsing in
  `markdown-parser.ts`, and serialization. `md.disable("table")` currently
  keeps table syntax as text to avoid a prosemirror-markdown crash.
- **Verify:** A page with a GFM table round-trips through load, edit, and save
  without changing unrelated Markdown. Component tests cover header, alignment,
  and escaped pipe cells.

## Later

- Sidebar reads workspace context instead of 14 callback props.
- Retry controls for failed operations.
- `aria-label` on icon-only buttons that rely on `title`.
- Logging for table lookups that reference missing columns.
- Audit that every event listener is removed on cleanup.
- Bulk record actions.
- Regional date and number formatting.
- Command palette (Ctrl+K), table virtualization for 50k+ records,
  relationship graph, and per-organization themes.
