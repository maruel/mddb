// Generic undo/redo stack hook with a maximum depth of 50 actions.

import { createSignal } from 'solid-js';

export interface UndoAction {
  description: string;
  undo(): Promise<void>;
  redo?(): Promise<void>;
}

const MAX_STACK = 50;

/**
 * useUndo provides a bounded undo/redo stack.
 *
 * - push(action): add an action and clear the redo stack.
 * - undo(): pop the latest action, run action.undo(), push to redo if action.redo exists.
 * - redo(): pop the latest redo action, run action.redo(), push back to undo stack.
 * - clear(): empty both stacks (e.g. on node navigation).
 * - canUndo() / canRedo(): reactive accessors for UI state.
 */
export function useUndo() {
  const [undoStack, setUndoStack] = createSignal<UndoAction[]>([]);
  const [redoStack, setRedoStack] = createSignal<UndoAction[]>([]);

  function push(action: UndoAction) {
    setUndoStack((s) => {
      const trimmed = s.length >= MAX_STACK ? s.slice(s.length - MAX_STACK + 1) : s;
      return [...trimmed, action];
    });
    setRedoStack([]);
  }

  async function undo() {
    const s = undoStack();
    if (s.length === 0) return;
    const action = s[s.length - 1] as UndoAction;
    setUndoStack(s.slice(0, -1));
    if (action.redo !== undefined) {
      setRedoStack((r) => [...r, action]);
    }
    await action.undo();
  }

  async function redo() {
    const r = redoStack();
    if (r.length === 0) return;
    const action = r[r.length - 1] as UndoAction;
    setRedoStack(r.slice(0, -1));
    setUndoStack((s) => [...s, action]);
    if (action.redo) await action.redo();
  }

  function clear() {
    setUndoStack([]);
    setRedoStack([]);
  }

  const canUndo = () => undoStack().length > 0;
  const canRedo = () => redoStack().length > 0;

  return { push, undo, redo, clear, canUndo, canRedo };
}
