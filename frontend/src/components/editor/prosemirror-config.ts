// ProseMirror configuration: input rules, keymap, markdown parser/serializer, and task list support.

import { EditorState, type Transaction, Plugin } from 'prosemirror-state';
import {
  schema as baseSchema,
  defaultMarkdownSerializer,
  MarkdownSerializer,
  MarkdownParser,
} from 'prosemirror-markdown';
import { Schema, type DOMOutputSpec } from 'prosemirror-model';
import { history, undo, redo } from 'prosemirror-history';
import { keymap } from 'prosemirror-keymap';
import { baseKeymap, toggleMark } from 'prosemirror-commands';
import { dropCursor } from 'prosemirror-dropcursor';
import { gapCursor } from 'prosemirror-gapcursor';
import { Decoration, DecorationSet } from 'prosemirror-view';
import { splitListItem, liftListItem, sinkListItem } from 'prosemirror-schema-list';
import {
  inputRules,
  wrappingInputRule,
  textblockTypeInputRule,
  smartQuotes,
  emDash,
  ellipsis,
  InputRule,
} from 'prosemirror-inputrules';
import type { Node as ProseMirrorNode } from 'prosemirror-model';
import MarkdownIt from 'markdown-it';
import type Token from 'markdown-it/lib/token.mjs';

// Custom schema extending list_item with checked attribute for task lists
const listItemSpec = baseSchema.spec.nodes.get('list_item');
if (!listItemSpec) throw new Error('list_item node not found in schema');

export const schema = new Schema({
  nodes: baseSchema.spec.nodes.update('list_item', {
    ...listItemSpec,
    attrs: { checked: { default: null } },
    toDOM(node): DOMOutputSpec {
      if (node.attrs.checked !== null) {
        return ['li', { class: 'task-list-item', 'data-checked': String(node.attrs.checked) }, 0];
      }
      return ['li', 0];
    },
    parseDOM: [
      {
        tag: 'li',
        getAttrs(dom: HTMLElement) {
          if (dom.classList.contains('task-list-item')) {
            return { checked: dom.dataset.checked === 'true' };
          }
          return { checked: null };
        },
      },
    ],
  }),
  marks: baseSchema.spec.marks,
});

// Helper to get node type with runtime check
function getNodeType(name: string) {
  const nodeType = schema.nodes[name];
  if (!nodeType) throw new Error(`Node type '${name}' not found in schema`);
  return nodeType;
}

// Helper to get mark type with runtime check
function getMarkType(name: string) {
  const markType = schema.marks[name];
  if (!markType) throw new Error(`Mark type '${name}' not found in schema`);
  return markType;
}

// Extract node and mark types with runtime checks for type safety
export const nodes = {
  doc: getNodeType('doc'),
  list_item: getNodeType('list_item'),
  bullet_list: getNodeType('bullet_list'),
  ordered_list: getNodeType('ordered_list'),
  paragraph: getNodeType('paragraph'),
  blockquote: getNodeType('blockquote'),
  code_block: getNodeType('code_block'),
  heading: getNodeType('heading'),
};

export const marks = {
  strong: getMarkType('strong'),
  em: getMarkType('em'),
  code: getMarkType('code'),
  link: getMarkType('link'),
};

// Create markdown-it instance with task list detection
const md = new MarkdownIt();

// Add core rule to detect checkbox syntax in list items
md.core.ruler.after('inline', 'task_list', (state) => {
  const tokens = state.tokens;
  for (let i = 0; i < tokens.length; i++) {
    const token = tokens[i];
    if (!token) continue;
    if (token.type === 'list_item_open') {
      // Look for inline content in this list item
      const inlineToken = tokens[i + 2];
      if (inlineToken && inlineToken.type === 'inline' && inlineToken.children) {
        const firstChild = inlineToken.children[0];
        if (firstChild && firstChild.type === 'text' && firstChild.content) {
          const match = firstChild.content.match(/^\[([ xX])\]\s*/);
          if (match && match[1]) {
            // Set checked attribute on list_item_open token
            token.attrSet('checked', match[1].toLowerCase() === 'x' ? 'true' : 'false');
            // Remove checkbox syntax from text
            firstChild.content = firstChild.content.slice(match[0].length);
          }
        }
      }
    }
  }
  return true;
});

// Custom markdown parser using our schema
// Note: Table tokens are mapped to paragraph/ignore since ProseMirror schema doesn't support tables
export const markdownParser = new MarkdownParser(schema, md, {
  blockquote: { block: 'blockquote' },
  paragraph: { block: 'paragraph' },
  list_item: {
    block: 'list_item',
    getAttrs: (tok: Token) => {
      const checked = tok.attrGet('checked');
      if (checked === 'true') return { checked: true };
      if (checked === 'false') return { checked: false };
      return { checked: null };
    },
  },
  bullet_list: { block: 'bullet_list' },
  ordered_list: { block: 'ordered_list', getAttrs: (tok: Token) => ({ order: +(tok.attrGet('start') || 1) }) },
  heading: { block: 'heading', getAttrs: (tok: Token) => ({ level: +tok.tag.slice(1) }) },
  code_block: { block: 'code_block', noCloseToken: true },
  fence: { block: 'code_block', getAttrs: (tok: Token) => ({ params: tok.info || '' }), noCloseToken: true },
  hr: { node: 'horizontal_rule' },
  image: {
    node: 'image',
    getAttrs: (tok: Token) => ({
      src: tok.attrGet('src'),
      title: tok.attrGet('title') || null,
      alt: tok.children?.[0]?.content || null,
    }),
  },
  hardbreak: { node: 'hard_break' },
  em: { mark: 'em' },
  strong: { mark: 'strong' },
  link: {
    mark: 'link',
    getAttrs: (tok: Token) => ({
      href: tok.attrGet('href'),
      title: tok.attrGet('title') || null,
    }),
  },
  code_inline: { mark: 'code' },
  // Table support: render table cells as paragraphs (tables not fully supported in editor)
  table: { block: 'paragraph', noCloseToken: false },
  thead: { ignore: true },
  tbody: { ignore: true },
  tr: { ignore: true },
  th: { block: 'paragraph' },
  td: { block: 'paragraph' },
});

// Custom markdown serializer that uses "-" for bullet lists and handles task list items
export const markdownSerializer = new MarkdownSerializer(
  {
    ...defaultMarkdownSerializer.nodes,
    bullet_list(state, node) {
      state.renderList(node, '  ', () => '- ');
    },
    list_item(state, node) {
      // Prefix with checkbox syntax if this is a task list item
      if (node.attrs.checked !== null) {
        const checkbox = node.attrs.checked ? '[x] ' : '[ ] ';
        state.write(checkbox);
      }
      state.renderContent(node);
    },
  },
  defaultMarkdownSerializer.marks
);

// Input rule for task list items: - [ ] or - [x] at start of line
function taskListInputRule() {
  return new InputRule(/^\s*-\s+\[([ xX])\]\s$/, (state, match, start, end) => {
    const $start = state.doc.resolve(start);
    const isChecked = match[1]?.toLowerCase() === 'x';

    // Check if we're in a list item already
    for (let d = $start.depth; d > 0; d--) {
      const node = $start.node(d);
      if (node.type === nodes.list_item) {
        // Update the existing list item's checked attribute
        const tr = state.tr.delete(start, end);
        tr.setNodeMarkup($start.before(d), undefined, { checked: isChecked });
        return tr;
      }
    }

    // Not in a list - create a new task list
    const listItem = nodes.list_item.create({ checked: isChecked }, nodes.paragraph.create());
    const bulletList = nodes.bullet_list.create(null, listItem);
    return state.tr.replaceWith(start, end, bulletList);
  });
}

// Build input rules for markdown-like shortcuts
function buildInputRules() {
  const rules = smartQuotes.concat(ellipsis, emDash);

  // Task list: - [ ] or - [x] at start of line (must come before bullet list)
  rules.push(taskListInputRule());

  // Blockquote: > at start of line
  rules.push(wrappingInputRule(/^\s*>\s$/, nodes.blockquote));

  // Bullet list: - or * at start of line
  rules.push(wrappingInputRule(/^\s*([-*])\s$/, nodes.bullet_list));

  // Ordered list: 1. at start of line
  rules.push(
    wrappingInputRule(
      /^(\d+)\.\s$/,
      nodes.ordered_list,
      (match) => ({ order: +(match[1] ?? 1) }),
      (match, node) => node.childCount + (node.attrs.order as number) === +(match[1] ?? 1)
    )
  );

  // Code block: ``` at start of line
  rules.push(textblockTypeInputRule(/^```$/, nodes.code_block));

  // Headings: # ## ### etc at start of line
  for (let i = 1; i <= 6; i++) {
    const pattern = new RegExp(`^(#{${i}})\\s$`);
    rules.push(textblockTypeInputRule(pattern, nodes.heading, { level: i }));
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
  keys['Mod-b'] = toggleMark(marks.strong);
  keys['Mod-i'] = toggleMark(marks.em);
  keys['Mod-`'] = toggleMark(marks.code);

  // List operations - custom splitListItem that preserves task list state
  keys['Enter'] = (state, dispatch) => {
    const { $from } = state.selection;
    // Find if we're in a task list item
    for (let d = $from.depth; d > 0; d--) {
      const node = $from.node(d);
      if (node.type === nodes.list_item && node.attrs.checked !== null) {
        // In a task list item - split and create new unchecked item
        if (dispatch) {
          const result = splitListItem(nodes.list_item)(state, (tr) => {
            // After split, set the new item to unchecked
            const newPos = tr.selection.$from;
            for (let nd = newPos.depth; nd > 0; nd--) {
              const n = newPos.node(nd);
              if (n.type === nodes.list_item) {
                tr.setNodeMarkup(newPos.before(nd), undefined, { checked: false });
                break;
              }
            }
            dispatch(tr);
          });
          return result;
        }
        return splitListItem(nodes.list_item)(state);
      }
    }
    return splitListItem(nodes.list_item)(state, dispatch);
  };
  keys['Tab'] = sinkListItem(nodes.list_item);
  keys['Shift-Tab'] = liftListItem(nodes.list_item);

  return keymap(keys);
}

// Plugin to render checkboxes as widget decorations for task list items
function taskListPlugin() {
  return new Plugin({
    props: {
      decorations(state) {
        const decorations: Decoration[] = [];
        state.doc.descendants((node, pos) => {
          if (node.type === nodes.list_item && node.attrs.checked !== null) {
            // Create a checkbox widget at the start of the list item content
            const checkbox = document.createElement('input');
            checkbox.type = 'checkbox';
            checkbox.checked = node.attrs.checked;
            checkbox.className = 'task-checkbox';
            checkbox.addEventListener('mousedown', (e) => {
              e.preventDefault();
              // Find the EditorView from the event target
              const target = e.target as HTMLElement;
              const prosemirrorEl = target.closest('.ProseMirror');
              if (!prosemirrorEl) return;
              // Access view through the DOM element's custom property
              const view = (
                prosemirrorEl as HTMLElement & { pmView?: { dispatch: (tr: Transaction) => void; state: EditorState } }
              ).pmView;
              if (!view) return;
              const tr = view.state.tr.setNodeMarkup(pos, undefined, {
                checked: !node.attrs.checked,
              });
              view.dispatch(tr);
            });

            decorations.push(Decoration.widget(pos + 1, checkbox, { side: -1 }));
          }
        });
        return DecorationSet.create(state.doc, decorations);
      },
    },
  });
}

// Create editor state with all plugins
export function createEditorState(doc: ProseMirrorNode, extraPlugins?: Plugin[]) {
  return EditorState.create({
    doc,
    plugins: [
      buildInputRules(),
      buildKeymap(),
      keymap(baseKeymap),
      history(),
      dropCursor({ color: 'var(--c-primary)', width: 2 }),
      gapCursor(),
      taskListPlugin(),
      ...(extraPlugins || []),
    ],
  });
}
