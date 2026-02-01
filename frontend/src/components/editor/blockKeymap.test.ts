import { describe, it, expect } from 'vitest';
import { EditorState, TextSelection, type Command, type Transaction } from 'prosemirror-state';
import type { Node as ProseMirrorNode } from 'prosemirror-model';
import { schema } from './schema';
import { blockKeymapCommands } from './blockKeymap';

const { indent, outdent, enter, backspace } = blockKeymapCommands;

// Helpers to create nodes
const doc = (content: ProseMirrorNode | ProseMirrorNode[]) => schema.nodes.doc!.create(null, content);
const block = (attrs: Record<string, unknown>, content: ProseMirrorNode | ProseMirrorNode[] = []) =>
  schema.nodes.block!.create(attrs, content);
const text = (str: string) => schema.text(str);

// Helper to check command execution
function applyCommand(command: Command, docNode: ProseMirrorNode, selectionPos: number) {
  const state = EditorState.create({
    doc: docNode,
    selection: TextSelection.create(docNode, selectionPos),
  });

  let dispatched = false;
  let newState = state;
  const dispatch = (tr: Transaction) => {
    dispatched = true;
    newState = state.apply(tr);
  };

  const result = command(state, dispatch);
  return { result, dispatched, newState };
}

describe('Block Keymap Commands', () => {
  describe('Indent (Tab)', () => {
    it('indents a bullet list item', () => {
      const startDoc = doc(block({ type: 'bullet', indent: 0 }, text('Item')));
      const { result, newState } = applyCommand(indent, startDoc, 1);

      expect(result).toBe(true);
      const newBlock = newState.doc.firstChild!;
      expect(newBlock.attrs.indent).toBe(1);
    });

    it('does not indent paragraph', () => {
      const startDoc = doc(block({ type: 'paragraph', indent: 0 }, text('Para')));
      const { result } = applyCommand(indent, startDoc, 1);

      expect(result).toBe(false);
    });

    it('does not indent beyond max level (8)', () => {
      const startDoc = doc(block({ type: 'bullet', indent: 8 }, text('Item')));
      const { result } = applyCommand(indent, startDoc, 1);

      expect(result).toBe(false);
    });
  });

  describe('Outdent (Shift-Tab)', () => {
    it('outdents a bullet list item', () => {
      const startDoc = doc(block({ type: 'bullet', indent: 1 }, text('Item')));
      const { result, newState } = applyCommand(outdent, startDoc, 1);

      expect(result).toBe(true);
      const newBlock = newState.doc.firstChild!;
      expect(newBlock.attrs.indent).toBe(0);
    });

    it('does not outdent at level 0', () => {
      const startDoc = doc(block({ type: 'bullet', indent: 0 }, text('Item')));
      const { result } = applyCommand(outdent, startDoc, 1);

      expect(result).toBe(false);
    });
  });

  describe('Enter', () => {
    it('splits a bullet list item', () => {
      const startDoc = doc(block({ type: 'bullet' }, text('Item')));
      // Cursor at end: "Item|"
      const { result, newState } = applyCommand(enter, startDoc, 5);

      expect(result).toBe(true);
      expect(newState.doc.childCount).toBe(2);
      expect(newState.doc.child(0).textContent).toBe('Item');
      expect(newState.doc.child(1).attrs.type).toBe('bullet');
    });

    it('converts empty bullet to paragraph (outdent behavior)', () => {
      const startDoc = doc(block({ type: 'bullet', indent: 0 }));
      const { result, newState } = applyCommand(enter, startDoc, 1);

      expect(result).toBe(true);
      expect(newState.doc.firstChild!.attrs.type).toBe('paragraph');
    });

    it('outdents empty nested bullet', () => {
      const startDoc = doc(block({ type: 'bullet', indent: 1 }));
      const { result, newState } = applyCommand(enter, startDoc, 1);

      expect(result).toBe(true);
      expect(newState.doc.firstChild!.attrs.indent).toBe(0);
      expect(newState.doc.firstChild!.attrs.type).toBe('bullet');
    });

    it('unchecks new task item', () => {
      const startDoc = doc(block({ type: 'task', checked: true }, text('Task 1')));
      const { result, newState } = applyCommand(enter, startDoc, 7); // End of "Task 1"

      expect(result).toBe(true);
      expect(newState.doc.childCount).toBe(2);
      expect(newState.doc.child(1).attrs.type).toBe('task');
      expect(newState.doc.child(1).attrs.checked).toBe(false);
    });
  });

  describe('Backspace', () => {
    it('outdents indented list item at start', () => {
      const startDoc = doc(block({ type: 'bullet', indent: 1 }, text('Item')));
      const { result, newState } = applyCommand(backspace, startDoc, 1); // Start of content

      expect(result).toBe(true);
      expect(newState.doc.firstChild!.attrs.indent).toBe(0);
    });

    it('converts list to paragraph at start (level 0)', () => {
      const startDoc = doc(block({ type: 'bullet', indent: 0 }, text('Item')));
      const { result, newState } = applyCommand(backspace, startDoc, 1);

      expect(result).toBe(true);
      expect(newState.doc.firstChild!.attrs.type).toBe('paragraph');
    });

    it('does nothing if cursor not at start', () => {
      const startDoc = doc(block({ type: 'bullet' }, text('Item')));
      const { result } = applyCommand(backspace, startDoc, 2); // "I|tem"

      expect(result).toBe(false);
    });
  });
});
