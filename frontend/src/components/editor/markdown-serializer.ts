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

  doc.forEach((block) => {
    const { type, indent, checked, level, language } = block.attrs as BlockAttrs;
    const content = serializeInline(block);

    // Close list contexts that no longer apply
    while (listContext.length > 0) {
      const ctx = listContext[listContext.length - 1];
      // Close if: indent has decreased, or type has changed at same indent
      if (ctx.indent > indent || (ctx.indent === indent && ctx.type !== type && isListType(type))) {
        listContext.pop();
      } else {
        break;
      }
    }

    // Build line with appropriate prefix
    const indentStr = '  '.repeat(indent);
    let prefix = '';

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
        return;
      }

      case 'divider': {
        lines.push('---');
        return;
      }

      default:
        prefix = '';
        numberCounters.delete(indent);
    }

    lines.push(indentStr + prefix + content);
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
          text = `_${text}_`;
        } else if (mark.type.name === 'code') {
          text = `\`${text}\``;
        } else if (mark.type.name === 'strikethrough') {
          text = `~~${text}~~`;
        } else if (mark.type.name === 'underline') {
          text = `<u>${text}</u>`;
        } else if (mark.type.name === 'link') {
          const href = mark.attrs.href || '#';
          text = `[${text}](${href})`;
        }
      });

      result += text;
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
