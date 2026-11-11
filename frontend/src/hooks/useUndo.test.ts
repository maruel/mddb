// Unit tests for the useUndo hook.

import { describe, it } from "node:test";
import { expect, vi } from "@tests/expect";
import { createRoot } from "solid-js";
import { useUndo } from "./useUndo";

describe("useUndo", () => {
  it("retains failed undo and redo actions", async () => {
    await createRoot(async (dispose) => {
      const { push, undo, redo, canUndo, canRedo } = useUndo();
      let failUndo = true;
      push({
        description: "record",
        undo: async () => {
          if (failUndo) throw new Error("conflict");
        },
        redo: async () => {
          throw new Error("conflict");
        },
      });
      await expect(undo()).rejects.toThrow("conflict");
      expect(canUndo()).toBe(true);
      expect(canRedo()).toBe(false);
      failUndo = false;
      await undo();
      await expect(redo()).rejects.toThrow("conflict");
      expect(canUndo()).toBe(false);
      expect(canRedo()).toBe(true);
      dispose();
    });
  });

  it("retains both actions when an undo fails after a newer action is pushed", async () => {
    await createRoot(async (dispose) => {
      const { push, undo } = useUndo();
      let reject!: (error: Error) => void;
      const gate = new Promise<void>((_resolve, fail) => {
        reject = fail;
      });
      const applied: string[] = [];
      let first = true;
      push({
        description: "older",
        undo: async () => {
          if (first) {
            first = false;
            await gate;
          }
          applied.push("older");
        },
      });
      const pending = undo();
      push({
        description: "newer",
        undo: async () => {
          applied.push("newer");
        },
      });
      reject(new Error("conflict"));
      await expect(pending).rejects.toThrow("conflict");
      await undo();
      await undo();
      expect(applied).toEqual(["newer", "older"]);
      dispose();
    });
  });

  it("does not restore history after a clear during a successful undo", async () => {
    await createRoot(async (dispose) => {
      const { push, undo, clear, canUndo, canRedo } = useUndo();
      let release!: () => void;
      const gate = new Promise<void>((resolve) => {
        release = resolve;
      });
      push({ description: "record", undo: async () => gate, redo: async () => undefined });
      const pending = undo();
      clear();
      release();
      await pending;
      expect(canUndo()).toBe(false);
      expect(canRedo()).toBe(false);
      dispose();
    });
  });

  it("keeps a redo completed after a newer action as the next undo", async () => {
    await createRoot(async (dispose) => {
      const { push, undo, redo } = useUndo();
      let release!: () => void;
      const gate = new Promise<void>((resolve) => {
        release = resolve;
      });
      const applied: string[] = [];
      push({
        description: "older",
        undo: async () => {
          applied.push("older");
        },
        redo: async () => gate,
      });
      await undo();
      applied.length = 0;
      const pending = redo();
      push({
        description: "newer",
        undo: async () => {
          applied.push("newer");
        },
      });
      release();
      await pending;
      await undo();
      await undo();
      expect(applied).toEqual(["older", "newer"]);
      dispose();
    });
  });

  it("starts with empty stacks", () => {
    createRoot((dispose) => {
      const { canUndo, canRedo } = useUndo();
      expect(canUndo()).toBe(false);
      expect(canRedo()).toBe(false);
      dispose();
    });
  });

  it("push enables canUndo and clears redo stack", () => {
    createRoot((dispose) => {
      const { push, canUndo, canRedo } = useUndo();
      const action = { description: "test", undo: vi.fn().mockResolvedValue(undefined) };
      push(action);
      expect(canUndo()).toBe(true);
      expect(canRedo()).toBe(false);
      dispose();
    });
  });

  it("undo calls action.undo and disables canUndo", async () => {
    await createRoot(async (dispose) => {
      const { push, undo, canUndo } = useUndo();
      const undoFn = vi.fn().mockResolvedValue(undefined);
      push({ description: "test", undo: undoFn });
      await undo();
      expect(undoFn).toHaveBeenCalledOnce();
      expect(canUndo()).toBe(false);
      dispose();
    });
  });

  it("undo with redo function enables canRedo", async () => {
    await createRoot(async (dispose) => {
      const { push, undo, canRedo } = useUndo();
      push({
        description: "test",
        undo: vi.fn().mockResolvedValue(undefined),
        redo: vi.fn().mockResolvedValue(undefined),
      });
      await undo();
      expect(canRedo()).toBe(true);
      dispose();
    });
  });

  it("undo without redo function does not populate redo stack", async () => {
    await createRoot(async (dispose) => {
      const { push, undo, canRedo } = useUndo();
      push({ description: "test", undo: vi.fn().mockResolvedValue(undefined) });
      await undo();
      expect(canRedo()).toBe(false);
      dispose();
    });
  });

  it("redo calls action.redo and re-enables canUndo", async () => {
    await createRoot(async (dispose) => {
      const { push, undo, redo, canUndo, canRedo } = useUndo();
      const redoFn = vi.fn().mockResolvedValue(undefined);
      push({
        description: "test",
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

  it("push clears redo stack", async () => {
    await createRoot(async (dispose) => {
      const { push, undo, canRedo } = useUndo();
      push({
        description: "first",
        undo: vi.fn().mockResolvedValue(undefined),
        redo: vi.fn().mockResolvedValue(undefined),
      });
      await undo();
      expect(canRedo()).toBe(true);
      push({ description: "second", undo: vi.fn().mockResolvedValue(undefined) });
      expect(canRedo()).toBe(false);
      dispose();
    });
  });

  it("undo is no-op when stack is empty", async () => {
    await createRoot(async (dispose) => {
      const { undo, canUndo } = useUndo();
      await undo(); // should not throw
      expect(canUndo()).toBe(false);
      dispose();
    });
  });

  it("redo is no-op when stack is empty", async () => {
    await createRoot(async (dispose) => {
      const { redo, canRedo } = useUndo();
      await redo(); // should not throw
      expect(canRedo()).toBe(false);
      dispose();
    });
  });

  it("clear empties both stacks", async () => {
    await createRoot(async (dispose) => {
      const { push, undo, clear, canUndo, canRedo } = useUndo();
      push({
        description: "a",
        undo: vi.fn().mockResolvedValue(undefined),
        redo: vi.fn().mockResolvedValue(undefined),
      });
      push({ description: "b", undo: vi.fn().mockResolvedValue(undefined) });
      await undo();
      clear();
      expect(canUndo()).toBe(false);
      expect(canRedo()).toBe(false);
      dispose();
    });
  });

  it("releases trimmed actions and clears retained history after more than 50 edits", async () => {
    await createRoot(async (dispose) => {
      const { push, undo, clear, canUndo, canRedo } = useUndo();
      const undone: number[] = [];
      for (let i = 0; i < 60; i++) {
        const snapshot = { index: i, payload: "x".repeat(4096) };
        push({
          description: `edit ${i}`,
          undo: async () => {
            undone.push(snapshot.index);
          },
          redo: async () => undefined,
        });
      }
      for (let i = 0; i < 60; i++) await undo();
      expect(undone).toEqual(Array.from({ length: 50 }, (_, i) => 59 - i));
      expect(canUndo()).toBe(false);
      expect(canRedo()).toBe(true);
      clear();
      expect(canUndo()).toBe(false);
      expect(canRedo()).toBe(false);
      dispose();
    });
  });

  it("caps stack at 50 items", () => {
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
