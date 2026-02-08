// E2E tests for clicking the checkbox pseudo-element to toggle task completion.

import { test, expect, registerUser, getWorkspaceId, createClient } from './helpers';

test('clicking the checkbox toggles task completion', async ({ page, request }) => {
  const { token } = await registerUser(request, 'checkbox-toggle');
  await page.goto(`/?token=${token}`);
  await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

  const wsID = await getWorkspaceId(page);

  // Create a page with unchecked and checked tasks
  const client = createClient(request, token);
  const pageData = await client.ws(wsID).nodes.page.createPage('0', {
    title: 'Checkbox Toggle Test',
    content: '- [ ] Unchecked task\n- [x] Checked task',
  });

  await page.reload();
  await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

  // Navigate to the page
  await page.locator(`[data-testid="sidebar-node-${pageData.id}"]`).click();

  const editor = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
  await expect(editor).toBeVisible({ timeout: 5000 });

  // Wait for task blocks to render
  const taskBlocks = editor.locator('.block-row[data-type="task"]');
  await expect(taskBlocks).toHaveCount(2, { timeout: 5000 });

  // First task should be unchecked
  const firstTask = taskBlocks.nth(0);
  await expect(firstTask).toHaveAttribute('data-checked', 'false');

  // Click the checkbox area (left side where the ::before pseudo-element is)
  const taskContent = firstTask.locator('.block-task');
  const box = await taskContent.boundingBox();
  expect(box).toBeTruthy();
  // Click at x=8 (center of the 16px checkbox in the left padding area)
  await page.mouse.click(box!.x + 8, box!.y + 10);

  // First task should now be checked
  await expect(firstTask).toHaveAttribute('data-checked', 'true', { timeout: 3000 });

  // Click the checkbox again to uncheck
  const box2 = await taskContent.boundingBox();
  await page.mouse.click(box2!.x + 8, box2!.y + 10);

  // Should be unchecked again
  await expect(firstTask).toHaveAttribute('data-checked', 'false', { timeout: 3000 });
});
