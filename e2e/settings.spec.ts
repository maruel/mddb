import type { Page } from '@playwright/test';
import { test, expect, registerUser } from './helpers';

// Helper to open user menu and click an option
async function openUserMenuAndClick(page: Page, optionText: string) {
  // Click on user menu avatar button (shows initials like "PT")
  const avatarButton = page.locator('[class*="avatarButton"]').first();
  await avatarButton.click();

  // Click on option in dropdown (menuitem role for accessibility)
  const option = page.getByRole('menuitem', { name: optionText, exact: true });
  await expect(option).toBeVisible({ timeout: 3000 });
  await option.click();
}

test.describe('User Profile Settings', () => {
  test.screenshot('navigate to profile and verify user info displayed', async ({ page, request, takeScreenshot }) => {
    const { email, token } = await registerUser(request, 'profile-view');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    await openUserMenuAndClick(page, 'Profile');

    // Should navigate to profile page
    await expect(page).toHaveURL('/settings/user', { timeout: 5000 });

    // User info should be displayed
    await expect(page.getByText(email)).toBeVisible({ timeout: 5000 });
    await expect(page.getByText('profile-view Test User')).toBeVisible();

    await takeScreenshot('profile-page');
  });

  test('change language setting and verify UI updates', async ({ page, request }) => {
    const { token } = await registerUser(request, 'language-change');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    await openUserMenuAndClick(page, 'Profile');
    await expect(page).toHaveURL('/settings/user', { timeout: 5000 });

    // Find language selector
    const languageSelect = page.locator('select').filter({ has: page.locator('option[value="en"]') });
    await expect(languageSelect).toBeVisible({ timeout: 5000 });

    // Change to French
    await languageSelect.selectOption('fr');

    // Save changes
    const saveButton = page.locator('button[type="submit"]');
    await saveButton.click();

    // Should show success message (in French or English)
    await expect(page.locator('[class*="success"]')).toBeVisible({ timeout: 5000 });
  });

  test('change theme setting', async ({ page, request }) => {
    const { token } = await registerUser(request, 'theme-change');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    await openUserMenuAndClick(page, 'Profile');
    await expect(page).toHaveURL('/settings/user', { timeout: 5000 });

    // Find theme selector
    const themeSelect = page.locator('select').filter({ has: page.locator('option[value="dark"]') });
    await expect(themeSelect).toBeVisible({ timeout: 5000 });

    // Change to dark
    await themeSelect.selectOption('dark');

    // Save
    const saveButton = page.locator('button[type="submit"]');
    await saveButton.click();

    await expect(page.locator('[class*="success"]')).toBeVisible({ timeout: 5000 });
  });

  test('back button returns to previous page', async ({ page, request }) => {
    const { token } = await registerUser(request, 'profile-back');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    await openUserMenuAndClick(page, 'Profile');
    await expect(page).toHaveURL('/settings/user', { timeout: 5000 });

    // Click back button
    const backButton = page.locator('button', { hasText: /Back|←/ });
    await backButton.click();

    // Should return to workspace view
    await expect(page.locator('aside')).toBeVisible({ timeout: 5000 });
    await expect(page).not.toHaveURL('/settings/user');
  });
});

test.describe('Workspace Settings', () => {
  test('navigate to workspace settings via sidebar', async ({ page, request }) => {
    const { token } = await registerUser(request, 'ws-settings');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Hover over workspace header to reveal settings button, then click
    const workspaceHeader = page.locator('aside [class*="workspaceHeader"]');
    await workspaceHeader.hover();
    const settingsButton = page.locator('[data-testid="workspace-settings-button"]');
    await expect(settingsButton).toBeVisible({ timeout: 3000 });
    await settingsButton.click();

    // Should be on settings page (URL contains workspace ID)
    await expect(page).toHaveURL(/\/settings\/workspace\//, { timeout: 5000 });

    // Settings tabs should be visible (use exact: true to avoid matching workspace button in header)
    await expect(page.getByRole('button', { name: 'Members', exact: true })).toBeVisible({ timeout: 5000 });
    await expect(page.getByRole('button', { name: 'Workspace', exact: true })).toBeVisible();
  });

  test.screenshot('workspace settings tabs navigation', async ({ page, request, takeScreenshot }) => {
    const { token } = await registerUser(request, 'ws-tabs');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Navigate to settings via sidebar settings button
    const workspaceHeader = page.locator('aside [class*="workspaceHeader"]');
    await workspaceHeader.hover();
    const settingsButton = page.locator('[data-testid="workspace-settings-button"]');
    await expect(settingsButton).toBeVisible({ timeout: 3000 });
    await settingsButton.click();
    await expect(page).toHaveURL(/\/settings\/workspace\//, { timeout: 5000 });

    // Click Members tab (use exact match to avoid conflicts)
    const membersTab = page.getByRole('button', { name: 'Members', exact: true });
    await membersTab.click();
    await expect(membersTab).toHaveClass(/active/i, { timeout: 3000 });
    await takeScreenshot('settings-members');

    // Click Workspace tab (use exact match)
    const workspaceTab = page.getByRole('button', { name: 'Workspace', exact: true });
    await workspaceTab.click();
    await expect(workspaceTab).toHaveClass(/active/i, { timeout: 3000 });
    await takeScreenshot('settings-workspace');

    // Git Sync tab (only for admins)
    const gitTab = page.getByRole('button', { name: 'Git Sync', exact: true });
    if (await gitTab.isVisible()) {
      await gitTab.click();
      await expect(gitTab).toHaveClass(/active/i, { timeout: 3000 });
      await takeScreenshot('settings-git-sync');
    }
  });

  test('members list shows current user', async ({ page, request }) => {
    const { email, token } = await registerUser(request, 'members-list');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Navigate to settings via sidebar settings button
    const workspaceHeader = page.locator('aside [class*="workspaceHeader"]');
    await workspaceHeader.hover();
    const settingsButton = page.locator('[data-testid="workspace-settings-button"]');
    await expect(settingsButton).toBeVisible({ timeout: 3000 });
    await settingsButton.click();
    await expect(page).toHaveURL(/\/settings\/workspace\//, { timeout: 5000 });

    // Members tab should be active by default - click to be sure
    const membersTab = page.getByRole('button', { name: 'Members', exact: true });
    await membersTab.click();

    // Current user should be in the list
    await expect(page.getByText(email)).toBeVisible({ timeout: 5000 });
    await expect(page.getByText('members-list Test User')).toBeVisible();
  });

  test('rename workspace', async ({ page, request }) => {
    const { token } = await registerUser(request, 'rename-ws');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Navigate to settings via sidebar settings button
    const workspaceHeader = page.locator('aside [class*="workspaceHeader"]');
    await workspaceHeader.hover();
    const settingsButton = page.locator('[data-testid="workspace-settings-button"]');
    await expect(settingsButton).toBeVisible({ timeout: 3000 });
    await settingsButton.click();
    await expect(page).toHaveURL(/\/settings\/workspace\//, { timeout: 5000 });

    // Click Workspace tab (use exact match to avoid matching workspace button in header)
    const workspaceTab = page.getByRole('button', { name: 'Workspace', exact: true });
    await workspaceTab.click();

    // Find workspace name input (labeled "Workspace Name")
    const wsNameInput = page.locator('label', { hasText: 'Workspace Name' }).locator('..').locator('input');
    await expect(wsNameInput).toBeVisible({ timeout: 5000 });

    // Change the name
    await wsNameInput.fill('Renamed Workspace');

    // Save
    const saveButton = page.locator('button[type="submit"]');
    await saveButton.click();

    // Should show success
    await expect(page.locator('[class*="success"]')).toBeVisible({ timeout: 5000 });
  });
});

test.describe('Authentication', () => {
  test('logout clears session and redirects to login', async ({ page, request }) => {
    const { token } = await registerUser(request, 'logout-test');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    await openUserMenuAndClick(page, 'Logout');

    // Logout is async - wait for the sidebar to disappear first
    await expect(page.locator('aside')).not.toBeVisible({ timeout: 10000 });

    // Should see login form (auth page)
    await expect(page.locator('form')).toBeVisible({ timeout: 10000 });
  });

  test.screenshot('invalid token redirects to login', async ({ page, takeScreenshot }) => {
    // Try to access with an invalid token
    await page.goto('/?token=invalid_token_12345');

    // Should eventually see login form after auth failure
    await expect(page.locator('form')).toBeVisible({ timeout: 10000 });

    await takeScreenshot('login-page');
  });
});
