// Slash command registry defining available block types for the editor menu.

import type { EditorView } from 'prosemirror-view';
import { setBlockType } from 'prosemirror-commands';
import { wrapIn, lift } from 'prosemirror-commands';
import { Selection } from 'prosemirror-state';
import { schema, nodes } from './prosemirror-config';

export interface SlashCommand {
  id: string;
  labelKey: string; // i18n key under slashMenu.*
  keywords: string[];
  icon: string;
  execute: (view: EditorView, from: number, to: number) => void;
}

// Helper to delete the "/" trigger text and execute a command
function deleteSlashAndExecute(view: EditorView, from: number, to: number, action: (view: EditorView) => void) {
  const tr = view.state.tr.delete(from, to);
  view.dispatch(tr);
  action(view);
}

// Helper to wrap current block in a list
function wrapInList(
  view: EditorView,
  listType: typeof nodes.bullet_list | typeof nodes.ordered_list,
  listItemAttrs?: { checked: boolean | null }
) {
  const { state } = view;
  const { $from } = state.selection;

  // Check if already in a list
  for (let d = $from.depth; d > 0; d--) {
    const node = $from.node(d);
    if (node.type === nodes.bullet_list || node.type === nodes.ordered_list) {
      // Already in a list, lift out first
      lift(state, view.dispatch);
      return;
    }
  }

  // Get the position before replacement
  const replaceFrom = $from.before($from.depth);

  // Create list item with empty paragraph (since "/" was already deleted)
  const newParagraph = nodes.paragraph.create();
  const newListItem = nodes.list_item.create(listItemAttrs || null, newParagraph);
  const newList = listType.create(null, newListItem);

  // Replace current block with list and set cursor inside the list item
  const tr = view.state.tr.replaceWith(replaceFrom, $from.after($from.depth), newList);

  // Position cursor inside the paragraph within the list item
  // Structure is: list > list_item > paragraph, so we need to go 3 levels deep
  const cursorPos = replaceFrom + 3; // list(+1) + list_item(+1) + paragraph(+1)
  tr.setSelection(Selection.near(tr.doc.resolve(cursorPos)));

  view.dispatch(tr);
}

export const slashCommands: SlashCommand[] = [
  {
    id: 'paragraph',
    labelKey: 'paragraph',
    keywords: ['paragraph', 'text', 'plain'],
    icon: 'T',
    execute: (view, from, to) => {
      deleteSlashAndExecute(view, from, to, (v) => {
        setBlockType(nodes.paragraph)(v.state, v.dispatch);
      });
    },
  },
  {
    id: 'heading1',
    labelKey: 'heading1',
    keywords: ['heading', 'h1', 'title', 'header'],
    icon: 'H1',
    execute: (view, from, to) => {
      deleteSlashAndExecute(view, from, to, (v) => {
        setBlockType(nodes.heading, { level: 1 })(v.state, v.dispatch);
      });
    },
  },
  {
    id: 'heading2',
    labelKey: 'heading2',
    keywords: ['heading', 'h2', 'subtitle', 'header'],
    icon: 'H2',
    execute: (view, from, to) => {
      deleteSlashAndExecute(view, from, to, (v) => {
        setBlockType(nodes.heading, { level: 2 })(v.state, v.dispatch);
      });
    },
  },
  {
    id: 'heading3',
    labelKey: 'heading3',
    keywords: ['heading', 'h3', 'header'],
    icon: 'H3',
    execute: (view, from, to) => {
      deleteSlashAndExecute(view, from, to, (v) => {
        setBlockType(nodes.heading, { level: 3 })(v.state, v.dispatch);
      });
    },
  },
  {
    id: 'bulletList',
    labelKey: 'bulletList',
    keywords: ['bullet', 'list', 'unordered', 'ul'],
    icon: '•',
    execute: (view, from, to) => {
      deleteSlashAndExecute(view, from, to, (v) => {
        wrapInList(v, nodes.bullet_list);
      });
    },
  },
  {
    id: 'orderedList',
    labelKey: 'orderedList',
    keywords: ['ordered', 'list', 'numbered', 'ol'],
    icon: '1.',
    execute: (view, from, to) => {
      deleteSlashAndExecute(view, from, to, (v) => {
        wrapInList(v, nodes.ordered_list);
      });
    },
  },
  {
    id: 'taskList',
    labelKey: 'taskList',
    keywords: ['task', 'checkbox', 'todo', 'checklist', 'check'],
    icon: '☐',
    execute: (view, from, to) => {
      deleteSlashAndExecute(view, from, to, (v) => {
        wrapInList(v, nodes.bullet_list, { checked: false });
      });
    },
  },
  {
    id: 'blockquote',
    labelKey: 'blockquote',
    keywords: ['quote', 'blockquote', 'citation'],
    icon: '"',
    execute: (view, from, to) => {
      deleteSlashAndExecute(view, from, to, (v) => {
        wrapIn(nodes.blockquote)(v.state, v.dispatch);
      });
    },
  },
  {
    id: 'codeBlock',
    labelKey: 'codeBlock',
    keywords: ['code', 'codeblock', 'pre', 'programming'],
    icon: '</>',
    execute: (view, from, to) => {
      deleteSlashAndExecute(view, from, to, (v) => {
        setBlockType(nodes.code_block)(v.state, v.dispatch);
      });
    },
  },
  {
    id: 'divider',
    labelKey: 'divider',
    keywords: ['divider', 'hr', 'horizontal', 'rule', 'line'],
    icon: '—',
    execute: (view, from, to) => {
      const hrType = schema.nodes.horizontal_rule;
      if (!hrType) return;

      // Delete the slash text
      let tr = view.state.tr.delete(from, to);
      view.dispatch(tr);

      // Insert horizontal rule after current block
      const insertPos = view.state.selection.$from.after(view.state.selection.$from.depth);
      const hr = hrType.create();
      const paragraph = nodes.paragraph.create();

      tr = view.state.tr.insert(insertPos, [hr, paragraph]);
      tr.setSelection(Selection.near(tr.doc.resolve(insertPos + 2)));
      view.dispatch(tr);
    },
  },
];

/**
 * Filter commands by query string (prefix match on keywords and label key).
 */
export function filterCommands(query: string): SlashCommand[] {
  if (!query) return slashCommands;

  const lowerQuery = query.toLowerCase();
  return slashCommands.filter((cmd) => {
    // Match on label key
    if (cmd.labelKey.toLowerCase().startsWith(lowerQuery)) return true;
    // Match on any keyword
    return cmd.keywords.some((kw) => kw.toLowerCase().startsWith(lowerQuery));
  });
}
