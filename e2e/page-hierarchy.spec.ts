import { test, expect, registerUser } from './helpers';

test.describe('Page Hierarchy', () => {
  test.screenshot('create and navigate page hierarchy', async ({ page, request, takeScreenshot }) => {
    // 1. Register a new user (with retry logic for rate limiting)
    const { token } = await registerUser(request, 'hierarchy');

    // 2. Login via token in URL (simulating OAuth callback flow)
    await page.goto(`/?token=${token}`);

    // Wait for the app to load and authenticate - sidebar indicates logged in state
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // 3. Wait for auto-created welcome page to appear in sidebar
    // The first-login flow auto-creates a welcome page
    const welcomePageLink = page.locator('[data-testid^="sidebar-node-"]').first();
    await expect(welcomePageLink).toBeVisible({ timeout: 10000 });

    // Get the welcome page info
    const welcomeNodeId = await welcomePageLink.getAttribute('data-testid');
    expect(welcomeNodeId).toBeTruthy();
    const topLevelPageId = welcomeNodeId!.replace('sidebar-node-', '');

    // 4. Get the workspace ID from the URL
    await expect(page).toHaveURL(/\/w\/[^/]+/, { timeout: 5000 });
    const url = page.url();
    const wsMatch = url.match(/\/w\/([^+/]+)/);
    expect(wsMatch).toBeTruthy();
    const wsID = wsMatch![1];

    // 5. Create a child page via API
    const createChildResponse = await request.post(`/api/workspaces/${wsID}/nodes/${topLevelPageId}/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: {
        title: 'Child Page',
        content: 'This is a child page content',
      },
    });
    expect(createChildResponse.ok()).toBe(true);
    const childData = await createChildResponse.json();
    const childID = childData.id;

    // 6. Refresh to see the child page
    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // 7. Click on the parent to expand it and see children
    const parentNode = page.locator(`[data-testid="sidebar-node-${topLevelPageId}"]`);
    await expect(parentNode).toBeVisible({ timeout: 5000 });
    await parentNode.click();

    // Wait for children to load (lazy loading) - use sidebar-specific selector
    const childNodeInSidebar = page.locator(`[data-testid="sidebar-node-${childID}"]`);
    await expect(childNodeInSidebar).toBeVisible({ timeout: 5000 });

    // 8. Click on the child page to navigate to it
    await childNodeInSidebar.click();

    // 9. Verify the child page content is displayed
    await expect(page.getByText('This is a child page content')).toBeVisible({ timeout: 5000 });

    // 10. Verify URL contains workspace
    expect(page.url()).toContain('/w/');

    // 11. Create a grandchild via API
    const createGrandchildResponse = await request.post(`/api/workspaces/${wsID}/nodes/${childID}/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: {
        title: 'Grandchild Page',
        content: 'This is a grandchild page',
      },
    });
    expect(createGrandchildResponse.ok()).toBe(true);

    // 12. Reload and verify the hierarchy
    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Navigate to parent page by clicking on its pageItem (not the whole li, which may click child)
    const parentPageItem = page.locator(`[data-testid="sidebar-node-${topLevelPageId}"] > div`).first();
    await parentPageItem.click();
    const childNodeAfterReload = page.locator(`[data-testid="sidebar-node-${childID}"]`);
    await expect(childNodeAfterReload).toBeVisible({ timeout: 5000 });

    // Click the expand icon next to Child Page (not the text, which navigates)
    // Note: Child Page may already be expanded if we reloaded at its URL (ancestorIds includes it)
    const expandIcon = page.locator(`[data-testid="expand-icon-${childID}"]`);
    await expect(expandIcon).toBeVisible();
    // Verify the expand icon has a reasonable clickable size (at least 16x16)
    const box = await expandIcon.boundingBox();
    expect(box).toBeTruthy();
    expect(box!.width).toBeGreaterThanOrEqual(16);
    expect(box!.height).toBeGreaterThanOrEqual(16);

    // Check if Child Page is already expanded (transform indicates rotation)
    const transform = await expandIcon.evaluate((el) => getComputedStyle(el).transform);
    const isAlreadyExpanded = transform !== 'none' && transform !== 'matrix(1, 0, 0, 1, 0, 0)';
    if (!isAlreadyExpanded) {
      await expandIcon.click();
    }

    // Wait for grandchild to appear in sidebar
    const grandchildData = await createGrandchildResponse.json();
    const grandchildID = grandchildData.id;
    const grandchildNode = page.locator(`[data-testid="sidebar-node-${grandchildID}"]`);
    // Give more time for the async fetch to complete
    await expect(grandchildNode).toBeVisible({ timeout: 10000 });

    await takeScreenshot('hierarchy-expanded');

    // 13. Navigate to grandchild and verify
    await grandchildNode.click();
    await expect(page.getByText('This is a grandchild page')).toBeVisible({ timeout: 5000 });

    await takeScreenshot('grandchild-page');
  });

  test('delete grandchild page refreshes sidebar correctly', async ({ page, request }) => {
    // This test verifies the fix for sidebar not refreshing when deleting nested pages
    const { token } = await registerUser(request, 'delete-gc');

    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

    // Get workspace ID from URL
    await expect(page).toHaveURL(/\/w\/[^/]+/, { timeout: 5000 });
    const url = page.url();
    const wsMatch = url.match(/\/w\/([^+/]+)/);
    expect(wsMatch).toBeTruthy();
    const wsID = wsMatch![1];

    // Get the welcome page ID
    const welcomePageLink = page.locator('[data-testid^="sidebar-node-"]').first();
    await expect(welcomePageLink).toBeVisible({ timeout: 10000 });
    const welcomeNodeId = await welcomePageLink.getAttribute('data-testid');
    const rootPageId = welcomeNodeId!.replace('sidebar-node-', '');

    // Create a child page
    const childResponse = await request.post(`/api/workspaces/${wsID}/nodes/${rootPageId}/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { title: 'Child Page', content: 'Child content' },
    });
    expect(childResponse.ok()).toBe(true);
    const childData = await childResponse.json();
    const childID = childData.id;

    // Create a grandchild page
    const grandchildResponse = await request.post(`/api/workspaces/${wsID}/nodes/${childID}/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { title: 'Grandchild Page', content: 'Grandchild content' },
    });
    expect(grandchildResponse.ok()).toBe(true);
    const grandchildData = await grandchildResponse.json();
    const grandchildID = grandchildData.id;

    // Reload to see the hierarchy
    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Navigate to root page and expand to see the child
    const rootNode = page.locator(`[data-testid="sidebar-node-${rootPageId}"] > div`).first();
    await rootNode.click();
    const childNode = page.locator(`[data-testid="sidebar-node-${childID}"]`);
    await expect(childNode).toBeVisible({ timeout: 5000 });

    // Expand child to see grandchild
    const childExpandIcon = page.locator(`[data-testid="expand-icon-${childID}"]`);
    await expect(childExpandIcon).toBeVisible({ timeout: 5000 });
    const transform = await childExpandIcon.evaluate((el) => getComputedStyle(el).transform);
    const isAlreadyExpanded = transform !== 'none' && transform !== 'matrix(1, 0, 0, 1, 0, 0)';
    if (!isAlreadyExpanded) {
      await childExpandIcon.click();
    }

    // Verify grandchild is visible
    const grandchildNode = page.locator(`[data-testid="sidebar-node-${grandchildID}"]`);
    await expect(grandchildNode).toBeVisible({ timeout: 10000 });

    // Navigate to grandchild first
    await grandchildNode.click();
    await expect(page.getByText('Grandchild content')).toBeVisible({ timeout: 5000 });

    // Delete the grandchild using the delete button (use > div > button to avoid matching nested nodes)
    const grandchildPageItem = grandchildNode.locator('> div').first();
    await grandchildPageItem.hover();
    const deleteButton = grandchildPageItem.locator('button').filter({ hasText: '🗑' });
    page.once('dialog', (dialog) => dialog.accept());
    await deleteButton.click();

    // Verify the grandchild is removed from the sidebar
    await expect(grandchildNode).not.toBeVisible({ timeout: 10000 });

    // Verify navigation went to parent (child page)
    await expect(page.getByText('Child content')).toBeVisible({ timeout: 5000 });

    // Also verify the child still exists in sidebar
    await expect(childNode).toBeVisible({ timeout: 5000 });
  });

  test('delete child page with grandchildren refreshes sidebar correctly', async ({ page, request }) => {
    // Test deleting a child page that has its own children
    const { token } = await registerUser(request, 'delete-child');

    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

    // Get workspace ID
    await expect(page).toHaveURL(/\/w\/[^/]+/, { timeout: 5000 });
    const wsID = page.url().match(/\/w\/([^+/]+)/)![1];

    // Get root page
    const welcomePageLink = page.locator('[data-testid^="sidebar-node-"]').first();
    await expect(welcomePageLink).toBeVisible({ timeout: 10000 });
    const rootPageId = (await welcomePageLink.getAttribute('data-testid'))!.replace('sidebar-node-', '');

    // Create child page
    const childResponse = await request.post(`/api/workspaces/${wsID}/nodes/${rootPageId}/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { title: 'Child With Grandchildren', content: 'Child content' },
    });
    expect(childResponse.ok()).toBe(true);
    const childData = await childResponse.json();
    const childID = childData.id;

    // Create grandchild
    const grandchildResponse = await request.post(`/api/workspaces/${wsID}/nodes/${childID}/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { title: 'Nested Grandchild', content: 'Grandchild content' },
    });
    expect(grandchildResponse.ok()).toBe(true);
    const grandchildData = await grandchildResponse.json();
    const grandchildID = grandchildData.id;

    // Reload and expand hierarchy
    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    const rootNode = page.locator(`[data-testid="sidebar-node-${rootPageId}"] > div`).first();
    await rootNode.click();
    const childNode = page.locator(`[data-testid="sidebar-node-${childID}"]`);
    await expect(childNode).toBeVisible({ timeout: 5000 });

    // Expand child to see grandchild
    const childExpandIcon = page.locator(`[data-testid="expand-icon-${childID}"]`);
    await expect(childExpandIcon).toBeVisible({ timeout: 5000 });
    const transform = await childExpandIcon.evaluate((el) => getComputedStyle(el).transform);
    if (transform === 'none' || transform === 'matrix(1, 0, 0, 1, 0, 0)') {
      await childExpandIcon.click();
    }

    const grandchildNode = page.locator(`[data-testid="sidebar-node-${grandchildID}"]`);
    await expect(grandchildNode).toBeVisible({ timeout: 10000 });

    // Navigate to child page first (click on page item div, not the li wrapper)
    const childPageItem = childNode.locator('> div').first();
    await childPageItem.click();
    await expect(page.getByText('Child content')).toBeVisible({ timeout: 5000 });

    // Delete the child page (which should also remove grandchild from view)
    await childPageItem.hover();
    const deleteButton = childPageItem.locator('button').filter({ hasText: '🗑' });
    page.once('dialog', (dialog) => dialog.accept());
    await deleteButton.click();

    // Both child and grandchild should be removed from sidebar
    await expect(childNode).not.toBeVisible({ timeout: 10000 });
    await expect(grandchildNode).not.toBeVisible({ timeout: 5000 });

    // Navigation should go to parent (root page) since child has no siblings
    // Wait for the URL to change to indicate navigation completed (URL format: /w/{wsId}/{nodeId}+{title})
    await expect(page).toHaveURL(new RegExp(`/${rootPageId}\\+`), { timeout: 5000 });
  });

  test('delete page navigates to sibling when no parent', async ({ page, request }) => {
    const { token } = await registerUser(request, 'delete-sib');

    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

    // Get workspace ID
    await expect(page).toHaveURL(/\/w\/[^/]+/, { timeout: 5000 });
    const wsID = page.url().match(/\/w\/([^+/]+)/)![1];

    // Create two sibling pages at root level
    const page1Response = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { title: 'Page One', content: 'Content one' },
    });
    expect(page1Response.ok()).toBe(true);
    const page1ID = (await page1Response.json()).id;

    const page2Response = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { title: 'Page Two', content: 'Content two' },
    });
    expect(page2Response.ok()).toBe(true);
    const page2ID = (await page2Response.json()).id;

    // Reload and navigate to Page Two (second sibling)
    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    const page2Node = page.locator(`[data-testid="sidebar-node-${page2ID}"]`);
    await page2Node.click();
    await expect(page.getByText('Content two')).toBeVisible({ timeout: 5000 });

    // Delete Page Two - should navigate to Page One (previous sibling)
    const page2Item = page2Node.locator('> div').first();
    await page2Item.hover();
    const deleteButton = page2Item.locator('button').filter({ hasText: '🗑' });
    page.once('dialog', (dialog) => dialog.accept());
    await deleteButton.click();

    // Page Two should be removed
    await expect(page2Node).not.toBeVisible({ timeout: 10000 });

    // Should navigate to Page One (previous sibling)
    await expect(page.getByText('Content one')).toBeVisible({ timeout: 5000 });
    const page1NodeItem = page.locator(`[data-testid="sidebar-node-${page1ID}"] > div`).first();
    await expect(page1NodeItem).toHaveClass(/active/, { timeout: 5000 });
  });

  test('navigate between sibling pages', async ({ page, request }) => {
    // Register and login (with retry logic for rate limiting)
    const { token } = await registerUser(request, 'sibling');

    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Wait for workspace URL
    await expect(page).toHaveURL(/\/w\/[^/]+/, { timeout: 5000 });
    const url = page.url();
    const wsMatch = url.match(/\/w\/([^+/]+)/);
    expect(wsMatch).toBeTruthy();
    const wsID = wsMatch![1];

    // Create two sibling pages via API
    const page1Response = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: {
        title: 'First Page',
        content: 'Content of first page',
      },
    });
    expect(page1Response.ok()).toBe(true);
    const page1Data = await page1Response.json();
    const page1ID = page1Data.id;

    const page2Response = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: {
        title: 'Second Page',
        content: 'Content of second page',
      },
    });
    expect(page2Response.ok()).toBe(true);
    const page2Data = await page2Response.json();
    const page2ID = page2Data.id;

    // Reload to see both pages
    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Both pages should be visible at top level (use data-testid for precision)
    const firstPageNode = page.locator(`[data-testid="sidebar-node-${page1ID}"]`);
    const secondPageNode = page.locator(`[data-testid="sidebar-node-${page2ID}"]`);
    await expect(firstPageNode).toBeVisible({ timeout: 5000 });
    await expect(secondPageNode).toBeVisible({ timeout: 5000 });

    // Navigate to first page
    await firstPageNode.click();
    await expect(page.getByText('Content of first page')).toBeVisible({ timeout: 5000 });

    // Navigate to second page
    await secondPageNode.click();
    await expect(page.getByText('Content of second page')).toBeVisible({ timeout: 5000 });

    // Navigate back to first page
    await firstPageNode.click();
    await expect(page.getByText('Content of first page')).toBeVisible({ timeout: 5000 });
  });
});
