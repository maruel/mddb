// Unit tests for markdown parser: converting markdown to flat blocks.

import { describe, it, expect } from 'vitest';
import { parseMarkdown } from './markdown-parser';
import { serializeToMarkdown } from './markdown-serializer';

describe('markdown-parser', () => {
  describe('parseMarkdown', () => {
    it('parses simple paragraph', () => {
      const markdown = 'Hello world';
      const doc = parseMarkdown(markdown);

      expect(doc.type.name).toBe('doc');
      expect(doc.childCount).toBe(1);

      const block = doc.firstChild!;
      expect(block.attrs.type).toBe('paragraph');
      expect(block.textContent).toBe('Hello world');
    });

    it('parses heading with level', () => {
      const markdown = '## Subheading';
      const doc = parseMarkdown(markdown);

      const block = doc.firstChild!;
      expect(block.attrs.type).toBe('heading');
      expect(block.attrs.level).toBe(2);
      expect(block.textContent).toBe('Subheading');
    });

    it('parses all heading levels', () => {
      for (let level = 1; level <= 6; level++) {
        const markdown = `${'#'.repeat(level)} Heading ${level}`;
        const doc = parseMarkdown(markdown);

        const block = doc.firstChild!;
        expect(block.attrs.level).toBe(level);
      }
    });

    it('parses bullet list items with indent', () => {
      const markdown = '- Item 1\n- Item 2';
      const doc = parseMarkdown(markdown);

      expect(doc.childCount).toBe(2);

      const item1 = doc.firstChild!;
      expect(item1.attrs.type).toBe('bullet');
      expect(item1.attrs.indent).toBe(0);
      expect(item1.textContent).toBe('Item 1');

      const item2 = doc.childAfter(item1.nodeSize).node!;
      expect(item2.attrs.type).toBe('bullet');
      expect(item2.attrs.indent).toBe(0);
      expect(item2.textContent).toBe('Item 2');
    });

    it('parses ordered list with numbers', () => {
      const markdown = '1. First\n2. Second\n3. Third';
      const doc = parseMarkdown(markdown);

      expect(doc.childCount).toBe(3);

      doc.forEach((block) => {
        expect(block.attrs.type).toBe('number');
        expect(block.attrs.indent).toBe(0);
      });
    });

    it('parses nested bullet lists', () => {
      const markdown = '- Item 1\n  - Nested 1\n  - Nested 2\n- Item 2';
      const doc = parseMarkdown(markdown);

      const blocks = Array.from(doc.content.content);
      expect(blocks[0].attrs.indent).toBe(0); // Item 1
      expect(blocks[1].attrs.indent).toBe(1); // Nested 1
      expect(blocks[2].attrs.indent).toBe(1); // Nested 2
      expect(blocks[3].attrs.indent).toBe(0); // Item 2
    });

    it('parses deeply nested lists (3+ levels)', () => {
      const markdown = '- L1\n  - L2\n    - L3\n    - L3b\n  - L2b';
      const doc = parseMarkdown(markdown);

      const blocks = Array.from(doc.content.content);
      expect(blocks[0].attrs.indent).toBe(0);
      expect(blocks[1].attrs.indent).toBe(1);
      expect(blocks[2].attrs.indent).toBe(2);
      expect(blocks[3].attrs.indent).toBe(2);
      expect(blocks[4].attrs.indent).toBe(1);
    });

    it('parses task list items with unchecked', () => {
      const markdown = '- [ ] Task 1\n- [ ] Task 2';
      const doc = parseMarkdown(markdown);

      expect(doc.childCount).toBe(2);

      doc.forEach((block) => {
        expect(block.attrs.type).toBe('task');
        expect(block.attrs.checked).toBe(false);
      });
    });

    it('parses task list items with checked', () => {
      const markdown = '- [x] Done 1\n- [X] Done 2';
      const doc = parseMarkdown(markdown);

      expect(doc.childCount).toBe(2);

      doc.forEach((block) => {
        expect(block.attrs.type).toBe('task');
        expect(block.attrs.checked).toBe(true);
      });
    });

    it('parses mixed task and bullet lists', () => {
      const markdown = '- [ ] Task\n- Regular bullet\n- [x] Done';
      const doc = parseMarkdown(markdown);

      expect(doc.childCount).toBe(3);
      expect(doc.firstChild!.attrs.type).toBe('task');
      expect(doc.childAfter(doc.firstChild!.nodeSize).node!.attrs.type).toBe('bullet');
    });

    it('parses blockquote', () => {
      const markdown = '> A quote';
      const doc = parseMarkdown(markdown);

      const block = doc.firstChild!;
      expect(block.attrs.type).toBe('quote');
      expect(block.textContent).toContain('A quote');
    });

    it('parses code block with language', () => {
      const markdown = '```typescript\nconst x = 1;\n```';
      const doc = parseMarkdown(markdown);

      const block = doc.firstChild!;
      expect(block.attrs.type).toBe('code');
      expect(block.attrs.language).toBe('typescript');
      expect(block.textContent).toContain('const x = 1;');
    });

    it('parses code block without language', () => {
      const markdown = '```\nplain code\n```';
      const doc = parseMarkdown(markdown);

      const block = doc.firstChild!;
      expect(block.attrs.type).toBe('code');
      expect(block.attrs.language || '').toBe('');
      expect(block.textContent).toContain('plain code');
    });

    it('parses horizontal rule', () => {
      const markdown = '---';
      const doc = parseMarkdown(markdown);

      const block = doc.firstChild!;
      expect(block.type.name).toBe('divider');
    });

    it('preserves block order', () => {
      const markdown = '# Title\nParagraph\n- List\n> Quote';
      const doc = parseMarkdown(markdown);

      const blocks = Array.from(doc.content.content);
      expect(blocks[0].attrs.type).toBe('heading');
      expect(blocks[1].attrs.type).toBe('paragraph');
      expect(blocks[2].attrs.type).toBe('bullet');
      expect(blocks[3].attrs.type).toBe('quote');
    });

    it('handles mixed ordered and bullet lists at different indents', () => {
      const markdown = '- Bullet\n  1. Ordered nested\n  2. Ordered nested 2\n- Bullet 2';
      const doc = parseMarkdown(markdown);

      const blocks = Array.from(doc.content.content);
      expect(blocks[0].attrs.type).toBe('bullet');
      expect(blocks[0].attrs.indent).toBe(0);
      expect(blocks[1].attrs.type).toBe('number');
      expect(blocks[1].attrs.indent).toBe(1);
      expect(blocks[2].attrs.type).toBe('number');
      expect(blocks[2].attrs.indent).toBe(1);
      expect(blocks[3].attrs.type).toBe('bullet');
      expect(blocks[3].attrs.indent).toBe(0);
    });

    it('roundtrips simple content through markdown', () => {
      const original = 'Hello world';
      const doc = parseMarkdown(original);
      const serialized = serializeToMarkdown(doc);

      expect(serialized).toBe('Hello world');
    });

    it('roundtrips bullet list', () => {
      const original = '- Item 1\n- Item 2';
      const doc = parseMarkdown(original);
      const serialized = serializeToMarkdown(doc);

      expect(serialized).toBe('- Item 1\n- Item 2');
    });

    it('roundtrips nested list', () => {
      const original = '- Item 1\n  - Nested 1\n  - Nested 2\n- Item 2';
      const doc = parseMarkdown(original);
      const serialized = serializeToMarkdown(doc);

      expect(serialized).toBe(original);
    });

    it('roundtrips task list with mixed states', () => {
      const original = '- [ ] Task 1\n- [x] Task 2';
      const doc = parseMarkdown(original);
      const serialized = serializeToMarkdown(doc);

      expect(serialized).toBe(original);
    });

    it('roundtrips code block', () => {
      const original = '```typescript\nconst x = 1;\n```';
      const doc = parseMarkdown(original);
      const serialized = serializeToMarkdown(doc);

      expect(serialized).toBe(original);
    });

    it('handles empty lines in markdown', () => {
      const markdown = 'Paragraph 1\n\nParagraph 2';
      const doc = parseMarkdown(markdown);

      // Should have at least 2 blocks
      expect(doc.childCount).toBeGreaterThanOrEqual(2);
    });

    it('handles task list with nested indentation', () => {
      const markdown = '- [ ] Task 1\n  - [ ] Subtask\n  - [x] Done subtask\n- [x] Task 2';
      const doc = parseMarkdown(markdown);

      const blocks = Array.from(doc.content.content);
      expect(blocks[0].attrs.type).toBe('task');
      expect(blocks[0].attrs.indent).toBe(0);
      expect(blocks[1].attrs.type).toBe('task');
      expect(blocks[1].attrs.indent).toBe(1);
      expect(blocks[2].attrs.type).toBe('task');
      expect(blocks[2].attrs.indent).toBe(1);
      expect(blocks[3].attrs.type).toBe('task');
      expect(blocks[3].attrs.indent).toBe(0);
    });
  });
});
