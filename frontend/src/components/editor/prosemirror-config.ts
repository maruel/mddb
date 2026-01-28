// ProseMirror configuration: input rules, keymap, and markdown serializer.

import { EditorState, type Transaction } from 'prosemirror-state';
import { schema, defaultMarkdownSerializer, MarkdownSerializer } from 'prosemirror-markdown';
import { history, undo, redo } from 'prosemirror-history';
import { keymap } from 'prosemirror-keymap';
import { baseKeymap, toggleMark } from 'prosemirror-commands';
import { dropCursor } from 'prosemirror-dropcursor';
import { gapCursor } from 'prosemirror-gapcursor';
import { splitListItem, liftListItem, sinkListItem } from 'prosemirror-schema-list';
import {
  inputRules,
  wrappingInputRule,
  textblockTypeInputRule,
  smartQuotes,
  emDash,
  ellipsis,
} from 'prosemirror-inputrules';
import type { Node as ProseMirrorNode } from 'prosemirror-model';

// Custom markdown serializer that uses "-" for bullet lists instead of "*"
export const markdownSerializer = new MarkdownSerializer(
  {
    ...defaultMarkdownSerializer.nodes,
    bullet_list(state, node) {
      state.renderList(node, '  ', () => '- ');
    },
  },
  defaultMarkdownSerializer.marks
);

// Build input rules for markdown-like shortcuts
function buildInputRules() {
  const rules = smartQuotes.concat(ellipsis, emDash);

  // Blockquote: > at start of line
  rules.push(wrappingInputRule(/^\s*>\s$/, schema.nodes.blockquote));

  // Bullet list: - or * at start of line
  rules.push(wrappingInputRule(/^\s*([-*])\s$/, schema.nodes.bullet_list));

  // Ordered list: 1. at start of line
  rules.push(
    wrappingInputRule(
      /^(\d+)\.\s$/,
      schema.nodes.ordered_list,
      (match) => ({ order: +(match[1] ?? 1) }),
      (match, node) => node.childCount + (node.attrs.order as number) === +(match[1] ?? 1)
    )
  );

  // Code block: ``` at start of line
  rules.push(textblockTypeInputRule(/^```$/, schema.nodes.code_block));

  // Headings: # ## ### etc at start of line
  for (let i = 1; i <= 6; i++) {
    const pattern = new RegExp(`^(#{${i}})\\s$`);
    rules.push(textblockTypeInputRule(pattern, schema.nodes.heading, { level: i }));
  }

  return inputRules({ rules });
}

// Build keymap for list operations and formatting
function buildKeymap() {
  const keys: { [key: string]: (state: EditorState, dispatch?: (tr: Transaction) => void) => boolean } = {};

  // History
  keys['Mod-z'] = undo;
  keys['Mod-y'] = redo;
  keys['Mod-Shift-z'] = redo;

  // Formatting marks
  keys['Mod-b'] = toggleMark(schema.marks.strong);
  keys['Mod-i'] = toggleMark(schema.marks.em);
  keys['Mod-`'] = toggleMark(schema.marks.code);

  // List operations
  keys['Enter'] = splitListItem(schema.nodes.list_item);
  keys['Tab'] = sinkListItem(schema.nodes.list_item);
  keys['Shift-Tab'] = liftListItem(schema.nodes.list_item);

  return keymap(keys);
}

// Create editor state with all plugins
export function createEditorState(doc: ProseMirrorNode) {
  return EditorState.create({
    doc,
    plugins: [
      buildInputRules(),
      buildKeymap(),
      keymap(baseKeymap),
      history(),
      dropCursor({ color: 'var(--c-primary)', width: 2 }),
      gapCursor(),
    ],
  });
}
