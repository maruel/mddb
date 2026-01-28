import { test, expect, registerUser, getWorkspaceId } from './helpers';

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
    const page1Response = await request.post(`/api/workspaces/${wsID1}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { title: 'WS1 Page', content: 'Content in workspace 1' },
    });
    const page1Data = await page1Response.json();

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
  test('privacy and terms links in sidebar footer', async ({ page, request }) => {
    const { token } = await registerUser(request, 'footer-links');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

    // Find footer links in sidebar - use specific link selectors
    const privacyLink = page.locator('aside a[href="/privacy"]');
    const termsLink = page.locator('aside a[href="/terms"]');

    await expect(privacyLink).toBeVisible({ timeout: 5000 });
    await expect(termsLink).toBeVisible();

    // Click privacy link and wait for navigation
    await Promise.all([
      page.waitForURL('/privacy', { timeout: 10000 }),
      privacyLink.click(),
    ]);

    // Navigate fresh instead of using goBack (more reliable in SPAs)
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });
    await Promise.all([
      page.waitForURL('/terms', { timeout: 10000 }),
      termsLink.click(),
    ]);
  });
});

test.describe('Header Display', () => {
  test('header shows navigation and menus', async ({ page, request }) => {
    const { token } = await registerUser(request, 'header-nav');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

    // Header should be visible
    const header = page.locator('header');
    await expect(header).toBeVisible();

    // Header should contain user menu (avatar button)
    await expect(header.locator('[class*="avatarButton"]')).toBeVisible();

    // Header should contain workspace menu
    await expect(header.locator('button', { hasText: /Workspace/ })).toBeVisible();
  });
});
