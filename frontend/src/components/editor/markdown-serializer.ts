// Flat-blocks-to-markdown serializer: reconstructs nested markdown from flat block structure.
// Groups consecutive list items by indent and type, emits proper list markers and indentation.

import type { Node as ProseMirrorNode } from 'prosemirror-model';
import type { BlockAttrs } from './schema';

interface ListContext {
  type: 'bullet' | 'number' | 'task';
  indent: number;
}

/**
 * Serializes a flat block document back to markdown.
 * Reconstructs list structure by tracking consecutive blocks of same type/indent.
 */
export function serializeToMarkdown(doc: ProseMirrorNode): string {
  const lines: string[] = [];
  const listContext: ListContext[] = [];
  const numberCounters = new Map<number, number>(); // indent → counter
  let prevType: string | null = null;

  doc.forEach((block) => {
    const { type, indent, checked, level, language } = block.attrs as BlockAttrs;
    const content = serializeInline(block);

    // Close list contexts that no longer apply
    while (listContext.length > 0) {
      const ctx = listContext[listContext.length - 1];
      if (ctx === undefined) break;
      // Close if: indent has decreased, or type has changed at same indent
      if (ctx.indent > indent || (ctx.indent === indent && ctx.type !== type && isListType(type))) {
        listContext.pop();
      } else {
        break;
      }
    }

    // Add blank line before paragraphs that follow non-paragraphs (e.g., after list items)
    // This ensures proper separation in markdown so they parse correctly
    const prevWasList = prevType === 'bullet' || prevType === 'number' || prevType === 'task';
    const currentIsParagraph = type === 'paragraph';
    if (lines.length > 0 && prevWasList && currentIsParagraph) {
      lines.push('');
    }
    // Also add blank line between paragraphs
    if (lines.length > 0 && prevType === 'paragraph' && currentIsParagraph) {
      lines.push('');
    }
    // Add blank line before list items that follow paragraphs
    if (lines.length > 0 && prevType === 'paragraph' && isListType(type)) {
      lines.push('');
    }

    // Build line with appropriate prefix
    const indentStr = '  '.repeat(indent);
    let prefix: string;

    switch (type) {
      case 'bullet': {
        prefix = '- ';
        if (!listContext.some((ctx) => ctx.indent === indent && ctx.type === 'bullet')) {
          listContext.push({ type: 'bullet', indent });
        }
        break;
      }

      case 'number': {
        const counter = (numberCounters.get(indent) || 0) + 1;
        numberCounters.set(indent, counter);
        prefix = `${counter}. `;
        if (!listContext.some((ctx) => ctx.indent === indent && ctx.type === 'number')) {
          listContext.push({ type: 'number', indent });
        }
        break;
      }

      case 'task': {
        prefix = checked ? '- [x] ' : '- [ ] ';
        if (!listContext.some((ctx) => ctx.indent === indent && ctx.type === 'task')) {
          listContext.push({ type: 'task', indent });
        }
        break;
      }

      case 'heading': {
        prefix = '#'.repeat(level || 1) + ' ';
        // Reset number counters at this indent level when leaving list
        numberCounters.delete(indent);
        break;
      }

      case 'quote': {
        prefix = '> ';
        numberCounters.delete(indent);
        break;
      }

      case 'code': {
        lines.push('```' + (language || ''));
        lines.push(content);
        lines.push('```');
        prevType = type;
        return;
      }

      case 'divider': {
        lines.push('---');
        prevType = type;
        return;
      }

      default:
        prefix = '';
        numberCounters.delete(indent);
    }

    lines.push(indentStr + prefix + content);
    prevType = type;
  });

  return lines.join('\n');
}

/**
 * Serializes the inline content of a block (handles marks and nested formatting).
 */
function serializeInline(block: ProseMirrorNode): string {
  let result = '';

  block.forEach((node) => {
    if (node.isText) {
      let text = node.text || '';

      // Apply marks
      node.marks.forEach((mark) => {
        if (mark.type.name === 'strong') {
          text = `**${text}**`;
        } else if (mark.type.name === 'em') {
          text = `*${text}*`;
        } else if (mark.type.name === 'code') {
          text = `\`${text}\``;
        } else if (mark.type.name === 'strikethrough') {
          text = `~~${text}~~`;
        } else if (mark.type.name === 'underline') {
          text = `<u>${text}</u>`;
        } else if (mark.type.name === 'link') {
          const href = mark.attrs.href || '#';
          const title = mark.attrs.title;
          text = `[${text}](${href}${title ? ` "${title}"` : ''})`;
        }
      });

      result += text;
    } else if (node.type.name === 'image') {
      const { src, alt, title } = node.attrs;
      result += `![${alt || ''}](${src || ''}${title ? ` "${title}"` : ''})`;
    } else if (node.type.name === 'hard_break') {
      result += '\\\n';
    }
  });

  return result;
}

/**
 * Checks if a block type is a list type.
 */
function isListType(type: string): boolean {
  return type === 'bullet' || type === 'number' || type === 'task';
}
