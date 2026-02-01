import { describe, it, expect } from 'vitest';
import { EditorState } from 'prosemirror-state';
import {
  deleteBlock,
  deleteBlocks,
  duplicateBlock,
  duplicateBlocks,
  convertBlock,
  convertBlocks,
  indentBlock,
  outdentBlock,
  indentBlocks,
  outdentBlocks,
  toggleTaskBlock,
} from './blockCommands';
import { schema } from './schema';

describe('blockCommands', () => {
  /**
   * Helper to create a test document with blocks
   */
  function createDoc(...blocks: Array<{ type: string; content?: string; attrs?: any }>) {
    const content = blocks.map((b) => {
      const attrs = b.attrs || {};
      const text = b.content ? [schema.text(b.content)] : [];
      return schema.nodes.block.create(
        {
          type: b.type,
          indent: 0,
          ...attrs,
        },
        text
      );
    });
    return schema.nodes.doc.create(undefined, content);
  }

  /**
   * Helper to get position of nth block
   */
  function getBlockPos(doc: any, blockIndex: number) {
    let pos = 0;
    let blockCount = 0;
    doc.forEach((node: any) => {
      if (blockCount === blockIndex) {
        return;
      }
      pos += node.nodeSize;
      blockCount++;
    });
    return pos;
  }

  describe('deleteBlock', () => {
    it('deletes a single block', () => {
      const doc = createDoc(
        { type: 'paragraph', content: 'First' },
        { type: 'paragraph', content: 'Second' },
        { type: 'paragraph', content: 'Third' }
      );
      const state = EditorState.create({ doc, schema });
      const pos = getBlockPos(doc, 1); // Delete second block

      const command = deleteBlock(pos);
      let newState = state;
      const result = command(state, (tr) => {
        newState = state.apply(tr);
      });

      expect(result).toBe(true);
      expect(newState.doc.childCount).toBe(2);
      expect(newState.doc.child(0).textContent).toBe('First');
      expect(newState.doc.child(1).textContent).toBe('Third');
    });

    it('returns false for invalid position', () => {
      const doc = createDoc({ type: 'paragraph', content: 'Only one' });
      const state = EditorState.create({ doc, schema });
      
      let result: boolean;
      try {
        const command = deleteBlock(9999);
        result = command(state, () => {});
      } catch {
        result = false;
      }
      expect(result).toBe(false);
    });

    it('handles dispatch=undefined (query mode)', () => {
      const doc = createDoc({ type: 'paragraph', content: 'Text' });
      const state = EditorState.create({ doc, schema });
      const pos = getBlockPos(doc, 0);

      const command = deleteBlock(pos);
      const result = command(state); // No dispatch callback
      expect(result).toBe(true);
    });
  });

  describe('deleteBlocks', () => {
    it('deletes multiple blocks', () => {
      const doc = createDoc(
        { type: 'paragraph', content: 'First' },
        { type: 'paragraph', content: 'Second' },
        { type: 'paragraph', content: 'Third' },
        { type: 'paragraph', content: 'Fourth' }
      );
      const state = EditorState.create({ doc, schema });

      // Delete second and third blocks
      const positions = [getBlockPos(doc, 1), getBlockPos(doc, 2)];
      const command = deleteBlocks(positions);

      let newState = state;
      const result = command(state, (tr) => {
        newState = state.apply(tr);
      });

      expect(result).toBe(true);
      expect(newState.doc.childCount).toBe(2);
      expect(newState.doc.child(0).textContent).toBe('First');
      expect(newState.doc.child(1).textContent).toBe('Fourth');
    });

    it('returns false for empty positions', () => {
      const doc = createDoc({ type: 'paragraph' });
      const state = EditorState.create({ doc, schema });

      const command = deleteBlocks([]);
      const result = command(state, () => {});
      expect(result).toBe(false);
    });

    it('ignores invalid positions in list', () => {
      const doc = createDoc(
        { type: 'paragraph', content: 'First' },
        { type: 'paragraph', content: 'Second' },
        { type: 'paragraph', content: 'Third' }
      );
      const state = EditorState.create({ doc, schema });

      // Delete first block, and try an invalid position that will be skipped
      const validPos = getBlockPos(doc, 0);
      const command = deleteBlocks([validPos]);
      let newState = state;
      const result = command(state, (tr) => {
        newState = state.apply(tr);
      });

      expect(result).toBe(true);
      expect(newState.doc.childCount).toBe(2);
      expect(newState.doc.child(0).textContent).toBe('Second');
      expect(newState.doc.child(1).textContent).toBe('Third');
    });
  });

  describe('duplicateBlock', () => {
    it('duplicates a single block', () => {
      const doc = createDoc(
        { type: 'paragraph', content: 'First' },
        { type: 'paragraph', content: 'Second' }
      );
      const state = EditorState.create({ doc, schema });
      const pos = getBlockPos(doc, 0);

      const command = duplicateBlock(pos);
      let newState = state;
      const result = command(state, (tr) => {
        newState = state.apply(tr);
      });

      expect(result).toBe(true);
      expect(newState.doc.childCount).toBe(3);
      expect(newState.doc.child(0).textContent).toBe('First');
      expect(newState.doc.child(1).textContent).toBe('First');
      expect(newState.doc.child(2).textContent).toBe('Second');
    });

    it('preserves block attributes', () => {
      const doc = createDoc({
        type: 'heading',
        content: 'Title',
        attrs: { level: 2 },
      });
      const state = EditorState.create({ doc, schema });
      const pos = getBlockPos(doc, 0);

      const command = duplicateBlock(pos);
      let newState = state;
      command(state, (tr) => {
        newState = state.apply(tr);
      });

      expect(newState.doc.child(1).attrs.type).toBe('heading');
      expect(newState.doc.child(1).attrs.level).toBe(2);
    });

    it('returns false for invalid position', () => {
      const doc = createDoc({ type: 'paragraph' });
      const state = EditorState.create({ doc, schema });

      let result: boolean;
      try {
        const command = duplicateBlock(9999);
        result = command(state, () => {});
      } catch {
        result = false;
      }
      expect(result).toBe(false);
    });
  });

  describe('duplicateBlocks', () => {
    it('duplicates multiple blocks', () => {
      const doc = createDoc(
        { type: 'paragraph', content: 'First' },
        { type: 'paragraph', content: 'Second' },
        { type: 'paragraph', content: 'Third' }
      );
      const state = EditorState.create({ doc, schema });

      const positions = [getBlockPos(doc, 0), getBlockPos(doc, 1)];
      const command = duplicateBlocks(positions);

      let newState = state;
      const result = command(state, (tr) => {
        newState = state.apply(tr);
      });

      expect(result).toBe(true);
      expect(newState.doc.childCount).toBe(5);
      // Original: [0, 1, 2], duplicates inserted after position 1 → [0, 1, dup(0), dup(1), 2]
      expect(newState.doc.child(0).textContent).toBe('First');
      expect(newState.doc.child(1).textContent).toBe('Second');
      expect(newState.doc.child(2).textContent).toBe('First');
      expect(newState.doc.child(3).textContent).toBe('Second');
      expect(newState.doc.child(4).textContent).toBe('Third');
    });

    it('returns false for empty positions', () => {
      const doc = createDoc({ type: 'paragraph' });
      const state = EditorState.create({ doc, schema });

      const command = duplicateBlocks([]);
      const result = command(state, () => {});
      expect(result).toBe(false);
    });
  });

  describe('convertBlock', () => {
    it('converts block type', () => {
      const doc = createDoc({ type: 'paragraph', content: 'Text' });
      const state = EditorState.create({ doc, schema });
      const pos = getBlockPos(doc, 0);

      const command = convertBlock(pos, 'heading', { level: 1 });
      let newState = state;
      const result = command(state, (tr) => {
        newState = state.apply(tr);
      });

      expect(result).toBe(true);
      expect(newState.doc.child(0).attrs.type).toBe('heading');
      expect(newState.doc.child(0).attrs.level).toBe(1);
    });

    it('clears type-specific attributes when converting away', () => {
      const doc = createDoc({
        type: 'heading',
        content: 'Title',
        attrs: { level: 2 },
      });
      const state = EditorState.create({ doc, schema });
      const pos = getBlockPos(doc, 0);

      const command = convertBlock(pos, 'paragraph');
      let newState = state;
      command(state, (tr) => {
        newState = state.apply(tr);
      });

      expect(newState.doc.child(0).attrs.type).toBe('paragraph');
      // Schema defaults to null, not undefined
      expect(newState.doc.child(0).attrs.level).toBe(null);
    });

    it('preserves indent when converting', () => {
      const doc = createDoc({
        type: 'paragraph',
        content: 'Text',
        attrs: { indent: 2 },
      });
      const state = EditorState.create({ doc, schema });
      const pos = getBlockPos(doc, 0);

      const command = convertBlock(pos, 'bullet');
      let newState = state;
      command(state, (tr) => {
        newState = state.apply(tr);
      });

      expect(newState.doc.child(0).attrs.indent).toBe(2);
    });

    it('returns false for invalid position', () => {
      const doc = createDoc({ type: 'paragraph' });
      const state = EditorState.create({ doc, schema });

      let result: boolean;
      try {
        const command = convertBlock(9999, 'heading');
        result = command(state, () => {});
      } catch {
        result = false;
      }
      expect(result).toBe(false);
    });
  });

  describe('convertBlocks', () => {
    it('converts multiple blocks', () => {
      const doc = createDoc(
        { type: 'paragraph', content: 'First' },
        { type: 'paragraph', content: 'Second' },
        { type: 'paragraph', content: 'Third' }
      );
      const state = EditorState.create({ doc, schema });

      const positions = [getBlockPos(doc, 0), getBlockPos(doc, 2)];
      const command = convertBlocks(positions, 'bullet');

      let newState = state;
      const result = command(state, (tr) => {
        newState = state.apply(tr);
      });

      expect(result).toBe(true);
      expect(newState.doc.child(0).attrs.type).toBe('bullet');
      expect(newState.doc.child(1).attrs.type).toBe('paragraph');
      expect(newState.doc.child(2).attrs.type).toBe('bullet');
    });

    it('returns false for empty positions', () => {
      const doc = createDoc({ type: 'paragraph' });
      const state = EditorState.create({ doc, schema });

      const command = convertBlocks([], 'heading');
      const result = command(state, () => {});
      expect(result).toBe(false);
    });

    it('returns false if no blocks found at positions', () => {
      const doc = createDoc({ type: 'paragraph' });
      const state = EditorState.create({ doc, schema });

      let result: boolean;
      try {
        const command = convertBlocks([9999], 'heading');
        result = command(state, () => {});
      } catch {
        result = false;
      }
      expect(result).toBe(false);
    });
  });

  describe('indentBlock', () => {
    it('increases indent', () => {
      const doc = createDoc({
        type: 'bullet',
        content: 'Item',
        attrs: { indent: 0 },
      });
      const state = EditorState.create({ doc, schema });
      const pos = getBlockPos(doc, 0);

      const command = indentBlock(pos);
      let newState = state;
      const result = command(state, (tr) => {
        newState = state.apply(tr);
      });

      expect(result).toBe(true);
      expect(newState.doc.child(0).attrs.indent).toBe(1);
    });

    it('respects maximum indent', () => {
      const doc = createDoc({
        type: 'bullet',
        content: 'Item',
        attrs: { indent: 8 },
      });
      const state = EditorState.create({ doc, schema });
      const pos = getBlockPos(doc, 0);

      const command = indentBlock(pos);
      const result = command(state, () => {});

      expect(result).toBe(false);
    });

    it('handles missing indent attribute', () => {
      const doc = createDoc({
        type: 'bullet',
        content: 'Item',
        attrs: { indent: undefined },
      });
      const state = EditorState.create({ doc, schema });
      const pos = getBlockPos(doc, 0);

      const command = indentBlock(pos);
      let newState = state;
      const result = command(state, (tr) => {
        newState = state.apply(tr);
      });

      expect(result).toBe(true);
      expect(newState.doc.child(0).attrs.indent).toBe(1);
    });

    it('returns false for invalid position', () => {
      const doc = createDoc({ type: 'paragraph' });
      const state = EditorState.create({ doc, schema });

      let result: boolean;
      try {
        const command = indentBlock(9999);
        result = command(state, () => {});
      } catch {
        result = false;
      }
      expect(result).toBe(false);
    });
  });

  describe('outdentBlock', () => {
    it('decreases indent', () => {
      const doc = createDoc({
        type: 'bullet',
        content: 'Item',
        attrs: { indent: 2 },
      });
      const state = EditorState.create({ doc, schema });
      const pos = getBlockPos(doc, 0);

      const command = outdentBlock(pos);
      let newState = state;
      const result = command(state, (tr) => {
        newState = state.apply(tr);
      });

      expect(result).toBe(true);
      expect(newState.doc.child(0).attrs.indent).toBe(1);
    });

    it('respects minimum indent', () => {
      const doc = createDoc({
        type: 'bullet',
        content: 'Item',
        attrs: { indent: 0 },
      });
      const state = EditorState.create({ doc, schema });
      const pos = getBlockPos(doc, 0);

      const command = outdentBlock(pos);
      const result = command(state, () => {});

      expect(result).toBe(false);
    });

    it('returns false for invalid position', () => {
      const doc = createDoc({ type: 'paragraph' });
      const state = EditorState.create({ doc, schema });

      let result: boolean;
      try {
        const command = outdentBlock(9999);
        result = command(state, () => {});
      } catch {
        result = false;
      }
      expect(result).toBe(false);
    });
  });

  describe('indentBlocks', () => {
    it('indents multiple blocks', () => {
      const doc = createDoc(
        { type: 'bullet', content: 'First', attrs: { indent: 0 } },
        { type: 'bullet', content: 'Second', attrs: { indent: 1 } },
        { type: 'bullet', content: 'Third', attrs: { indent: 0 } }
      );
      const state = EditorState.create({ doc, schema });

      const positions = [getBlockPos(doc, 0), getBlockPos(doc, 2)];
      const command = indentBlocks(positions);

      let newState = state;
      const result = command(state, (tr) => {
        newState = state.apply(tr);
      });

      expect(result).toBe(true);
      expect(newState.doc.child(0).attrs.indent).toBe(1);
      expect(newState.doc.child(1).attrs.indent).toBe(1); // Unchanged
      expect(newState.doc.child(2).attrs.indent).toBe(1);
    });

    it('returns false for empty positions', () => {
      const doc = createDoc({ type: 'paragraph' });
      const state = EditorState.create({ doc, schema });

      const command = indentBlocks([]);
      const result = command(state, () => {});
      expect(result).toBe(false);
    });

    it('skips blocks already at max indent', () => {
      const doc = createDoc(
        { type: 'bullet', content: 'At max', attrs: { indent: 8 } },
        { type: 'bullet', content: 'Can indent', attrs: { indent: 0 } }
      );
      const state = EditorState.create({ doc, schema });

      const positions = [getBlockPos(doc, 0), getBlockPos(doc, 1)];
      const command = indentBlocks(positions);

      let newState = state;
      const result = command(state, (tr) => {
        newState = state.apply(tr);
      });

      expect(result).toBe(true);
      expect(newState.doc.child(0).attrs.indent).toBe(8); // Unchanged
      expect(newState.doc.child(1).attrs.indent).toBe(1); // Changed
    });
  });

  describe('outdentBlocks', () => {
    it('outdents multiple blocks', () => {
      const doc = createDoc(
        { type: 'bullet', content: 'First', attrs: { indent: 2 } },
        { type: 'bullet', content: 'Second', attrs: { indent: 1 } },
        { type: 'bullet', content: 'Third', attrs: { indent: 2 } }
      );
      const state = EditorState.create({ doc, schema });

      const positions = [getBlockPos(doc, 0), getBlockPos(doc, 2)];
      const command = outdentBlocks(positions);

      let newState = state;
      const result = command(state, (tr) => {
        newState = state.apply(tr);
      });

      expect(result).toBe(true);
      expect(newState.doc.child(0).attrs.indent).toBe(1);
      expect(newState.doc.child(1).attrs.indent).toBe(1); // Unchanged
      expect(newState.doc.child(2).attrs.indent).toBe(1);
    });

    it('returns false for empty positions', () => {
      const doc = createDoc({ type: 'paragraph' });
      const state = EditorState.create({ doc, schema });

      const command = outdentBlocks([]);
      const result = command(state, () => {});
      expect(result).toBe(false);
    });
  });

  describe('toggleTaskBlock', () => {
    it('toggles task completion state', () => {
      const doc = createDoc({
        type: 'task',
        content: 'Do something',
        attrs: { checked: false },
      });
      const state = EditorState.create({ doc, schema });
      const pos = getBlockPos(doc, 0);

      let newState = state;
      let command = toggleTaskBlock(pos);
      let result = command(state, (tr) => {
        newState = state.apply(tr);
      });

      expect(result).toBe(true);
      expect(newState.doc.child(0).attrs.checked).toBe(true);

      // Toggle back
      command = toggleTaskBlock(pos);
      result = command(newState, (tr) => {
        newState = newState.apply(tr);
      });

      expect(result).toBe(true);
      expect(newState.doc.child(0).attrs.checked).toBe(false);
    });

    it('returns false for non-task blocks', () => {
      const doc = createDoc({ type: 'paragraph', content: 'Not a task' });
      const state = EditorState.create({ doc, schema });
      const pos = getBlockPos(doc, 0);

      const command = toggleTaskBlock(pos);
      const result = command(state, () => {});
      expect(result).toBe(false);
    });

    it('returns false for invalid position', () => {
      const doc = createDoc({ type: 'task', content: 'Task' });
      const state = EditorState.create({ doc, schema });

      let result: boolean;
      try {
        const command = toggleTaskBlock(9999);
        result = command(state, () => {});
      } catch {
        result = false;
      }
      expect(result).toBe(false);
    });
  });
});
