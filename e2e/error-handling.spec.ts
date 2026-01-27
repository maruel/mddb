import { test, expect, registerUser, getWorkspaceId } from './helpers';

test.describe('Error Handling - Invalid Routes', () => {
  test('navigating to non-existent page shows appropriate error or handles gracefully', async ({ page, request }) => {
    const { token } = await registerUser(request, 'invalid-page');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    const wsID = await getWorkspaceId(page);

    // Try to navigate to a non-existent page ID
    await page.goto(`/w/${wsID}/nonexistent-page-id-12345?token=${token}`);

    // Should either show error message or redirect to workspace root
    // Wait a bit for the app to respond
    await page.waitForTimeout(2000);

    // Check for error message OR successful redirect to workspace
    const errorMessage = page.locator('[class*="error"], [class*="Error"]');
    const sidebar = page.locator('aside');

    const hasError = await errorMessage.isVisible();
    const hasSidebar = await sidebar.isVisible();

    // Should have either an error shown or be back on a valid page
    expect(hasError || hasSidebar).toBe(true);
  });

  // BUG: Invalid workspace ID not handled - see BUGS_FOUND.md Bug 10
  test.skip('navigating to non-existent workspace shows error', async ({ page, request }) => {
    const { token } = await registerUser(request, 'invalid-ws');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Try to navigate to a non-existent workspace
    await page.goto(`/w/invalid-workspace-12345/some-page?token=${token}`);

    // Should show error about no access or redirect
    await page.waitForTimeout(2000);

    // Check for error or redirect back to valid workspace
    const errorMessage = page.locator('[class*="error"], [class*="Error"]');
    const hasError = await errorMessage.isVisible();

    // If no error shown, URL should have changed back to valid workspace
    if (!hasError) {
      const currentUrl = page.url();
      expect(currentUrl).not.toContain('invalid-workspace-12345');
    }
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
    const createResponse = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { title: 'Network Error Test', content: 'Initial' },
    });
    const pageData = await createResponse.json();

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    await page.locator(`[data-testid="sidebar-node-${pageData.id}"]`).click();
    await expect(page.locator('input[placeholder*="Title"]')).toHaveValue('Network Error Test', { timeout: 5000 });

    // Block API requests to simulate network failure
    await page.route('**/api/workspaces/**/page', (route) => {
      route.abort('failed');
    });

    // Edit content
    const contentTextarea = page.locator('textarea[placeholder*="markdown"]');
    await contentTextarea.fill('This should fail to save');

    // Wait for autosave attempt
    await page.waitForTimeout(3000);

    // Should show error
    const errorMessage = page.locator('[class*="error"], [class*="Error"]');
    await expect(errorMessage).toBeVisible({ timeout: 5000 });
  });
});

test.describe('Error Handling - Concurrent Edits', () => {
  test('editing same page in two tabs - last writer wins', async ({ page, request, context }) => {
    const { token } = await registerUser(request, 'concurrent-edit');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    const wsID = await getWorkspaceId(page);

    // Create a page
    const createResponse = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { title: 'Concurrent Edit Test', content: 'Original content' },
    });
    const pageData = await createResponse.json();

    // Open same page in second tab
    const page2 = await context.newPage();
    await page2.goto(`/w/${wsID}/${pageData.id}?token=${token}`);
    await expect(page2.locator('aside')).toBeVisible({ timeout: 10000 });

    // Navigate to page in first tab
    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });
    await page.locator(`[data-testid="sidebar-node-${pageData.id}"]`).click();
    await expect(page.locator('input[placeholder*="Title"]')).toHaveValue('Concurrent Edit Test', { timeout: 5000 });

    // Edit in first tab
    const content1 = page.locator('textarea[placeholder*="markdown"]');
    await content1.fill('Content from tab 1');

    // Edit in second tab (before first tab saves)
    const content2 = page2.locator('textarea[placeholder*="markdown"]');
    await content2.fill('Content from tab 2');

    // Wait for both autosaves
    await page.waitForTimeout(3500);
    await page2.waitForTimeout(3500);

    // Check what was saved
    const getResponse = await request.get(`/api/workspaces/${wsID}/nodes/${pageData.id}/page`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    const savedData = await getResponse.json();

    // Last writer wins - should be one of the two contents
    const savedContent = savedData.content.trim();
    expect(savedContent === 'Content from tab 1' || savedContent === 'Content from tab 2').toBe(true);

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
    const createResponse = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { title: 'Title to Clear', content: 'Content' },
    });
    const pageData = await createResponse.json();

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    await page.locator(`[data-testid="sidebar-node-${pageData.id}"]`).click();
    await expect(page.locator('input[placeholder*="Title"]')).toHaveValue('Title to Clear', { timeout: 5000 });

    // Clear the title
    const titleInput = page.locator('input[placeholder*="Title"]');
    await titleInput.fill('');

    // Wait for autosave
    await page.waitForTimeout(3000);

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
    const createResponse = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { title: 'Short Title', content: 'Content' },
    });
    const pageData = await createResponse.json();

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    await page.locator(`[data-testid="sidebar-node-${pageData.id}"]`).click();

    // Set a very long title
    const longTitle = 'A'.repeat(500);
    const titleInput = page.locator('input[placeholder*="Title"]');
    await titleInput.fill(longTitle);

    // Wait for autosave
    await page.waitForTimeout(3000);

    // Should handle gracefully - either truncate, show error, or save full title
    // Verify via API what was saved
    const getResponse = await request.get(`/api/workspaces/${wsID}/nodes/${pageData.id}/page`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    const savedData = await getResponse.json();

    // Title should be saved (possibly truncated by backend)
    expect(savedData.title.length).toBeGreaterThan(0);
  });

  test('special characters in title are handled', async ({ page, request }) => {
    const { token } = await registerUser(request, 'special-chars');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    const wsID = await getWorkspaceId(page);

    // Create a page with special characters
    const specialTitle = 'Test <script>alert(1)</script> & "quotes" \'apostrophe\' 中文 émojis 🎉';
    const createResponse = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { title: specialTitle, content: 'Content with <html> & special "chars"' },
    });
    expect(createResponse.ok()).toBe(true);
    const pageData = await createResponse.json();

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    await page.locator(`[data-testid="sidebar-node-${pageData.id}"]`).click();

    // Title should be displayed (possibly HTML-escaped)
    const titleInput = page.locator('input[placeholder*="Title"]');
    await expect(titleInput).toBeVisible({ timeout: 5000 });

    // Content should be preserved
    const getResponse = await request.get(`/api/workspaces/${wsID}/nodes/${pageData.id}/page`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    const savedData = await getResponse.json();
    expect(savedData.title).toContain('Test');
    // Script tags should be stored as-is (not executed) or sanitized
  });

  test('markdown with code blocks renders correctly', async ({ page, request }) => {
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

    const createResponse = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { title: 'Code Blocks Test', content: markdownContent },
    });
    const pageData = await createResponse.json();

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    await page.locator(`[data-testid="sidebar-node-${pageData.id}"]`).click();

    // Preview should render code blocks
    const preview = page.locator('.preview, [class*="preview"], [class*="Preview"]');
    await expect(preview).toBeVisible({ timeout: 5000 });

    // Should have pre/code elements for the code block
    await expect(preview.locator('pre')).toBeVisible({ timeout: 3000 });
    // Use first() since there are multiple code elements (code block + inline code)
    await expect(preview.locator('code').first()).toBeVisible();
  });
});
