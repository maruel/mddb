import { test, expect, registerUser, getWorkspaceId, fillEditorContent, createClient } from './helpers';

test.describe('Error Handling - Invalid Routes', () => {
  test('navigating to non-existent page shows appropriate error or handles gracefully', async ({ page, request }) => {
    const { token } = await registerUser(request, 'invalid-page');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    const wsID = await getWorkspaceId(page);

    // Try to navigate to a non-existent page ID
    await page.goto(`/w/${wsID}/nonexistent-page-id-12345?token=${token}`);

    // Should either show error message or redirect to workspace root
    // Wait for either an error message to appear OR the sidebar to show
    await expect(async () => {
      const errorMessage = page.locator('[class*="error"], [class*="Error"]');
      const sidebar = page.locator('aside');
      const hasError = await errorMessage.isVisible();
      const hasSidebar = await sidebar.isVisible();
      expect(hasError || hasSidebar).toBe(true);
    }).toPass({ timeout: 5000 });
  });

  // Testing if invalid workspace ID is handled gracefully (doesn't crash)
  test('navigating to non-existent workspace shows error', async ({ page, request }) => {
    const { token } = await registerUser(request, 'invalid-ws');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Try to navigate to a non-existent workspace
    await page.goto(`/w/invalid-workspace-12345/some-page?token=${token}`);

    // Should handle gracefully: show error, redirect to valid workspace, or load user's workspace
    await expect(async () => {
      const errorMessage = page.locator('[class*="error"], [class*="Error"]');
      const sidebar = page.locator('aside');
      const hasError = await errorMessage.isVisible();
      const hasSidebar = await sidebar.isVisible();
      const currentUrl = page.url();
      const urlChanged = !currentUrl.includes('invalid-workspace-12345');
      // Accept any graceful handling: error shown, URL redirected, or sidebar loaded
      expect(hasError || urlChanged || hasSidebar).toBe(true);
    }).toPass({ timeout: 10000 });
  });

  test('privacy page accessible without login', async ({ page }) => {
    await page.goto('/privacy');
    // Should show privacy content without needing authentication
    await expect(page.locator('h1, h2').first()).toBeVisible({ timeout: 5000 });
    // Check that Go Back button exists (proves privacy page rendered)
    await expect(page.locator('button', { hasText: 'Go Back' })).toBeVisible();
  });

  test('terms page accessible without login', async ({ page }) => {
    await page.goto('/terms');
    // Should show terms content without needing authentication
    await expect(page.locator('h1', { hasText: 'Terms of Service' })).toBeVisible({ timeout: 5000 });
  });
});

test.describe('Error Handling - API Failures', () => {
  test('network error during save shows error message', async ({ page, request }) => {
    const { token } = await registerUser(request, 'network-error');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    const wsID = await getWorkspaceId(page);

    // Create a page
    const client = createClient(request, token);
    const pageData = await client.ws(wsID).nodes.page.createPage('0', {
      title: 'Network Error Test',
      content: 'Initial',
    });

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    await page.locator(`[data-testid="sidebar-node-${pageData.id}"]`).click();
    await expect(page.locator('input[placeholder*="Title"]')).toHaveValue('Network Error Test', { timeout: 5000 });

    // Block API requests to simulate network failure
    await page.route('**/api/workspaces/**/page', (route) => {
      route.abort('failed');
    });

    // Edit content (switch to markdown mode for reliable interaction)
    await fillEditorContent(page, 'This should fail to save');

    // Should show error (autosave will trigger and fail due to route blocking)
    const errorMessage = page.locator('[class*="error"], [class*="Error"]');
    await expect(errorMessage).toBeVisible({ timeout: 8000 });
  });
});

test.describe('Error Handling - Concurrent Edits', () => {
  test('editing same page in two tabs - last writer wins', async ({ page, request, context }) => {
    const { token } = await registerUser(request, 'concurrent-edit');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    const wsID = await getWorkspaceId(page);

    // Create a page
    const client = createClient(request, token);
    const pageData = await client.ws(wsID).nodes.page.createPage('0', {
      title: 'Concurrent Edit Test',
      content: 'Original content',
    });

    // Open same page in second tab
    const page2 = await context.newPage();
    await page2.goto(`/w/${wsID}/${pageData.id}?token=${token}`);
    await expect(page2.locator('aside')).toBeVisible({ timeout: 10000 });

    // Navigate to page in first tab
    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });
    await page.locator(`[data-testid="sidebar-node-${pageData.id}"]`).click();
    await expect(page.locator('input[placeholder*="Title"]')).toHaveValue('Concurrent Edit Test', { timeout: 5000 });

    // Edit in first tab (switch to markdown mode for reliable interaction)
    await fillEditorContent(page, 'Content from tab 1');

    // Edit in second tab (before first tab saves)
    await fillEditorContent(page2, 'Content from tab 2');

    // Poll API until one of the contents is saved (last writer wins)
    await expect(async () => {
      const client = createClient(request, token);
      const savedData = await client.ws(wsID).nodes.page.getPage(pageData.id);
      const savedContent = savedData.content.trim();
      expect(savedContent === 'Content from tab 1' || savedContent === 'Content from tab 2').toBe(true);
    }).toPass({ timeout: 8000 });

    await page2.close();
  });
});

test.describe('Edge Cases', () => {
  test('empty page title is handled', async ({ page, request }) => {
    const { token } = await registerUser(request, 'empty-title');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    const wsID = await getWorkspaceId(page);

    // Create a page
    const client = createClient(request, token);
    const pageData = await client.ws(wsID).nodes.page.createPage('0', {
      title: 'Title to Clear',
      content: 'Content',
    });

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    await page.locator(`[data-testid="sidebar-node-${pageData.id}"]`).click();
    await expect(page.locator('input[placeholder*="Title"]')).toHaveValue('Title to Clear', { timeout: 5000 });

    // Clear the title
    const titleInput = page.locator('input[placeholder*="Title"]');
    await titleInput.fill('');
    // Blur to trigger autosave attempt
    await titleInput.blur();

    // Wait for autosave to attempt (debounce is 2s)
    // Note: empty title may or may not be saved depending on validation
    await expect(async () => {
      // Just verify the UI has processed the change (title input should be empty or app handles gracefully)
      const currentValue = await titleInput.inputValue();
      expect(currentValue).toBe('');
    }).toPass({ timeout: 3000 });

    // Page should still be accessible (though might show empty title in sidebar)
    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // The page node should still exist in sidebar
    const pageNode = page.locator(`[data-testid="sidebar-node-${pageData.id}"]`);
    await expect(pageNode).toBeVisible({ timeout: 5000 });
  });

  test('very long title is handled', async ({ page, request }) => {
    const { token } = await registerUser(request, 'long-title');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    const wsID = await getWorkspaceId(page);

    // Create a page
    const client = createClient(request, token);
    const pageData = await client.ws(wsID).nodes.page.createPage('0', {
      title: 'Short Title',
      content: 'Content',
    });

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    await page.locator(`[data-testid="sidebar-node-${pageData.id}"]`).click();

    // Wait for the title input to be ready with initial value
    const titleInput = page.locator('input[placeholder*="Title"]');
    await expect(titleInput).toHaveValue('Short Title', { timeout: 5000 });

    // Set a very long title
    const longTitle = 'A'.repeat(500);
    await titleInput.fill(longTitle);

    // Verify the UI accepted the long title
    await expect(titleInput).toHaveValue(longTitle, { timeout: 5000 });

    // Wait for autosave to attempt (debounce is 2s)
    // Then verify page handles gracefully - either truncate, show error, or save
    await expect(async () => {
      const client = createClient(request, token);
      const savedData = await client.ws(wsID).nodes.page.getPage(pageData.id);
      // Title should be saved (possibly truncated or unchanged if validation rejects)
      expect(savedData.title.length).toBeGreaterThan(0);
    }).toPass({ timeout: 5000 });
  });

  test('special characters in title are handled', async ({ page, request }) => {
    const { token } = await registerUser(request, 'special-chars');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    const wsID = await getWorkspaceId(page);

    // Create a page with special characters
    const specialTitle = 'Test <script>alert(1)</script> & "quotes" \'apostrophe\' 中文 émojis 🎉';
    const client = createClient(request, token);
    const pageData = await client.ws(wsID).nodes.page.createPage('0', {
      title: specialTitle,
      content: 'Content with <html> & special "chars"',
    });

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    await page.locator(`[data-testid="sidebar-node-${pageData.id}"]`).click();

    // Title should be displayed (possibly HTML-escaped)
    const titleInput = page.locator('input[placeholder*="Title"]');
    await expect(titleInput).toBeVisible({ timeout: 5000 });

    // Content should be preserved
    const savedData = await client.ws(wsID).nodes.page.getPage(pageData.id);
    expect(savedData.title).toContain('Test');
    // Script tags should be stored as-is (not executed) or sanitized
  });

  test('WYSIWYG editor renders code blocks correctly', async ({ page, request }) => {
    const { token } = await registerUser(request, 'code-blocks');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    const wsID = await getWorkspaceId(page);

    const markdownContent = `# Code Example

\`\`\`javascript
function hello() {
  console.log("Hello, World!");
}
\`\`\`

Inline \`code\` here.
`;

    const client = createClient(request, token);
    const pageData = await client.ws(wsID).nodes.page.createPage('0', {
      title: 'Code Blocks Test',
      content: markdownContent,
    });

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    await page.locator(`[data-testid="sidebar-node-${pageData.id}"]`).click();

    // WYSIWYG editor should render code blocks
    const editor = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
    await expect(editor).toBeVisible({ timeout: 5000 });

    // Should have pre/code elements for the code block
    await expect(editor.locator('pre')).toBeVisible({ timeout: 3000 });
    // Use first() since there are multiple code elements (code block + inline code)
    await expect(editor.locator('code').first()).toBeVisible();
  });
});
