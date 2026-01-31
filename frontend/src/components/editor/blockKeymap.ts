// Keyboard bindings for flat block editor.
// Handles Tab/Shift-Tab for indentation, Enter for block splitting, Backspace for merging.

import { keymap } from 'prosemirror-keymap';
import { TextSelection, type Command, type EditorState } from 'prosemirror-state';
import { schema, type BlockType } from './schema';

/**
 * Check if a block type is list-like (supports indentation).
 */
function isListType(type: BlockType): boolean {
  return ['bullet', 'number', 'task'].includes(type);
}

/**
 * Get the current block node and position from cursor position.
 */
function getCurrentBlock(state: EditorState) {
  const { $from } = state.selection;
  const blockPos = $from.before(1);
  const node = state.doc.nodeAt(blockPos);
  return { node, pos: blockPos, $from };
}

/**
 * Tab: Increase indentation for list-like blocks.
 */
const indentCommand: Command = (state, dispatch) => {
  const { node, pos } = getCurrentBlock(state);
  if (!node) return false;

  const { type, indent } = node.attrs;
  if (!isListType(type as BlockType)) return false;
  if ((indent as number) >= 8) return false;

  if (dispatch) {
    dispatch(
      state.tr.setNodeMarkup(pos, undefined, {
        ...node.attrs,
        indent: (indent as number) + 1,
      })
    );
  }
  return true;
};

/**
 * Shift-Tab: Decrease indentation.
 */
const outdentCommand: Command = (state, dispatch) => {
  const { node, pos } = getCurrentBlock(state);
  if (!node) return false;

  const { indent } = node.attrs;
  if ((indent as number) <= 0) return false;

  if (dispatch) {
    dispatch(
      state.tr.setNodeMarkup(pos, undefined, {
        ...node.attrs,
        indent: (indent as number) - 1,
      })
    );
  }
  return true;
};

/**
 * Enter: Split block or convert empty list item to paragraph.
 */
const enterCommand: Command = (state, dispatch) => {
  const { $from, empty } = state.selection;
  if (!empty) return false;

  const { node, pos } = getCurrentBlock(state);
  if (!node) return false;

  const { type, indent } = node.attrs;

  // Only handle list-type blocks specially
  if (!isListType(type as BlockType)) return false;

  // If block is empty, convert to paragraph or decrease indent
  if (node.content.size === 0) {
    if (dispatch) {
      const newAttrs =
        (indent as number) > 0
          ? { type, indent: (indent as number) - 1, checked: type === 'task' ? false : null }
          : { type: 'paragraph', indent: 0, checked: null };
      dispatch(state.tr.setNodeMarkup(pos, undefined, newAttrs));
    }
    return true;
  }

  // Split at cursor position
  if (dispatch) {
    const tr = state.tr;
    const splitOffset = $from.parentOffset;
    const blockEndPos = pos + node.nodeSize;

    // Get content after cursor
    const afterSlice = node.content.cut(splitOffset);

    // Create new block with same type and indent
    const blockType = schema.nodes['block'];
    if (!blockType) return false;

    const newBlock = blockType.create(
      {
        type,
        indent,
        checked: type === 'task' ? false : null,
      },
      afterSlice
    );

    // Delete content after cursor in current block, then insert new block
    tr.delete($from.pos, blockEndPos - 1); // -1 to not delete closing tag
    tr.insert(pos + 1 + splitOffset, newBlock);

    // Move cursor to start of new block
    const newPos = pos + 2 + splitOffset;
    tr.setSelection(TextSelection.create(tr.doc, newPos));

    dispatch(tr);
  }
  return true;
};

/**
 * Backspace at start of block: Merge with previous or convert to paragraph.
 */
const backspaceCommand: Command = (state, dispatch) => {
  const { $from, empty } = state.selection;
  if (!empty || $from.parentOffset !== 0) return false;

  const { node, pos } = getCurrentBlock(state);
  if (!node) return false;

  const { type, indent } = node.attrs;

  // If indented, outdent first
  if ((indent as number) > 0) {
    if (dispatch) {
      dispatch(
        state.tr.setNodeMarkup(pos, undefined, {
          ...node.attrs,
          indent: (indent as number) - 1,
        })
      );
    }
    return true;
  }

  // If list/quote type, convert to paragraph
  if (['bullet', 'number', 'task', 'quote'].includes(type as string)) {
    if (dispatch) {
      dispatch(
        state.tr.setNodeMarkup(pos, undefined, {
          type: 'paragraph',
          indent: 0,
          checked: null,
          level: null,
          language: null,
        })
      );
    }
    return true;
  }

  // Let default backspace handle merging paragraphs
  return false;
};

/**
 * Build keymap for flat block editor.
 * Should be added to plugins list.
 */
export function buildBlockKeymap() {
  return keymap({
    Tab: indentCommand,
    'Shift-Tab': outdentCommand,
    Enter: enterCommand,
    Backspace: backspaceCommand,
  });
}

/**
 * Export individual commands for testing and programmatic use.
 */
export const blockKeymapCommands = {
  indent: indentCommand,
  outdent: outdentCommand,
  enter: enterCommand,
  backspace: backspaceCommand,
};
