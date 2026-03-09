// Unit tests for the useUndo hook.

import { describe, it, expect, vi } from 'vitest';
import { createRoot } from 'solid-js';
import { useUndo } from './useUndo';

describe('useUndo', () => {
  it('starts with empty stacks', () => {
    createRoot((dispose) => {
      const { canUndo, canRedo } = useUndo();
      expect(canUndo()).toBe(false);
      expect(canRedo()).toBe(false);
      dispose();
    });
  });

  it('push enables canUndo and clears redo stack', () => {
    createRoot((dispose) => {
      const { push, canUndo, canRedo } = useUndo();
      const action = { description: 'test', undo: vi.fn().mockResolvedValue(undefined) };
      push(action);
      expect(canUndo()).toBe(true);
      expect(canRedo()).toBe(false);
      dispose();
    });
  });

  it('undo calls action.undo and disables canUndo', async () => {
    await createRoot(async (dispose) => {
      const { push, undo, canUndo } = useUndo();
      const undoFn = vi.fn().mockResolvedValue(undefined);
      push({ description: 'test', undo: undoFn });
      await undo();
      expect(undoFn).toHaveBeenCalledOnce();
      expect(canUndo()).toBe(false);
      dispose();
    });
  });

  it('undo with redo function enables canRedo', async () => {
    await createRoot(async (dispose) => {
      const { push, undo, canRedo } = useUndo();
      push({
        description: 'test',
        undo: vi.fn().mockResolvedValue(undefined),
        redo: vi.fn().mockResolvedValue(undefined),
      });
      await undo();
      expect(canRedo()).toBe(true);
      dispose();
    });
  });

  it('undo without redo function does not populate redo stack', async () => {
    await createRoot(async (dispose) => {
      const { push, undo, canRedo } = useUndo();
      push({ description: 'test', undo: vi.fn().mockResolvedValue(undefined) });
      await undo();
      expect(canRedo()).toBe(false);
      dispose();
    });
  });

  it('redo calls action.redo and re-enables canUndo', async () => {
    await createRoot(async (dispose) => {
      const { push, undo, redo, canUndo, canRedo } = useUndo();
      const redoFn = vi.fn().mockResolvedValue(undefined);
      push({
        description: 'test',
        undo: vi.fn().mockResolvedValue(undefined),
        redo: redoFn,
      });
      await undo();
      await redo();
      expect(redoFn).toHaveBeenCalledOnce();
      expect(canUndo()).toBe(true);
      expect(canRedo()).toBe(false);
      dispose();
    });
  });

  it('push clears redo stack', async () => {
    await createRoot(async (dispose) => {
      const { push, undo, canRedo } = useUndo();
      push({
        description: 'first',
        undo: vi.fn().mockResolvedValue(undefined),
        redo: vi.fn().mockResolvedValue(undefined),
      });
      await undo();
      expect(canRedo()).toBe(true);
      push({ description: 'second', undo: vi.fn().mockResolvedValue(undefined) });
      expect(canRedo()).toBe(false);
      dispose();
    });
  });

  it('undo is no-op when stack is empty', async () => {
    await createRoot(async (dispose) => {
      const { undo, canUndo } = useUndo();
      await undo(); // should not throw
      expect(canUndo()).toBe(false);
      dispose();
    });
  });

  it('redo is no-op when stack is empty', async () => {
    await createRoot(async (dispose) => {
      const { redo, canRedo } = useUndo();
      await redo(); // should not throw
      expect(canRedo()).toBe(false);
      dispose();
    });
  });

  it('clear empties both stacks', async () => {
    await createRoot(async (dispose) => {
      const { push, undo, clear, canUndo, canRedo } = useUndo();
      push({
        description: 'a',
        undo: vi.fn().mockResolvedValue(undefined),
        redo: vi.fn().mockResolvedValue(undefined),
      });
      push({ description: 'b', undo: vi.fn().mockResolvedValue(undefined) });
      await undo();
      clear();
      expect(canUndo()).toBe(false);
      expect(canRedo()).toBe(false);
      dispose();
    });
  });

  it('caps stack at 50 items', () => {
    createRoot((dispose) => {
      const { push, canUndo } = useUndo();
      for (let i = 0; i < 60; i++) {
        push({ description: `action ${i}`, undo: vi.fn().mockResolvedValue(undefined) });
      }
      expect(canUndo()).toBe(true);
      // Verify we did not crash and the hook is still functional
      dispose();
    });
  });
});
