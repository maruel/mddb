// Unit tests for DOM parsing utilities.

import { describe, it, expect } from 'vitest';
import { transformPastedHTML, extractCodeLanguage } from './dom-parser';

describe('dom-parser', () => {
  describe('transformPastedHTML', () => {
    it('annotates bullet list items with data-type and data-indent', () => {
      const html = '<ul><li>Item 1</li><li>Item 2</li></ul>';
      const result = transformPastedHTML(html);

      expect(result).toContain('data-type="bullet"');
      expect(result).toContain('data-indent="0"');
    });

    it('annotates ordered list items with data-type="number"', () => {
      const html = '<ol><li>First</li><li>Second</li></ol>';
      const result = transformPastedHTML(html);

      expect(result).toContain('data-type="number"');
      expect(result).toContain('data-indent="0"');
    });

    it('tracks indentation levels for nested lists', () => {
      const html = `
        <ul>
          <li>Item 1
            <ul>
              <li>Nested 1</li>
              <li>Nested 2</li>
            </ul>
          </li>
        </ul>
      `;
      const result = transformPastedHTML(html);

      // Should have one top-level item and two nested items
      const lines = result.split('\n').filter((line) => line.includes('data-indent'));
      expect(lines.some((line) => line.includes('data-indent="0"'))).toBe(true);
      expect(lines.some((line) => line.includes('data-indent="1"'))).toBe(true);
    });

    it('handles mixed list types (ul and ol)', () => {
      const html = `
        <ul>
          <li>Bullet item
            <ol>
              <li>Ordered nested</li>
            </ol>
          </li>
        </ul>
      `;
      const result = transformPastedHTML(html);

      expect(result).toContain('data-type="bullet" data-indent="0"');
      expect(result).toContain('data-type="number" data-indent="1"');
    });

    it('recognizes task list items with checkbox syntax', () => {
      const html = '<ul><li>[ ] Unchecked task</li><li>[x] Checked task</li></ul>';
      const result = transformPastedHTML(html);

      expect(result).toContain('class="task-list-item"');
      expect(result).toContain('data-checked="false"');
      expect(result).toContain('data-checked="true"');
    });

    it('handles task lists with mixed content', () => {
      const html = `
        <ul>
          <li>[ ] Task 1</li>
          <li>Regular bullet</li>
          <li>[x] Task 2</li>
        </ul>
      `;
      const result = transformPastedHTML(html);

      // Count task list items
      const taskCount = (result.match(/class="task-list-item"/g) || []).length;
      expect(taskCount).toBe(2);

      // Verify data attributes
      expect(result).toContain('data-checked="false"');
      expect(result).toContain('data-checked="true"');
    });

    it('preserves non-list HTML content', () => {
      const html = '<p>Paragraph</p><ul><li>List item</li></ul>';
      const result = transformPastedHTML(html);

      expect(result).toContain('<p>Paragraph</p>');
      expect(result).toContain('data-type="bullet"');
    });

    it('handles deeply nested lists (3+ levels)', () => {
      const html = `
        <ul>
          <li>Level 1
            <ul>
              <li>Level 2
                <ul>
                  <li>Level 3</li>
                </ul>
              </li>
            </ul>
          </li>
        </ul>
      `;
      const result = transformPastedHTML(html);

      expect(result).toContain('data-indent="0"');
      expect(result).toContain('data-indent="1"');
      expect(result).toContain('data-indent="2"');
    });

    it('handles empty lists', () => {
      const html = '<ul></ul>';
      const result = transformPastedHTML(html);
      // Empty ul should just be preserved as-is (no li to annotate)
      expect(result).toContain('<ul></ul>');
    });

    it('handles whitespace-only list items', () => {
      const html = '<ul><li>  </li><li>Content</li></ul>';
      const result = transformPastedHTML(html);

      // Both items should be annotated
      const annotated = (result.match(/data-type="bullet"/g) || []).length;
      expect(annotated).toBe(2);
    });

    it('case-insensitive checkbox detection ([X] vs [x])', () => {
      const html = '<ul><li>[X] Task with capital X</li></ul>';
      const result = transformPastedHTML(html);

      expect(result).toContain('class="task-list-item"');
      expect(result).toContain('data-checked="true"');
    });

    it('handles checkbox with extra whitespace', () => {
      const html = '<ul><li>  [ ]  Task with space</li></ul>';
      const result = transformPastedHTML(html);

      expect(result).toContain('class="task-list-item"');
      expect(result).toContain('data-checked="false"');
    });
  });

  describe('extractCodeLanguage', () => {
    it('extracts language from language-* class', () => {
      const pre = document.createElement('pre');
      pre.className = 'language-typescript';

      const result = extractCodeLanguage(pre);
      expect(result).toBe('typescript');
    });

    it('extracts language from lang-* class', () => {
      const pre = document.createElement('pre');
      pre.className = 'lang-python';

      const result = extractCodeLanguage(pre);
      expect(result).toBe('python');
    });

    it('extracts language from hljs-* class', () => {
      const pre = document.createElement('pre');
      pre.className = 'hljs-javascript';

      const result = extractCodeLanguage(pre);
      expect(result).toBe('javascript');
    });

    it('extracts language from data-language attribute', () => {
      const pre = document.createElement('pre');
      pre.setAttribute('data-language', 'rust');

      const result = extractCodeLanguage(pre);
      expect(result).toBe('rust');
    });

    it('prefers language-* class over other patterns', () => {
      const pre = document.createElement('pre');
      pre.className = 'language-go lang-python';
      pre.setAttribute('data-language', 'rust');

      const result = extractCodeLanguage(pre);
      expect(result).toBe('go');
    });

    it('returns null when no language is found', () => {
      const pre = document.createElement('pre');

      const result = extractCodeLanguage(pre);
      expect(result).toBeNull();
    });

    it('extracts language from multiple classes', () => {
      const pre = document.createElement('pre');
      pre.className = 'hljs line-numbers language-bash';

      const result = extractCodeLanguage(pre);
      expect(result).toBe('bash');
    });

    it('ignores non-language classes', () => {
      const pre = document.createElement('pre');
      pre.className = 'code-block highlight';

      const result = extractCodeLanguage(pre);
      expect(result).toBeNull();
    });
  });
});
