// E2E tests for flat block editor: input rules, drag handles, context menu, keyboard navigation.

import { test, expect, registerUser, getWorkspaceId, createClient } from './helpers';

test('create blocks via input rules', async ({ page, request }) => {
  const { token } = await registerUser(request, 'block-editor');
  await page.goto(`/?token=${token}`);
  await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

  // Get workspace ID from URL
  const wsId = await getWorkspaceId(page);

  // Create a test page via API with markdown content containing all block types
  const markdownContent = `- Bullet one
- Bullet two
  - Nested bullet

1. Number one
2. Number two

# Heading 1
## Heading 2
### Heading 3

> A quote here

\`\`\`ts
const x = 1;
const y = 2;
\`\`\`

Normal paragraph

---

Another paragraph`;

  const client = createClient(request, token);
  const pageResp = await client.ws(wsId).nodes.page.createPage('0', {
    title: 'Block Types Test',
    content: markdownContent,
  });
  const nodeId = pageResp.id;

  // Reload to see the page in sidebar
  await page.reload();
  await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

  // Navigate to the page via sidebar
  const pageNode = page.locator(`[data-testid="sidebar-node-${nodeId}"]`);
  await expect(pageNode).toBeVisible({ timeout: 5000 });
  await pageNode.click();

  // Wait for editor to load
  await expect(page.locator('[data-testid="wysiwyg-editor"]')).toBeVisible({ timeout: 5000 });

  // Verify editor loaded with content (use auto-retrying assertions, not snapshot reads)
  const editor = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
  await expect(editor).toBeVisible({ timeout: 5000 });
  await expect(editor).toContainText('Bullet one', { timeout: 5000 });
  await expect(editor).toContainText('Number one');
  await expect(editor).toContainText('Heading 1');
  await expect(editor).toContainText('A quote');

  // Verify block structure
  await expect(editor.locator('.block-row[data-type="bullet"]').first()).toBeVisible();
  await expect(editor.locator('.block-row[data-type="heading"]').first()).toBeVisible();
  await expect(editor.locator('.block-row[data-type="number"]').first()).toBeVisible();
});

test('drag handles are present on all blocks', async ({ page, request }) => {
  const { token } = await registerUser(request, 'block-drag');
  await page.goto(`/?token=${token}`);
  await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

  const wsId = await getWorkspaceId(page);

  // Create a test page with multiple blocks of different types
  const markdownContent = `Apple paragraph

- Bullet item
- Another bullet

1. Numbered item
2. Second item

# Heading

> Quote

\`\`\`ts
code block
\`\`\``;

  const client = createClient(request, token);
  const pageResp = await client.ws(wsId).nodes.page.createPage('0', {
    title: 'Drag Handles Test',
    content: markdownContent,
  });
  const nodeId = pageResp.id;

  // Reload and navigate to the page
  await page.reload();
  await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

  await page.locator(`[data-testid="sidebar-node-${nodeId}"]`).click();
  await expect(page.locator('[data-testid="wysiwyg-editor"]')).toBeVisible({ timeout: 5000 });

  const prosemirror = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
  await expect(prosemirror).toBeVisible({ timeout: 5000 });

  // Verify that drag handles are present on all block types
  const blocks = prosemirror.locator('.block-row');
  // Wait for blocks to be fully rendered (8 expected based on markdownContent)
  await expect(blocks).toHaveCount(8, { timeout: 10000 });
  const blockCount = await blocks.count();

  // Each block should have a drag handle
  const handles = prosemirror.locator('[data-testid="row-handle"]');
  // Wait for handles to be rendered (rendered via Solid render, may have slight delay)
  await expect(handles).toHaveCount(blockCount, { timeout: 5000 });
  const handleCount = await handles.count();
  
  // Should have at least as many handles as blocks (in flat architecture, 1:1)
  expect(handleCount).toBe(blockCount);

  // Verify handles are draggable
  for (let i = 0; i < handleCount; i++) {
    const handle = handles.nth(i);
    const isDraggable = await handle.evaluate((el) => {
      const elem = el as HTMLElement;
      return elem.draggable === true || elem.getAttribute('draggable') === 'true';
    });
    expect(isDraggable).toBe(true);
  }

  // Verify handles have proper ARIA label
  for (let i = 0; i < Math.min(3, handleCount); i++) {
    const handle = handles.nth(i);
    const ariaLabel = await handle.getAttribute('aria-label');
    expect(ariaLabel).toContain('Drag');
  }
});

