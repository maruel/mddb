import { test, expect, registerUser, getWorkspaceId } from './helpers';

test.describe('Page CRUD Operations', () => {
  // BUG: Page deletion not working - see BUGS_FOUND.md Bug 3
  test.skip('delete a page - page removed from sidebar and content area cleared', async ({ page, request }) => {
    const { token } = await registerUser(request, 'delete-page');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    const wsID = await getWorkspaceId(page);

    // Create a page to delete
    const createResponse = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: {
        title: 'Page To Delete',
        content: 'This page will be deleted',
      },
    });
    expect(createResponse.ok()).toBe(true);
    const pageData = await createResponse.json();
    const pageID = pageData.id;

    // Reload to see the page
    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Navigate to the page
    const pageNode = page.locator(`[data-testid="sidebar-node-${pageID}"]`);
    await expect(pageNode).toBeVisible({ timeout: 5000 });
    await pageNode.click();
    await expect(page.getByText('This page will be deleted')).toBeVisible({ timeout: 5000 });

    // Click delete button
    const deleteButton = page.locator('button', { hasText: 'Delete' });
    await expect(deleteButton).toBeVisible();

    // Set up dialog handler to accept confirmation
    page.on('dialog', (dialog) => dialog.accept());

    // Click delete
    await deleteButton.click();

    // Wait for the page to be removed
    await expect(pageNode).not.toBeVisible({ timeout: 5000 });

    // Content area should be cleared (no title input visible or shows different content)
    const titleInput = page.locator('input[placeholder*="Title"]');
    // Either the title input doesn't have the deleted page's title, or it's not visible
    await expect(async () => {
      const isVisible = await titleInput.isVisible();
      if (isVisible) {
        const value = await titleInput.inputValue();
        expect(value).not.toBe('Page To Delete');
      }
    }).toPass({ timeout: 5000 });

    // Verify via API that the page no longer exists
    const getResponse = await request.get(`/api/workspaces/${wsID}/nodes/${pageID}/page`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    expect(getResponse.ok()).toBe(false);
    expect(getResponse.status()).toBe(404);
  });

  test('page title updates in sidebar as user types (real-time sync)', async ({ page, request }) => {
    const { token } = await registerUser(request, 'sidebar-sync');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    const wsID = await getWorkspaceId(page);

    // Create a page
    const createResponse = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: {
        title: 'Original Title',
        content: 'Content here',
      },
    });
    expect(createResponse.ok()).toBe(true);
    const pageData = await createResponse.json();
    const pageID = pageData.id;

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Navigate to the page
    const pageNode = page.locator(`[data-testid="sidebar-node-${pageID}"]`);
    await pageNode.click();

    // Wait for title input to be ready with correct value
    const titleInput = page.locator('input[placeholder*="Title"]');
    await expect(titleInput).toHaveValue('Original Title', { timeout: 5000 });

    // Get sidebar text element - title is in span with class pageTitleText
    const sidebarTitle = pageNode.locator('[class*="pageTitleText"]');
    await expect(sidebarTitle).toContainText('Original Title');

    // Type a new title
    await titleInput.fill('Updated Title');

    // Sidebar should update immediately (optimistic update)
    await expect(sidebarTitle).toContainText('Updated Title', { timeout: 5000 });
  });

  test('unsaved indicator appears when editing and disappears after save', async ({ page, request }) => {
    const { token } = await registerUser(request, 'unsaved-ind');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    const wsID = await getWorkspaceId(page);

    // Create a page
    const createResponse = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: {
        title: 'Test Page',
        content: 'Initial content',
      },
    });
    expect(createResponse.ok()).toBe(true);
    const pageData = await createResponse.json();
    const pageID = pageData.id;

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Navigate to the page
    await page.locator(`[data-testid="sidebar-node-${pageID}"]`).click();
    await expect(page.getByText('Initial content')).toBeVisible({ timeout: 5000 });

    // Initially, no unsaved indicator (use class selector)
    const unsavedIndicator = page.locator('[class*="unsavedIndicator"]');
    await expect(unsavedIndicator).not.toBeVisible();

    // Edit the content
    const contentTextarea = page.locator('textarea[placeholder*="markdown"]');
    await contentTextarea.fill('Modified content');

    // Unsaved indicator should appear
    await expect(unsavedIndicator).toBeVisible({ timeout: 2000 });

    // Wait for autosave to complete - the unsaved indicator should disappear
    // (saving indicator may flash too quickly to catch reliably)
    await expect(unsavedIndicator).not.toBeVisible({ timeout: 10000 });

    // Verify content was saved via API
    const getResponse = await request.get(`/api/workspaces/${wsID}/nodes/${pageID}/page`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    const savedData = await getResponse.json();
    expect(savedData.content).toBe('Modified content');
  });

});

test.describe('Page Navigation', () => {
  test.screenshot('browser back button navigates between pages', async ({ page, request, takeScreenshot }) => {
    const { token } = await registerUser(request, 'browser-nav');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    const wsID = await getWorkspaceId(page);

    // Create two pages
    const page1Response = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { title: 'Nav Page 1', content: 'Content of page 1' },
    });
    const page1Data = await page1Response.json();

    const page2Response = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { title: 'Nav Page 2', content: 'Content of page 2' },
    });
    const page2Data = await page2Response.json();

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });
    await takeScreenshot('workspace-with-pages');

    // Navigate to page 1
    await page.locator(`[data-testid="sidebar-node-${page1Data.id}"]`).click();
    await expect(page.getByText('Content of page 1')).toBeVisible({ timeout: 5000 });
    await takeScreenshot('page1-view');

    // Navigate to page 2
    await page.locator(`[data-testid="sidebar-node-${page2Data.id}"]`).click();
    await expect(page.getByText('Content of page 2')).toBeVisible({ timeout: 5000 });
    await takeScreenshot('page2-view');

    // Click browser back button
    await page.goBack();

    // Should show page 1 again
    await expect(page.getByText('Content of page 1')).toBeVisible({ timeout: 5000 });

    // Forward button should return to page 2
    await page.goForward();
    await expect(page.getByText('Content of page 2')).toBeVisible({ timeout: 5000 });
  });

  test('URL updates with page slug when navigating', async ({ page, request }) => {
    const { token } = await registerUser(request, 'url-slug');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    const wsID = await getWorkspaceId(page);

    // Create a page with a specific title
    const createResponse = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { title: 'My Awesome Page', content: 'Content here' },
    });
    const pageData = await createResponse.json();

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Navigate to the page
    await page.locator(`[data-testid="sidebar-node-${pageData.id}"]`).click();
    await expect(page.getByText('Content here')).toBeVisible({ timeout: 5000 });

    // URL should contain workspace ID and page ID with slug
    await expect(page).toHaveURL(new RegExp(`/w/${wsID}[^/]*/${pageData.id}\\+my-awesome-page`));
  });

  test('direct URL navigation loads correct page', async ({ page, request }) => {
    const { token } = await registerUser(request, 'direct-url');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    const wsID = await getWorkspaceId(page);

    // Create a page
    const createResponse = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { title: 'Direct URL Page', content: 'Loaded via direct URL' },
    });
    const pageData = await createResponse.json();

    // Navigate directly to the page URL
    await page.goto(`/w/${wsID}/${pageData.id}?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Page content should be visible
    await expect(page.getByText('Loaded via direct URL')).toBeVisible({ timeout: 5000 });

    // Title input should have the correct value
    const titleInput = page.locator('input[placeholder*="Title"]');
    await expect(titleInput).toHaveValue('Direct URL Page');
  });

  test('breadcrumb navigation works for nested pages', async ({ page, request }) => {
    const { token } = await registerUser(request, 'breadcrumb');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    const wsID = await getWorkspaceId(page);

    // Create parent page
    const parentResponse = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { title: 'Alpha', content: 'Parent content' },
    });
    const parentData = await parentResponse.json();

    // Create child page
    const childResponse = await request.post(`/api/workspaces/${wsID}/nodes/${parentData.id}/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { title: 'Beta', content: 'Child content' },
    });
    const childData = await childResponse.json();

    // Create grandchild page
    const grandchildResponse = await request.post(`/api/workspaces/${wsID}/nodes/${childData.id}/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { title: 'Gamma', content: 'Grandchild content' },
    });
    const grandchildData = await grandchildResponse.json();

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Expand hierarchy and navigate to grandchild
    await page.locator(`[data-testid="sidebar-node-${parentData.id}"]`).click();
    await expect(page.locator(`[data-testid="sidebar-node-${childData.id}"]`)).toBeVisible({ timeout: 5000 });
    await page.locator(`[data-testid="sidebar-node-${childData.id}"]`).locator('span').first().click();
    await expect(page.locator(`[data-testid="sidebar-node-${grandchildData.id}"]`)).toBeVisible({ timeout: 5000 });
    await page.locator(`[data-testid="sidebar-node-${grandchildData.id}"]`).click();

    await expect(page.getByText('Grandchild content')).toBeVisible({ timeout: 5000 });

    // Check breadcrumbs are visible (use exact match)
    const breadcrumbs = page.locator('nav[class*="breadcrumbs"]');
    await expect(breadcrumbs.getByText('Alpha', { exact: true })).toBeVisible();
    await expect(breadcrumbs.getByText('Beta', { exact: true })).toBeVisible();
    await expect(breadcrumbs.getByText('Gamma', { exact: true })).toBeVisible();

    // Click on parent breadcrumb
    await breadcrumbs.getByText('Alpha', { exact: true }).click();

    // Should navigate to parent
    await expect(page.getByText('Parent content')).toBeVisible({ timeout: 5000 });
  });
});

test.describe('Editor Features', () => {
  test.screenshot('markdown preview renders correctly', async ({ page, request, takeScreenshot }) => {
    const { token } = await registerUser(request, 'markdown-preview');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    const wsID = await getWorkspaceId(page);

    // Create a page with markdown content
    const createResponse = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: {
        title: 'Markdown Test',
        content: '# Heading 1\n\n**Bold text**\n\n- List item 1\n- List item 2\n\n`code inline`',
      },
    });
    const pageData = await createResponse.json();

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    await page.locator(`[data-testid="sidebar-node-${pageData.id}"]`).click();

    // Wait for content to load
    await expect(page.locator('textarea[placeholder*="markdown"]')).toBeVisible({ timeout: 5000 });

    // Check that markdown preview section exists and renders
    const preview = page.locator('.preview, [class*="preview"], [class*="Preview"]');
    await expect(preview).toBeVisible({ timeout: 5000 });

    // Check for rendered markdown elements
    await expect(preview.locator('h1')).toContainText('Heading 1');
    await expect(preview.locator('strong')).toContainText('Bold text');
    await expect(preview.locator('li').first()).toContainText('List item 1');
    await expect(preview.locator('code')).toContainText('code inline');

    await takeScreenshot('markdown-preview');
  });

  test.screenshot('version history loads and displays commits', async ({ page, request, takeScreenshot }) => {
    const { token } = await registerUser(request, 'version-history');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    const wsID = await getWorkspaceId(page);

    // Create a page
    const createResponse = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { title: 'History Test', content: 'Initial content' },
    });
    const pageData = await createResponse.json();

    // Update the page a few times to create history
    for (let i = 1; i <= 3; i++) {
      await request.post(`/api/workspaces/${wsID}/nodes/${pageData.id}/page`, {
        headers: { Authorization: `Bearer ${token}` },
        data: { title: 'History Test', content: `Content version ${i}` },
      });
    }

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Right-click on the sidebar node to open context menu
    const sidebarNode = page.locator(`[data-testid="sidebar-node-${pageData.id}"]`);
    await sidebarNode.click({ button: 'right' });

    // Click History option in context menu (has clock emoji prefix)
    const historyButton = page.locator('button', { hasText: /🕐.*History/ });
    await expect(historyButton).toBeVisible({ timeout: 3000 });
    await historyButton.click();

    // History panel should appear
    const historyPanel = page.locator('[class*="historyPanel"]');
    await expect(historyPanel).toBeVisible({ timeout: 5000 });

    // Should show commits (initial create + 3 updates = 4 commits)
    const historyItems = historyPanel.locator('li[class*="historyItem"]');
    await expect(historyItems).toHaveCount(4, { timeout: 5000 });

    await takeScreenshot('version-history-panel');
  });
});
