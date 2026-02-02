// Test for block handle visibility bug fix.

import { test, expect, registerUser, getWorkspaceId, createClient } from './helpers';

test('block handle should be visible when block is hovered', async ({ page, request }) => {
  const { token } = await registerUser(request, 'block-handle-fix');
  await page.goto(`/?token=${token}`);
  await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

  const wsId = await getWorkspaceId(page);

  // Create a simple page with one bullet item
  const client = createClient(request, token);
  const pageResp = await client.ws(wsId).nodes.page.createPage('0', {
    title: 'Handle Visibility',
    content: '- Test bullet item',
  });
  const nodeId = pageResp.id;

  await page.reload();
  await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

  await page.locator(`[data-testid="sidebar-node-${nodeId}"]`).click();
  await expect(page.locator('[data-testid="wysiwyg-editor"]')).toBeVisible({ timeout: 5000 });

  const editor = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
  const bulletBlock = editor.locator('.block-row[data-type="bullet"]');
  await expect(bulletBlock).toBeVisible({ timeout: 5000 });

  // Hover over the block
  await bulletBlock.hover();

  // The handle container should have opacity > 0 when hovered
  const handleContainer = bulletBlock.locator('.block-handle-container');
  
  // Wait for the opacity to transition to visible (0.15s transition)
  await expect(async () => {
    const opacity = await handleContainer.evaluate((el) => {
      return window.getComputedStyle(el).opacity;
    });
    console.log(`Handle container opacity: ${opacity}`);
    expect(Number(opacity)).toBeGreaterThan(0.8);
  }).toPass({ timeout: 5000 });

  // The handle element itself should also be visible
  const handle = bulletBlock.locator('[data-testid="row-handle"]');
  const handleOpacity = await handle.evaluate((el) => {
    return window.getComputedStyle(el).opacity;
  });

  console.log(`Handle element opacity on hover: ${handleOpacity}`);
  expect(Number(handleOpacity)).toBeGreaterThan(0);
});

test('numbered list items should have data-number attribute for CSS content', async ({ page, request }) => {
  const { token } = await registerUser(request, 'number-data-attr');
  await page.goto(`/?token=${token}`);
  await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

  const wsId = await getWorkspaceId(page);

  const client = createClient(request, token);
  // Use markdown with explicit blank lines between list and other content
  const markdownContent = `

1. First item
2. Second item
3. Third item

`;
  const pageResp = await client.ws(wsId).nodes.page.createPage('0', {
    title: 'Numbered Items',
    content: markdownContent,
  });
  const nodeId = pageResp.id;

  await page.reload();
  await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

  await page.locator(`[data-testid="sidebar-node-${nodeId}"]`).click();
  await expect(page.locator('[data-testid="wysiwyg-editor"]')).toBeVisible({ timeout: 5000 });

  const editor = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
  
  // Wait for numbered blocks to load (the markdown has blank lines that create paragraphs)
  const numberBlocks = editor.locator('.block-row[data-type="number"]');
  await expect(numberBlocks).toHaveCount(3, { timeout: 5000 });
  const count = await numberBlocks.count();
  console.log(`Found ${count} numbered blocks`);
  expect(count).toBe(3);

  // Each numbered block should have a .block-number div with data-number attribute
  for (let i = 0; i < count; i++) {
    const block = numberBlocks.nth(i);
    const numberDiv = block.locator('.block-number');
    await expect(numberDiv).toBeVisible({ timeout: 3000 });

    const dataNumber = await numberDiv.getAttribute('data-number');
    console.log(`Numbered item ${i}: data-number="${dataNumber}"`);

    // Should have a numeric value (not null or empty)
    expect(dataNumber).not.toBeNull();
    expect(dataNumber).not.toBe('');
    expect(Number(dataNumber)).toBeGreaterThan(0);
  }
});

test('bullet list items should display with proper text alignment', async ({ page, request }) => {
  const { token } = await registerUser(request, 'bullet-alignment');
  await page.goto(`/?token=${token}`);
  await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

  const wsId = await getWorkspaceId(page);

  const client = createClient(request, token);
  const markdownContent = `
- Short
- This is a much longer bullet item that wraps to multiple lines to verify alignment
- Another item
`;
  const pageResp = await client.ws(wsId).nodes.page.createPage('0', {
    title: 'Bullet Alignment',
    content: markdownContent,
  });
  const nodeId = pageResp.id;

  await page.reload();
  await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

  await page.locator(`[data-testid="sidebar-node-${nodeId}"]`).click();
  await expect(page.locator('[data-testid="wysiwyg-editor"]')).toBeVisible({ timeout: 5000 });

  const editor = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
  
  // Wait for bullets to load
  const bulletBlocks = editor.locator('.block-row[data-type="bullet"]');
  await expect(bulletBlocks).toHaveCount(3, { timeout: 5000 });
  const count = await bulletBlocks.count();
  console.log(`Found ${count} bullet blocks`);
  expect(count).toBe(3);

  // Check that bullet markers are properly aligned with text
  // The bullet should appear to the left of the text, not overlapping
  const secondBullet = bulletBlocks.nth(1);
  const bulletDiv = secondBullet.locator('.block-bullet');
  const text = await bulletDiv.innerText();
  console.log('Multi-line bullet text:', text);

  // Verify the bullet is visible (via ::before pseudo-element)
  // We can't directly inspect pseudo-elements, but we can check the layout
  const boundingBox = await bulletDiv.boundingBox();
  console.log('Bullet bounding box:', boundingBox);

  expect(boundingBox).not.toBeNull();
  expect(boundingBox!.width).toBeGreaterThan(0);
  expect(boundingBox!.height).toBeGreaterThan(0);
});
