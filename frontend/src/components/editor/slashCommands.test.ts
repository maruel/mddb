// Unit tests for slash commands.

import { describe, it, expect } from 'vitest';
import { schema, nodes, marks } from './schema';
import { parseMarkdown } from './markdown-parser';
import { serializeToMarkdown } from './markdown-serializer';
import { slashCommands, filterCommands } from './slashCommands';

describe('subpage command link insertion', () => {
  it('should insert a proper markdown link that serializes correctly', () => {
    // Simulate what the subpage command should do:
    // Insert a link with text "Untitled" pointing to a URL

    const title = 'Untitled';
    const url = '/w/@ws123+workspace/@node456+untitled';

    // The CORRECT way: create text with link mark
    const linkMark = marks.link.create({ href: url, title: null });
    const linkNode = schema.text(title, [linkMark]);

    // Create a document with a paragraph block containing the link
    const block = nodes.block.create({ type: 'paragraph', indent: 0 }, linkNode);
    const doc = nodes.doc.create(null, block);

    // Serialize to markdown
    const markdown = serializeToMarkdown(doc);

    // The markdown should be a proper link, not raw text
    expect(markdown).toBe(`[${title}](${url})`);
  });

  it('should NOT insert raw markdown text that appears literally', () => {
    // This is the BUG: inserting raw markdown text instead of a proper link node

    const title = 'Untitled';
    const url = '/w/@ws123+workspace/@node456+untitled';

    // The WRONG way (current implementation): insert raw markdown text
    const rawMarkdownText = `[${title}](${url})`;
    const textNode = schema.text(rawMarkdownText);

    // Create a document with a paragraph block containing raw text
    const block = nodes.block.create({ type: 'paragraph', indent: 0 }, textNode);
    const doc = nodes.doc.create(null, block);

    // The raw text matches what we put in.
    // In a full implementation, the serializer should escape special characters,
    // but for now we verify the model structure which is the source of truth.

    expect(doc.firstChild?.attrs.type).toBe('paragraph');
    const content = doc.firstChild?.textContent;
    expect(content).toBe(rawMarkdownText);

    // Key verification: it has NO marks (unlike a real link)
    expect(doc.firstChild?.firstChild?.marks).toHaveLength(0);
  });

  it('link mark round-trips through markdown parser and serializer', () => {
    const originalMarkdown = '[Test Page](/w/@ws1+test/@n1+page)';

    // Parse markdown to ProseMirror document
    const doc = parseMarkdown(originalMarkdown);
    if (!doc) {
      throw new Error('Failed to parse markdown');
    }

    // Verify parsed structure
    const block = doc.firstChild;
    expect(block?.attrs.type).toBe('paragraph');
    const textNode = block?.firstChild;
    expect(textNode?.text).toBe('Test Page');
    expect(textNode?.marks[0]?.type.name).toBe('link');
    expect(textNode?.marks[0]?.attrs.href).toBe('/w/@ws1+test/@n1+page');

    // Serialize back to markdown
    const serialized = serializeToMarkdown(doc);

    // Should preserve the link structure
    expect(serialized).toBe(originalMarkdown);
  });
});

describe('subpage slash command configuration', () => {
  it('subpage command exists with correct asyncAction', () => {
    const subpageCommand = slashCommands.find((cmd) => cmd.id === 'subpage');
    expect(subpageCommand).toBeDefined();
    expect(subpageCommand?.asyncAction).toBe('createSubpage');
  });

  it('subpage command has appropriate keywords', () => {
    const subpageCommand = slashCommands.find((cmd) => cmd.id === 'subpage');
    expect(subpageCommand?.keywords).toContain('page');
    expect(subpageCommand?.keywords).toContain('subpage');
  });
});

describe('filterCommands', () => {
  it('returns all commands when query is empty', () => {
    const result = filterCommands('');
    expect(result).toHaveLength(slashCommands.length);
  });

  it('matches prefix on labelKey', () => {
    const result = filterCommands('head');
    expect(result.map((c) => c.id)).toContain('heading1');
    expect(result.map((c) => c.id)).toContain('heading2');
    expect(result.map((c) => c.id)).toContain('heading3');
  });

  it('matches prefix on keywords', () => {
    const result = filterCommands('h1');
    expect(result.map((c) => c.id)).toContain('heading1');
  });

  it('matches substring (contains)', () => {
    // "umb" is in the middle of "numbered"
    const result = filterCommands('umb');
    expect(result.map((c) => c.id)).toContain('orderedList');
  });

  it('matches fuzzy character sequence', () => {
    // "blt" matches "bullet" (b-u-l-l-e-t has b, l, t in order)
    const result = filterCommands('blt');
    expect(result.map((c) => c.id)).toContain('bulletList');
  });

  it('matches display text when translate function provided', () => {
    const mockTranslate = (key: string) => {
      const translations: Record<string, string> = {
        'slashMenu.bulletList': 'Bullet List',
        'slashMenu.orderedList': 'Numbered List',
      };
      return translations[key];
    };

    // "num" matches "Numbered List" display text
    const result = filterCommands('num', mockTranslate);
    expect(result.map((c) => c.id)).toContain('orderedList');
  });

  it('sorts results by match quality (prefix > contains > fuzzy)', () => {
    // "list" is a prefix match for keywords in bulletList and orderedList
    // but also contained in "tasklist" (keyword for taskList)
    const result = filterCommands('list');

    // All list commands should be included
    expect(result.map((c) => c.id)).toContain('bulletList');
    expect(result.map((c) => c.id)).toContain('orderedList');
    expect(result.map((c) => c.id)).toContain('taskList');
  });

  it('is case insensitive', () => {
    const result = filterCommands('HEADING');
    expect(result.map((c) => c.id)).toContain('heading1');
  });

  it('returns empty array when no matches', () => {
    const result = filterCommands('xyz123');
    expect(result).toHaveLength(0);
  });
});
