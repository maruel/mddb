// Unit tests for prosemirror-config: verifies undo/redo keybindings are wired.
import { describe, it, expect } from 'vitest';
import { undo, redo } from 'prosemirror-history';
import { schema, createEditorState } from './prosemirror-config';

function makeDoc(text: string) {
  return schema.nodes.doc!.create(
    null,
    schema.nodes.block!.create({ type: 'paragraph', indent: 0, checked: null }, schema.text(text))
  );
}

describe('Undo/Redo keybindings', () => {
  it('undo reverts a text insertion', () => {
    const doc = makeDoc('Hello');
    const state = createEditorState(doc);

    // Insert " world" at end of content
    const endPos = state.doc.content.size - 1;
    const tr = state.tr.insertText(' world', endPos);
    const stateAfterInsert = state.apply(tr);

    expect(stateAfterInsert.doc.textContent).toBe('Hello world');

    // Undo
    let stateAfterUndo = stateAfterInsert;
    undo(stateAfterInsert, (undoTr) => {
      stateAfterUndo = stateAfterInsert.apply(undoTr);
    });

    expect(stateAfterUndo.doc.textContent).toBe('Hello');
  });

  it('redo re-applies after undo', () => {
    const doc = makeDoc('Hello');
    const state = createEditorState(doc);

    const endPos = state.doc.content.size - 1;
    const tr = state.tr.insertText(' world', endPos);
    const stateAfterInsert = state.apply(tr);

    // Undo
    let stateAfterUndo = stateAfterInsert;
    undo(stateAfterInsert, (undoTr) => {
      stateAfterUndo = stateAfterInsert.apply(undoTr);
    });

    expect(stateAfterUndo.doc.textContent).toBe('Hello');

    // Redo
    let stateAfterRedo = stateAfterUndo;
    redo(stateAfterUndo, (redoTr) => {
      stateAfterRedo = stateAfterUndo.apply(redoTr);
    });

    expect(stateAfterRedo.doc.textContent).toBe('Hello world');
  });

  it('undo is a no-op when history is empty', () => {
    const doc = makeDoc('Hello');
    const state = createEditorState(doc);

    let dispatched = false;
    undo(state, () => {
      dispatched = true;
    });

    expect(dispatched).toBe(false);
  });
});
