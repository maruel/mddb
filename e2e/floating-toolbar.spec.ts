// E2E tests for floating toolbar visibility lifecycle.

import { test, expect, registerUser, getWorkspaceId, createClient } from './helpers';

test.describe('Floating Toolbar Visibility', () => {
  test('toolbar lifecycle: hidden -> visible on selection -> hidden on blur', async ({ page, request }) => {
    const { token } = await registerUser(request, 'float-toolbar-lifecycle');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

    const wsID = await getWorkspaceId(page);

    // Create a page with simple content
    const client = createClient(request, token);
    const pageData = await client.ws(wsID).nodes.page.createPage('0', {
      title: 'Toolbar Visibility Test',
      content: 'Hello world',
    });

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Navigate to the page
    await page.locator(`[data-testid="sidebar-node-${pageData.id}"]`).click();

    // Wait for WYSIWYG editor to load
    const editor = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
    await expect(editor).toBeVisible({ timeout: 5000 });

    // 1. Initial State: Toolbar should be HIDDEN
    const toolbar = page.locator('[data-testid="floating-toolbar"]');
    await expect(toolbar).not.toBeVisible();

    // Mode toggle should be VISIBLE always (bottom-right indicator)
    const modeToggle = page.locator('[data-testid="editor-mode-visual"]');
    await expect(modeToggle).toBeVisible();

    // 2. Type text: Toolbar should REMAIN HIDDEN
    await editor.locator('p').first().click();
    await page.keyboard.type(' more text');
    await expect(toolbar).not.toBeVisible();

    // 3. Select text: Toolbar should become VISIBLE
    await editor.locator('p').first().selectText();
    await expect(toolbar).toBeVisible({ timeout: 3000 });

    // 4. Click away (clear selection): Toolbar should become HIDDEN
    await editor.locator('p').first().click();
    await expect(toolbar).not.toBeVisible({ timeout: 3000 });

    // Mode toggle should STILL be visible
    await expect(modeToggle).toBeVisible();
  });

  test('toolbar appears on double-click word selection', async ({ page, request }) => {
    const { token } = await registerUser(request, 'float-toolbar-dblclick');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

    const wsID = await getWorkspaceId(page);

    // Create a page with multiple words
    const client = createClient(request, token);
    const pageData = await client.ws(wsID).nodes.page.createPage('0', {
      title: 'Double Click Test',
      content: 'Hello world testing',
    });

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    await page.locator(`[data-testid="sidebar-node-${pageData.id}"]`).click();

    const editor = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
    await expect(editor).toBeVisible({ timeout: 5000 });

    // Wait for the actual page content to load (not just editor visibility)
    await expect(editor.locator('p')).toContainText('Hello', { timeout: 5000 });

    const toolbar = page.locator('[data-testid="floating-toolbar"]');
    await expect(toolbar).not.toBeVisible();

    // Double-click on text to select a word
    // Note: We use mouse.dblclick with coordinates near the text start because
    // locator.dblclick() clicks the center of the element which may be empty space
    const paragraph = editor.locator('p').first();
    const box = await paragraph.boundingBox();
    expect(box).toBeTruthy();

    // Click near the start of the paragraph (where the text begins)
    const x = box!.x + 30;
    const y = box!.y + box!.height / 2;
    await page.mouse.dblclick(x, y);

    // Toolbar should appear when text is selected
    await expect(toolbar).toBeVisible({ timeout: 3000 });

    // Verify text is selected
    const selection = await page.evaluate(() => window.getSelection()?.toString());
    expect(selection?.length).toBeGreaterThan(0);
  });
});
