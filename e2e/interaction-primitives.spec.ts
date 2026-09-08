// End-to-end coverage for dialog-contained field dropdowns and import-dialog focus behavior.

import { test, expect, createClient, getWorkspaceId, registerUser } from './helpers';

test.describe('Interaction primitives', () => {
  test('record detail keeps select and multi-select dropdowns inside its drawer', async ({ page, request }) => {
    const { token } = await registerUser(request, 'record-detail-dropdowns');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });
    const workspaceId = await getWorkspaceId(page);
    const client = createClient(request, token);
    const table = await client.ws(workspaceId).nodes.table.createTable('0', {
      title: 'Drawer dropdowns',
      properties: [
        { name: 'Title', type: 'text' },
        {
          name: 'Status',
          type: 'select',
          options: [
            { id: 'todo', name: 'To Do' },
            { id: 'done', name: 'Done' },
          ],
        },
        {
          name: 'Tags',
          type: 'multi_select',
          options: [
            { id: 'apple', name: 'Apple' },
            { id: 'banana', name: 'Banana' },
          ],
        },
      ],
    });
    await client.ws(workspaceId).nodes.table.records.createRecord(table.id, {
      data: { Title: 'Record in drawer', Status: '', Tags: '' },
    });

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });
    await page.locator(`[data-testid="sidebar-node-${table.id}"]`).click();
    await expect(page.locator('table')).toBeVisible({ timeout: 5000 });

    await page.locator('button[title="Open"]').first().click();
    const drawer = page.getByRole('dialog', { name: 'Record Detail' });
    await expect(drawer).toBeVisible({ timeout: 5000 });

    await drawer.locator('[class*="singleSelectWrapper"]').click();
    const selectDropdown = drawer.locator('[data-testid="select-dropdown"]');
    await expect(selectDropdown).toBeVisible({ timeout: 3000 });
    await expect(selectDropdown.locator('input')).toBeFocused();
    await selectDropdown.getByText('To Do', { exact: true }).click();
    await expect(selectDropdown).not.toBeVisible();

    await drawer.locator('[class*="multiSelectWrapper"]').click();
    const multiSelectDropdown = drawer.locator('[data-testid="select-dropdown"]');
    await expect(multiSelectDropdown).toBeVisible({ timeout: 3000 });
    await expect(multiSelectDropdown.locator('input')).toBeFocused();
    await multiSelectDropdown.getByText('Apple', { exact: true }).click();
    await expect(multiSelectDropdown).toBeVisible();
  });

  test('Notion import dialog focuses its token field and restores trigger focus after Escape', async ({
    page,
    request,
  }) => {
    const { token } = await registerUser(request, 'notion-import-dialog');
    await page.goto(`/?token=${token}`);
    const importButton = page.getByTestId('import-notion-button');
    await expect(importButton).toBeVisible({ timeout: 10000 });

    await importButton.click();
    const dialog = page.getByRole('dialog', { name: 'Import from Notion' });
    await expect(dialog).toBeVisible({ timeout: 3000 });
    await expect(dialog.locator('input[type="password"]')).toBeFocused();

    await page.keyboard.press('Escape');
    await expect(dialog).not.toBeVisible();
    await expect(importButton).toBeFocused();
  });

  test.describe('short touch landscape viewport', () => {
    test.use({ hasTouch: true });

    test('keeps Notion import actions reachable', async ({ page, request }) => {
      await page.setViewportSize({ width: 568, height: 240 });
      const { token } = await registerUser(request, 'notion-import-short-viewport');
      await page.goto(`/?token=${token}`);
      const menuToggle = page.getByRole('button', { name: 'Toggle menu' });
      await expect(menuToggle).toBeVisible({ timeout: 10000 });
      await menuToggle.click();
      const importButton = page.getByTestId('import-notion-button');
      await expect(importButton).toBeVisible({ timeout: 10000 });

      await importButton.click();
      const dialog = page.getByRole('dialog', { name: 'Import from Notion' });
      await expect(dialog).toBeVisible({ timeout: 3000 });
      const tokenInput = dialog.locator('input[type="password"]');
      await expect(tokenInput).toBeFocused();
      await tokenInput.fill('notion-secret');

      await page.keyboard.press('Tab');
      await page.keyboard.press('Tab');
      const startImport = dialog.getByRole('button', { name: 'Start Import' });
      await expect(startImport).toBeFocused();
      const box = await startImport.boundingBox();
      expect(box).not.toBeNull();
      expect((box?.y ?? 0) + (box?.height ?? 0)).toBeLessThanOrEqual(240);
    });
  });
});
