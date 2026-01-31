import { describe, it, expect } from 'vitest';
import { EditorState } from 'prosemirror-state';
import type { Node as ProseMirrorNode } from 'prosemirror-model';
import { schema, nodes } from './schema'; // Use flat block schema
import { convertBlocks, indentBlocks, outdentBlocks, deleteBlocks, toggleTaskBlock } from './blockCommands';

// Helper to create a doc with blocks
function createDoc(blocks: ProseMirrorNode[]) {
  return nodes.doc.create(null, blocks);
}

function createBlock(
  type: string,
  attrs: Record<string, unknown> = {},
  content: ProseMirrorNode | string | null = null
) {
  let contentNode = content;
  if (typeof content === 'string') {
    contentNode = schema.text(content);
  }
  return nodes.block.create({ type, indent: 0, ...attrs }, contentNode as ProseMirrorNode | null);
}

describe('blockCommands', () => {
  it('deleteBlocks deletes selected blocks', () => {
    const block1 = createBlock('paragraph', {}, 'Block 1');
    const block2 = createBlock('paragraph', {}, 'Block 2');
    const doc = createDoc([block1, block2]);

    let state = EditorState.create({ doc, schema });

    // Select first block
    // block1 size is content(7) + 2 (tags) = 9? No block is not leaf.
    // paragraph content 'Block 1' size is 7. block node around it: +2. Total 9.
    // doc start 0. block1 starts at 0? No, doc starts at 0, content at 1.
    // block1: 0..9.
    // ProseMirror positions:
    // 0: start of doc
    // 1: start of block1
    // 8: end of text 'Block 1'
    // 9: end of block1
    // 10: start of block2

    // deleteBlocks takes positions of the blocks (start pos)
    const cmd = deleteBlocks([0]);
    // Wait, doc.nodeAt(0) is first block. Pos 0.

    let dispatched = false;
    cmd(state, (tr) => {
      state = state.apply(tr);
      dispatched = true;
    });

    expect(dispatched).toBe(true);
    expect(state.doc.childCount).toBe(1);
    expect(state.doc.firstChild?.textContent).toBe('Block 2');
  });

  it('indentBlocks increases indent', () => {
    const block = createBlock('paragraph', { indent: 0 }, 'Text');
    const doc = createDoc([block]);
    let state = EditorState.create({ doc, schema });

    const cmd = indentBlocks([0]);

    cmd(state, (tr) => {
      state = state.apply(tr);
    });

    expect(state.doc.firstChild?.attrs.indent).toBe(1);
  });

  it('outdentBlocks decreases indent', () => {
    const block = createBlock('paragraph', { indent: 2 }, 'Text');
    const doc = createDoc([block]);
    let state = EditorState.create({ doc, schema });

    const cmd = outdentBlocks([0]);

    cmd(state, (tr) => {
      state = state.apply(tr);
    });

    expect(state.doc.firstChild?.attrs.indent).toBe(1);
  });

  it('convertBlocks changes block type', () => {
    const block = createBlock('paragraph', {}, 'Text');
    const doc = createDoc([block]);
    let state = EditorState.create({ doc, schema });

    const cmd = convertBlocks([0], 'heading', { level: 1 });

    cmd(state, (tr) => {
      state = state.apply(tr);
    });

    expect(state.doc.firstChild?.attrs.type).toBe('heading');
    expect(state.doc.firstChild?.attrs.level).toBe(1);
  });

  it('toggleTaskBlock toggles checked state', () => {
    const block = createBlock('task', { checked: false }, 'Task');
    const doc = createDoc([block]);
    let state = EditorState.create({ doc, schema });

    const cmd = toggleTaskBlock(0); // Pos 0

    cmd(state, (tr) => {
      state = state.apply(tr);
    });

    expect(state.doc.firstChild?.attrs.checked).toBe(true);

    cmd(state, (tr) => {
      state = state.apply(tr);
    });

    expect(state.doc.firstChild?.attrs.checked).toBe(false);
  });
});
