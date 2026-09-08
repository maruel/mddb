// End-to-end coverage for responsive workspace and settings sidebar behavior.

import { expect, registerUser, test } from './helpers';

test.describe('Responsive shell', () => {
  test('keeps desktop collapse separate from mobile overlay dismissal across resizes', async ({ page, request }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    const { token } = await registerUser(request, 'responsive-workspace');
    await page.goto(`/?token=${token}`);
    const sidebar = page.locator('aside').first();
    await expect(sidebar).toHaveClass(/open/i, { timeout: 10000 });

    await page.getByTitle('Collapse sidebar').click();
    await expect(sidebar).not.toHaveClass(/open/i);
    const menuButton = page.getByRole('button', { name: 'Toggle menu' });
    await expect(menuButton).toBeVisible();

    await page.setViewportSize({ width: 375, height: 667 });
    await expect(sidebar).not.toHaveClass(/open/i);
    await menuButton.click();
    await expect(sidebar).toHaveClass(/open/i);
    await page.keyboard.press('Escape');
    await expect(sidebar).not.toHaveClass(/open/i);

    await menuButton.click();
    const backdrop = page.getByTestId('workspace-sidebar-backdrop');
    await expect(backdrop).toHaveClass(/mobileBackdropVisible/i);
    const sidebarBox = await sidebar.boundingBox();
    const backdropBox = await backdrop.boundingBox();
    if (!sidebarBox || !backdropBox) throw new Error('Expected mobile sidebar and backdrop boxes');
    await backdrop.click({
      position: { x: sidebarBox.width + (backdropBox.width - sidebarBox.width) / 2, y: backdropBox.height / 2 },
    });
    await expect(sidebar).not.toHaveClass(/open/i);

    await page.setViewportSize({ width: 1280, height: 800 });
    await expect(sidebar).not.toHaveClass(/open/i);
    await expect(menuButton).toBeVisible();
  });

  test('keeps the settings sidebar closed on mobile after breakpoint changes', async ({ page, request }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    const { token } = await registerUser(request, 'responsive-settings');
    await page.goto(`/settings/user?token=${token}`);
    const sidebar = page.locator('aside').first();
    await expect(sidebar).toBeVisible({ timeout: 10000 });

    await page.setViewportSize({ width: 375, height: 667 });
    const menuButton = page.getByRole('button', { name: 'Toggle settings menu' });
    await expect(menuButton).toBeVisible();
    await expect(sidebar).not.toHaveClass(/mobileOpen/i);
    await menuButton.click();
    await expect(sidebar).toHaveClass(/mobileOpen/i);
    await page.keyboard.press('Escape');
    await expect(sidebar).not.toHaveClass(/mobileOpen/i);

    await page.setViewportSize({ width: 1280, height: 800 });
    await expect(sidebar).toBeVisible();
    await expect(sidebar).not.toHaveClass(/mobileOpen/i);
  });

  test('uses dismissible workspace creation when a workspace already exists', async ({ page, request }) => {
    const { token } = await registerUser(request, 'responsive-create-workspace');
    await page.goto(`/?token=${token}`);
    const createWorkspace = page.getByTestId('create-workspace-button');
    await expect(createWorkspace).toBeVisible({ timeout: 10000 });

    await createWorkspace.click();
    const dialog = page.getByRole('dialog', { name: 'Create Workspace' });
    await expect(dialog).toBeVisible();
    await page.keyboard.press('Escape');
    await expect(dialog).not.toBeVisible();

    await createWorkspace.click();
    await expect(dialog).toBeVisible();
    const backdrop = dialog.locator('..');
    const backdropBox = await backdrop.boundingBox();
    if (!backdropBox) throw new Error('Expected workspace dialog backdrop box');
    await backdrop.click({ position: { x: backdropBox.width - 8, y: backdropBox.height / 2 } });
    await expect(dialog).not.toBeVisible();
  });

  test('keeps the mobile sidebar open when Escape dismisses its workspace dialog', async ({ page, request }) => {
    await page.setViewportSize({ width: 375, height: 667 });
    const { token } = await registerUser(request, 'responsive-dialog-escape');
    await page.goto(`/?token=${token}`);
    const menuButton = page.getByRole('button', { name: 'Toggle menu' });
    const sidebar = page.locator('aside').first();
    await expect(menuButton).toBeVisible({ timeout: 10000 });

    await menuButton.click();
    await expect(sidebar).toHaveClass(/open/i);
    const createWorkspace = page.getByTestId('create-workspace-button');
    await createWorkspace.click();
    const dialog = page.getByRole('dialog', { name: 'Create Workspace' });
    await expect(dialog).toBeVisible();

    await page.keyboard.press('Escape');
    await expect(dialog).not.toBeVisible();
    await expect(sidebar).toHaveClass(/open/i);
    await expect(createWorkspace).toBeFocused();

    await page.keyboard.press('Escape');
    await expect(sidebar).not.toHaveClass(/open/i);
  });

  test('keeps the mobile sidebar open when a required first-workspace dialog consumes Escape', async ({
    page,
    request,
  }) => {
    await page.setViewportSize({ width: 375, height: 667 });
    const { token } = await registerUser(request, 'responsive-first-workspace');
    const meResponse = await request.get('/api/v1/auth/me', {
      headers: { Authorization: `Bearer ${token}` },
    });
    expect(meResponse.ok()).toBe(true);
    const firstWorkspaceUser = (await meResponse.json()) as Record<string, unknown>;
    delete firstWorkspaceUser.workspace_id;
    delete firstWorkspaceUser.workspace_name;
    delete firstWorkspaceUser.workspace_role;
    delete firstWorkspaceUser.workspaces;

    let interceptedMe = false;
    await page.route(/\/api\/v1\/auth\/me(?:\?.*)?$/, async (route) => {
      interceptedMe = true;
      await route.fulfill({
        contentType: 'application/json',
        body: JSON.stringify(firstWorkspaceUser),
      });
    });
    await page.goto(`/w/@first-workspace?token=${token}`);

    const menuButton = page.getByRole('button', { name: 'Toggle menu' });
    const sidebar = page.locator('aside').first();
    await expect(menuButton).toBeVisible({ timeout: 10000 });
    expect(interceptedMe).toBe(true);
    await menuButton.click();
    await expect(sidebar).toHaveClass(/open/i);

    await page.getByTestId('create-workspace-button').click();
    const dialog = page.getByRole('dialog', { name: 'Create Your First Workspace' });
    const nameInput = dialog.getByRole('textbox', { name: 'Workspace name' });
    await expect(dialog).toBeVisible();
    await expect(nameInput).toBeFocused();

    await page.keyboard.press('Escape');
    await expect(dialog).toBeVisible();
    await expect(nameInput).toBeFocused();
    await expect(sidebar).toHaveClass(/open/i);
  });
});
