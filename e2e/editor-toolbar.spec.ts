// E2E tests for editor toolbar formatting buttons.

import { test, expect, registerUser, getWorkspaceId, switchToMarkdownMode, createClient } from './helpers';

test.describe('Editor Toolbar Formatting', () => {
  test('paragraphs render as block-row elements', async ({ page, request }) => {
    const { token } = await registerUser(request, 'toolbar-block-row-debug');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

    const wsID = await getWorkspaceId(page);

    // Create a page with multiple paragraphs
    const client = createClient(request, token);
    const pageData = await client.ws(wsID).nodes.page.createPage('0', {
      title: 'Block Row Debug',
      content: 'Line one\n\nLine two\n\nLine three',
    });

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Navigate to the page
    await page.locator(`[data-testid="sidebar-node-${pageData.id}"]`).click();

    // Wait for WYSIWYG editor to load
    const editor = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
    await expect(editor).toBeVisible({ timeout: 5000 });

    // All three paragraphs should be block-row elements
    const blockRows = editor.locator('.block-row');
    await expect(blockRows).toHaveCount(3, { timeout: 5000 });

    // Each should have data-type="paragraph"
    const paragraphBlocks = editor.locator('.block-row[data-type="paragraph"]');
    await expect(paragraphBlocks).toHaveCount(3, { timeout: 5000 });

  });

  test('debug: task list button converts paragraphs', async ({ page, request }) => {
    const { token } = await registerUser(request, 'toolbar-task-debug');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

    const wsID = await getWorkspaceId(page);

    // Create a page with multiple paragraphs
    const client = createClient(request, token);
    const pageData = await client.ws(wsID).nodes.page.createPage('0', {
      title: 'Task Debug',
      content: 'Line one\n\nLine two\n\nLine three',
    });

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Navigate to the page
    await page.locator(`[data-testid="sidebar-node-${pageData.id}"]`).click();

    // Wait for WYSIWYG editor to load
    const editor = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
    await expect(editor).toBeVisible({ timeout: 5000 });

    // Verify initial state - 3 paragraph blocks
    await expect(editor.locator('.block-row[data-type="paragraph"]')).toHaveCount(3, { timeout: 5000 });
    console.log('Initial HTML:', await editor.innerHTML());

    // Select all text in the editor using keyboard
    await editor.click();
    await page.keyboard.press('Control+a');

    // Wait a bit for selection to be processed
    await page.waitForTimeout(100);

    // Check if toolbar is visible
    const toolbar = page.locator('[data-testid="floating-toolbar"]');
    const isToolbarVisible = await toolbar.isVisible();
    console.log('Toolbar visible after Ctrl+A:', isToolbarVisible);

    // Click the checkbox button in the toolbar
    const checkboxButton = page.locator('button[title="Task List"]');
    await expect(checkboxButton).toBeVisible({ timeout: 3000 });
    console.log('Task List button visible');

    await checkboxButton.click();
    console.log('Clicked Task List button');

    // Wait a bit for the conversion to happen
    await page.waitForTimeout(100);

    // Log the result
    console.log('After click HTML:', await editor.innerHTML());

    // Check task items
    const taskItems = editor.locator('.block-row[data-type="task"]');
    const taskCount = await taskItems.count();
    console.log('Task item count:', taskCount);

    await expect(taskItems).toHaveCount(3, { timeout: 5000 });
  });

  test('selecting multiple lines and clicking Checkbox converts all lines to task list', async ({ page, request }) => {
    const { token } = await registerUser(request, 'toolbar-checkbox-multi');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

    const wsID = await getWorkspaceId(page);

    // Create a page with multiple lines
    const client = createClient(request, token);
    const pageData = await client.ws(wsID).nodes.page.createPage('0', {
      title: 'Multi-line Checkbox Test',
      content: 'Line one\n\nLine two\n\nLine three',
    });

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Navigate to the page
    await page.locator(`[data-testid="sidebar-node-${pageData.id}"]`).click();

    // Wait for WYSIWYG editor to load
    const editor = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
    await expect(editor).toBeVisible({ timeout: 5000 });

    // Select all text in the editor using keyboard
    await editor.click();
    await page.keyboard.press('Control+a');

    // Click the checkbox button in the toolbar
    const checkboxButton = page.locator('button[title="Task List"]');
    await checkboxButton.click();

    // All three lines should be task list items
    const taskItems = editor.locator('.block-row[data-type="task"]');
    await expect(taskItems).toHaveCount(3, { timeout: 5000 });

    // Verify via markdown mode
    const markdownEditor = await switchToMarkdownMode(page);
    const markdown = await markdownEditor.inputValue();

    // Should have 3 task list items
    const taskMatches = markdown.match(/- \[ \]/g);
    expect(taskMatches).toHaveLength(3);
  });

  test('clicking numbered list button while inside numbered list toggles it off', async ({ page, request }) => {
    const { token } = await registerUser(request, 'toolbar-toggle-ol');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

    const wsID = await getWorkspaceId(page);

    // Create a page with a numbered list
    const client = createClient(request, token);
    const pageData = await client.ws(wsID).nodes.page.createPage('0', {
      title: 'Toggle Numbered List Test',
      content: '1. First item\n2. Second item\n3. Third item',
    });

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Navigate to the page
    await page.locator(`[data-testid="sidebar-node-${pageData.id}"]`).click();

    // Wait for WYSIWYG editor to load
    const editor = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
    await expect(editor).toBeVisible({ timeout: 5000 });

    // Verify the ordered list is rendered
    const listItems = editor.locator('.block-row[data-type="number"]');
    await expect(listItems.first()).toBeVisible({ timeout: 3000 });
    await expect(listItems).toHaveCount(3);

    // Select all list items (click first, shift-click last)
    await listItems.first().click();
    await listItems.last().click({ modifiers: ['Shift'] });

    // The numbered list button should be active (indicating we're inside an ordered list)
    const numberedListButton = page.locator('button[title="Numbered List"]');
    await expect(numberedListButton).toHaveClass(/isActive/, { timeout: 3000 });

    // Click the button to toggle it off
    await numberedListButton.click();

    // The ordered list should be removed - all items should become paragraphs
    await expect(editor.locator('.block-row[data-type="number"]')).toHaveCount(0, { timeout: 3000 });

    // Verify via markdown mode
    const markdownEditor = await switchToMarkdownMode(page);
    const markdown = await markdownEditor.inputValue();

    // Should NOT have numbered list syntax
    expect(markdown).not.toMatch(/^\d+\./m);
  });

  test('clicking bullet list button while inside bullet list toggles it off', async ({ page, request }) => {
    const { token } = await registerUser(request, 'toolbar-toggle-ul');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

    const wsID = await getWorkspaceId(page);

    // Create a page with a bullet list
    const client = createClient(request, token);
    const pageData = await client.ws(wsID).nodes.page.createPage('0', {
      title: 'Toggle Bullet List Test',
      content: '- First item\n- Second item\n- Third item',
    });

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Navigate to the page
    await page.locator(`[data-testid="sidebar-node-${pageData.id}"]`).click();

    // Wait for WYSIWYG editor to load
    const editor = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
    await expect(editor).toBeVisible({ timeout: 5000 });

    // Verify the bullet list is rendered
    const listItems = editor.locator('.block-row[data-type="bullet"]');
    await expect(listItems.first()).toBeVisible({ timeout: 3000 });
    await expect(listItems).toHaveCount(3);

    // Select all list items (click first, shift-click last)
    await listItems.first().click();
    await listItems.last().click({ modifiers: ['Shift'] });

    // The bullet list button should be active
    const bulletListButton = page.locator('button[title="Bullet List"]');
    await expect(bulletListButton).toHaveClass(/isActive/, { timeout: 3000 });

    // Click the button to toggle it off
    await bulletListButton.click();

    // The bullet list should be removed - all items should become paragraphs
    await expect(editor.locator('.block-row[data-type="bullet"]')).toHaveCount(0, { timeout: 3000 });

    // Verify via markdown mode
    const markdownEditor = await switchToMarkdownMode(page);
    const markdown = await markdownEditor.inputValue();

    // Should NOT have bullet list syntax
    expect(markdown).not.toMatch(/^- /m);
  });

  test('clicking checkbox button while inside task list toggles it off (unwraps to paragraphs)', async ({
    page,
    request,
  }) => {
    const { token } = await registerUser(request, 'toolbar-toggle-task');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

    const wsID = await getWorkspaceId(page);

    // Create a page with a task list
    const client = createClient(request, token);
    const pageData = await client.ws(wsID).nodes.page.createPage('0', {
      title: 'Toggle Task List Test',
      content: '- [ ] First task\n- [x] Second task\n- [ ] Third task',
    });

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Navigate to the page
    await page.locator(`[data-testid="sidebar-node-${pageData.id}"]`).click();

    // Wait for WYSIWYG editor to load
    const editor = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
    await expect(editor).toBeVisible({ timeout: 5000 });

    // Verify task list items are rendered
    const taskItems = editor.locator('.block-row[data-type="task"]');
    await expect(taskItems).toHaveCount(3, { timeout: 3000 });

    // Select all task items (click first, shift-click last)
    await taskItems.first().click();
    await taskItems.last().click({ modifiers: ['Shift'] });

    // The checkbox button should be active
    const checkboxButton = page.locator('button[title="Task List"]');
    await expect(checkboxButton).toHaveClass(/isActive/, { timeout: 3000 });

    // Click the button to toggle it off
    await checkboxButton.click();

    // The list should be completely removed (unwrapped to paragraphs)
    await expect(editor.locator('.block-row[data-type="task"]')).toHaveCount(0, { timeout: 3000 });

    // Content should now be paragraphs
    await expect(editor.locator('.block-row[data-type="paragraph"]')).toHaveCount(3, { timeout: 3000 });

    // Verify via markdown mode
    const markdownEditor = await switchToMarkdownMode(page);
    const markdown = await markdownEditor.inputValue();

    // Should not have any list syntax
    expect(markdown).not.toMatch(/^- /m);
  });

  test('selecting multiple lines and clicking numbered list wraps all in one list', async ({ page, request }) => {
    const { token } = await registerUser(request, 'toolbar-ol-multi');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

    const wsID = await getWorkspaceId(page);

    // Create a page with multiple paragraphs
    const client = createClient(request, token);
    const pageData = await client.ws(wsID).nodes.page.createPage('0', {
      title: 'Multi-line Numbered List Test',
      content: 'Line one\n\nLine two\n\nLine three',
    });

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Navigate to the page
    await page.locator(`[data-testid="sidebar-node-${pageData.id}"]`).click();

    // Wait for WYSIWYG editor to load
    const editor = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
    await expect(editor).toBeVisible({ timeout: 5000 });

    // Select all text in the editor
    await editor.click();
    await page.keyboard.press('Control+a');

    // Click the numbered list button
    const numberedListButton = page.locator('button[title="Numbered List"]');
    await numberedListButton.click();

    // All three lines should be in an ordered list
    const orderedListItems = editor.locator('.block-row[data-type="number"]');
    await expect(orderedListItems).toHaveCount(3, { timeout: 5000 });

    // Verify via markdown mode
    const markdownEditor = await switchToMarkdownMode(page);
    const markdown = await markdownEditor.inputValue();

    // Should have numbered list syntax
    expect(markdown).toMatch(/1\./);
    expect(markdown).toMatch(/2\./);
    expect(markdown).toMatch(/3\./);
  });

  test('selecting multiple lines and clicking bullet list wraps all in one list', async ({ page, request }) => {
    const { token } = await registerUser(request, 'toolbar-ul-multi');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

    const wsID = await getWorkspaceId(page);

    // Create a page with multiple paragraphs
    const client = createClient(request, token);
    const pageData = await client.ws(wsID).nodes.page.createPage('0', {
      title: 'Multi-line Bullet List Test',
      content: 'Line one\n\nLine two\n\nLine three',
    });

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Navigate to the page
    await page.locator(`[data-testid="sidebar-node-${pageData.id}"]`).click();

    // Wait for WYSIWYG editor to load
    const editor = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
    await expect(editor).toBeVisible({ timeout: 5000 });

    // Select all text in the editor
    await editor.click();
    await page.keyboard.press('Control+a');

    // Click the bullet list button
    const bulletListButton = page.locator('button[title="Bullet List"]');
    await bulletListButton.click();

    // All three lines should be in a bullet list
    const bulletListItems = editor.locator('.block-row[data-type="bullet"]');
    await expect(bulletListItems).toHaveCount(3, { timeout: 5000 });

    // Verify via markdown mode
    const markdownEditor = await switchToMarkdownMode(page);
    const markdown = await markdownEditor.inputValue();

    // Should have bullet list syntax (3 bullet items)
    const bulletMatches = markdown.match(/^- /gm);
    expect(bulletMatches).toHaveLength(3);
  });

  test('clicking checkbox on regular text creates a task list', async ({ page, request }) => {
    const { token } = await registerUser(request, 'toolbar-checkbox-create');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

    const wsID = await getWorkspaceId(page);

    // Create a page with plain text
    const client = createClient(request, token);
    const pageData = await client.ws(wsID).nodes.page.createPage('0', {
      title: 'Create Task List Test',
      content: 'Some regular text',
    });

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Navigate to the page
    await page.locator(`[data-testid="sidebar-node-${pageData.id}"]`).click();

    // Wait for WYSIWYG editor to load with actual page content
    const editor = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
    await expect(editor).toBeVisible({ timeout: 5000 });
    await expect(editor.locator('p')).toContainText('Some', { timeout: 5000 });

    // Double-click to select text and trigger the floating toolbar
    // Note: Use mouse.dblclick with coordinates near text start because
    // locator.dblclick() clicks center which may miss the text
    const paragraph = editor.locator('.block-row[data-type="paragraph"] .block-content').first();
    const box = await paragraph.boundingBox();
    expect(box).toBeTruthy();
    await page.mouse.dblclick(box!.x + 30, box!.y + box!.height / 2);

    // Wait for the floating toolbar to appear, then click the checkbox button
    const checkboxButton = page.locator('button[title="Task List"]');
    await expect(checkboxButton).toBeVisible({ timeout: 3000 });
    await checkboxButton.click();

    // Should create a task list item
    const taskItems = editor.locator('.block-row[data-type="task"]');
    await expect(taskItems).toHaveCount(1, { timeout: 3000 });
  });

  test('clicking checkbox on bullet list converts to task list', async ({ page, request }) => {
    const { token } = await registerUser(request, 'toolbar-bullet-to-task');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

    const wsID = await getWorkspaceId(page);

    // Create a page with a bullet list
    const client = createClient(request, token);
    const pageData = await client.ws(wsID).nodes.page.createPage('0', {
      title: 'Bullet to Task Test',
      content: '- First item\n- Second item',
    });

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Navigate to the page
    await page.locator(`[data-testid="sidebar-node-${pageData.id}"]`).click();

    // Wait for WYSIWYG editor to load
    const editor = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
    await expect(editor).toBeVisible({ timeout: 5000 });

    // Click inside the first bullet item
    await editor.locator('.block-row[data-type="bullet"] .block-content').first().dblclick();

    // Click the checkbox button
    const checkboxButton = page.locator('button[title="Task List"]');
    await checkboxButton.click();

    // First item should become a task list item
    const firstLi = editor.locator('.block-row').first();
    await expect(firstLi).toHaveAttribute('data-type', 'task', { timeout: 3000 });
  });
});

test.describe('Editor Toolbar Edge Cases', () => {
  test('selecting multiple task list items and clicking checkbox toggles all off', async ({ page, request }) => {
    const { token } = await registerUser(request, 'toolbar-multi-task-off');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

    const wsID = await getWorkspaceId(page);

    // Create a page with a task list
    const client = createClient(request, token);
    const pageData = await client.ws(wsID).nodes.page.createPage('0', {
      title: 'Multi Task Toggle Off Test',
      content: '- [ ] Task one\n- [x] Task two\n- [ ] Task three',
    });

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    await page.locator(`[data-testid="sidebar-node-${pageData.id}"]`).click();

    const editor = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
    await expect(editor).toBeVisible({ timeout: 5000 });

    // Select all task items
    await editor.click();
    await page.keyboard.press('Control+a');

    // Click checkbox button to toggle off
    const checkboxButton = page.locator('button[title="Task List"]');
    await checkboxButton.click();

    // The list should be completely removed (unwrapped to paragraphs)
    await expect(editor.locator('ul')).not.toBeVisible({ timeout: 3000 });

    // Content should now be paragraphs
    await expect(editor.locator('p')).toHaveCount(3, { timeout: 3000 });
  });

  test('converting numbered list to task list preserves all items', async ({ page, request }) => {
    const { token } = await registerUser(request, 'toolbar-ol-to-task');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

    const wsID = await getWorkspaceId(page);

    // Create a page with a numbered list
    const client = createClient(request, token);
    const pageData = await client.ws(wsID).nodes.page.createPage('0', {
      title: 'Numbered to Task Test',
      content: '1. First item\n2. Second item\n3. Third item',
    });

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    await page.locator(`[data-testid="sidebar-node-${pageData.id}"]`).click();

    const editor = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
    await expect(editor).toBeVisible({ timeout: 5000 });

    // Select all items
    await editor.click();
    await page.keyboard.press('Control+a');

    // Click checkbox button to convert to task list
    const checkboxButton = page.locator('button[title="Task List"]');
    await checkboxButton.click();

    // All three items should be task list items
    const taskItems = editor.locator('.block-row[data-type="task"]');
    await expect(taskItems).toHaveCount(3, { timeout: 3000 });
  });

  test('converting bullet list to numbered list works', async ({ page, request }) => {
    const { token } = await registerUser(request, 'toolbar-ul-to-ol');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

    const wsID = await getWorkspaceId(page);

    // Create a page with a bullet list
    const client = createClient(request, token);
    const pageData = await client.ws(wsID).nodes.page.createPage('0', {
      title: 'Bullet to Numbered Test',
      content: '- First item\n- Second item\n- Third item',
    });

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    await page.locator(`[data-testid="sidebar-node-${pageData.id}"]`).click();

    const editor = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
    await expect(editor).toBeVisible({ timeout: 5000 });

    // Click first item and shift-click last item to select all
    const items = editor.locator('.block-row[data-type="bullet"] .block-content');
    await items.first().click();
    await items.last().click({ modifiers: ['Shift'] });

    // Click numbered list button
    const numberedListButton = page.locator('button[title="Numbered List"]');
    await numberedListButton.click();

    // Should be a numbered list now
    await expect(editor.locator('.block-row[data-type="number"]')).toHaveCount(3);
  });

  test('converting numbered list to bullet list works', async ({ page, request }) => {
    const { token } = await registerUser(request, 'toolbar-ol-to-ul');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

    const wsID = await getWorkspaceId(page);

    // Create a page with a numbered list
    const client = createClient(request, token);
    const pageData = await client.ws(wsID).nodes.page.createPage('0', {
      title: 'Numbered to Bullet Test',
      content: '1. First item\n2. Second item\n3. Third item',
    });

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Navigate to the page
    await page.locator(`[data-testid="sidebar-node-${pageData.id}"]`).click();

    const editor = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
    await expect(editor).toBeVisible({ timeout: 5000 });

    // Click first item and shift-click last item to select all
    const items = editor.locator('.block-row[data-type="number"] .block-content');
    await items.first().click();
    await items.last().click({ modifiers: ['Shift'] });

    // Click bullet list button
    const bulletListButton = page.locator('button[title="Bullet List"]');
    await bulletListButton.click();

    // Should be a bullet list now
    await expect(editor.locator('.block-row[data-type="bullet"]')).toHaveCount(3);
  });

  test('converting task list to numbered list works', async ({ page, request }) => {
    const { token } = await registerUser(request, 'toolbar-task-to-ol');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

    const wsID = await getWorkspaceId(page);

    // Create a page with a task list
    const client = createClient(request, token);
    const pageData = await client.ws(wsID).nodes.page.createPage('0', {
      title: 'Task to Numbered Test',
      content: '- [ ] Task one\n- [x] Task two\n- [ ] Task three',
    });

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    await page.locator(`[data-testid="sidebar-node-${pageData.id}"]`).click();

    // Wait for WYSIWYG editor to load
    const editor = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
    await expect(editor).toBeVisible({ timeout: 5000 });

    // Click first item and shift-click last item to select all
    const items = editor.locator('.block-row[data-type="task"] .block-content');
    await items.first().click();
    await items.last().click({ modifiers: ['Shift'] });

    // Click numbered list button
    const numberedListButton = page.locator('button[title="Numbered List"]');
    await numberedListButton.click();

    // Should be a numbered list now with no task items
    await expect(editor.locator('.block-row[data-type="number"]')).toHaveCount(3, { timeout: 3000 });
    await expect(editor.locator('.block-row[data-type="task"]')).toHaveCount(0);
  });

  test('selection is preserved through all list type transitions', async ({ page, request }) => {
    const { token } = await registerUser(request, 'toolbar-sel-transitions');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

    const wsID = await getWorkspaceId(page);

    // Create a page with 4 lines of plain text
    const client = createClient(request, token);
    const pageData = await client.ws(wsID).nodes.page.createPage('0', {
      title: 'Selection Transitions Test',
      content: 'Line one\n\nLine two\n\nLine three\n\nLine four',
    });

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    await page.locator(`[data-testid="sidebar-node-${pageData.id}"]`).click();

    const editor = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
    await expect(editor).toBeVisible({ timeout: 5000 });

    const bulletButton = page.locator('button[title="Bullet List"]');
    const numberedButton = page.locator('button[title="Numbered List"]');
    const checkboxButton = page.locator('button[title="Task List"]');

    // Verify we have 4 paragraphs
    const paragraphs = editor.locator('.block-row[data-type="paragraph"]');
    await expect(paragraphs).toHaveCount(4, { timeout: 3000 });

    // Select lines 2 and 3
    const p2 = paragraphs.nth(1).locator('.block-content');
    const p3 = paragraphs.nth(2).locator('.block-content');
    await p2.click();
    await p3.click({ modifiers: ['Shift'] });

    // Wait for floating toolbar to appear (needed on slower CI machines)
    const toolbar = page.locator('[data-testid="floating-toolbar"]');
    await expect(toolbar).toBeVisible({ timeout: 3000 });

    // Step 1: Click bullet list - should convert lines 2-3 to bullet list
    await bulletButton.click();
    await expect(editor.locator('.block-row[data-type="bullet"]')).toHaveCount(2, { timeout: 3000 });
    // Lines 1 and 4 remain as direct paragraph children (not in lists)
    await expect(editor.locator('.block-row[data-type="paragraph"]')).toHaveCount(2);

    // Verify markdown after bullet list
    let markdownEditor = await switchToMarkdownMode(page);
    let markdown = await markdownEditor.inputValue();
    expect(markdown).toContain('Line one');
    expect(markdown).toContain('- Line two');
    expect(markdown).toContain('- Line three');
    expect(markdown).toContain('Line four');
    expect(markdown).not.toContain('- Line one');
    expect(markdown).not.toContain('- Line four');

    // Switch back to WYSIWYG
    await page.locator('[data-testid="editor-mode-visual"]').click();
    await expect(editor).toBeVisible({ timeout: 3000 });

    // Re-select the list items (click first list item content, shift-click second)
    const listItems = editor.locator('.block-row[data-type="bullet"] .block-content');
    await listItems.first().click();
    await listItems.last().click({ modifiers: ['Shift'] });

    // Step 2: Click numbered list - should convert to numbered list
    await numberedButton.click();
    await expect(editor.locator('.block-row[data-type="number"]')).toHaveCount(2, { timeout: 3000 });
    await expect(editor.locator('.block-row[data-type="bullet"]')).toHaveCount(0);

    // Verify markdown after numbered list
    markdownEditor = await switchToMarkdownMode(page);
    markdown = await markdownEditor.inputValue();
    expect(markdown).toContain('Line one');
    expect(markdown).toContain('1. Line two');
    expect(markdown).toContain('2. Line three');
    expect(markdown).toContain('Line four');

    // Switch back to WYSIWYG
    await page.locator('[data-testid="editor-mode-visual"]').click();
    await expect(editor).toBeVisible({ timeout: 3000 });

    // Re-select the list items
    const orderedItems = editor.locator('.block-row[data-type="number"] .block-content');
    await orderedItems.first().click();
    await orderedItems.last().click({ modifiers: ['Shift'] });

    // Step 3: Click checkbox - should convert to task list
    await checkboxButton.click();
    await expect(editor.locator('.block-row[data-type="task"]')).toHaveCount(2, { timeout: 3000 });

    // Verify markdown after task list
    markdownEditor = await switchToMarkdownMode(page);
    markdown = await markdownEditor.inputValue();
    expect(markdown).toContain('Line one');
    expect(markdown).toContain('- [ ] Line two');
    expect(markdown).toContain('- [ ] Line three');
    expect(markdown).toContain('Line four');

    // Switch back to WYSIWYG
    await page.locator('[data-testid="editor-mode-visual"]').click();
    await expect(editor).toBeVisible({ timeout: 3000 });

    // Re-select the task list items
    const taskItems = editor.locator('.block-row[data-type="task"] .block-content');
    await taskItems.first().click();
    await taskItems.last().click({ modifiers: ['Shift'] });

    // Step 4: Click checkbox again - should toggle off to paragraphs
    await checkboxButton.click();
    await expect(editor.locator('.block-row[data-type="bullet"]')).toHaveCount(0, { timeout: 3000 });
    await expect(editor.locator('.block-row[data-type="number"]')).toHaveCount(0);
    await expect(editor.locator('.block-row[data-type="task"]')).toHaveCount(0);
    await expect(editor.locator('.block-row[data-type="paragraph"]')).toHaveCount(4, { timeout: 3000 });

    // Verify markdown - all 4 lines should be plain paragraphs
    markdownEditor = await switchToMarkdownMode(page);
    markdown = await markdownEditor.inputValue();
    expect(markdown).toContain('Line one');
    expect(markdown).toContain('Line two');
    expect(markdown).toContain('Line three');
    expect(markdown).toContain('Line four');
    expect(markdown).not.toContain('- ');
    expect(markdown).not.toMatch(/^\d+\./m);
  });
});

test.describe('Editor Toolbar Inline Formatting', () => {
  test('underline formatting is preserved when switching to markdown mode', async ({ page, request }) => {
    const { token } = await registerUser(request, 'toolbar-underline');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

    const wsID = await getWorkspaceId(page);

    // Create a page with plain text
    const client = createClient(request, token);
    const pageData = await client.ws(wsID).nodes.page.createPage('0', {
      title: 'Underline Test',
      content: 'Some text to underline',
    });

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Navigate to the page
    await page.locator(`[data-testid="sidebar-node-${pageData.id}"]`).click();

    // Wait for WYSIWYG editor to load
    const editor = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
    await expect(editor).toBeVisible({ timeout: 5000 });
    await expect(editor.locator('p')).toContainText('Some text', { timeout: 5000 });

    // Select text using mouse drag
    const paragraph = editor.locator('.block-row[data-type="paragraph"] .block-content').first();
    await paragraph.selectText();

    // Wait for the floating toolbar to appear
    const underlineButton = page.locator('button[title="Underline (Ctrl+U)"]');
    await expect(underlineButton).toBeVisible({ timeout: 3000 });

    // Click the underline button
    await underlineButton.click();

    // Verify the text is underlined in the editor
    await expect(editor.locator('u')).toBeVisible({ timeout: 3000 });

    // Switch to markdown mode and verify underline syntax
    const markdownEditor = await switchToMarkdownMode(page);
    const markdown = await markdownEditor.inputValue();

    // Should have <u> tags for underline
    expect(markdown).toContain('<u>');
    expect(markdown).toContain('</u>');
    expect(markdown).toMatch(/<u>Some text to underline<\/u>/);
  });
});

test.describe('Editor Toolbar Button States', () => {
  test('task list only highlights checkbox button, not bullet or numbered list buttons', async ({ page, request }) => {
    const { token } = await registerUser(request, 'toolbar-task-highlight');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

    const wsID = await getWorkspaceId(page);

    // Create a page with a task list
    const client = createClient(request, token);
    const pageData = await client.ws(wsID).nodes.page.createPage('0', {
      title: 'Task List Highlight Test',
      content: '- [ ] Task one\n- [x] Task two\n- [ ] Task three',
    });

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Navigate to the page
    await page.locator(`[data-testid="sidebar-node-${pageData.id}"]`).click();

    // Wait for WYSIWYG editor to load
    const editor = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
    await expect(editor).toBeVisible({ timeout: 5000 });

    // Click inside the first task list item and select text
    await editor.locator('.block-row[data-type="task"] .block-content').first().selectText();

    // The checkbox button should be active
    const checkboxButton = page.locator('button[title="Task List"]');
    await expect(checkboxButton).toHaveClass(/isActive/, { timeout: 3000 });

    // The bullet list button should NOT be active
    const bulletListButton = page.locator('button[title="Bullet List"]');
    await expect(bulletListButton).not.toHaveClass(/isActive/);

    // The numbered list button should NOT be active
    const numberedListButton = page.locator('button[title="Numbered List"]');
    await expect(numberedListButton).not.toHaveClass(/isActive/);
  });

  test('bullet list only highlights bullet button, not checkbox or numbered buttons', async ({ page, request }) => {
    const { token } = await registerUser(request, 'toolbar-bullet-highlight');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

    const wsID = await getWorkspaceId(page);

    // Create a page with a bullet list
    const client = createClient(request, token);
    const pageData = await client.ws(wsID).nodes.page.createPage('0', {
      title: 'Bullet List Highlight Test',
      content: '- Item one\n- Item two\n- Item three',
    });

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Navigate to the page
    await page.locator(`[data-testid="sidebar-node-${pageData.id}"]`).click();

    // Wait for WYSIWYG editor to load
    const editor = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
    await expect(editor).toBeVisible({ timeout: 5000 });

    // Click inside the first bullet list item and select text
    await editor.locator('.block-row[data-type="bullet"] .block-content').first().selectText();

    // The bullet list button should be active
    const bulletListButton = page.locator('button[title="Bullet List"]');
    await expect(bulletListButton).toHaveClass(/isActive/, { timeout: 3000 });

    // The checkbox button should NOT be active
    const checkboxButton = page.locator('button[title="Task List"]');
    await expect(checkboxButton).not.toHaveClass(/isActive/);

    // The numbered list button should NOT be active
    const numberedListButton = page.locator('button[title="Numbered List"]');
    await expect(numberedListButton).not.toHaveClass(/isActive/);
  });

  test('numbered list only highlights numbered button, not checkbox or bullet buttons', async ({ page, request }) => {
    const { token } = await registerUser(request, 'toolbar-num-highlight');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

    const wsID = await getWorkspaceId(page);

    // Create a page with a numbered list
    const client = createClient(request, token);
    const pageData = await client.ws(wsID).nodes.page.createPage('0', {
      title: 'Numbered List Highlight Test',
      content: '1. Item one\n2. Item two\n3. Item three',
    });

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Navigate to the page
    await page.locator(`[data-testid="sidebar-node-${pageData.id}"]`).click();

    // Wait for WYSIWYG editor to load
    const editor = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
    await expect(editor).toBeVisible({ timeout: 5000 });

    // Click inside the first numbered list item and select text
    await editor.locator('.block-row[data-type="number"] .block-content').first().selectText();

    // The numbered list button should be active
    const numberedListButton = page.locator('button[title="Numbered List"]');
    await expect(numberedListButton).toHaveClass(/isActive/, { timeout: 3000 });

    // The checkbox button should NOT be active
    const checkboxButton = page.locator('button[title="Task List"]');
    await expect(checkboxButton).not.toHaveClass(/isActive/);

    // The bullet list button should NOT be active
    const bulletListButton = page.locator('button[title="Bullet List"]');
    await expect(bulletListButton).not.toHaveClass(/isActive/);
  });
});