import { test, expect, registerUser, createClient, getWorkspaceId } from './helpers';

test.describe('Page Hierarchy', () => {
  test.screenshot('create and navigate page hierarchy', async ({ page, request, takeScreenshot }) => {
    // 1. Register a new user (with retry logic for rate limiting)
    const { token } = await registerUser(request, 'hierarchy');
    const client = createClient(request, token);

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
    const topLevelPageId = welcomeNodeId!.replace('sidebar-node-', '') as string;

    // 4. Get the workspace ID from the URL
    const wsID = await getWorkspaceId(page);

    // 5. Create a child page via API
    const childData = await client.ws(wsID).nodes.page.createPage(topLevelPageId, {
      title: 'Child Page',
      content: 'This is a child page content',
    });
    const childID = childData.id as string;

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
    const grandchildData = await client.ws(wsID).nodes.page.createPage(childID, {
      title: 'Grandchild Page',
      content: 'This is a grandchild page',
    });
    const grandchildID = grandchildData.id as string;

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

    // Check if Child Page is already expanded (transform indicates rotation on the inner expandIcon)
    const expandIconInner = expandIcon.locator('[class*="expandIcon"]');
    const transform = await expandIconInner.evaluate((el) => getComputedStyle(el).transform);
    const isAlreadyExpanded = transform !== 'none' && transform !== 'matrix(1, 0, 0, 1, 0, 0)';
    if (!isAlreadyExpanded) {
      await expandIcon.click();
    }

    // Wait for grandchild to appear in sidebar
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
    const client = createClient(request, token);

    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

    const wsID = await getWorkspaceId(page);

    // Get the welcome page ID
    const welcomePageLink = page.locator('[data-testid^="sidebar-node-"]').first();
    await expect(welcomePageLink).toBeVisible({ timeout: 10000 });
    const welcomeNodeId = await welcomePageLink.getAttribute('data-testid');
    const rootPageId = welcomeNodeId!.replace('sidebar-node-', '') as string;

    // Create a child page
    const childData = await client.ws(wsID).nodes.page.createPage(rootPageId, {
      title: 'Child Page',
      content: 'Child content',
    });
    const childID = childData.id as string;

    // Create a grandchild page
    const grandchildData = await client.ws(wsID).nodes.page.createPage(childID, {
      title: 'Grandchild Page',
      content: 'Grandchild content',
    });
    const grandchildID = grandchildData.id as string;

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
    const deleteButton = grandchildPageItem.locator('[data-testid="delete-node-button"]');
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
    const client = createClient(request, token);

    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

    const wsID = await getWorkspaceId(page);

    // Get root page
    const welcomePageLink = page.locator('[data-testid^="sidebar-node-"]').first();
    await expect(welcomePageLink).toBeVisible({ timeout: 10000 });
    const rootPageId = (await welcomePageLink.getAttribute('data-testid'))!.replace('sidebar-node-', '') as string;

    // Create child page
    const childData = await client.ws(wsID).nodes.page.createPage(rootPageId, {
      title: 'Child With Grandchildren',
      content: 'Child content',
    });
    const childID = childData.id as string;

    // Create grandchild
    const grandchildData = await client.ws(wsID).nodes.page.createPage(childID, {
      title: 'Nested Grandchild',
      content: 'Grandchild content',
    });
    const grandchildID = grandchildData.id as string;

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
    const deleteButton = childPageItem.locator('[data-testid="delete-node-button"]');
    page.once('dialog', (dialog) => dialog.accept());
    await deleteButton.click();

    // Both child and grandchild should be removed from sidebar
    await expect(childNode).not.toBeVisible({ timeout: 10000 });
    await expect(grandchildNode).not.toBeVisible({ timeout: 5000 });

    // Navigation should go to parent (root page) since child has no siblings
    // Wait for the URL to change to indicate navigation completed (URL format: /w/@{wsId}/@{nodeId}+{title})
    await expect(page).toHaveURL(new RegExp(`/@${rootPageId}\\+`), { timeout: 5000 });
  });

  test('delete page navigates to sibling when no parent', async ({ page, request }) => {
    const { token } = await registerUser(request, 'delete-sib');
    const client = createClient(request, token);

    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

    const wsID = await getWorkspaceId(page);

    // Create two sibling pages at root level
    const page1Data = await client.ws(wsID).nodes.page.createPage('0', {
      title: 'Page One',
      content: 'Content one',
    });
    const page1ID = page1Data.id as string;

    const page2Data = await client.ws(wsID).nodes.page.createPage('0', {
      title: 'Page Two',
      content: 'Content two',
    });
    const page2ID = page2Data.id as string;

    // Reload and navigate to Page Two (second sibling)
    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    const page2Node = page.locator(`[data-testid="sidebar-node-${page2ID}"]`);
    await page2Node.click();
    await expect(page.getByText('Content two')).toBeVisible({ timeout: 5000 });

    // Delete Page Two - should navigate to Page One (previous sibling)
    const page2Item = page2Node.locator('> div').first();
    await page2Item.hover();
    const deleteButton = page2Item.locator('[data-testid="delete-node-button"]');
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
    const client = createClient(request, token);

    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    const wsID = await getWorkspaceId(page);

    // Create two sibling pages via API
    const page1Data = await client.ws(wsID).nodes.page.createPage('0', {
      title: 'First Page',
      content: 'Content of first page',
    });
    const page1ID = page1Data.id as string;

    const page2Data = await client.ws(wsID).nodes.page.createPage('0', {
      title: 'Second Page',
      content: 'Content of second page',
    });
    const page2ID = page2Data.id as string;

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
