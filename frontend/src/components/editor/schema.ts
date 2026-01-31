// Flat block schema for Notion-style editor with uniform drag-drop.
// Every visible line is a top-level block node with attributes for type, indentation, and styling.

import { Schema, type NodeSpec, type DOMOutputSpec } from 'prosemirror-model';
import {
  schema as baseSchema,
  MarkdownSerializer,
  MarkdownParser,
} from 'prosemirror-markdown';

// Block types: correspond to different content kinds
export type BlockType = 'paragraph' | 'heading' | 'bullet' | 'number' | 'task' | 'quote' | 'code' | 'divider';

// Block attributes: describe a single block's type, nesting level, and metadata
export interface BlockAttrs {
  type: BlockType;
  level?: number; // heading level (1-6)
  indent: number; // nesting depth (0+), enables drag-and-drop without changing type
  checked?: boolean; // task completion state (for type=task)
  language?: string; // code block language (for type=code)
}

// Block node specification: all content lines are unified under this node
const blockSpec: NodeSpec = {
  attrs: {
    type: { default: 'paragraph' },
    level: { default: null },
    indent: { default: 0 },
    checked: { default: null },
    language: { default: null },
  },
  content: 'inline*',
  group: 'block',
  parseDOM: [
    // Headings: h1-h6
    { tag: 'h1', attrs: { type: 'heading', level: 1 } },
    { tag: 'h2', attrs: { type: 'heading', level: 2 } },
    { tag: 'h3', attrs: { type: 'heading', level: 3 } },
    { tag: 'h4', attrs: { type: 'heading', level: 4 } },
    { tag: 'h5', attrs: { type: 'heading', level: 5 } },
    { tag: 'h6', attrs: { type: 'heading', level: 6 } },

    // Task list items: li.task-list-item with optional data-checked
    {
      tag: 'li.task-list-item',
      getAttrs(dom: HTMLElement) {
        return {
          type: 'task',
          checked: dom.dataset.checked === 'true',
          indent: parseInt(dom.dataset.indent || '0', 10),
        };
      },
    },

    // Bullet and numbered list items: plain li, context determines bullet vs number
    // For now, default to bullet; number type is handled via attributes from HTML5 data
    {
      tag: 'li',
      getAttrs(dom: HTMLElement) {
        return {
          type: dom.dataset.type === 'number' ? 'number' : 'bullet',
          indent: parseInt(dom.dataset.indent || '0', 10),
        };
      },
    },

    // Blockquote: blockquote > p or just blockquote
    {
      tag: 'blockquote',
      attrs: { type: 'quote' },
    },

    // Code block: pre with optional data-language
    {
      tag: 'pre',
      getAttrs(dom: HTMLElement) {
        // Try to extract language from class names or data attribute
        // Import extractCodeLanguage from dom-parser
        let language: string | null = dom.dataset.language || null;

        if (!language) {
          const classes = Array.from(dom.classList);
          for (const cls of classes) {
            if (cls.startsWith('language-')) {
              language = cls.slice(9);
              break;
            }
            if (cls.startsWith('lang-')) {
              language = cls.slice(5);
              break;
            }
            if (cls.startsWith('hljs-')) {
              language = cls.slice(5);
              break;
            }
          }
        }

        return {
          type: 'code',
          language,
        };
      },
    },

    // Horizontal rule: hr
    {
      tag: 'hr',
      attrs: { type: 'divider' },
    },

    // Paragraph: default, catch-all
    { tag: 'p', attrs: { type: 'paragraph' } },
  ],
  toDOM(node): DOMOutputSpec {
    const { type, level, indent, checked, language } = node.attrs;
    const baseAttrs: Record<string, string> = {
      'data-type': type,
      'data-indent': String(indent),
    };

    switch (type) {
      case 'heading': {
        const tag = `h${level || 1}`;
        return [tag, baseAttrs, 0];
      }

      case 'bullet':
      case 'number': {
        const attrs = { ...baseAttrs, class: `block-${type}` };
        return ['div', attrs, 0];
      }

      case 'task': {
        const attrs = {
          ...baseAttrs,
          class: 'block-task',
          'data-checked': String(checked || false),
        };
        return ['div', attrs, 0];
      }

      case 'quote': {
        return ['blockquote', baseAttrs, 0];
      }

      case 'code': {
        const attrs = {
          ...baseAttrs,
          'data-language': language || '',
        };
        return ['pre', attrs, ['code', 0]];
      }

      case 'divider': {
        return ['hr', baseAttrs];
      }

      default:
        return ['p', baseAttrs, 0];
    }
  },
};

// Divider node: empty block for horizontal rules
const dividerSpec: NodeSpec = {
  attrs: {
    type: { default: 'divider' },
    indent: { default: 0 },
  },
  content: 'inline*',
  group: 'block',
  parseDOM: [{ tag: 'hr', attrs: { type: 'divider' } }],
  toDOM(node): DOMOutputSpec {
    return ['hr', { 'data-type': 'divider', 'data-indent': String(node.attrs.indent) }];
  },
};

// Create flat block schema: replace nested list/heading/paragraph nodes with single block node
export const schema = new Schema({
  nodes: baseSchema.spec.nodes
    .remove('bullet_list')
    .remove('ordered_list')
    .remove('list_item')
    .remove('paragraph')
    .remove('heading')
    .remove('blockquote')
    .remove('code_block')
    .remove('horizontal_rule')
    .addToEnd('block', blockSpec)
    .addToEnd('divider', dividerSpec),
  marks: baseSchema.spec.marks
    .addToEnd('underline', {
      parseDOM: [{ tag: 'u' }, { style: 'text-decoration=underline' }],
      toDOM(): DOMOutputSpec {
        return ['u', 0];
      },
    })
    .addToEnd('strikethrough', {
      parseDOM: [{ tag: 's' }, { tag: 'del' }, { style: 'text-decoration=line-through' }],
      toDOM(): DOMOutputSpec {
        return ['s', 0];
      },
    }),
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

// Exported node types for use in plugins and commands
export const nodes = {
  doc: getNodeType('doc'),
  block: getNodeType('block'),
  divider: getNodeType('divider'),
};

// Exported mark types for use in plugins and commands
export const marks = {
  strong: getMarkType('strong'),
  em: getMarkType('em'),
  code: getMarkType('code'),
  link: getMarkType('link'),
  underline: getMarkType('underline'),
  strikethrough: getMarkType('strikethrough'),
};
