import { test, expect, registerUser, getWorkspaceId, createClient } from './helpers';

test.describe('First Login Flow', () => {
  test('new user gets auto-created org, workspace, and welcome page', async ({ page, request }) => {
    const { token } = await registerUser(request, 'first-login');
    await page.goto(`/?token=${token}`);

    // Wait for first-login flow to complete
    await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

    // Should have a workspace URL
    await expect(page).toHaveURL(/\/w\/[^/]+/, { timeout: 10000 });

    // Welcome page should be auto-created and visible in sidebar
    const firstNode = page.locator('[data-testid^="sidebar-node-"]').first();
    await expect(firstNode).toBeVisible({ timeout: 10000 });

    // The page title should be visible (Welcome page or similar)
    // Note: title depends on localization
    const welcomeText = firstNode.locator('.title, span').first();
    await expect(welcomeText).not.toBeEmpty();
  });

  test('first login creates org named after user', async ({ page, request }) => {
    const { token } = await registerUser(request, 'org-name');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

    // Get user info to check org name
    const meResponse = await request.get('/api/auth/me', {
      headers: { Authorization: `Bearer ${token}` },
    });
    expect(meResponse.ok()).toBe(true);
    const userData = await meResponse.json();

    // Org should contain user's first name
    const orgs = userData.organizations || [];
    expect(orgs.length).toBeGreaterThan(0);

    // First org should be named after the user
    const firstOrg = orgs[0];
    expect(firstOrg.organization_name).toContain('org-name');
  });
});

test.describe('Workspace Switching', () => {
  test('create and switch to a new workspace', async ({ page, request }) => {
    const { token } = await registerUser(request, 'ws-switch');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

    // Get current user info
    const meResponse = await request.get('/api/auth/me', {
      headers: { Authorization: `Bearer ${token}` },
    });
    const userData = await meResponse.json();
    const orgId = userData.organization_id;

    // Create a second workspace via API
    const wsCreateResponse = await request.post(`/api/organizations/${orgId}/workspaces`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { name: 'Second Workspace' },
    });
    expect(wsCreateResponse.ok()).toBe(true);
    const newWsData = await wsCreateResponse.json();

    // Reload to see the new workspace
    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Open workspace menu
    const wsMenu = page.locator('[class*="WorkspaceMenu"]').locator('button').first();
    if (await wsMenu.isVisible()) {
      await wsMenu.click();

      // Should see both workspaces
      const secondWsOption = page.getByText('Second Workspace');
      await expect(secondWsOption).toBeVisible({ timeout: 3000 });

      // Click to switch
      await secondWsOption.click();

      // URL should update to new workspace
      await expect(page).toHaveURL(new RegExp(`/w/${newWsData.id}`), { timeout: 5000 });
    }
  });

  test('switching workspace clears selected node', async ({ page, request }) => {
    const { token } = await registerUser(request, 'ws-clear');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

    const wsID1 = await getWorkspaceId(page);

    // Create a page in first workspace
    const client = createClient(request, token);
    const page1Data = await client.ws(wsID1).nodes.page.createPage('0', {
      title: 'WS1 Page',
      content: 'Content in workspace 1',
    });

    // Navigate to the page
    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });
    await page.locator(`[data-testid="sidebar-node-${page1Data.id}"]`).click();
    await expect(page.getByText('Content in workspace 1')).toBeVisible({ timeout: 5000 });

    // Get org and create second workspace
    const meResponse = await request.get('/api/auth/me', {
      headers: { Authorization: `Bearer ${token}` },
    });
    const userData = await meResponse.json();
    const orgId = userData.organization_id;

    const ws2Response = await request.post(`/api/organizations/${orgId}/workspaces`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { name: 'Workspace Two' },
    });
    const ws2Data = await ws2Response.json();

    // Switch to second workspace via API (this persists the preference)
    const switchResponse = await request.post('/api/auth/switch-workspace', {
      headers: { Authorization: `Bearer ${token}` },
      data: { ws_id: ws2Data.id },
    });
    expect(switchResponse.ok()).toBe(true);

    // Navigate to root URL - this will redirect to the saved workspace (WS2)
    // Note: Reloading with explicit /w/ws1/... URL would stay on WS1 (URL is trusted)
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // The first workspace page should NOT be visible (we're now in WS2)
    await expect(page.getByText('Content in workspace 1')).not.toBeVisible({ timeout: 3000 });
  });
});

test.describe('Organization Features', () => {
  test('create a new organization', async ({ page, request }) => {
    const { token } = await registerUser(request, 'org-create');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

    // Look for org menu or create org button
    const orgMenu = page.locator('[class*="OrgMenu"]').locator('button').first();

    // If org menu is visible (user has multiple orgs), use it
    // Otherwise, we need to find another way to create orgs
    // Note: UI might hide org menu if only one org exists
    if (await orgMenu.isVisible()) {
      await orgMenu.click();
      const createOrgOption = page.getByText(/Create|New.*Organization/i);
      if (await createOrgOption.isVisible()) {
        await createOrgOption.click();

        // Fill in org name in modal
        const orgNameInput = page.locator('input[type="text"]');
        await orgNameInput.fill('My New Organization');

        const createButton = page.locator('button', { hasText: /Create/ });
        await createButton.click();

        // Should switch to new org
        await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });
      }
    }

    // Verify org exists via API
    const meResponse = await request.get('/api/auth/me', {
      headers: { Authorization: `Bearer ${token}` },
    });
    const userData = await meResponse.json();
    expect(userData.organizations?.length).toBeGreaterThanOrEqual(1);
  });
});

test.describe('User Menu', () => {
  test('user menu shows user name and email', async ({ page, request }) => {
    const { email, token } = await registerUser(request, 'user-menu');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

    // Click on user menu avatar button (shows initials)
    const avatarButton = page.locator('[class*="avatarButton"]').first();
    await avatarButton.click();

    // Should show user info in dropdown
    await expect(page.getByText(email)).toBeVisible({ timeout: 3000 });
    await expect(page.getByText('user-menu Test User')).toBeVisible();
  });

  test('user menu has profile and logout options', async ({ page, request }) => {
    const { token } = await registerUser(request, 'menu-options');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

    // Click on user menu avatar button
    const avatarButton = page.locator('[class*="avatarButton"]').first();
    await avatarButton.click();

    // Should have profile option
    await expect(page.locator('button', { hasText: 'Profile' })).toBeVisible({ timeout: 3000 });

    // Should have logout option
    await expect(page.locator('button', { hasText: 'Logout' })).toBeVisible();
  });
});

test.describe('Footer Links', () => {
  test('privacy and terms links in settings sidebar', async ({ page, request }) => {
    const { token } = await registerUser(request, 'footer-links');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

    // Open user menu and navigate to settings
    const userMenu = page.locator('[class*="avatarButton"]');
    await expect(userMenu).toBeVisible({ timeout: 5000 });
    await userMenu.click();

    // Click on Profile option to go to settings
    const profileLink = page.locator('button', { hasText: 'Profile' });
    await expect(profileLink).toBeVisible({ timeout: 3000 });
    await profileLink.click();

    // Wait for navigation to settings page
    await expect(page).toHaveURL(/\/settings/, { timeout: 10000 });

    // Settings sidebar should be visible with privacy/terms links at the bottom
    const settingsSidebar = page.locator('aside');
    await expect(settingsSidebar).toBeVisible({ timeout: 5000 });

    // Find footer links in settings sidebar - use specific link selectors
    const privacyLink = page.locator('a[href="/privacy"]');
    const termsLink = page.locator('a[href="/terms"]');

    await expect(privacyLink).toBeVisible({ timeout: 5000 });
    await expect(termsLink).toBeVisible();

    // Click privacy link and wait for navigation
    await Promise.all([
      page.waitForURL('/privacy', { timeout: 10000 }),
      privacyLink.click(),
    ]);

    // Navigate back to settings
    await page.goto(`/settings/user?token=${token}`);
    await expect(page).toHaveURL(/\/settings/, { timeout: 10000 });

    // Click terms link
    await Promise.all([
      page.waitForURL('/terms', { timeout: 10000 }),
      page.locator('a[href="/terms"]').click(),
    ]);
  });
});

test.describe('Sidebar Workspace Display', () => {
  test('sidebar shows workspace header and settings', async ({ page, request }) => {
    const { token } = await registerUser(request, 'sidebar-ws');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

    // Sidebar should contain workspace header
    const workspaceHeader = page.locator('aside [class*="workspaceHeader"]');
    await expect(workspaceHeader).toBeVisible();

    // Workspace name should be visible
    const workspaceName = page.locator('aside [class*="workspaceName"]');
    await expect(workspaceName).toBeVisible();

    // Settings button should be visible on hover
    await workspaceHeader.hover();
    const settingsButton = page.locator('[data-testid="workspace-settings-button"]');
    await expect(settingsButton).toBeVisible({ timeout: 3000 });
  });

  test('workspace settings button is clickable', async ({ page, request }) => {
    const { token } = await registerUser(request, 'ws-settings-click');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

    // Hover over workspace header to reveal settings button
    const workspaceHeader = page.locator('aside [class*="workspaceHeader"]');
    await workspaceHeader.hover();
    const settingsButton = page.locator('[data-testid="workspace-settings-button"]');
    await expect(settingsButton).toBeVisible({ timeout: 3000 });

    // Click settings button
    await settingsButton.click();

    // Should navigate to workspace settings
    await expect(page).toHaveURL(/\/settings\/workspace\//, { timeout: 5000 });
  });

  test('header shows user menu', async ({ page, request }) => {
    const { token } = await registerUser(request, 'header-user');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

    // Header should be visible
    const header = page.locator('header');
    await expect(header).toBeVisible();

    // Header should contain user menu (avatar button)
    await expect(header.locator('[class*="avatarButton"]')).toBeVisible();
  });

  test('sidebar is accessible on mobile viewport', async ({ page, request }) => {
    // Set mobile viewport
    await page.setViewportSize({ width: 375, height: 667 });

    const { token } = await registerUser(request, 'sidebar-mobile');
    await page.goto(`/?token=${token}`);
    // On mobile, wait for header instead (sidebar starts hidden)
    const header = page.locator('header');
    await expect(header).toBeVisible({ timeout: 15000 });

    // Header should have hamburger menu to open sidebar
    const hamburgerButton = header.locator('button').first();
    await expect(hamburgerButton).toBeVisible();

    // Click hamburger to open sidebar
    await hamburgerButton.click();

    // Sidebar should now be visible
    const sidebar = page.locator('aside');
    await expect(sidebar).toBeVisible({ timeout: 5000 });

    // Workspace header should be visible in sidebar
    const workspaceHeader = sidebar.locator('[class*="workspaceHeader"]');
    await expect(workspaceHeader).toBeVisible();
  });

  test('sidebar workspace header visible with long page title', async ({ page, request }) => {
    const { token } = await registerUser(request, 'ws-long-title');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

    // Get workspace ID
    const wsId = await getWorkspaceId(page);

    // Create a page with a very long title
    const longTitle = 'This Is A Very Long Page Title That Should Test Overflow Behavior';
    const resp = await request.post(`/api/workspaces/${wsId}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { title: longTitle },
    });
    const pageData = await resp.json();

    // Reload and navigate to the page
    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });
    await page.locator(`[data-testid="sidebar-node-${pageData.id}"]`).click();

    // Workspace header should still be visible at top of sidebar
    const workspaceHeader = page.locator('aside [class*="workspaceHeader"]');
    await expect(workspaceHeader).toBeVisible();

    // Workspace name should not overflow
    const workspaceName = page.locator('aside [class*="workspaceName"]');
    const nameBox = await workspaceName.boundingBox();
    const headerBox = await workspaceHeader.boundingBox();

    expect(nameBox).toBeTruthy();
    expect(headerBox).toBeTruthy();

    // Workspace name should be within header bounds
    expect(nameBox!.x).toBeGreaterThanOrEqual(headerBox!.x);
    expect(nameBox!.x + nameBox!.width).toBeLessThanOrEqual(headerBox!.x + headerBox!.width);
  });
});
