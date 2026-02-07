import { test, expect, registerUser, fillEditorContent, switchToMarkdownMode, createClient, getWorkspaceId } from './helpers';

test.describe('Page rename with navigation', () => {
  test('rename page then quickly navigate away - rename must persist', async ({ page, request }) => {
    // 1. Register a new user (with retry logic for rate limiting)
    const { token } = await registerUser(request, 'rename-nav');
    const client = createClient(request, token);

    // 2. Login via token
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // 3. Get workspace ID from URL
    const wsID = await getWorkspaceId(page);

    // 4. Create two pages via API
    const page1Data = await client.ws(wsID).nodes.page.createPage('0', {
      title: 'Page To Rename',
      content: 'Content of page to rename',
    });
    const page1ID = page1Data.id as string;

    const page2Data = await client.ws(wsID).nodes.page.createPage('0', {
      title: 'Other Page',
      content: 'Content of other page',
    });
    const page2ID = page2Data.id as string;

    // 5. Reload to see both pages
    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // 6. Navigate to Page 1
    const page1Node = page.locator(`[data-testid="sidebar-node-${page1ID}"]`);
    await expect(page1Node).toBeVisible({ timeout: 5000 });
    await page1Node.click();

    // Wait for page content to load
    await expect(page.getByText('Content of page to rename')).toBeVisible({ timeout: 5000 });

    // 7. Rename the page by editing the title input
    const titleInput = page.locator('input[placeholder*="Title"]');
    await expect(titleInput).toBeVisible();
    await expect(titleInput).toHaveValue('Page To Rename');

    // Clear and type new title
    await titleInput.fill('RENAMED PAGE TITLE');

    // Verify the title input has the new value
    await expect(titleInput).toHaveValue('RENAMED PAGE TITLE');

    // 8. IMMEDIATELY navigate to Page 2 without waiting for autosave
    // The autosave has a 2-second debounce, but flush() should save immediately
    const page2Node = page.locator(`[data-testid="sidebar-node-${page2ID}"]`);
    await page2Node.click();

    // Wait for Page 2 to load
    await expect(page.getByText('Content of other page')).toBeVisible({ timeout: 5000 });
    await expect(titleInput).toHaveValue('Other Page');

    // 9. Navigate back to Page 1 to verify rename persisted
    await page1Node.click();
    await expect(page.getByText('Content of page to rename')).toBeVisible({ timeout: 5000 });

    // 10. Verify the title was saved - this is the critical assertion
    await expect(titleInput).toHaveValue('RENAMED PAGE TITLE');

    // 11. Verify via API that the title was persisted to storage
    const pageData = await client.ws(wsID).nodes.page.getPage(page1ID);
    expect(pageData.title).toBe('RENAMED PAGE TITLE');

    // 12. Reload page and verify rename still persists (survives page refresh)
    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });
    await page1Node.click();
    await expect(titleInput).toHaveValue('RENAMED PAGE TITLE');
  });

  test('rename page, wait for autosave, then navigate - rename must persist', async ({ page, request }) => {
    // Control test: verifies rename works when waiting for autosave
    const { token } = await registerUser(request, 'rename-wait');
    const client = createClient(request, token);

    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    const wsID = await getWorkspaceId(page);

    // Create a page
    const pageData = await client.ws(wsID).nodes.page.createPage('0', {
      title: 'Original Title',
      content: 'Some content',
    });
    const pageID = pageData.id as string;

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Navigate to the page
    const pageNode = page.locator(`[data-testid="sidebar-node-${pageID}"]`);
    await expect(pageNode).toBeVisible({ timeout: 5000 });
    await pageNode.click();
    await expect(page.getByText('Some content')).toBeVisible({ timeout: 5000 });

    // Rename the page
    const titleInput = page.locator('input[placeholder*="Title"]');
    await titleInput.fill('Updated Title');

    // Poll API until title is saved
    await expect(async () => {
      const savedData = await client.ws(wsID).nodes.page.getPage(pageID);
      expect(savedData.title).toBe('Updated Title');
    }).toPass({ timeout: 8000 });

    // Reload and verify the rename persisted
    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    await pageNode.click();
    await expect(titleInput).toHaveValue('Updated Title');

    // Verify via API
    const verifyData = await client.ws(wsID).nodes.page.getPage(pageID);
    expect(verifyData.title).toBe('Updated Title');
  });

  test('rapid renames with navigation - last rename must persist', async ({ page, request }) => {
    // Stress test: rapidly rename multiple times, then navigate
    const { token } = await registerUser(request, 'rename-rapid');
    const client = createClient(request, token);

    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    const wsID = await getWorkspaceId(page);

    // Create two pages
    const page1Data = await client.ws(wsID).nodes.page.createPage('0', {
      title: 'Rapid Rename Page',
      content: 'Content for rapid rename test',
    });
    const page1ID = page1Data.id as string;

    const page2Data = await client.ws(wsID).nodes.page.createPage('0', {
      title: 'Navigation Target',
      content: 'Navigation target content',
    });
    const page2ID = page2Data.id as string;

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Navigate to Page 1
    const page1Node = page.locator(`[data-testid="sidebar-node-${page1ID}"]`);
    await page1Node.click();
    await expect(page.getByText('Content for rapid rename test')).toBeVisible({ timeout: 5000 });

    const titleInput = page.locator('input[placeholder*="Title"]');

    // Rapidly change title multiple times
    await titleInput.fill('First Rename');
    await titleInput.fill('Second Rename');
    await titleInput.fill('Third Rename');
    await titleInput.fill('FINAL RENAME');

    // Immediately navigate away
    const page2Node = page.locator(`[data-testid="sidebar-node-${page2ID}"]`);
    await page2Node.click();
    await expect(page.getByText('Navigation target content')).toBeVisible({ timeout: 5000 });

    // Navigate back and verify the FINAL rename persisted
    await page1Node.click();
    await expect(titleInput).toHaveValue('FINAL RENAME');

    // Verify via API
    const pageData = await client.ws(wsID).nodes.page.getPage(page1ID);
    expect(pageData.title).toBe('FINAL RENAME');
  });

  test('rename content then navigate - content must persist', async ({ page, request }) => {
    // Test that content changes are also flushed on navigation
    const { token } = await registerUser(request, 'content-nav');
    const client = createClient(request, token);

    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    const wsID = await getWorkspaceId(page);

    // Create two pages
    const page1Data = await client.ws(wsID).nodes.page.createPage('0', {
      title: 'Content Edit Page',
      content: 'Original content',
    });
    const page1ID = page1Data.id as string;

    const page2Data = await client.ws(wsID).nodes.page.createPage('0', {
      title: 'Other Page',
      content: 'Other page content',
    });
    const page2ID = page2Data.id as string;

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Navigate to Page 1
    const page1Node = page.locator(`[data-testid="sidebar-node-${page1ID}"]`);
    await page1Node.click();
    await expect(page.getByText('Original content')).toBeVisible({ timeout: 5000 });

    // Edit content (switch to markdown mode for reliable interaction)
    await fillEditorContent(page, 'MODIFIED CONTENT - this should persist');

    // Immediately navigate away
    const page2Node = page.locator(`[data-testid="sidebar-node-${page2ID}"]`);
    await page2Node.click();
    // Wait for navigation by checking title input changes to "Other Page"
    const titleInput = page.locator('input[placeholder*="Title"]');
    await expect(titleInput).toHaveValue('Other Page', { timeout: 5000 });

    // Navigate back and verify content persisted
    await page1Node.click();
    // Wait for page 1's title to be visible to ensure navigation completed
    await expect(titleInput).toHaveValue('Content Edit Page', { timeout: 5000 });
    // Content may have leading/trailing whitespace from markdown format
    const markdownEditor = await switchToMarkdownMode(page);
    const contentValue = await markdownEditor.inputValue();
    expect(contentValue.trim()).toBe('MODIFIED CONTENT - this should persist');

    // Verify via API
    const pageData = await client.ws(wsID).nodes.page.getPage(page1ID);
    expect(pageData.content.trim()).toBe('MODIFIED CONTENT - this should persist');
  });

  test('rename and content change then navigate - both must persist', async ({ page, request }) => {
    // Test that both title and content changes are flushed together
    const { token } = await registerUser(request, 'both-nav');
    const client = createClient(request, token);

    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    const wsID = await getWorkspaceId(page);

    // Create two pages
    const page1Data = await client.ws(wsID).nodes.page.createPage('0', {
      title: 'Original Title',
      content: 'Original content',
    });
    const page1ID = page1Data.id as string;

    const page2Data = await client.ws(wsID).nodes.page.createPage('0', {
      title: 'Other Page',
      content: 'Other page content',
    });
    const page2ID = page2Data.id as string;

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Navigate to Page 1
    const page1Node = page.locator(`[data-testid="sidebar-node-${page1ID}"]`);
    await page1Node.click();
    await expect(page.getByText('Original content')).toBeVisible({ timeout: 5000 });

    // Edit both title and content
    const titleInput = page.locator('input[placeholder*="Title"]');

    await titleInput.fill('NEW TITLE');
    // Switch to markdown mode for reliable content editing
    await fillEditorContent(page, 'NEW CONTENT');

    // Immediately navigate away
    const page2Node = page.locator(`[data-testid="sidebar-node-${page2ID}"]`);
    await page2Node.click();
    // Wait for navigation by checking title input changes to "Other Page"
    await expect(titleInput).toHaveValue('Other Page', { timeout: 5000 });

    // Navigate back and verify both persisted
    await page1Node.click();
    await expect(titleInput).toHaveValue('NEW TITLE');
    // Content may have leading/trailing whitespace from markdown format
    const markdownEditor = await switchToMarkdownMode(page);
    const contentValue = await markdownEditor.inputValue();
    expect(contentValue.trim()).toBe('NEW CONTENT');

    // Verify via API
    const pageData = await client.ws(wsID).nodes.page.getPage(page1ID);
    expect(pageData.title).toBe('NEW TITLE');
    expect(pageData.content.trim()).toBe('NEW CONTENT');
  });
});
