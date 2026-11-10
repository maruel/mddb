// Generic undo/redo stack hook with a maximum depth of 50 actions.

import { createSignal } from "solid-js";

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
  let generation = 0;
  let busy = false;

  function push(action: UndoAction) {
    generation++;
    setUndoStack((s) => {
      const trimmed = s.length >= MAX_STACK ? s.slice(s.length - MAX_STACK + 1) : s;
      return [...trimmed, action];
    });
    setRedoStack([]);
  }

  async function undo() {
    if (busy) return;
    const s = undoStack();
    if (s.length === 0) return;
    const action = s[s.length - 1] as UndoAction;
    const previousRedo = redoStack();
    const attempt = ++generation;
    busy = true;
    setUndoStack(s.slice(0, -1));
    if (action.redo !== undefined) {
      setRedoStack((r) => [...r, action]);
    }
    try {
      await action.undo();
    } catch (error) {
      if (generation === attempt) {
        setUndoStack(s);
        setRedoStack(previousRedo);
      }
      throw error;
    } finally {
      busy = false;
    }
  }

  async function redo() {
    if (busy) return;
    const r = redoStack();
    if (r.length === 0) return;
    const action = r[r.length - 1] as UndoAction;
    const previousUndo = undoStack();
    const attempt = ++generation;
    busy = true;
    setRedoStack(r.slice(0, -1));
    setUndoStack((s) => [...s, action]);
    try {
      if (action.redo) await action.redo();
    } catch (error) {
      if (generation === attempt) {
        setUndoStack(previousUndo);
        setRedoStack(r);
      }
      throw error;
    } finally {
      busy = false;
    }
  }

  function clear() {
    generation++;
    setUndoStack([]);
    setRedoStack([]);
  }

  const canUndo = () => undoStack().length > 0;
  const canRedo = () => redoStack().length > 0;

  return { push, undo, redo, clear, canUndo, canRedo };
}
