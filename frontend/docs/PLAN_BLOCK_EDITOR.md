# Flat Block Editor — Implementation Status

Flat block model (Notion-style) is fully implemented and tested. Schema, markdown round-trip,
drag-and-drop, keyboard commands, input rules, migration, comprehensive unit tests (308+),
and E2E tests all working.

## Recent Fixes (Completed)

### List Item Vertical Alignment — FIXED

Bullet, numbered, and task list items now have consistent vertical alignment via `::before`
pseudo-elements. All three list types correctly align with text content.

**Location:** `Editor.module.css` (`.block-bullet`, `.block-number`, `.block-task`)

### Context Menu — FIXED

1. ✓ Hover highlighting works on all menu items via `onMouseEnter` state update
2. ✓ Arrow key navigation fully implemented (ArrowUp/ArrowDown with wraparound)
3. ✓ Undo stack integrity maintained via single-transaction commands

**Location:** `BlockContextMenu.tsx`, `shared/ContextMenu.tsx`

## E2E Tests

- ✓ `e2e/block-context-menu.spec.ts` — context menu hover, clicks, multi-block actions
- ✓ `e2e/block-drag-reorder.spec.ts` — drag-drop reordering via synthetic events
- ✓ `e2e/block-editor.spec.ts` — input rules, keyboard navigation, block creation
- ✓ `e2e/block-handle-visibility.spec.ts` — drag handle rendering

## Deferred Work

- [ ] Keyboard undo/redo (Ctrl-Z) — ProseMirror `history()` plugin is loaded but keybindings
      not wired; tracked in `PLAN_UNDO.md`
- [ ] Table row reorder persistence (pending backend sort API decision)
- [ ] Touch drag support (long-press initiation)
- [ ] Nested container blocks (toggles, callouts, columns) — `nested-schema.ts` exists but unused

## Key Decisions

1. **Multi-block selection:** Implemented — topmost handle visible, drag moves all, context menu applies to all.
2. **Code blocks:** Single movable unit (entire fenced block).
3. **Cross-context drag:** Not supported (editor ↔ table out of scope).
4. **Block IDs:** Not needed — markdown is storage, positions suffice at runtime.
5. **Nested blocks:** Future — flat `indent` model sufficient for hierarchical lists.

## E2E Drag Testing Note

Playwright's native drag methods don't reliably trigger `draggable="true"` events. Use synthetic
`DragEvent` dispatch via `page.evaluate()`. See `e2e/block-drag-reorder.spec.ts` for the pattern.
Y coordinate determines drop position: upper third = above, lower third = below.
