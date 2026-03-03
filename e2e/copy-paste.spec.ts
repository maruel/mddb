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

  await page.keyboard.press('Control+C');

  // Retry clipboard read until content is available
  await expect(async () => {
    const text = await page.evaluate(() => navigator.clipboard.readText());
    expect(text).toContain('- Bullet item');
    expect(text).toContain('- Second item');
  }).toPass();
});

test('paste HTML headings into the editor', async ({ page, request }) => {
  const { token } = await registerUser(request, 'paste-h');
  await page.goto(`/?token=${token}`);
  await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

  const wsId = await getWorkspaceId(page);
  const client = createClient(request, token);
  const pageResp = await client.ws(wsId).nodes.page.createPage('0', {
    title: 'Paste Headings',
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

  // Paste HTML with h1, h2, and a paragraph
  await page.keyboard.press('Control+A');
  await page.evaluate(() => {
    const editor = document.querySelector('.ProseMirror');
    const dt = new DataTransfer();
    dt.setData('text/html', '<h1>Title</h1><h2>Subtitle</h2><p>Body text</p>');
    const event = new ClipboardEvent('paste', {
      clipboardData: dt,
      bubbles: true,
      cancelable: true,
    });
    editor?.dispatchEvent(event);
  });

  // Verify heading and paragraph blocks were created
  const h1 = editor.locator('.block-row[data-type="heading"][data-level="1"]');
  const h2 = editor.locator('.block-row[data-type="heading"][data-level="2"]');
  await expect(h1).toHaveCount(1);
  await expect(h2).toHaveCount(1);
  await expect(editor).toContainText('Title');
  await expect(editor).toContainText('Subtitle');
  await expect(editor).toContainText('Body text');
});

test('copy headings from editor as markdown', async ({ page, request, context }) => {
  const { token } = await registerUser(request, 'copy-h');
  await context.grantPermissions(['clipboard-read', 'clipboard-write']);

  await page.goto(`/?token=${token}`);
  await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

  const wsId = await getWorkspaceId(page);
  const client = createClient(request, token);
  const pageResp = await client.ws(wsId).nodes.page.createPage('0', {
    title: 'Copy Headings',
    content: '# Big Title\n## Section\nParagraph here',
  });
  const nodeId = pageResp.id;

  await page.reload();
  await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });
  await page.locator(`[data-testid="sidebar-node-${nodeId}"]`).click();
  await expect(page.locator('[data-testid="wysiwyg-editor"]')).toBeVisible({ timeout: 5000 });

  const editor = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
  await expect(editor).toBeVisible({ timeout: 5000 });

  // Select all and copy
  await editor.focus();
  await page.keyboard.press('Control+A');
  await page.keyboard.press('Control+C');

  // Retry clipboard read until content is available
  await expect(async () => {
    const text = await page.evaluate(() => navigator.clipboard.readText());
    expect(text).toContain('# Big Title');
    expect(text).toContain('## Section');
    expect(text).toContain('Paragraph here');
  }).toPass();
});

test('heading copy-paste round-trip preserves structure', async ({ page, request, context }) => {
  const { token } = await registerUser(request, 'rt-h');
  await context.grantPermissions(['clipboard-read', 'clipboard-write']);

  await page.goto(`/?token=${token}`);
  await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

  const wsId = await getWorkspaceId(page);
  const client = createClient(request, token);

  // Create page with mixed heading/paragraph/list content
  const pageResp = await client.ws(wsId).nodes.page.createPage('0', {
    title: 'Round Trip',
    content: '# Title\n\nSome text\n\n## Section\n\n- Item 1\n- Item 2\n\n### Subsection\n\nMore text',
  });
  const nodeId = pageResp.id;

  await page.reload();
  await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });
  await page.locator(`[data-testid="sidebar-node-${nodeId}"]`).click();
  await expect(page.locator('[data-testid="wysiwyg-editor"]')).toBeVisible({ timeout: 5000 });

  const editor = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
  await expect(editor).toBeVisible({ timeout: 5000 });

  // Verify original blocks loaded correctly
  await expect(editor.locator('.block-row[data-type="heading"][data-level="1"]')).toHaveCount(1);
  await expect(editor.locator('.block-row[data-type="heading"][data-level="2"]')).toHaveCount(1);
  await expect(editor.locator('.block-row[data-type="heading"][data-level="3"]')).toHaveCount(1);
  await expect(editor.locator('.block-row[data-type="bullet"]')).toHaveCount(2);
  await expect(editor.locator('.block-row[data-type="paragraph"]')).toHaveCount(2);

  // Copy all content
  await editor.focus();
  await page.keyboard.press('Control+A');
  await page.keyboard.press('Control+C');

  // Retry clipboard read until content is available
  await expect(async () => {
    const text = await page.evaluate(() => navigator.clipboard.readText());
    expect(text).toContain('# Title');
    expect(text).toContain('## Section');
    expect(text).toContain('### Subsection');
  }).toPass();

  // Create a fresh empty page and paste into it
  const page2Resp = await client.ws(wsId).nodes.page.createPage('0', {
    title: 'Paste Target',
    content: '',
  });
  const nodeId2 = page2Resp.id;

  await page.reload();
  await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });
  await page.locator(`[data-testid="sidebar-node-${nodeId2}"]`).click();
  await expect(page.locator('[data-testid="wysiwyg-editor"]')).toBeVisible({ timeout: 5000 });

  const editor2 = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
  await expect(editor2).toBeVisible({ timeout: 5000 });
  await editor2.focus();

  // Paste the copied content
  await page.keyboard.press('Control+V');

  // Verify all block types survived the round-trip
  await expect(editor2.locator('.block-row[data-type="heading"][data-level="1"]')).toHaveCount(1);
  await expect(editor2.locator('.block-row[data-type="heading"][data-level="2"]')).toHaveCount(1);
  await expect(editor2.locator('.block-row[data-type="heading"][data-level="3"]')).toHaveCount(1);
  await expect(editor2.locator('.block-row[data-type="bullet"]')).toHaveCount(2);
  await expect(editor2).toContainText('Title');
  await expect(editor2).toContainText('Section');
  await expect(editor2).toContainText('Subsection');
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
