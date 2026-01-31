import { Plugin, PluginKey } from 'prosemirror-state';
import { Decoration, DecorationSet } from 'prosemirror-view';
import { getSelectedBlockPositions } from './blockDragPlugin';

/**
 * Plugin to handle visual states for multi-block selections.
 * Adds 'in-selection' class to selected blocks when more than one block is selected.
 */
export const blockSelectionPluginKey = new PluginKey('blockSelection');

export const blockSelectionPlugin = new Plugin({
  key: blockSelectionPluginKey,
  props: {
    decorations(state) {
      const positions = getSelectedBlockPositions(state);

      // We only apply special styling when multiple blocks are selected.
      // Standard ProseMirror selection handles single-block text selection.
      if (positions.length <= 1) {
        return DecorationSet.empty;
      }

      const decorations: Decoration[] = [];
      const { doc } = state;

      for (const pos of positions) {
        const node = doc.nodeAt(pos);
        if (node) {
          // Add 'in-selection' class to the block node
          decorations.push(Decoration.node(pos, pos + node.nodeSize, { class: 'in-selection' }));
        }
      }

      return DecorationSet.create(doc, decorations);
    },
  },
});
