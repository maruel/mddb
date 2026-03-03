// E2E tests for copy and paste functionality in the block editor.

import { test, expect, registerUser, getWorkspaceId, createClient } from './helpers';

test('paste standard HTML lists into the editor', async ({ page, request }) => {
  const { token } = await registerUser(request, 'paste-test');
  await page.goto(`/?token=${token}`);
  await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

  const wsId = await getWorkspaceId(page);
  const client = createClient(request, token);
  const pageResp = await client.ws(wsId).nodes.page.createPage('0', {
    title: 'Paste Test',
    content: '',
  });
  const nodeId = pageResp.id;

  await page.reload();
  await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });
  await page.locator(`[data-testid="sidebar-node-${nodeId}"]`).click();
  await expect(page.locator('[data-testid="wysiwyg-editor"]')).toBeVisible({ timeout: 5000 });

  const editor = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
  await expect(editor).toBeVisible({ timeout: 5000 });
  await editor.focus();

  // Paste a bullet list
  await page.keyboard.press('Control+A');
  await page.evaluate(() => {
    const editor = document.querySelector('.ProseMirror');
    const dt = new DataTransfer();
    dt.setData('text/html', '<ul><li>Bullet 1</li><li>Bullet 2</li></ul>');
    const event = new ClipboardEvent('paste', {
      clipboardData: dt,
      bubbles: true,
      cancelable: true,
    });
    editor?.dispatchEvent(event);
  });

  // Verify bullet list blocks were created
  await expect(editor.locator('.block-row[data-type="bullet"]')).toHaveCount(2);
  await expect(editor).toContainText('Bullet 1');
  await expect(editor).toContainText('Bullet 2');

  // Clear editor for next paste (select all and delete)
  await page.keyboard.press('Control+A');
  await page.keyboard.press('Backspace');

  // Paste a numbered list
  await page.keyboard.press('Control+A');
  await page.evaluate(() => {
    const editor = document.querySelector('.ProseMirror');
    const dt = new DataTransfer();
    dt.setData('text/html', '<ol><li>Number 1</li><li>Number 2</li></ol>');
    const event = new ClipboardEvent('paste', {
      clipboardData: dt,
      bubbles: true,
      cancelable: true,
    });
    editor?.dispatchEvent(event);
  });

  // Verify numbered list blocks were created
  await expect(editor.locator('.block-row[data-type="number"]')).toHaveCount(2);
  await expect(editor).toContainText('Number 1');
  await expect(editor).toContainText('Number 2');
});

test('copy content from editor as markdown', async ({ page, request, context }) => {
  const { token } = await registerUser(request, 'copy-test');
  await context.grantPermissions(['clipboard-read', 'clipboard-write']);

  await page.goto(`/?token=${token}`);
  await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

  const wsId = await getWorkspaceId(page);
  const client = createClient(request, token);
  const pageResp = await client.ws(wsId).nodes.page.createPage('0', {
    title: 'Copy Test',
    content: '- Bullet item\n- Second item',
  });
  const nodeId = pageResp.id;

  await page.reload();
  await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });
  await page.locator(`[data-testid="sidebar-node-${nodeId}"]`).click();
  await expect(page.locator('[data-testid="wysiwyg-editor"]')).toBeVisible({ timeout: 5000 });

  const editor = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
  await expect(editor).toBeVisible({ timeout: 5000 });

  // Select all content
  await editor.focus();
  await page.keyboard.press('Control+A');

  // Trigger copy
  // Note: Standard keyboard.press('Control+C') might not work for navigator.clipboard
  // so we might need to use document.execCommand('copy') or trigger the event.
  await page.keyboard.press('Control+C');

  // Wait a bit for clipboard to be updated
  await page.waitForTimeout(500);

  // Check clipboard content
  const clipboardText = await page.evaluate(() => navigator.clipboard.readText());
  expect(clipboardText).toContain('- Bullet item');
  expect(clipboardText).toContain('- Second item');
});

test('paste markdown into editor', async ({ page, request }) => {
  const { token } = await registerUser(request, 'paste-md');
  await page.goto(`/?token=${token}`);
  await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

  const wsId = await getWorkspaceId(page);
  const client = createClient(request, token);
  const pageResp = await client.ws(wsId).nodes.page.createPage('0', {
    title: 'Paste MD Test',
    content: '',
  });
  const nodeId = pageResp.id;

  await page.reload();
  await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });
  await page.locator(`[data-testid="sidebar-node-${nodeId}"]`).click();
  await expect(page.locator('[data-testid="wysiwyg-editor"]')).toBeVisible({ timeout: 5000 });

  const editor = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
  await expect(editor).toBeVisible({ timeout: 5000 });
  await editor.focus();

  // Paste markdown text
  await page.evaluate(() => {
    const editor = document.querySelector('.ProseMirror');
    const dt = new DataTransfer();
    dt.setData('text/plain', '# Heading\n\n- Item 1\n- Item 2');
    const event = new ClipboardEvent('paste', {
      clipboardData: dt,
      bubbles: true,
      cancelable: true,
    });
    editor?.dispatchEvent(event);
  });

  // Verify blocks were created
  // Note: Standard ProseMirror might just paste this as text if not handled.
  // We want it to be parsed as markdown blocks.
  await expect(editor.locator('.block-row[data-type="heading"]')).toBeVisible();
  await expect(editor.locator('.block-row[data-type="bullet"]')).toHaveCount(2);
});
