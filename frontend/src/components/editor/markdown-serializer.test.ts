// Unit tests for markdown serializer: converting flat blocks back to markdown.

import { describe, it, expect } from 'vitest';
import { serializeToMarkdown } from './markdown-serializer';
import { parseMarkdown } from './markdown-parser';
import { schema } from './schema';
import type { BlockAttrs } from './schema';

describe('markdown-serializer', () => {
  describe('serializeToMarkdown', () => {
    it('serializes paragraph blocks', () => {
      const blocks = [
        schema.nodes.block.create({ type: 'paragraph', indent: 0 } as BlockAttrs, schema.text('Hello world')),
      ];
      const doc = schema.nodes.doc.create(null, blocks);

      const result = serializeToMarkdown(doc);
      expect(result).toBe('Hello world');
    });

    it('serializes heading blocks with proper level', () => {
      const blocks = [
        schema.nodes.block.create({ type: 'heading', level: 2, indent: 0 } as BlockAttrs, schema.text('Subheading')),
      ];
      const doc = schema.nodes.doc.create(null, blocks);

      const result = serializeToMarkdown(doc);
      expect(result).toBe('## Subheading');
    });

    it('serializes bullet list blocks', () => {
      const blocks = [
        schema.nodes.block.create({ type: 'bullet', indent: 0 } as BlockAttrs, schema.text('Item 1')),
        schema.nodes.block.create({ type: 'bullet', indent: 0 } as BlockAttrs, schema.text('Item 2')),
      ];
      const doc = schema.nodes.doc.create(null, blocks);

      const result = serializeToMarkdown(doc);
      expect(result).toBe('- Item 1\n- Item 2');
    });

    it('serializes numbered list blocks with proper counters', () => {
      const blocks = [
        schema.nodes.block.create({ type: 'number', indent: 0 } as BlockAttrs, schema.text('First')),
        schema.nodes.block.create({ type: 'number', indent: 0 } as BlockAttrs, schema.text('Second')),
        schema.nodes.block.create({ type: 'number', indent: 0 } as BlockAttrs, schema.text('Third')),
      ];
      const doc = schema.nodes.doc.create(null, blocks);

      const result = serializeToMarkdown(doc);
      expect(result).toBe('1. First\n2. Second\n3. Third');
    });

    it('serializes nested bullet list with proper indentation', () => {
      const blocks = [
        schema.nodes.block.create({ type: 'bullet', indent: 0 } as BlockAttrs, schema.text('Item 1')),
        schema.nodes.block.create({ type: 'bullet', indent: 1 } as BlockAttrs, schema.text('Nested 1')),
        schema.nodes.block.create({ type: 'bullet', indent: 1 } as BlockAttrs, schema.text('Nested 2')),
        schema.nodes.block.create({ type: 'bullet', indent: 0 } as BlockAttrs, schema.text('Item 2')),
      ];
      const doc = schema.nodes.doc.create(null, blocks);

      const result = serializeToMarkdown(doc);
      expect(result).toBe('- Item 1\n  - Nested 1\n  - Nested 2\n- Item 2');
    });

    it('serializes deeply nested lists (3+ levels)', () => {
      const blocks = [
        schema.nodes.block.create({ type: 'bullet', indent: 0 } as BlockAttrs, schema.text('L1')),
        schema.nodes.block.create({ type: 'bullet', indent: 1 } as BlockAttrs, schema.text('L2')),
        schema.nodes.block.create({ type: 'bullet', indent: 2 } as BlockAttrs, schema.text('L3')),
        schema.nodes.block.create({ type: 'bullet', indent: 2 } as BlockAttrs, schema.text('L3b')),
        schema.nodes.block.create({ type: 'bullet', indent: 1 } as BlockAttrs, schema.text('L2b')),
      ];
      const doc = schema.nodes.doc.create(null, blocks);

      const result = serializeToMarkdown(doc);
      expect(result).toBe('- L1\n  - L2\n    - L3\n    - L3b\n  - L2b');
    });

    it('serializes task list with unchecked items', () => {
      const blocks = [
        schema.nodes.block.create(
          { type: 'task', indent: 0, checked: false } as BlockAttrs,
          schema.text('Task 1')
        ),
        schema.nodes.block.create(
          { type: 'task', indent: 0, checked: false } as BlockAttrs,
          schema.text('Task 2')
        ),
      ];
      const doc = schema.nodes.doc.create(null, blocks);

      const result = serializeToMarkdown(doc);
      expect(result).toBe('- [ ] Task 1\n- [ ] Task 2');
    });

    it('serializes task list with checked items', () => {
      const blocks = [
        schema.nodes.block.create(
          { type: 'task', indent: 0, checked: true } as BlockAttrs,
          schema.text('Done 1')
        ),
        schema.nodes.block.create(
          { type: 'task', indent: 0, checked: true } as BlockAttrs,
          schema.text('Done 2')
        ),
      ];
      const doc = schema.nodes.doc.create(null, blocks);

      const result = serializeToMarkdown(doc);
      expect(result).toBe('- [x] Done 1\n- [x] Done 2');
    });

    it('serializes mixed task and bullet lists', () => {
      const blocks = [
        schema.nodes.block.create(
          { type: 'task', indent: 0, checked: false } as BlockAttrs,
          schema.text('Task')
        ),
        schema.nodes.block.create({ type: 'bullet', indent: 0 } as BlockAttrs, schema.text('Regular')),
        schema.nodes.block.create(
          { type: 'task', indent: 0, checked: true } as BlockAttrs,
          schema.text('Done')
        ),
      ];
      const doc = schema.nodes.doc.create(null, blocks);

      const result = serializeToMarkdown(doc);
      expect(result).toBe('- [ ] Task\n- Regular\n- [x] Done');
    });

    it('serializes blockquote', () => {
      const blocks = [
        schema.nodes.block.create({ type: 'quote', indent: 0 } as BlockAttrs, schema.text('A quote')),
      ];
      const doc = schema.nodes.doc.create(null, blocks);

      const result = serializeToMarkdown(doc);
      expect(result).toBe('> A quote');
    });

    it('serializes code block with language', () => {
      const blocks = [
        schema.nodes.block.create(
          { type: 'code', indent: 0, language: 'typescript' } as BlockAttrs,
          schema.text('const x = 1;')
        ),
      ];
      const doc = schema.nodes.doc.create(null, blocks);

      const result = serializeToMarkdown(doc);
      expect(result).toBe('```typescript\nconst x = 1;\n```');
    });

    it('serializes code block without language', () => {
      const blocks = [
        schema.nodes.block.create({ type: 'code', indent: 0 } as BlockAttrs, schema.text('plain code')),
      ];
      const doc = schema.nodes.doc.create(null, blocks);

      const result = serializeToMarkdown(doc);
      expect(result).toBe('```\nplain code\n```');
    });

    it('serializes horizontal rule', () => {
      const blocks = [schema.nodes.divider.create({ type: 'divider', indent: 0 } as BlockAttrs)];
      const doc = schema.nodes.doc.create(null, blocks);

      const result = serializeToMarkdown(doc);
      expect(result).toBe('---');
    });

    it('serializes mixed block types in order', () => {
      const blocks = [
        schema.nodes.block.create({ type: 'heading', level: 1, indent: 0 } as BlockAttrs, schema.text('Title')),
        schema.nodes.block.create({ type: 'paragraph', indent: 0 } as BlockAttrs, schema.text('Paragraph')),
        schema.nodes.block.create({ type: 'bullet', indent: 0 } as BlockAttrs, schema.text('List')),
        schema.nodes.block.create({ type: 'quote', indent: 0 } as BlockAttrs, schema.text('Quote')),
      ];
      const doc = schema.nodes.doc.create(null, blocks);

      const result = serializeToMarkdown(doc);
      expect(result).toBe('# Title\nParagraph\n- List\n> Quote');
    });

    it('handles nested ordered lists with counter per indent', () => {
      const blocks = [
        schema.nodes.block.create({ type: 'number', indent: 0 } as BlockAttrs, schema.text('First')),
        schema.nodes.block.create({ type: 'number', indent: 1 } as BlockAttrs, schema.text('Nested 1')),
        schema.nodes.block.create({ type: 'number', indent: 1 } as BlockAttrs, schema.text('Nested 2')),
        schema.nodes.block.create({ type: 'number', indent: 0 } as BlockAttrs, schema.text('Second')),
      ];
      const doc = schema.nodes.doc.create(null, blocks);

      const result = serializeToMarkdown(doc);
      expect(result).toBe('1. First\n  1. Nested 1\n  2. Nested 2\n2. Second');
    });

    it('resets number counter when leaving ordered list', () => {
      const blocks = [
        schema.nodes.block.create({ type: 'number', indent: 0 } as BlockAttrs, schema.text('One')),
        schema.nodes.block.create({ type: 'number', indent: 0 } as BlockAttrs, schema.text('Two')),
        schema.nodes.block.create({ type: 'paragraph', indent: 0 } as BlockAttrs, schema.text('Paragraph')),
        schema.nodes.block.create({ type: 'number', indent: 0 } as BlockAttrs, schema.text('Three')),
      ];
      const doc = schema.nodes.doc.create(null, blocks);

      const result = serializeToMarkdown(doc);
      expect(result).toBe('1. One\n2. Two\nParagraph\n1. Three');
    });

    it('handles mixed ordered and bullet at different indents', () => {
      const blocks = [
        schema.nodes.block.create({ type: 'bullet', indent: 0 } as BlockAttrs, schema.text('Bullet')),
        schema.nodes.block.create({ type: 'number', indent: 1 } as BlockAttrs, schema.text('Ordered nested')),
        schema.nodes.block.create({ type: 'number', indent: 1 } as BlockAttrs, schema.text('Ordered nested 2')),
        schema.nodes.block.create({ type: 'bullet', indent: 0 } as BlockAttrs, schema.text('Bullet 2')),
      ];
      const doc = schema.nodes.doc.create(null, blocks);

      const result = serializeToMarkdown(doc);
      expect(result).toBe('- Bullet\n  1. Ordered nested\n  2. Ordered nested 2\n- Bullet 2');
    });

    it('roundtrips through parse and serialize', () => {
      const original = '# Title\n- Item 1\n  - Nested\n- Item 2\n> Quote';
      const doc = parseMarkdown(original);
      const serialized = serializeToMarkdown(doc);

      expect(serialized).toBe(original);
    });

    it('preserves task list state in roundtrip', () => {
      const original = '- [ ] Unchecked\n- [x] Checked';
      const doc = parseMarkdown(original);
      const serialized = serializeToMarkdown(doc);

      expect(serialized).toBe(original);
    });

    it('handles task list with nesting in roundtrip', () => {
      const original = '- [ ] Task 1\n  - [ ] Subtask\n  - [x] Done\n- [x] Task 2';
      const doc = parseMarkdown(original);
      const serialized = serializeToMarkdown(doc);

      expect(serialized).toBe(original);
    });
  });
});
