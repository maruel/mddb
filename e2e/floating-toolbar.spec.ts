// E2E tests for floating toolbar visibility lifecycle.

import { test, expect, registerUser, getWorkspaceId } from './helpers';

test.describe('Floating Toolbar Visibility', () => {
  test('toolbar lifecycle: hidden -> visible on selection -> hidden on blur', async ({ page, request }) => {
    const { token } = await registerUser(request, 'float-toolbar-lifecycle');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

    const wsID = await getWorkspaceId(page);

    // Create a page with simple content
    const createResponse = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: {
        title: 'Toolbar Visibility Test',
        content: 'Hello world',
      },
    });
    expect(createResponse.ok()).toBe(true);
    const pageData = await createResponse.json();

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
});
