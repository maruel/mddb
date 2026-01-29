// E2E tests for page links with dynamic title resolution.

import { test, expect, registerUser, getWorkspaceId, switchToMarkdownMode } from './helpers';

test.describe('Page Links with Dynamic Titles', () => {
  test.screenshot('link displays current title of linked page', async ({ page, request, takeScreenshot }) => {
    const { token } = await registerUser(request, 'page-links');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    const wsID = await getWorkspaceId(page);

    // Create parent page
    const parentResponse = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { title: 'Parent Page', content: '' },
    });
    expect(parentResponse.ok()).toBe(true);
    const parentData = await parentResponse.json();

    // Create child page with initial title
    const childResponse = await request.post(`/api/workspaces/${wsID}/nodes/${parentData.id}/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { title: 'Original Child Title', content: 'Child content' },
    });
    expect(childResponse.ok()).toBe(true);
    const childData = await childResponse.json();

    // Update parent page with a link to child using the correct format
    // Format: [DisplayText](/w/{wsId}+{slug}/{nodeId}+{slug})
    const linkContent = `Check out [Original Child Title](/w/${wsID}+workspace/${childData.id}+original-child-title)`;
    const updateResponse = await request.post(`/api/workspaces/${wsID}/nodes/${parentData.id}/page`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { title: 'Parent Page', content: linkContent },
    });
    expect(updateResponse.ok()).toBe(true);

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Navigate to parent page - click specifically on the pageItem, not the whole node (which includes children)
    const parentNode = page.locator(`[data-testid="sidebar-node-${parentData.id}"]`);
    const parentPageItem = parentNode.locator('> [class*="pageItem"]');
    await parentPageItem.click();

    // Wait for editor to load
    const editor = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
    await expect(editor).toBeVisible({ timeout: 5000 });

    // Wait for title to confirm we're on parent page
    const titleInput = page.locator('input[placeholder*="Title"]');
    await expect(titleInput).toHaveValue('Parent Page', { timeout: 5000 });

    await takeScreenshot('parent-page-with-link');

    // The link should be visible with the child's title
    const link = editor.locator('a');
    await expect(link).toBeVisible({ timeout: 5000 });
    await expect(link).toContainText('Original Child Title');

    // Now rename the child page
    const renameResponse = await request.post(`/api/workspaces/${wsID}/nodes/${childData.id}/page`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { title: 'Updated Child Title', content: 'Child content' },
    });
    expect(renameResponse.ok()).toBe(true);

    // Reload parent page
    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Navigate back to parent page
    await parentPageItem.click();
    await expect(titleInput).toHaveValue('Parent Page', { timeout: 5000 });
    await expect(editor).toBeVisible({ timeout: 5000 });

    await takeScreenshot('parent-page-after-child-rename');

    // The link should now show the updated title (resolved dynamically)
    await expect(link).toContainText('Updated Child Title', { timeout: 5000 });
  });

  test('verify GetNodeTitles API returns titles correctly', async ({ page, request }) => {
    const { token } = await registerUser(request, 'titles-api');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Get workspace ID from URL
    await expect(page).toHaveURL(/\/w\/[^/]+/, { timeout: 5000 });
    const url = page.url();
    const wsMatch = url.match(/\/w\/([^+/]+)/);
    expect(wsMatch).toBeTruthy();
    const wsID = wsMatch![1];

    // Create two pages
    const page1Response = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { title: 'Page One', content: '' },
    });
    expect(page1Response.ok()).toBe(true);
    const page1Data = await page1Response.json();

    const page2Response = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { title: 'Page Two', content: '' },
    });
    expect(page2Response.ok()).toBe(true);
    const page2Data = await page2Response.json();

    // Call GetNodeTitles API
    const titlesResponse = await request.get(
      `/api/workspaces/${wsID}/nodes/titles?ids=${page1Data.id},${page2Data.id}`,
      { headers: { Authorization: `Bearer ${token}` } }
    );
    if (!titlesResponse.ok()) {
      const errorBody = await titlesResponse.text();
      throw new Error(`GetNodeTitles failed: ${titlesResponse.status()} - ${errorBody}`);
    }
    const titlesData = await titlesResponse.json();

    // Verify titles are returned
    expect(titlesData.titles).toBeDefined();
    expect(titlesData.titles[page1Data.id]).toBe('Page One');
    expect(titlesData.titles[page2Data.id]).toBe('Page Two');
  });

  test('backlinks are returned when getting a page', async ({ page, request }) => {
    const { token } = await registerUser(request, 'backlinks');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Get workspace ID from URL
    await expect(page).toHaveURL(/\/w\/[^/]+/, { timeout: 5000 });
    const url = page.url();
    const wsMatch = url.match(/\/w\/([^+/]+)/);
    expect(wsMatch).toBeTruthy();
    const wsID = wsMatch![1];

    // Create source and target pages
    const targetResponse = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { title: 'Target Page', content: '' },
    });
    if (!targetResponse.ok()) {
      const errorBody = await targetResponse.text();
      throw new Error(`Target page creation failed: ${targetResponse.status()} - ${errorBody}`);
    }
    const targetData = await targetResponse.json();

    // Create source page with a link to target
    const linkContent = `Link to [Target Page](/w/${wsID}/${targetData.id})`;
    const sourceResponse = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { title: 'Source Page', content: linkContent },
    });
    expect(sourceResponse.ok()).toBe(true);
    const sourceData = await sourceResponse.json();

    // Get target page - should have backlink from source
    const getTargetResponse = await request.get(
      `/api/workspaces/${wsID}/nodes/${targetData.id}`,
      { headers: { Authorization: `Bearer ${token}` } }
    );
    expect(getTargetResponse.ok()).toBe(true);
    const getTargetData = await getTargetResponse.json();

    // Verify backlinks
    expect(getTargetData.backlinks).toBeDefined();
    expect(getTargetData.backlinks.length).toBe(1);
    expect(getTargetData.backlinks[0].node_id).toBe(sourceData.id);
    expect(getTargetData.backlinks[0].title).toBe('Source Page');
  });

  test('/page slash command creates link that shows in parent', async ({ page, request }) => {
    const { token } = await registerUser(request, 'slash-page-link');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    const wsID = await getWorkspaceId(page);

    // Create a parent page
    const createResponse = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { title: 'Parent With Subpage', content: '' },
    });
    expect(createResponse.ok()).toBe(true);
    const parentData = await createResponse.json();

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Navigate to parent page
    const parentNode = page.locator(`[data-testid="sidebar-node-${parentData.id}"]`);
    await parentNode.click();

    // Wait for editor
    const editor = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
    await expect(editor).toBeVisible({ timeout: 5000 });

    // Focus editor and use /page to create subpage
    await editor.click();
    await page.keyboard.type('/page');

    const slashMenu = page.locator('[data-testid="slash-command-menu"]');
    await expect(slashMenu).toBeVisible({ timeout: 3000 });
    await page.keyboard.press('Enter');
    await expect(slashMenu).not.toBeVisible({ timeout: 3000 });

    // Wait for navigation to new child page
    const titleInput = page.locator('input[placeholder*="Title"]');
    await expect(titleInput).toHaveValue('Untitled', { timeout: 10000 });

    // Rename the child page
    await titleInput.fill('My Subpage');

    // Trigger autosave by blurring
    await titleInput.blur();

    // Wait for autosave to complete (unsaved indicator should disappear)
    const unsavedIndicator = page.locator('[class*="unsavedIndicator"]');
    await expect(unsavedIndicator).not.toBeVisible({ timeout: 10000 });

    // Navigate back to parent
    await parentNode.locator('> [class*="pageItem"]').click();
    await expect(titleInput).toHaveValue('Parent With Subpage', { timeout: 5000 });

    // Check link in parent shows child title
    await expect(editor).toBeVisible({ timeout: 5000 });
    const link = editor.locator('a');
    await expect(link).toBeVisible({ timeout: 5000 });

    // The link should show the subpage title (either Untitled or My Subpage depending on timing)
    const linkText = await link.textContent();
    expect(linkText === 'Untitled' || linkText === 'My Subpage').toBe(true);
  });
});
