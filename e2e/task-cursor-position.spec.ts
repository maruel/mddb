// E2E test for task block cursor positioning when navigating with arrow keys.

import { test, expect, registerUser, getWorkspaceId, createClient } from './helpers';

test('right arrow from end of task line places cursor after checkbox, not before', async ({
  page,
  request,
}) => {
  const { token } = await registerUser(request, 'task-cursor');
  await page.goto(`/?token=${token}`);
  await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

  const wsID = await getWorkspaceId(page);

  // Create a page with two task items
  const client = createClient(request, token);
  const pageData = await client.ws(wsID).nodes.page.createPage('0', {
    title: 'Task Cursor Test',
    content: '- [ ] First task\n- [ ] Second task',
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

  // Click at the end of the first task's text
  const firstTaskContent = taskBlocks.nth(0).locator('.block-task');
  await firstTaskContent.click();

  // Move to end of line
  await page.keyboard.press('End');

  // Press right arrow to move to the next task block
  await page.keyboard.press('ArrowRight');

  // The cursor should be visually to the right of the second task's checkbox.
  // With the pseudo-element approach, the checkbox is a ::before on .block-task
  // and the cursor at offset 0 of the block content is visually after the checkbox.
  const cursorInfo = await page.evaluate(() => {
    const sel = window.getSelection();
    if (!sel || sel.rangeCount === 0) return null;

    const range = sel.getRangeAt(0);

    // Get bounding rect of cursor position
    const cursorRect = range.getBoundingClientRect();
    const secondTask = document.querySelectorAll('.block-row[data-type="task"]')[1];
    const taskContent = secondTask?.querySelector('.block-task') as HTMLElement | null;

    if (!taskContent) return null;

    const taskRect = taskContent.getBoundingClientRect();
    // The checkbox is in the left 24px padding area of .block-task
    // The cursor should be at or after padding-left (24px from task left edge)
    const checkboxRightEdge = taskRect.left + 22;

    return {
      cursorX: cursorRect.x,
      checkboxRightEdge,
      taskLeft: taskRect.left,
    };
  });

  expect(cursorInfo).toBeTruthy();
  // Cursor X should be >= checkbox right edge (cursor is after the checkbox visually)
  expect(cursorInfo!.cursorX).toBeGreaterThanOrEqual(cursorInfo!.checkboxRightEdge - 2);
});
