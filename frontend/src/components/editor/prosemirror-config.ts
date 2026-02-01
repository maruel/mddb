// ProseMirror configuration for the flat block editor.
// Integrates schema, plugins, and keymaps for the new block-based architecture.

import { EditorState, type Plugin } from 'prosemirror-state';
import { history } from 'prosemirror-history';
import { keymap } from 'prosemirror-keymap';
import { baseKeymap } from 'prosemirror-commands';
import { dropCursor } from 'prosemirror-dropcursor';
import { gapCursor } from 'prosemirror-gapcursor';
import type { Node as ProseMirrorNode } from 'prosemirror-model';

// Import flat block schema
import { schema } from './schema';

// Import new block-specific plugins and rules
import { buildBlockInputRules } from './blockInputRules';
import { buildBlockKeymap } from './blockKeymap';
import { blockDragPlugin } from './blockDragPlugin';
import { blockSelectionPlugin } from './blockSelectionPlugin';
import { numberCounterPlugin } from './numberCounterPlugin';

// Re-export schema components for use in other files
export { schema };
export { nodes, marks } from './schema';

/**
 * Create a new EditorState with all necessary plugins for the block editor.
 */
export function createEditorState(doc: ProseMirrorNode, extraPlugins?: Plugin[]) {
  return EditorState.create({
    doc,
    schema,
    plugins: [
      // Input rules for converting markdown syntax to blocks (e.g. "- " -> bullet block)
      buildBlockInputRules(),

      // Keymap for block operations (Enter, Backspace, Tab, etc.)
      buildBlockKeymap(),

      // Standard ProseMirror keymap (Ctrl+Z, basic editing)
      keymap(baseKeymap),

      // Undo/Redo history
      history(),

      // Drag and drop system
      blockDragPlugin,
      blockSelectionPlugin,
      numberCounterPlugin,

      // UI polish
      dropCursor({ color: 'var(--c-primary)', width: 2 }),
      gapCursor(),

      // Extra plugins passed from Editor (e.g. slash menu, upload)
      ...(extraPlugins || []),
    ],
  });
}
