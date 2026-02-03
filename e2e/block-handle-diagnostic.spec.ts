// Diagnostic tests for block handle visibility and text alignment issues.

import { test, expect, registerUser, getWorkspaceId, createClient } from './helpers';

test('diagnose: block handle visibility on hover', async ({ page, request }) => {
  const { token } = await registerUser(request, 'block-handle-visual');
  await page.goto(`/?token=${token}`);
  await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

  const wsId = await getWorkspaceId(page);

  // Create a test page with multiple block types
  const markdownContent = `Simple paragraph

- Bullet item one
- Bullet item two

1. Numbered item one
2. Numbered item two

# Heading Level 1

> A quote

\`\`\`ts
code block
\`\`\``;

  const client = createClient(request, token);
  const pageResp = await client.ws(wsId).nodes.page.createPage('0', {
    title: 'Handle Visibility Test',
    content: markdownContent,
  });
  const nodeId = pageResp.id;

  await page.reload();
  await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

  await page.locator(`[data-testid="sidebar-node-${nodeId}"]`).click();
  await expect(page.locator('[data-testid="wysiwyg-editor"]')).toBeVisible({ timeout: 5000 });

  const editor = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
  await expect(editor).toBeVisible({ timeout: 5000 });

  // Get all blocks
  const blocks = editor.locator('.block-row');
  const blockCount = await blocks.count();
  expect(blockCount).toBeGreaterThan(0);

  // For each block, hover and check handle visibility
  for (let i = 0; i < Math.min(3, blockCount); i++) {
    const block = blocks.nth(i);

    // Hover over the block
    await block.hover();

    // Check if handle element exists
    const handle = block.locator('[data-testid="row-handle"]');
    expect(await handle.count()).toBe(1);

    // Check computed styles
    const handleStyles = await handle.evaluate((el) => {
      const computed = window.getComputedStyle(el);
      return {
        opacity: computed.opacity,
        visibility: computed.visibility,
        display: computed.display,
      };
    });

    // Check if handle is actually visible (opacity > 0, visibility != hidden, display != none)
    const isVisible = handleStyles.opacity !== '0' && handleStyles.visibility !== 'hidden' && handleStyles.display !== 'none';
    expect(isVisible).toBe(true);
  }

  // Take a screenshot showing the editor with a hovered block
  const firstBlock = blocks.first();
  await firstBlock.hover();

  // Wait for the CSS opacity transition to complete (opacity should reach 1)
  const firstBlockHandle = firstBlock.locator('.block-handle-container');
  await expect(async () => {
    const opacity = await firstBlockHandle.evaluate((el) => window.getComputedStyle(el).opacity);
    expect(Number(opacity)).toBeGreaterThan(0.9);
  }).toPass({ timeout: 1000 });

  await page.screenshot({ path: '/tmp/block-handle-hover.png' });
});

test('diagnose: text alignment in lists (bullets, numbers, tasks)', async ({ page, request }) => {
  const { token } = await registerUser(request, 'text-alignment-diagnostic');
  await page.goto(`/?token=${token}`);
  await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

  const wsId = await getWorkspaceId(page);

  // Create a page with various list types
  const markdownContent = `- Bullet item 1
- Bullet item 2
  - Nested bullet item
- Bullet item 3

1. Numbered item 1
2. Numbered item 2
   1. Nested numbered
3. Numbered item 3

- [ ] Task item 1
- [x] Task item 2 (checked)
- [ ] Task item 3`;

  const client = createClient(request, token);
  const pageResp = await client.ws(wsId).nodes.page.createPage('0', {
    title: 'Text Alignment Test',
    content: markdownContent,
  });
  const nodeId = pageResp.id;

  await page.reload();
  await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

  await page.locator(`[data-testid="sidebar-node-${nodeId}"]`).click();
  await expect(page.locator('[data-testid="wysiwyg-editor"]')).toBeVisible({ timeout: 5000 });

  const editor = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
  await expect(editor).toBeVisible({ timeout: 5000 });

  // Check bullet items - wait for them to load
  const bulletBlocks = editor.locator('.block-row[data-type="bullet"]');
  await expect(bulletBlocks.first()).toBeVisible({ timeout: 5000 });
  const bulletCount = await bulletBlocks.count();
  expect(bulletCount).toBeGreaterThanOrEqual(2);

  for (let i = 0; i < Math.min(2, bulletCount); i++) {
    const block = bulletBlocks.nth(i);
    const styles = await block.evaluate((el) => {
      const computed = window.getComputedStyle(el);
      return {
        display: computed.display,
        alignItems: computed.alignItems,
      };
    });
    expect(styles.display).toBe('flex');
    expect(styles.alignItems).toBe('flex-start');
  }

  // Check numbered items - wait for them to load
  const numberBlocks = editor.locator('.block-row[data-type="number"]');
  await expect(numberBlocks.first()).toBeVisible({ timeout: 5000 });
  const numberCount = await numberBlocks.count();
  expect(numberCount).toBeGreaterThanOrEqual(2);

  for (let i = 0; i < Math.min(2, numberCount); i++) {
    const block = numberBlocks.nth(i);
    const numberDiv = block.locator('.block-number');
    const numberAttr = await numberDiv.getAttribute('data-number');
    expect(numberAttr).not.toBeNull();
  }

  // Check task items - wait for them to load
  const taskBlocks = editor.locator('.block-row[data-type="task"]');
  await expect(taskBlocks.first()).toBeVisible({ timeout: 5000 });
  const taskCount = await taskBlocks.count();
  expect(taskCount).toBeGreaterThanOrEqual(2);

  for (let i = 0; i < Math.min(2, taskCount); i++) {
    const block = taskBlocks.nth(i);
    const checkedAttr = await block.getAttribute('data-checked');
    expect(checkedAttr).not.toBeNull();
  }

  // Take a screenshot showing text alignment
  await page.screenshot({ path: '/tmp/text-alignment.png' });
});
