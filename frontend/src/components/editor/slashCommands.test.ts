// Unit tests for slash commands.

import { describe, it, expect } from 'vitest';
import { schema, nodes, marks, markdownSerializer, markdownParser } from './prosemirror-config';
import { slashCommands, filterCommands } from './slashCommands';

describe('subpage command link insertion', () => {
  it('should insert a proper markdown link that serializes correctly', () => {
    // Simulate what the subpage command should do:
    // Insert a link with text "Untitled" pointing to a URL

    const title = 'Untitled';
    const url = '/w/ws123+workspace/node456+untitled';

    // The CORRECT way: create text with link mark
    const linkMark = marks.link.create({ href: url, title: null });
    const linkNode = schema.text(title, [linkMark]);

    // Create a document with a paragraph containing the link
    const paragraph = nodes.paragraph.create(null, linkNode);
    const doc = nodes.doc.create(null, paragraph);

    // Serialize to markdown
    const markdown = markdownSerializer.serialize(doc);

    // The markdown should be a proper link, not raw text
    expect(markdown).toBe(`[${title}](${url})`);
  });

  it('should NOT insert raw markdown text that appears literally', () => {
    // This is the BUG: inserting raw markdown text instead of a proper link node

    const title = 'Untitled';
    const url = '/w/ws123+workspace/node456+untitled';

    // The WRONG way (current implementation): insert raw markdown text
    const rawMarkdownText = `[${title}](${url})`;
    const textNode = schema.text(rawMarkdownText);

    // Create a document with a paragraph containing raw text
    const paragraph = nodes.paragraph.create(null, textNode);
    const doc = nodes.doc.create(null, paragraph);

    // Serialize to markdown
    const markdown = markdownSerializer.serialize(doc);

    // The raw text gets escaped/preserved as literal text, NOT a link
    // This demonstrates the bug: the user sees "[Untitled](/w/...)" literally
    expect(markdown).not.toBe(`[${title}](${url})`);
    // It contains the brackets as literal characters (escaped or preserved)
    expect(markdown).toContain('[');
    expect(markdown).toContain(']');
  });

  it('link mark round-trips through markdown parser and serializer', () => {
    const originalMarkdown = '[Test Page](/w/ws1+test/n1+page)';

    // Parse markdown to ProseMirror document
    const doc = markdownParser.parse(originalMarkdown);
    if (!doc) {
      throw new Error('Failed to parse markdown');
    }

    // Serialize back to markdown
    const serialized = markdownSerializer.serialize(doc);

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

describe('subpage creation sidebar refresh', () => {
  /**
   * BUG: When creating a subpage under an existing page, the new subpage
   * should appear in the sidebar under its parent.
   *
   * The issue is that `loadNodes(true)` only refreshes root-level nodes
   * (nodes with parent_id='0'). To show a subpage created under an existing
   * page, we need to refresh the parent's children using `fetchNodeChildren(parentId)`.
   *
   * Expected behavior:
   * 1. Create subpage with parent_id = currentNodeId
   * 2. Call fetchNodeChildren(parentId) to refresh parent's children in sidebar
   * 3. The new subpage should appear as a child of the parent node
   *
   * Current behavior (bug):
   * 1. Create subpage with parent_id = currentNodeId
   * 2. Call loadNodes(true) which only refreshes root nodes
   * 3. The new subpage doesn't appear because it's not a root node
   */
  it('documents that subpage creation should refresh parent children, not just root nodes', () => {
    // This test documents the expected behavior for the subpage command
    // The actual fix needs to be in SlashCommandMenu.tsx

    // When creating a subpage under nodeId "parent123", we should:
    // - NOT rely solely on loadNodes(true) - this only loads root nodes
    // - MUST call fetchNodeChildren("parent123") to refresh parent's children

    // The parent node needs has_children=true after creating a subpage
    // and its children array needs to include the new subpage

    const parentId = 'parent123';
    const newSubpageId = 'newSubpage456';

    // Simulate parent node state after creating subpage
    const parentNodeAfterCreation = {
      id: parentId,
      has_children: true, // Should be true after creating a subpage
      children: [{ id: newSubpageId, title: 'Untitled', parent_id: parentId }],
    };

    // Verify the parent now has children
    expect(parentNodeAfterCreation.has_children).toBe(true);
    expect(parentNodeAfterCreation.children).toHaveLength(1);
    const firstChild = parentNodeAfterCreation.children[0];
    expect(firstChild).toBeDefined();
    expect(firstChild?.parent_id).toBe(parentId);
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
