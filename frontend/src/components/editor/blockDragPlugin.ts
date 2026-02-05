// ProseMirror plugin for block-level drag-and-drop functionality.
// Manages drag state and provides drop indicator decorations.

import { Plugin, PluginKey, type EditorState, type Transaction } from 'prosemirror-state';
import { DecorationSet, type EditorView } from 'prosemirror-view';

// Drag state interface (only tracks drop target for visual indicator)
export interface DragState {
  /** Position to drop (between blocks) */
  dropTarget: number | null;
  /** Y coordinate for visual indicator */
  dropIndicatorY: number | null;
}

// Plugin key for accessing drag state
export const blockDragPluginKey = new PluginKey<DragState>('blockDrag');

// Initial state
const initialState: DragState = {
  dropTarget: null,
  dropIndicatorY: null,
};

/**
 * Get positions of all blocks in the current selection.
 * Used for multi-block drag operations.
 */
export function getSelectedBlockPositions(state: EditorState): number[] {
  const positions: number[] = [];
  // Use blockRange to identify the range of blocks
  const $from = state.doc.resolve(state.selection.from);
  const $to = state.doc.resolve(state.selection.to);
  const range = $from.blockRange($to);

  if (range && range.parent.type.name === 'doc') {
    // Iterate manually through the range indices to ensure we capture all top-level blocks
    // `range.start` is the position of the start of the range (before first block)
    let currentPos = range.start;
    for (let i = range.startIndex; i < range.endIndex; i++) {
      const child = range.parent.child(i);
      if (child.type.name === 'block') {
        positions.push(currentPos);
      }
      currentPos += child.nodeSize;
    }
    return positions;
  }

  // Handle single block selection specifically
  // If the selection is inside a block, blockRange might return null (inline content range)
  // or the parent might be the block itself (not doc)
  // We want to return the position of the block itself.
  if ($from.sameParent($to) && $from.parent.type.name === 'block') {
    // The position of the block is before its content starts?
    // nodeAt(pos) expects the pos BEFORE the node.
    // $from.before(depth) returns the pos before the start of the node at depth.
    // Since blocks are top-level in flat schema, depth is likely 1.
    // But we use $from.depth to be safe if checking sameParent.
    positions.push($from.before($from.depth));
    return positions;
  }

  // Fallback to nodesBetween if blockRange doesn't give us doc-level blocks
  state.doc.nodesBetween(state.selection.from, state.selection.to, (node, pos) => {
    // Only include flat blocks, not the doc or inline content
    if (node.type.name === 'block' && pos >= 0) {
      positions.push(pos);
    }
    return true; // Continue traversal
  });

  return positions;
}

/**
 * Create a transaction to update drag state.
 */
export function setDragState(tr: Transaction, updates: Partial<DragState>): Transaction {
  return tr.setMeta(blockDragPluginKey, updates);
}

/**
 * Get current drag state from editor state.
 */
export function getDragState(state: EditorState): DragState {
  return blockDragPluginKey.getState(state) ?? initialState;
}

/**
 * Clear all drag state (call on drop or dragend).
 */
export function clearDragState(tr: Transaction): Transaction {
  return tr.setMeta(blockDragPluginKey, initialState);
}

/**
 * Find the nearest block boundary position from coordinates.
 * Returns the position and whether to drop above or below.
 */
export function findDropTarget(
  view: EditorView,
  clientX: number,
  clientY: number
): { pos: number; y: number; above: boolean } | null {
  const coords = { left: clientX, top: clientY };
  const posInfo = view.posAtCoords(coords);
  if (!posInfo) return null;

  const $pos = view.state.doc.resolve(posInfo.pos);

  // Find the block-level parent
  let depth = $pos.depth;
  while (depth > 0 && !$pos.node(depth).isBlock) {
    depth--;
  }

  // If depth is 0, the coordinates are outside block content.
  // This can happen when dropping at the very edge/bottom of the editor.
  // In this case, find the nearest block by iterating through all blocks.
  if (depth === 0) {
    const doc = view.state.doc;
    let nearestBlock: { pos: number; top: number; bottom: number } | null = null;
    let smallestDistance = Infinity;

    doc.forEach((node, pos) => {
      if (node.type.name === 'block') {
        const nodeStartCoords = view.coordsAtPos(pos);
        const nodeEndCoords = view.coordsAtPos(pos + node.nodeSize);
        const nodeMidY = (nodeStartCoords.top + nodeEndCoords.bottom) / 2;
        const distance = Math.abs(clientY - nodeMidY);

        if (distance < smallestDistance) {
          smallestDistance = distance;
          nearestBlock = { pos, top: nodeStartCoords.top, bottom: nodeEndCoords.bottom };
        }
      }
    });

    if (!nearestBlock) return null;

    // Type assertion needed: TS doesn't track assignments inside callbacks
    const foundBlock = nearestBlock as { pos: number; top: number; bottom: number };

    // Determine if we should drop above or below this block
    const nearestMidY = (foundBlock.top + foundBlock.bottom) / 2;
    const dropAboveNearest = clientY < nearestMidY;

    // For the nearest block, calculate start and end positions
    const blockNode = doc.nodeAt(foundBlock.pos);
    if (!blockNode) return null;

    return {
      pos: dropAboveNearest ? foundBlock.pos : foundBlock.pos + blockNode.nodeSize,
      y: dropAboveNearest ? foundBlock.top : foundBlock.bottom,
      above: dropAboveNearest,
    };
  }

  const blockStart = $pos.before(depth);
  const blockEnd = $pos.after(depth);
  const startCoords = view.coordsAtPos(blockStart);
  const endCoords = view.coordsAtPos(blockEnd);

  // Determine if we should drop above or below this block
  const blockMidY = (startCoords.top + endCoords.bottom) / 2;
  const dropAbove = clientY < blockMidY;

  return {
    pos: dropAbove ? blockStart : blockEnd,
    y: dropAbove ? startCoords.top : endCoords.bottom,
    above: dropAbove,
  };
}

/**
 * Execute a block move transaction.
 * Handles moving single or multiple blocks.
 */
export function moveBlocks(view: EditorView, sourcePositions: number[], targetPos: number): boolean {
  if (sourcePositions.length === 0) return false;

  const { state } = view;
  let tr = state.tr;

  // Sort positions in descending order to handle deletions correctly
  const sortedPositions = [...sourcePositions].sort((a, b) => b - a);

  // Collect nodes to move
  const nodesToMove: { node: typeof state.doc.nodeAt extends (p: number) => infer R ? R : never; pos: number }[] = [];
  for (const pos of sortedPositions) {
    const node = state.doc.nodeAt(pos);
    if (node) {
      nodesToMove.push({ node, pos });
    }
  }

  if (nodesToMove.length === 0) return false;

  // Calculate adjusted target position after deletions
  let adjustedTarget = targetPos;
  for (const { node, pos } of nodesToMove) {
    if (node && pos < targetPos) {
      adjustedTarget -= node.nodeSize;
    }
  }

  // Delete nodes from highest position first (so positions remain valid)
  for (const { node, pos } of nodesToMove) {
    if (node) {
      tr = tr.delete(pos, pos + node.nodeSize);
    }
  }

  // Insert nodes at target (in original order)
  const reversedNodes = [...nodesToMove].reverse();
  let insertPos = adjustedTarget;
  for (const { node } of reversedNodes) {
    if (node) {
      tr = tr.insert(insertPos, node);
      insertPos += node.nodeSize;
    }
  }

  // Clear drag state
  tr = clearDragState(tr);

  view.dispatch(tr);
  return true;
}

// Note: Drop indicator is rendered externally based on dropIndicatorY from state

/**
 * Block drag-and-drop plugin.
 * Tracks drag state and provides decorations for drop indicator.
 */
export const blockDragPlugin = new Plugin<DragState>({
  key: blockDragPluginKey,

  state: {
    init: () => initialState,
    apply(tr, state) {
      const meta = tr.getMeta(blockDragPluginKey);
      if (meta) {
        return { ...state, ...meta };
      }
      return state;
    },
  },

  props: {
    // Handle drag events at the view level
    handleDOMEvents: {
      dragstart(_view, event) {
        // Check if this drag started from a block handle (has our MIME type)
        const hasBlockData =
          event.dataTransfer?.types.includes(BLOCK_DRAG_MIME) || event.dataTransfer?.types.includes(BLOCKS_DRAG_MIME);

        // Return true to prevent ProseMirror's built-in dragstart handler
        // which calls clearData() and would erase our MIME types
        return hasBlockData;
      },

      dragover(view, event) {
        // Check if we're dragging a block (via dataTransfer MIME types)
        const hasBlockData =
          event.dataTransfer?.types.includes(BLOCK_DRAG_MIME) || event.dataTransfer?.types.includes(BLOCKS_DRAG_MIME);

        if (!hasBlockData) {
          return false;
        }

        event.preventDefault();
        if (event.dataTransfer) {
          event.dataTransfer.dropEffect = 'move';
        }

        const target = findDropTarget(view, event.clientX, event.clientY);
        if (target) {
          view.dispatch(
            setDragState(view.state.tr, {
              dropTarget: target.pos,
              dropIndicatorY: target.y,
            })
          );
        }

        return true;
      },

      drop(view, event) {
        const state = getDragState(view.state);
        const { dropTarget } = state;

        // Clean up dragging class from all blocks
        view.dom.querySelectorAll('.block-row.dragging').forEach((el) => {
          el.classList.remove('dragging');
        });

        // Get source positions from dataTransfer (not plugin state)
        const multiData = event.dataTransfer?.getData(BLOCKS_DRAG_MIME);
        const singleData = event.dataTransfer?.getData(BLOCK_DRAG_MIME);

        if (dropTarget === null) {
          view.dispatch(clearDragState(view.state.tr));
          return false;
        }

        // Handle multi-block drop
        if (multiData) {
          const selectedPositions = JSON.parse(multiData) as number[];
          if (selectedPositions.length > 0) {
            event.preventDefault();
            return moveBlocks(view, selectedPositions, dropTarget);
          }
        }

        // Handle single block drop
        if (singleData) {
          const sourcePos = parseInt(singleData, 10);
          if (!isNaN(sourcePos)) {
            event.preventDefault();
            return moveBlocks(view, [sourcePos], dropTarget);
          }
        }

        // Clear state if we didn't handle the drop
        view.dispatch(clearDragState(view.state.tr));
        return false;
      },

      dragend(view) {
        // Clean up dragging class from all blocks
        view.dom.querySelectorAll('.block-row.dragging').forEach((el) => {
          el.classList.remove('dragging');
        });
        view.dispatch(clearDragState(view.state.tr));
        return false;
      },

      dragleave(view, event) {
        // Only clear if leaving the editor entirely
        const relatedTarget = event.relatedTarget as Node | null;
        if (!view.dom.contains(relatedTarget)) {
          view.dispatch(
            setDragState(view.state.tr, {
              dropTarget: null,
              dropIndicatorY: null,
            })
          );
        }
        return false;
      },
    },

    // Provide decorations for drop indicator
    decorations(state) {
      const dragState = this.getState(state);
      if (!dragState || dragState.dropIndicatorY === null) {
        return DecorationSet.empty;
      }

      // Return empty for now - actual indicator rendered via CSS/view
      // The indicator Y position is available for external rendering
      return DecorationSet.empty;
    },
  },
});

/**
 * MIME type for block drag data.
 */
export const BLOCK_DRAG_MIME = 'application/x-prosemirror-block';
export const BLOCKS_DRAG_MIME = 'application/x-prosemirror-blocks';
