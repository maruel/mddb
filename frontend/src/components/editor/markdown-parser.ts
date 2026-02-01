// Markdown-to-flat-blocks parser: converts markdown to ProseMirror document with flat block structure.
// Strategy: Parse markdown using MarkdownParser (which handles nesting correctly), then flatten the result.

import MarkdownIt from 'markdown-it';
import type Token from 'markdown-it/lib/token.mjs';
import { MarkdownParser } from 'prosemirror-markdown';
import { nestedSchema } from './nested-schema';
import { schema, nodes, type BlockAttrs } from './schema';
import type { Node as ProseMirrorNode } from 'prosemirror-model';

// Create markdown-it instance with task list and underline support
function createMarkdownIt(): MarkdownIt {
  const md = new MarkdownIt();

  // Parse task list checkboxes: [ ] and [x] at start of content
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
              // Mark this as a task list item
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

  // Parse inline underline: <u>text</u>
  md.inline.ruler.before('html_inline', 'underline', (state, silent) => {
    const start = state.pos;
    const max = state.posMax;
    const src = state.src;

    if (src.slice(start, start + 3).toLowerCase() !== '<u>') return false;

    const closeTag = '</u>';
    const closePos = src.toLowerCase().indexOf(closeTag, start + 3);
    if (closePos === -1 || closePos >= max) return false;

    if (!silent) {
      const openToken = state.push('underline_open', 'u', 1);
      openToken.markup = '<u>';

      const content = src.slice(start + 3, closePos);
      const textToken = state.push('text', '', 0);
      textToken.content = content;

      const closeToken = state.push('underline_close', 'u', -1);
      closeToken.markup = '</u>';
    }

    state.pos = closePos + closeTag.length;
    return true;
  });

  return md;
}

/**
 * Convert inline content from nested schema to flat schema.
 * This is necessary because the nested schema creates text nodes with a different schema instance.
 */
function convertInlineContent(content: ProseMirrorNode | null): ProseMirrorNode[] {
  if (!content) return [];

  const result: ProseMirrorNode[] = [];
  content.content.forEach((child) => {
    if (child.isText) {
      // Convert text node: create new text with same content and converted marks
      const marks = child.marks
        .map((m) => {
          const markType = schema.marks[m.type.name];
          return markType ? markType.create(m.attrs) : null;
        })
        .filter((m): m is NonNullable<typeof m> => m !== null);

      result.push(schema.text(child.text || '', marks));
    } else if (child.type.name === 'hard_break') {
      // Convert hard_break
      const hardBreakType = schema.nodes.hard_break;
      if (hardBreakType) result.push(hardBreakType.create());
    } else if (child.type.name === 'image') {
      // Convert image
      const imageType = schema.nodes.image;
      if (imageType) result.push(imageType.create(child.attrs));
    }
    // Other inline types can be added here if needed
  });

  return result;
}

/**
 * Converts markdown text to a ProseMirror document with flat block structure.
 * Parses using nested schema first, then flattens the structure.
 */
export function parseMarkdown(markdown: string): ProseMirrorNode {
  const md = createMarkdownIt();
  const parser = new MarkdownParser(nestedSchema, md, {
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
    em: { mark: 'em' },
    strong: { mark: 'strong' },
    underline: { mark: 'underline' },
    s: { mark: 'strikethrough' },
    link: {
      mark: 'link',
      getAttrs: (tok: Token) => ({
        href: tok.attrGet('href'),
        title: tok.attrGet('title') || null,
      }),
    },
    code_inline: { mark: 'code' },
  });

  // Parse markdown using nested schema
  const nestedDoc = parser.parse(markdown);

  // Flatten the nested structure to flat blocks
  const flatBlocks: ProseMirrorNode[] = [];
  flattenDocument(nestedDoc, flatBlocks, 0);

  return nodes.doc.create(null, flatBlocks);
}

/**
 * Recursively flattens nested ProseMirror document into flat blocks.
 * Converts nested lists to flat blocks with indent attributes.
 */
function flattenDocument(node: ProseMirrorNode, blocks: ProseMirrorNode[], baseIndent: number): void {
  node.forEach((child) => {
    if (child.type.name === 'bullet_list' || child.type.name === 'ordered_list') {
      flattenList(child, blocks, baseIndent, child.type.name as 'bullet_list' | 'ordered_list');
    } else if (child.type.name === 'list_item') {
      // Should not happen at top level, but handle it
      flattenListItem(child, blocks, baseIndent, 'bullet');
    } else if (child.type.name === 'heading') {
      const content = convertInlineContent(child);
      const blockNode = nodes.block.create({ type: 'heading', level: child.attrs.level, indent: 0 }, content);
      blocks.push(blockNode);
    } else if (child.type.name === 'paragraph') {
      const content = convertInlineContent(child);
      const blockNode = nodes.block.create({ type: 'paragraph', indent: 0 }, content);
      blocks.push(blockNode);
    } else if (child.type.name === 'blockquote') {
      // Extract text from blockquote
      const text = child.textContent;
      const blockNode = nodes.block.create({ type: 'quote', indent: 0 }, schema.text(text));
      blocks.push(blockNode);
    } else if (child.type.name === 'code_block') {
      const content = child.textContent;
      const language = child.attrs.params || '';
      const blockNode = nodes.block.create(
        { type: 'code', language: language || undefined, indent: 0 },
        schema.text(content)
      );
      blocks.push(blockNode);
    } else if (child.type.name === 'horizontal_rule') {
      const blockNode = nodes.divider.create({ type: 'divider', indent: 0 });
      blocks.push(blockNode);
    }
  });
}

/**
 * Flattens a list (bullet or ordered) into flat blocks with proper indentation.
 */
function flattenList(
  listNode: ProseMirrorNode,
  blocks: ProseMirrorNode[],
  baseIndent: number,
  listType: 'bullet_list' | 'ordered_list'
): void {
  let counter = 1;
  listNode.forEach((item) => {
    if (item.type.name === 'list_item') {
      const number = listType === 'ordered_list' ? counter++ : undefined;
      flattenListItem(item, blocks, baseIndent, listType === 'ordered_list' ? 'number' : 'bullet', number);
    }
  });
}

/**
 * Flattens a single list item and any nested lists within it.
 */
function flattenListItem(
  itemNode: ProseMirrorNode,
  blocks: ProseMirrorNode[],
  baseIndent: number,
  itemType: 'bullet' | 'number' | 'task',
  number?: number
): void {
  let itemContent: ProseMirrorNode | null = null;

  // Separate item content from nested lists
  itemNode.forEach((child) => {
    if (child.type.name === 'paragraph') {
      itemContent = child;
    }
  });

  // Create block for this item
  const isTask = itemNode.attrs.checked !== null;
  const blockType = isTask ? 'task' : itemType;
  const blockAttrs: BlockAttrs = {
    type: blockType,
    indent: baseIndent,
    ...(isTask && { checked: itemNode.attrs.checked }),
    ...(blockType === 'number' && typeof number === 'number' && { number }),
  };

  const blockContent = itemContent ? convertInlineContent(itemContent as ProseMirrorNode) : [];
  const blockNode = nodes.block.create(blockAttrs, blockContent);
  blocks.push(blockNode);

  // Flatten nested lists
  itemNode.forEach((child) => {
    if (child.type.name === 'bullet_list' || child.type.name === 'ordered_list') {
      flattenList(child, blocks, baseIndent + 1, child.type.name as 'bullet_list' | 'ordered_list');
    }
  });
}
