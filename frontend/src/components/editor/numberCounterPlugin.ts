// ProseMirror plugin that auto-numbers numbered list blocks based on indentation level.
import { Plugin } from "prosemirror-state";

/**
 * Plugin to automatically manage numbering for numbered lists.
 * It computes the sequential number for each 'number' block based on its indentation level
 * and updates the 'number' attribute via a transaction.
 */
export const numberCounterPlugin = new Plugin({
  appendTransaction(transactions, _oldState, newState) {
    // Only recompute if document changed
    if (!transactions.some((transaction) => transaction.docChanged)) return null;

    const tr = newState.tr;
    let needsUpdate = false;
    const counters = new Map<number, number>(); // indent -> current count

    newState.doc.forEach((node, pos) => {
      // In flat architecture, blocks are always direct children of doc
      if (node.type.name !== "block") return;

      const indent = (node.attrs.indent as number | undefined) || 0;

      if (node.attrs.type === "number") {
        const currentCount = (counters.get(indent) || 0) + 1;
        counters.set(indent, currentCount);

        // Reset deeper counters
        for (const [level] of counters) {
          if (level > indent) counters.delete(level);
        }

        const oldNumber: unknown = node.attrs.number;
        if (oldNumber !== currentCount) {
          tr.setNodeMarkup(pos, undefined, { ...node.attrs, number: currentCount });
          needsUpdate = true;
        }
      } else {
        // Non-numbered block.
        // Resets counters at and below this indent
        // This logic ensures that a paragraph at the same level breaks the list
        for (const [level] of counters) {
          if (indent <= level) {
            counters.delete(level);
          }
        }
      }
    });

    // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition -- A synchronous traversal callback updates this local variable.
    return needsUpdate ? tr : null;
  },
});
