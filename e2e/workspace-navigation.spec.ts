// End-to-end keyboard navigation coverage for the workspace sidebar tree.

import { createClient, expect, getWorkspaceId, registerUser, test } from './helpers';

test.describe('Workspace tree navigation', () => {
  test('activates tree items and moves a page with the keyboard context-menu workflow', async ({ page, request }) => {
    const { token } = await registerUser(request, 'keyboard-tree');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });
    const workspaceId = await getWorkspaceId(page);
    const client = createClient(request, token);
    const source = await client.ws(workspaceId).nodes.page.createPage('0', {
      title: 'A keyboard source',
      content: 'Keyboard source content',
    });
    const target = await client.ws(workspaceId).nodes.page.createPage('0', {
      title: 'B keyboard target',
      content: 'Keyboard target content',
    });

    await page.reload();
    const tree = page.getByRole('tree', { name: 'Workspace pages' });
    await expect(tree).toBeVisible({ timeout: 10000 });
    const sourceItem = tree.getByRole('treeitem', { name: 'A keyboard source' });
    const targetItem = tree.getByRole('treeitem', { name: 'B keyboard target' });
    await expect(sourceItem).toHaveAttribute('aria-level', '1');

    const activeTreeItem = tree.locator('[role="treeitem"][tabindex="0"]');
    await expect(activeTreeItem).toHaveCount(1);
    for (let attempt = 0; attempt < 20; attempt += 1) {
      await page.keyboard.press('Tab');
      if (await activeTreeItem.evaluate((item) => document.activeElement === item)) break;
    }
    await expect(activeTreeItem).toBeFocused();
    await page.keyboard.press('ArrowDown');
    await expect(sourceItem).toBeFocused();
    await page.keyboard.press('Tab');
    await expect(tree.locator('[role="treeitem"]:focus')).toHaveCount(0);
    await page.keyboard.press('Shift+Tab');
    await expect(sourceItem).toBeFocused();
    await page.keyboard.press('Enter');
    await expect(page.getByText('Keyboard source content', { exact: true })).toBeVisible({ timeout: 5000 });
    await expect(sourceItem).toHaveAttribute('aria-current', 'page');

    await page.keyboard.press('Shift+F10');
    await expect(page.getByRole('menu')).toBeVisible({ timeout: 3000 });
    await page.keyboard.press('ArrowDown');
    await page.keyboard.press('ArrowDown');
    await page.keyboard.press('ArrowDown');
    await page.keyboard.press('Enter');
    await expect(sourceItem).toBeFocused();

    await page.keyboard.press('ArrowDown');
    await expect(targetItem).toBeFocused();
    await page.keyboard.press('Control+V');

    const targetNode = page.locator(`[data-testid="sidebar-node-${target.id}"]`);
    await expect(targetNode.locator(`[data-testid="sidebar-node-${source.id}"]`)).toBeVisible({ timeout: 5000 });
    await expect(sourceItem).toBeFocused();
    const breadcrumbs = page.getByRole('navigation', { name: 'Breadcrumbs' });
    await expect(breadcrumbs).toContainText('B keyboard target');
    await expect(breadcrumbs).toContainText('A keyboard source');

    await page.keyboard.press('Shift+F10');
    await expect(page.getByRole('menu')).toBeVisible({ timeout: 3000 });
    await page.keyboard.press('ArrowDown');
    await page.keyboard.press('ArrowDown');
    await page.keyboard.press('ArrowDown');
    await page.keyboard.press('ArrowDown');
    await page.keyboard.press('Enter');

    await expect(tree.locator(`[data-node-id="${source.id}"]`)).toBeFocused({ timeout: 5000 });
    await expect(targetNode.locator(`[data-testid="sidebar-node-${source.id}"]`)).not.toBeVisible();
  });
});
