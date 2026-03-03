# Flat Block Editor — Remaining Work

Flat block model (Notion-style) is implemented. Schema, markdown round-trip, drag-and-drop,
keyboard commands, input rules, migration, and tests (308 unit, E2E drag reorder) all working.

## Open Bugs

### List Item Vertical Alignment — MEDIUM

Numbered list numbers and bullet points are not vertically aligned with text content.
Task list checkboxes are correctly aligned.

**Location:** `Editor.module.css` (`.block-row[data-type="bullet"]`, `.block-row[data-type="number"]`)

**Suggested fix:** Compare CSS for task vs bullet/number blocks — check `::before` pseudo-element
positioning, `align-items`, and `line-height`. May need explicit vertical centering with flexbox.

### Context Menu — CRITICAL

1. Only first menu item highlights on mouse hover
2. Up/down arrow keyboard navigation has no effect
3. Clicking items works but corrupts undo stack afterward

**Location:** `BlockContextMenu.tsx`, `blockCommands.ts`

**Suggested debug steps:**
- **Hover:** Check CSS `:hover` selectors and z-index stacking
- **Keyboard nav:** Verify `onKeyDown` handler is attached and `e.preventDefault()` called
- **Undo corruption:** Check ProseMirror transactions — ensure single transaction per action,
  no state mutations outside transactions, no `addToHistory: false` meta

## Deferred Work

- [ ] Table row reorder persistence (pending backend sort API decision)
- [ ] Context menu E2E tests
- [ ] Undo/redo E2E tests
- [ ] Touch drag support (long-press initiation)
- [ ] Nested container blocks (toggles, callouts, columns)

## Key Decisions

1. **Multi-block selection:** Yes — topmost handle visible, drag moves all, context menu applies to all.
2. **Code blocks:** Single movable unit (entire fenced block).
3. **Cross-context drag:** Not supported (editor ↔ table out of scope).
4. **Block IDs:** Not needed — markdown is storage, positions suffice at runtime.
5. **Nested blocks:** Future — flat `indent` model sufficient for hierarchical lists.

## E2E Drag Testing Note

Playwright's native drag methods don't reliably trigger `draggable="true"` events. Use synthetic
`DragEvent` dispatch via `page.evaluate()`. See `e2e/block-drag-reorder.spec.ts` for the pattern.
Y coordinate determines drop position: upper third = above, lower third = below.
