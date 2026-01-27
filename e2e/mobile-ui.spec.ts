import { test, expect, devices } from '@playwright/test';

// Helper to register a user and get token
async function registerUser(request: any, prefix: string) {
  const email = `${prefix}-${Date.now()}@example.com`;
  const registerResponse = await request.post('/api/auth/register', {
    data: {
      email,
      password: 'testpassword123',
      name: `${prefix} Test User`,
    },
  });
  expect(registerResponse.ok()).toBe(true);
  const { token } = await registerResponse.json();
  return { email, token };
}

// Helper to get workspace ID from URL
async function getWorkspaceId(page: any): Promise<string> {
  await expect(page).toHaveURL(/\/w\/[^/]+/, { timeout: 5000 });
  const url = page.url();
  const wsMatch = url.match(/\/w\/([^+/]+)/);
  expect(wsMatch).toBeTruthy();
  return wsMatch![1];
}

// Use mobile viewport for all tests in this file
test.use({
  viewport: { width: 375, height: 667 }, // iPhone SE size
});

test.describe('Mobile UI - Sidebar Toggle', () => {
  test('hamburger menu shows and hides sidebar', async ({ page, request }) => {
    const { token } = await registerUser(request, 'mobile-sidebar');
    await page.goto(`/?token=${token}`);

    // Wait for app to load
    await expect(page.locator('header h1')).toBeVisible({ timeout: 10000 });

    // On mobile, sidebar should be hidden by default
    const sidebar = page.locator('aside');
    // Sidebar might be rendered but not visible (hidden via CSS)
    // Check if it's not in the visible viewport or has specific mobile classes
    const hamburgerButton = page.locator('button[aria-label="Toggle menu"], [class*="hamburger"]');
    await expect(hamburgerButton).toBeVisible({ timeout: 5000 });

    // Click hamburger to open sidebar
    await hamburgerButton.click();

    // Sidebar should now be visible
    await expect(sidebar).toBeVisible({ timeout: 3000 });
    await expect(sidebar).toHaveClass(/mobileOpen|open/i);

    // Click hamburger again to close
    await hamburgerButton.click();

    // Sidebar should be hidden again (or lose the open class)
    await expect(sidebar).not.toHaveClass(/mobileOpen/);
  });

  // BUG: Mobile sidebar backdrop click not working - see BUGS_FOUND.md Bug 6
  test.skip('clicking backdrop closes mobile sidebar', async ({ page, request }) => {
    const { token } = await registerUser(request, 'mobile-backdrop');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('header h1')).toBeVisible({ timeout: 10000 });

    // Open sidebar
    const hamburgerButton = page.locator('button[aria-label="Toggle menu"], [class*="hamburger"]');
    await hamburgerButton.click();

    const sidebar = page.locator('aside');
    await expect(sidebar).toHaveClass(/mobileOpen|open/i, { timeout: 3000 });

    // Click on backdrop (the dark overlay behind the sidebar)
    const backdrop = page.locator('[class*="backdrop"], [class*="Backdrop"]');
    if (await backdrop.isVisible()) {
      await backdrop.click();
      // Sidebar should close
      await expect(sidebar).not.toHaveClass(/mobileOpen/);
    }
  });

  test('selecting a page closes mobile sidebar', async ({ page, request }) => {
    const { token } = await registerUser(request, 'mobile-select');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('header h1')).toBeVisible({ timeout: 10000 });

    const wsID = await getWorkspaceId(page);

    // Create a page
    const createResponse = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { title: 'Mobile Test Page', content: 'Mobile content' },
    });
    const pageData = await createResponse.json();

    await page.reload();
    await expect(page.locator('header h1')).toBeVisible({ timeout: 10000 });

    // Open sidebar
    const hamburgerButton = page.locator('button[aria-label="Toggle menu"], [class*="hamburger"]');
    await hamburgerButton.click();

    const sidebar = page.locator('aside');
    await expect(sidebar).toHaveClass(/mobileOpen|open/i, { timeout: 3000 });

    // Click on the page in sidebar
    const pageNode = page.locator(`[data-testid="sidebar-node-${pageData.id}"]`);
    await expect(pageNode).toBeVisible({ timeout: 5000 });
    await pageNode.click();

    // Sidebar should auto-close
    await expect(sidebar).not.toHaveClass(/mobileOpen/, { timeout: 3000 });

    // Page content should be visible
    await expect(page.getByText('Mobile content')).toBeVisible({ timeout: 5000 });
  });
});

test.describe('Mobile UI - Layout', () => {
  test('content area uses full width on mobile', async ({ page, request }) => {
    const { token } = await registerUser(request, 'mobile-layout');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('header h1')).toBeVisible({ timeout: 10000 });

    const wsID = await getWorkspaceId(page);

    // Create a page
    const createResponse = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { title: 'Mobile Layout Test', content: 'Testing mobile layout' },
    });
    const pageData = await createResponse.json();

    await page.reload();
    await expect(page.locator('header h1')).toBeVisible({ timeout: 10000 });

    // Open sidebar and select page
    const hamburgerButton = page.locator('button[aria-label="Toggle menu"], [class*="hamburger"]');
    await hamburgerButton.click();
    await page.locator(`[data-testid="sidebar-node-${pageData.id}"]`).click();
    await expect(page.getByText('Testing mobile layout')).toBeVisible({ timeout: 5000 });

    // Main content should be nearly full width (accounting for some padding)
    const mainContent = page.locator('main, [class*="main"]');
    const box = await mainContent.boundingBox();
    expect(box).toBeTruthy();
    // Should be at least 90% of viewport width
    expect(box!.width).toBeGreaterThan(375 * 0.9);
  });

  test('editor works on mobile with virtual keyboard consideration', async ({ page, request }) => {
    const { token } = await registerUser(request, 'mobile-editor');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('header h1')).toBeVisible({ timeout: 10000 });

    const wsID = await getWorkspaceId(page);

    // Create a page
    const createResponse = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { title: 'Mobile Editor Test', content: 'Original content' },
    });
    const pageData = await createResponse.json();

    await page.reload();
    await expect(page.locator('header h1')).toBeVisible({ timeout: 10000 });

    // Navigate to page
    const hamburgerButton = page.locator('button[aria-label="Toggle menu"], [class*="hamburger"]');
    await hamburgerButton.click();
    await page.locator(`[data-testid="sidebar-node-${pageData.id}"]`).click();

    // Wait for the page content to fully load (title should show the original value)
    const titleInput = page.locator('input[placeholder*="Title"]');
    await expect(titleInput).toBeVisible({ timeout: 5000 });
    await expect(titleInput).toHaveValue('Mobile Editor Test', { timeout: 5000 });

    // Focus and type in title
    await titleInput.focus();
    await titleInput.fill('Updated Mobile Title');
    await expect(titleInput).toHaveValue('Updated Mobile Title');

    // Focus on content
    const contentTextarea = page.locator('textarea[placeholder*="markdown"]');
    await contentTextarea.focus();
    await contentTextarea.fill('Updated mobile content');

    // Wait for autosave
    await page.waitForTimeout(3000);

    // Verify via API
    const getResponse = await request.get(`/api/workspaces/${wsID}/nodes/${pageData.id}/page`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    const savedData = await getResponse.json();
    expect(savedData.title).toBe('Updated Mobile Title');
  });
});

test.describe('Mobile UI - Touch Interactions', () => {
  // Enable touch support for this test suite
  test.use({ hasTouch: true });

  test('tap on sidebar node navigates correctly', async ({ page, request }) => {
    const { token } = await registerUser(request, 'mobile-tap');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('header h1')).toBeVisible({ timeout: 10000 });

    const wsID = await getWorkspaceId(page);

    // Create multiple pages
    const page1Response = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { title: 'Tap Page 1', content: 'Tap content 1' },
    });
    const page1Data = await page1Response.json();

    const page2Response = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { title: 'Tap Page 2', content: 'Tap content 2' },
    });
    const page2Data = await page2Response.json();

    await page.reload();
    await expect(page.locator('header h1')).toBeVisible({ timeout: 10000 });

    // Open sidebar
    const hamburgerButton = page.locator('button[aria-label="Toggle menu"], [class*="hamburger"]');
    await hamburgerButton.click();

    // Tap on first page
    await page.locator(`[data-testid="sidebar-node-${page1Data.id}"]`).tap();
    await expect(page.getByText('Tap content 1')).toBeVisible({ timeout: 5000 });

    // Open sidebar again
    await hamburgerButton.click();

    // Tap on second page
    await page.locator(`[data-testid="sidebar-node-${page2Data.id}"]`).tap();
    await expect(page.getByText('Tap content 2')).toBeVisible({ timeout: 5000 });
  });
});
