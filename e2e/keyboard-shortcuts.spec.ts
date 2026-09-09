// End-to-end coverage for discoverable and editor-safe workspace keyboard shortcuts.

import { createClient, expect, getWorkspaceId, registerUser, test } from './helpers';

test.describe('Workspace keyboard shortcuts', () => {
  test('shows shortcut help, preserves editor input, and focuses tree navigation', async ({ page, request }) => {
    const { token } = await registerUser(request, 'keyboard-shortcuts');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });
    const workspaceId = await getWorkspaceId(page);
    const client = createClient(request, token);
    await client.ws(workspaceId).nodes.page.createPage('0', {
      title: 'A shortcut source',
      content: 'Source editor content',
    });
    await client.ws(workspaceId).nodes.page.createPage('0', {
      title: 'B shortcut target',
      content: 'Target editor content',
    });

    await page.reload();
    const tree = page.getByRole('tree', { name: 'Workspace pages' });
    const sourceItem = tree.getByRole('treeitem', { name: 'A shortcut source' });
    const targetItem = tree.getByRole('treeitem', { name: 'B shortcut target' });
    await expect(sourceItem).toBeVisible({ timeout: 10000 });
    await sourceItem.click();

    await test.step('editor input takes precedence over workspace shortcuts', async () => {
      const editor = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
      await expect(editor).toBeVisible({ timeout: 5000 });
      await editor.click();
      await page.keyboard.press('g');
      await page.keyboard.press('Shift+/');

      await expect(editor).toContainText('Source editor contentg/');
      await expect(page.getByRole('dialog', { name: 'Keyboard shortcuts' })).not.toBeVisible();
    });

    const shortcutsButton = page.getByTestId('keyboard-shortcuts-button');
    await test.step('shortcut help is discoverable and restores trigger focus', async () => {
      await shortcutsButton.click();
      const dialog = page.getByRole('dialog', { name: 'Keyboard shortcuts' });
      await expect(dialog).toBeVisible({ timeout: 3000 });
      await expect(dialog).toContainText('Focus workspace pages');

      await page.keyboard.press('Escape');
      await expect(dialog).not.toBeVisible();
      await expect(shortcutsButton).toBeFocused();

      await page.keyboard.press('Shift+/');
      await expect(dialog).toBeVisible({ timeout: 3000 });
      await page.keyboard.press('Escape');
      await expect(dialog).not.toBeVisible();
      await expect(shortcutsButton).toBeFocused();
    });

    await test.step('G enters page navigation and Enter opens the focused page', async () => {
      await page.keyboard.press('g');
      await expect(sourceItem).toBeFocused();
      await page.keyboard.press('ArrowDown');
      await expect(targetItem).toBeFocused();
      await page.keyboard.press('Enter');
      await expect(page.getByText('Target editor content', { exact: true })).toBeVisible({ timeout: 5000 });
    });
  });
});
