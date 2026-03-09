# Undo System Implementation Plan

## Current State

**ProseMirror editor undo is natively supported** via the `prosemirror-history` plugin loaded in
`prosemirror-config.ts`. `baseKeymap` provides Ctrl-Z / Cmd-Z within the editor for text edits.
This covers content editing but NOT title edits, table operations, or node tree changes.

**Application-level undo is NOT implemented.** No `src/stores/` directory exists. No
`UndoCommand`, `undoStack`, or Ctrl-Z listener exists outside the ProseMirror editor.

## Scope

Given ProseMirror handles editor content natively, the remaining work is:

1. **Phase 1 — Table record CRUD** (`addRecord`, `updateRecord`, `deleteRecord`)
2. **Phase 2 — Title changes** (simple old/new string swap)
3. **Phase 3 — Node tree CRUD** (create/delete pages/tables, lower priority)

## Design Constraints

- **Context-based architecture:** State lives in `EditorContext.tsx`, `RecordsContext.tsx`, and
  `WorkspaceContext.tsx`. There is no central `App.tsx` handler.
- **Auto-save:** Content has a 2-second debounced auto-save. Undo granularity should not conflict.
- **API-backed:** All undo actions require API calls to persist rollbacks.

## Recommended Architecture: context-local stacks + global Ctrl-Z listener

Each context maintains its own undo stack via a shared `useUndo()` hook
(`frontend/src/hooks/useUndo.ts`). A single Ctrl-Z listener in `WorkspaceLayout.tsx` dispatches
to whichever context is currently active. Clear the stack on node navigation.

```typescript
interface UndoAction {
  description: string;
  undo(): Promise<void>;
  redo(): Promise<void>;
}
```

## Implementation Steps

1. [ ] Create `frontend/src/hooks/useUndo.ts` — generic stack (max 50, push/pop/clear)
2. [ ] Add Ctrl-Z / Ctrl-Shift-Z listener in `WorkspaceLayout.tsx`
3. [ ] Wrap `RecordsContext` operations:
   - `addRecord` → undo deletes by ID
   - `updateRecord` → undo restores previous field values
   - `deleteRecord` → undo re-creates with full data
4. [ ] Wrap `EditorContext.handleTitleChange` — store old/new title
5. [ ] Add i18n keys: `common.undo`, `common.redo` to all 4 dictionaries
6. [ ] (Optional) Toast on Ctrl-Z: "Undid: [description]"
7. [ ] Write unit tests for the hook; E2E tests for record undo

## Open Questions

- Should rapid field edits in RecordDetail be merged into one undo action?
- Show toast feedback on undo, or silent?
- Clear undo stack on node navigation? (Recommended: yes)
