import { test, expect } from './helpers';

test.describe('Page rename with navigation', () => {
  test('rename page then quickly navigate away - rename must persist', async ({ page, request }) => {
    // 1. Register a new user
    const email = `rename-nav-${Date.now()}@example.com`;
    const registerResponse = await request.post('/api/auth/register', {
      data: {
        email,
        password: 'testpassword123',
        name: 'Rename Nav Test User',
      },
    });
    expect(registerResponse.ok()).toBe(true);
    const { token } = await registerResponse.json();

    // 2. Login via token
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // 3. Get workspace ID from URL
    await expect(page).toHaveURL(/\/w\/[^/]+/, { timeout: 5000 });
    const url = page.url();
    const wsMatch = url.match(/\/w\/([^+/]+)/);
    expect(wsMatch).toBeTruthy();
    const wsID = wsMatch![1];

    // 4. Create two pages via API
    const page1Response = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: {
        title: 'Page To Rename',
        content: 'Content of page to rename',
      },
    });
    expect(page1Response.ok()).toBe(true);
    const page1Data = await page1Response.json();
    const page1ID = page1Data.id;

    const page2Response = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: {
        title: 'Other Page',
        content: 'Content of other page',
      },
    });
    expect(page2Response.ok()).toBe(true);
    const page2Data = await page2Response.json();
    const page2ID = page2Data.id;

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
    const getPageResponse = await request.get(`/api/workspaces/${wsID}/nodes/${page1ID}/page`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    expect(getPageResponse.ok()).toBe(true);
    const pageData = await getPageResponse.json();
    expect(pageData.title).toBe('RENAMED PAGE TITLE');

    // 12. Reload page and verify rename still persists (survives page refresh)
    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });
    await page1Node.click();
    await expect(titleInput).toHaveValue('RENAMED PAGE TITLE');
  });

  test('rename page, wait for autosave, then navigate - rename must persist', async ({ page, request }) => {
    // Control test: verifies rename works when waiting for autosave
    const email = `rename-wait-${Date.now()}@example.com`;
    const registerResponse = await request.post('/api/auth/register', {
      data: {
        email,
        password: 'testpassword123',
        name: 'Rename Wait Test User',
      },
    });
    expect(registerResponse.ok()).toBe(true);
    const { token } = await registerResponse.json();

    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    await expect(page).toHaveURL(/\/w\/[^/]+/, { timeout: 5000 });
    const url = page.url();
    const wsMatch = url.match(/\/w\/([^+/]+)/);
    expect(wsMatch).toBeTruthy();
    const wsID = wsMatch![1];

    // Create a page
    const pageResponse = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: {
        title: 'Original Title',
        content: 'Some content',
      },
    });
    expect(pageResponse.ok()).toBe(true);
    const pageData = await pageResponse.json();
    const pageID = pageData.id;

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
      const getResponse = await request.get(`/api/workspaces/${wsID}/nodes/${pageID}/page`, {
        headers: { Authorization: `Bearer ${token}` },
      });
      const savedData = await getResponse.json();
      expect(savedData.title).toBe('Updated Title');
    }).toPass({ timeout: 8000 });

    // Reload and verify the rename persisted
    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    await pageNode.click();
    await expect(titleInput).toHaveValue('Updated Title');

    // Verify via API
    const getPageResponse = await request.get(`/api/workspaces/${wsID}/nodes/${pageID}/page`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    expect(getPageResponse.ok()).toBe(true);
    const verifyData = await getPageResponse.json();
    expect(verifyData.title).toBe('Updated Title');
  });

  test('rapid renames with navigation - last rename must persist', async ({ page, request }) => {
    // Stress test: rapidly rename multiple times, then navigate
    const email = `rename-rapid-${Date.now()}@example.com`;
    const registerResponse = await request.post('/api/auth/register', {
      data: {
        email,
        password: 'testpassword123',
        name: 'Rename Rapid Test User',
      },
    });
    expect(registerResponse.ok()).toBe(true);
    const { token } = await registerResponse.json();

    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    await expect(page).toHaveURL(/\/w\/[^/]+/, { timeout: 5000 });
    const url = page.url();
    const wsMatch = url.match(/\/w\/([^+/]+)/);
    expect(wsMatch).toBeTruthy();
    const wsID = wsMatch![1];

    // Create two pages
    const page1Response = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: {
        title: 'Rapid Rename Page',
        content: 'Content for rapid rename test',
      },
    });
    expect(page1Response.ok()).toBe(true);
    const page1Data = await page1Response.json();
    const page1ID = page1Data.id;

    const page2Response = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: {
        title: 'Navigation Target',
        content: 'Navigation target content',
      },
    });
    expect(page2Response.ok()).toBe(true);
    const page2Data = await page2Response.json();
    const page2ID = page2Data.id;

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
    const getPageResponse = await request.get(`/api/workspaces/${wsID}/nodes/${page1ID}/page`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    expect(getPageResponse.ok()).toBe(true);
    const pageData = await getPageResponse.json();
    expect(pageData.title).toBe('FINAL RENAME');
  });

  test('rename content then navigate - content must persist', async ({ page, request }) => {
    // Test that content changes are also flushed on navigation
    const email = `content-nav-${Date.now()}@example.com`;
    const registerResponse = await request.post('/api/auth/register', {
      data: {
        email,
        password: 'testpassword123',
        name: 'Content Nav Test User',
      },
    });
    expect(registerResponse.ok()).toBe(true);
    const { token } = await registerResponse.json();

    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    await expect(page).toHaveURL(/\/w\/[^/]+/, { timeout: 5000 });
    const url = page.url();
    const wsMatch = url.match(/\/w\/([^+/]+)/);
    expect(wsMatch).toBeTruthy();
    const wsID = wsMatch![1];

    // Create two pages
    const page1Response = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: {
        title: 'Content Edit Page',
        content: 'Original content',
      },
    });
    expect(page1Response.ok()).toBe(true);
    const page1Data = await page1Response.json();
    const page1ID = page1Data.id;

    const page2Response = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: {
        title: 'Other Page',
        content: 'Other page content',
      },
    });
    expect(page2Response.ok()).toBe(true);
    const page2Data = await page2Response.json();
    const page2ID = page2Data.id;

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Navigate to Page 1
    const page1Node = page.locator(`[data-testid="sidebar-node-${page1ID}"]`);
    await page1Node.click();
    await expect(page.getByText('Original content')).toBeVisible({ timeout: 5000 });

    // Edit content
    const contentTextarea = page.locator('textarea[placeholder*="markdown"]');
    await expect(contentTextarea).toBeVisible();
    await contentTextarea.fill('MODIFIED CONTENT - this should persist');

    // Immediately navigate away
    const page2Node = page.locator(`[data-testid="sidebar-node-${page2ID}"]`);
    await page2Node.click();
    await expect(page.getByText('Other page content')).toBeVisible({ timeout: 5000 });

    // Navigate back and verify content persisted
    await page1Node.click();
    // Wait for page 1's title to be visible to ensure navigation completed
    const titleInput = page.locator('input[placeholder*="Title"]');
    await expect(titleInput).toHaveValue('Content Edit Page', { timeout: 5000 });
    // Content may have leading/trailing whitespace from markdown format
    const contentValue = await contentTextarea.inputValue();
    expect(contentValue.trim()).toBe('MODIFIED CONTENT - this should persist');

    // Verify via API
    const getPageResponse = await request.get(`/api/workspaces/${wsID}/nodes/${page1ID}/page`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    expect(getPageResponse.ok()).toBe(true);
    const pageData = await getPageResponse.json();
    expect(pageData.content.trim()).toBe('MODIFIED CONTENT - this should persist');
  });

  test('rename and content change then navigate - both must persist', async ({ page, request }) => {
    // Test that both title and content changes are flushed together
    const email = `both-nav-${Date.now()}@example.com`;
    const registerResponse = await request.post('/api/auth/register', {
      data: {
        email,
        password: 'testpassword123',
        name: 'Both Nav Test User',
      },
    });
    expect(registerResponse.ok()).toBe(true);
    const { token } = await registerResponse.json();

    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    await expect(page).toHaveURL(/\/w\/[^/]+/, { timeout: 5000 });
    const url = page.url();
    const wsMatch = url.match(/\/w\/([^+/]+)/);
    expect(wsMatch).toBeTruthy();
    const wsID = wsMatch![1];

    // Create two pages
    const page1Response = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: {
        title: 'Original Title',
        content: 'Original content',
      },
    });
    expect(page1Response.ok()).toBe(true);
    const page1Data = await page1Response.json();
    const page1ID = page1Data.id;

    const page2Response = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: {
        title: 'Other Page',
        content: 'Other page content',
      },
    });
    expect(page2Response.ok()).toBe(true);
    const page2Data = await page2Response.json();
    const page2ID = page2Data.id;

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Navigate to Page 1
    const page1Node = page.locator(`[data-testid="sidebar-node-${page1ID}"]`);
    await page1Node.click();
    await expect(page.getByText('Original content')).toBeVisible({ timeout: 5000 });

    // Edit both title and content
    const titleInput = page.locator('input[placeholder*="Title"]');
    const contentTextarea = page.locator('textarea[placeholder*="markdown"]');

    await titleInput.fill('NEW TITLE');
    await contentTextarea.fill('NEW CONTENT');

    // Immediately navigate away
    const page2Node = page.locator(`[data-testid="sidebar-node-${page2ID}"]`);
    await page2Node.click();
    await expect(page.getByText('Other page content')).toBeVisible({ timeout: 5000 });

    // Navigate back and verify both persisted
    await page1Node.click();
    await expect(titleInput).toHaveValue('NEW TITLE');
    // Content may have leading/trailing whitespace from markdown format
    const contentValue = await contentTextarea.inputValue();
    expect(contentValue.trim()).toBe('NEW CONTENT');

    // Verify via API
    const getPageResponse = await request.get(`/api/workspaces/${wsID}/nodes/${page1ID}/page`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    expect(getPageResponse.ok()).toBe(true);
    const pageData = await getPageResponse.json();
    expect(pageData.title).toBe('NEW TITLE');
    expect(pageData.content.trim()).toBe('NEW CONTENT');
  });
});
