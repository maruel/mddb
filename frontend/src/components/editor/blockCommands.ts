// ProseMirror commands for block operations.
// Used by context menu and keyboard shortcuts.

import { type Command } from 'prosemirror-state';
import { type BlockType } from './schema';

/**
 * Delete a block at the given position.
 */
export function deleteBlock(pos: number): Command {
  return (state, dispatch) => {
    const node = state.doc.nodeAt(pos);
    if (!node) return false;

    if (dispatch) {
      dispatch(state.tr.delete(pos, pos + node.nodeSize));
    }
    return true;
  };
}

/**
 * Delete multiple blocks at the given positions.
 * Positions should be in document order.
 */
export function deleteBlocks(positions: number[]): Command {
  return (state, dispatch) => {
    if (positions.length === 0) return false;

    // Sort in descending order to delete from end first
    const sortedPositions = [...positions].sort((a, b) => b - a);

    let tr = state.tr;
    for (const pos of sortedPositions) {
      const node = tr.doc.nodeAt(pos);
      if (node) {
        // Remap position through previous deletions
        const mappedPos = tr.mapping.map(pos);
        tr = tr.delete(mappedPos, mappedPos + node.nodeSize);
      }
    }

    if (dispatch) {
      dispatch(tr);
    }
    return true;
  };
}

/**
 * Duplicate a block at the given position.
 * New block is inserted immediately after.
 */
export function duplicateBlock(pos: number): Command {
  return (state, dispatch) => {
    const node = state.doc.nodeAt(pos);
    if (!node) return false;

    if (dispatch) {
      const insertPos = pos + node.nodeSize;
      dispatch(state.tr.insert(insertPos, node.copy(node.content)));
    }
    return true;
  };
}

/**
 * Duplicate multiple blocks at the given positions.
 * New blocks are inserted after the last selected block.
 */
export function duplicateBlocks(positions: number[]): Command {
  return (state, dispatch) => {
    if (positions.length === 0) return false;

    // Sort positions in ascending order
    const sortedPositions = [...positions].sort((a, b) => a - b);

    // Find last position and collect nodes
    const nodes: (typeof state.doc.nodeAt extends (p: number) => infer R ? Exclude<R, null> : never)[] = [];
    let lastPos = 0;
    let lastNodeSize = 0;

    for (const pos of sortedPositions) {
      const node = state.doc.nodeAt(pos);
      if (node) {
        nodes.push(node);
        if (pos > lastPos) {
          lastPos = pos;
          lastNodeSize = node.nodeSize;
        }
      }
    }

    if (nodes.length === 0) return false;

    if (dispatch) {
      let tr = state.tr;
      let insertPos = lastPos + lastNodeSize;

      for (const node of nodes) {
        tr = tr.insert(insertPos, node.copy(node.content));
        insertPos += node.nodeSize;
      }

      dispatch(tr);
    }
    return true;
  };
}

/**
 * Convert a block to a different type.
 */
export function convertBlock(
  pos: number,
  toType: BlockType,
  attrs?: { level?: number; checked?: boolean; language?: string }
): Command {
  return (state, dispatch) => {
    const node = state.doc.nodeAt(pos);
    if (!node) return false;

    if (dispatch) {
      const newAttrs = {
        ...node.attrs,
        type: toType,
        ...attrs,
      };

      // Clear type-specific attrs when converting
      if (toType !== 'heading') {
        newAttrs.level = undefined;
      }
      if (toType !== 'task') {
        newAttrs.checked = undefined;
      }
      if (toType !== 'code') {
        newAttrs.language = undefined;
      }

      dispatch(state.tr.setNodeMarkup(pos, undefined, newAttrs));
    }
    return true;
  };
}

/**
 * Increase indentation of a block.
 * Maximum indent level is 8.
 */
export function indentBlock(pos: number): Command {
  return (state, dispatch) => {
    const node = state.doc.nodeAt(pos);
    if (!node) return false;

    const currentIndent = (node.attrs.indent as number) || 0;
    if (currentIndent >= 8) return false; // Max indent

    if (dispatch) {
      dispatch(
        state.tr.setNodeMarkup(pos, undefined, {
          ...node.attrs,
          indent: currentIndent + 1,
        })
      );
    }
    return true;
  };
}

/**
 * Decrease indentation of a block.
 * Minimum indent level is 0.
 */
export function outdentBlock(pos: number): Command {
  return (state, dispatch) => {
    const node = state.doc.nodeAt(pos);
    if (!node) return false;

    const currentIndent = (node.attrs.indent as number) || 0;
    if (currentIndent <= 0) return false;

    if (dispatch) {
      dispatch(
        state.tr.setNodeMarkup(pos, undefined, {
          ...node.attrs,
          indent: currentIndent - 1,
        })
      );
    }
    return true;
  };
}

/**
 * Indent multiple blocks.
 */
export function indentBlocks(positions: number[]): Command {
  return (state, dispatch) => {
    if (positions.length === 0) return false;

    let tr = state.tr;
    let modified = false;

    for (const pos of positions) {
      const node = tr.doc.nodeAt(pos);
      if (!node) continue;

      const currentIndent = (node.attrs.indent as number) || 0;
      if (currentIndent >= 8) continue;

      tr = tr.setNodeMarkup(pos, undefined, {
        ...node.attrs,
        indent: currentIndent + 1,
      });
      modified = true;
    }

    if (!modified) return false;

    if (dispatch) {
      dispatch(tr);
    }
    return true;
  };
}

/**
 * Outdent multiple blocks.
 */
export function outdentBlocks(positions: number[]): Command {
  return (state, dispatch) => {
    if (positions.length === 0) return false;

    let tr = state.tr;
    let modified = false;

    for (const pos of positions) {
      const node = tr.doc.nodeAt(pos);
      if (!node) continue;

      const currentIndent = (node.attrs.indent as number) || 0;
      if (currentIndent <= 0) continue;

      tr = tr.setNodeMarkup(pos, undefined, {
        ...node.attrs,
        indent: currentIndent - 1,
      });
      modified = true;
    }

    if (!modified) return false;

    if (dispatch) {
      dispatch(tr);
    }
    return true;
  };
}

/**
 * Toggle task completion state.
 */
export function toggleTaskBlock(pos: number): Command {
  return (state, dispatch) => {
    const node = state.doc.nodeAt(pos);
    if (!node || node.attrs.type !== 'task') return false;

    if (dispatch) {
      dispatch(
        state.tr.setNodeMarkup(pos, undefined, {
          ...node.attrs,
          checked: !node.attrs.checked,
        })
      );
    }
    return true;
  };
}
