# Undo System Implementation Plan

## Overview

Add Ctrl-Z (Cmd-Z) undo functionality to the frontend with a command stack.

## Undoable Operations

1. **Document edits** - title and markdown content changes (in `App.tsx`)
2. **Table record CRUD** - create, update, delete records
3. **Node tree changes** - create/delete pages/tables

## Implementation Options

### Option 1: Command Pattern

Store invertible commands on a stack. Each action has an `execute()` and `undo()` method.

```typescript
interface UndoCommand {
  execute(): Promise<void>;
  undo(): Promise<void>;
  description: string;
}
```

**Pros:** Memory efficient, clean semantics, can batch/coalesce edits
**Cons:** Must define inverse for every operation type, complex for API-backed changes

### Option 2: State Snapshots

Store complete state snapshots before each operation.

```typescript
const [history, setHistory] = createSignal<AppState[]>([]);
```

**Pros:** Simple, always correct
**Cons:** Memory heavy, especially with large content

### Option 3: Hybrid (Recommended)

Different strategies per operation type:
- **Text edits**: Coalesced diffs (only store changes, batch rapid keystrokes)
- **Table/node CRUD**: Command pattern with API rollback

This matches how Notion works - text has fine-grained undo, structural changes are discrete.

## Key Design Decisions

| Decision | Options | Recommendation |
|----------|---------|----------------|
| Scope | Global vs per-document | Per-document (clearer mental model) |
| Persistence | Session-only vs localStorage | Session-only (simpler) |
| API sync | Local-only vs server rollback | Server rollback (consistency) |
| Granularity | Every keystroke vs batched | Batched (matches 2s auto-save) |

## Proposed Architecture

### File Structure

```
src/
  stores/
    undo.ts          # Undo stack and command definitions
  utils/
    textDiff.ts      # Text diffing for content changes (optional)
```

### Core Interface

```typescript
// src/stores/undo.ts
import { createSignal } from 'solid-js';

interface UndoCommand {
  type: string;
  execute(): Promise<void>;
  undo(): Promise<void>;
  // For coalescing rapid text edits
  canMerge?(other: UndoCommand): boolean;
  merge?(other: UndoCommand): UndoCommand;
}

const MAX_STACK_SIZE = 50;
const [undoStack, setUndoStack] = createSignal<UndoCommand[]>([]);
const [redoStack, setRedoStack] = createSignal<UndoCommand[]>([]);

export function pushCommand(cmd: UndoCommand) { ... }
export function undo(): Promise<void> { ... }
export function redo(): Promise<void> { ... }
export function clearHistory() { ... }
```

### Command Types

1. **TextEditCommand** - For title/content changes
   - Stores old/new values
   - Coalesces rapid edits within ~500ms
   - Calls API on undo to persist

2. **RecordCreateCommand** - For table record creation
   - Stores record ID and data
   - Undo deletes the record via API

3. **RecordUpdateCommand** - For table record updates
   - Stores record ID, old data, new data
   - Undo restores old data via API

4. **RecordDeleteCommand** - For table record deletion
   - Stores full record data
   - Undo recreates the record via API

5. **NodeCreateCommand** / **NodeDeleteCommand** - For page/table CRUD

### Keyboard Handler

```typescript
// In App.tsx onMount
const handleKeyDown = (e: KeyboardEvent) => {
  const isMac = navigator.platform.includes('Mac');
  const modKey = isMac ? e.metaKey : e.ctrlKey;

  if (modKey && e.key === 'z') {
    e.preventDefault();
    if (e.shiftKey) {
      redo();
    } else {
      undo();
    }
  }
};

window.addEventListener('keydown', handleKeyDown);
```

## Implementation Steps

1. [ ] Create `src/stores/undo.ts` with stack management
2. [ ] Add keyboard listener in `App.tsx`
3. [ ] Implement `TextEditCommand` for document edits
4. [ ] Wrap existing handlers to push commands:
   - `setTitle()` / `setContent()` changes
   - `handleAddRecord()` / `handleUpdateRecord()` / `handleDeleteRecord()`
5. [ ] Add undo/redo UI indicators (optional)
6. [ ] Add i18n strings for undo feedback
7. [ ] Write tests

## Open Questions

- Should undo work across node navigation? (Probably not - clear stack on node change)
- Should we show an undo toast/snackbar on Ctrl-Z?
- Redo support (Ctrl-Shift-Z / Ctrl-Y)?
