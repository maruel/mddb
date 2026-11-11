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
 * - undo(): run the latest action and move it to redo after success.
 * - redo(): run the latest redo action and move it back after success.
 * - clear(): empty both stacks (e.g. on node navigation).
 * - canUndo() / canRedo(): reactive accessors for UI state.
 */
export function useUndo() {
  const [undoStack, setUndoStack] = createSignal<UndoAction[]>([]);
  const [redoStack, setRedoStack] = createSignal<UndoAction[]>([]);
  let generation = 0;
  let epoch = 0;
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
    const action = undoStack().at(-1);
    if (!action) return;
    const attempt = generation;
    const owner = epoch;
    busy = true;
    try {
      await action.undo();
      if (owner !== epoch) return;
      setUndoStack((stack) => {
        const index = stack.lastIndexOf(action);
        return index < 0 ? stack : [...stack.slice(0, index), ...stack.slice(index + 1)];
      });
      if (generation === attempt && action.redo) setRedoStack((stack) => [...stack, action]);
    } finally {
      busy = false;
    }
  }

  async function redo() {
    if (busy) return;
    const action = redoStack().at(-1);
    if (!action?.redo) return;
    const owner = epoch;
    busy = true;
    try {
      await action.redo();
      if (owner !== epoch) return;
      setRedoStack((stack) => {
        const index = stack.lastIndexOf(action);
        return index < 0 ? stack : [...stack.slice(0, index), ...stack.slice(index + 1)];
      });
      setUndoStack((stack) => [...stack.slice(-(MAX_STACK - 1)), action]);
    } finally {
      busy = false;
    }
  }

  function clear() {
    generation++;
    epoch++;
    setUndoStack([]);
    setRedoStack([]);
  }

  const canUndo = () => undoStack().length > 0;
  const canRedo = () => redoStack().length > 0;

  return { push, undo, redo, clear, canUndo, canRedo };
}
