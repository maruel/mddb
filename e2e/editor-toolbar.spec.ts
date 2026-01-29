// E2E tests for editor toolbar formatting buttons.

import { test, expect, registerUser, getWorkspaceId, switchToMarkdownMode } from './helpers';

test.describe('Editor Toolbar Formatting', () => {
  test('selecting multiple lines and clicking Checkbox converts all lines to task list', async ({ page, request }) => {
    const { token } = await registerUser(request, 'toolbar-checkbox-multi');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

    const wsID = await getWorkspaceId(page);

    // Create a page with multiple lines
    const createResponse = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: {
        title: 'Multi-line Checkbox Test',
        content: 'Line one\n\nLine two\n\nLine three',
      },
    });
    expect(createResponse.ok()).toBe(true);
    const pageData = await createResponse.json();

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
    const taskItems = editor.locator('li.task-list-item');
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
    const createResponse = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: {
        title: 'Toggle Numbered List Test',
        content: '1. First item\n2. Second item\n3. Third item',
      },
    });
    expect(createResponse.ok()).toBe(true);
    const pageData = await createResponse.json();

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Navigate to the page
    await page.locator(`[data-testid="sidebar-node-${pageData.id}"]`).click();

    // Wait for WYSIWYG editor to load
    const editor = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
    await expect(editor).toBeVisible({ timeout: 5000 });

    // Verify the ordered list is rendered
    await expect(editor.locator('ol')).toBeVisible({ timeout: 3000 });

    // Click inside the first list item
    await editor.locator('ol li p').first().dblclick();

    // The numbered list button should be active (indicating we're inside an ordered list)
    const numberedListButton = page.locator('button[title="Numbered List"]');
    await expect(numberedListButton).toHaveClass(/isActive/, { timeout: 3000 });

    // Click the button to toggle it off
    await numberedListButton.click();

    // The ordered list should be removed - items should become paragraphs
    await expect(editor.locator('ol')).not.toBeVisible({ timeout: 3000 });

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
    const createResponse = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: {
        title: 'Toggle Bullet List Test',
        content: '- First item\n- Second item\n- Third item',
      },
    });
    expect(createResponse.ok()).toBe(true);
    const pageData = await createResponse.json();

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Navigate to the page
    await page.locator(`[data-testid="sidebar-node-${pageData.id}"]`).click();

    // Wait for WYSIWYG editor to load
    const editor = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
    await expect(editor).toBeVisible({ timeout: 5000 });

    // Verify the bullet list is rendered
    await expect(editor.locator('ul')).toBeVisible({ timeout: 3000 });

    // Click inside the first list item
    await editor.locator('ul li p').first().dblclick();

    // The bullet list button should be active
    const bulletListButton = page.locator('button[title="Bullet List"]');
    await expect(bulletListButton).toHaveClass(/isActive/, { timeout: 3000 });

    // Click the button to toggle it off
    await bulletListButton.click();

    // The bullet list should be removed
    await expect(editor.locator('ul:not(.task-list)')).not.toBeVisible({ timeout: 3000 });

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
    const createResponse = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: {
        title: 'Toggle Task List Test',
        content: '- [ ] First task\n- [x] Second task\n- [ ] Third task',
      },
    });
    expect(createResponse.ok()).toBe(true);
    const pageData = await createResponse.json();

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Navigate to the page
    await page.locator(`[data-testid="sidebar-node-${pageData.id}"]`).click();

    // Wait for WYSIWYG editor to load
    const editor = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
    await expect(editor).toBeVisible({ timeout: 5000 });

    // Verify task list items are rendered
    await expect(editor.locator('li.task-list-item')).toHaveCount(3, { timeout: 3000 });

    // Click inside the first task list item and select text
    await editor.locator('li.task-list-item p').first().selectText();

    // The checkbox button should be active
    const checkboxButton = page.locator('button[title="Task List"]');
    await expect(checkboxButton).toHaveClass(/isActive/, { timeout: 3000 });

    // Click the button to toggle it off
    await checkboxButton.click();

    // The list should be completely removed (unwrapped to paragraphs)
    await expect(editor.locator('ul')).not.toBeVisible({ timeout: 3000 });

    // Content should now be paragraphs
    await expect(editor.locator('p')).toHaveCount(3, { timeout: 3000 });

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
    const createResponse = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: {
        title: 'Multi-line Numbered List Test',
        content: 'Line one\n\nLine two\n\nLine three',
      },
    });
    expect(createResponse.ok()).toBe(true);
    const pageData = await createResponse.json();

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
    const orderedListItems = editor.locator('ol li');
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
    const createResponse = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: {
        title: 'Multi-line Bullet List Test',
        content: 'Line one\n\nLine two\n\nLine three',
      },
    });
    expect(createResponse.ok()).toBe(true);
    const pageData = await createResponse.json();

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
    const bulletListItems = editor.locator('ul li');
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
    const createResponse = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: {
        title: 'Create Task List Test',
        content: 'Some regular text',
      },
    });
    expect(createResponse.ok()).toBe(true);
    const pageData = await createResponse.json();

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
    const paragraph = editor.locator('p').first();
    const box = await paragraph.boundingBox();
    expect(box).toBeTruthy();
    await page.mouse.dblclick(box!.x + 30, box!.y + box!.height / 2);

    // Wait for the floating toolbar to appear, then click the checkbox button
    const checkboxButton = page.locator('button[title="Task List"]');
    await expect(checkboxButton).toBeVisible({ timeout: 3000 });
    await checkboxButton.click();

    // Should create a task list item
    const taskItems = editor.locator('li.task-list-item');
    await expect(taskItems).toHaveCount(1, { timeout: 3000 });
  });

  test('clicking checkbox on bullet list converts to task list', async ({ page, request }) => {
    const { token } = await registerUser(request, 'toolbar-bullet-to-task');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

    const wsID = await getWorkspaceId(page);

    // Create a page with a bullet list
    const createResponse = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: {
        title: 'Bullet to Task Test',
        content: '- First item\n- Second item',
      },
    });
    expect(createResponse.ok()).toBe(true);
    const pageData = await createResponse.json();

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Navigate to the page
    await page.locator(`[data-testid="sidebar-node-${pageData.id}"]`).click();

    // Wait for WYSIWYG editor to load
    const editor = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
    await expect(editor).toBeVisible({ timeout: 5000 });

    // Click inside the first bullet item
    await editor.locator('ul li p').first().dblclick();

    // Click the checkbox button
    const checkboxButton = page.locator('button[title="Task List"]');
    await checkboxButton.click();

    // First item should become a task list item
    const firstLi = editor.locator('ul li').first();
    await expect(firstLi).toHaveClass(/task-list-item/, { timeout: 3000 });
  });
});

test.describe('Editor Toolbar Edge Cases', () => {
  test('selecting multiple task list items and clicking checkbox toggles all off', async ({ page, request }) => {
    const { token } = await registerUser(request, 'toolbar-multi-task-off');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

    const wsID = await getWorkspaceId(page);

    // Create a page with a task list
    const createResponse = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: {
        title: 'Multi Task Toggle Off Test',
        content: '- [ ] Task one\n- [x] Task two\n- [ ] Task three',
      },
    });
    expect(createResponse.ok()).toBe(true);
    const pageData = await createResponse.json();

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
    const createResponse = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: {
        title: 'Numbered to Task Test',
        content: '1. First item\n2. Second item\n3. Third item',
      },
    });
    expect(createResponse.ok()).toBe(true);
    const pageData = await createResponse.json();

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
    const taskItems = editor.locator('li.task-list-item');
    await expect(taskItems).toHaveCount(3, { timeout: 3000 });
  });

  test('converting bullet list to numbered list works', async ({ page, request }) => {
    const { token } = await registerUser(request, 'toolbar-ul-to-ol');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

    const wsID = await getWorkspaceId(page);

    // Create a page with a bullet list
    const createResponse = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: {
        title: 'Bullet to Numbered Test',
        content: '- First item\n- Second item\n- Third item',
      },
    });
    expect(createResponse.ok()).toBe(true);
    const pageData = await createResponse.json();

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    await page.locator(`[data-testid="sidebar-node-${pageData.id}"]`).click();

    const editor = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
    await expect(editor).toBeVisible({ timeout: 5000 });

    // Click inside the bullet list
    await editor.locator('ul li p').first().dblclick();

    // Click numbered list button
    const numberedListButton = page.locator('button[title="Numbered List"]');
    await numberedListButton.click();

    // Should be a numbered list now
    await expect(editor.locator('ol')).toBeVisible({ timeout: 3000 });
    await expect(editor.locator('ol li')).toHaveCount(3);
  });

  test('converting numbered list to bullet list works', async ({ page, request }) => {
    const { token } = await registerUser(request, 'toolbar-ol-to-ul');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

    const wsID = await getWorkspaceId(page);

    // Create a page with a numbered list
    const createResponse = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: {
        title: 'Numbered to Bullet Test',
        content: '1. First item\n2. Second item\n3. Third item',
      },
    });
    expect(createResponse.ok()).toBe(true);
    const pageData = await createResponse.json();

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Navigate to the page
    await page.locator(`[data-testid="sidebar-node-${pageData.id}"]`).click();

    const editor = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
    await expect(editor).toBeVisible({ timeout: 5000 });

    // Click inside the numbered list
    await editor.locator('ol li p').first().dblclick();

    // Click bullet list button
    const bulletListButton = page.locator('button[title="Bullet List"]');
    await bulletListButton.click();

    // Should be a bullet list now
    await expect(editor.locator('ul')).toBeVisible({ timeout: 3000 });
    await expect(editor.locator('ul li')).toHaveCount(3);
  });

  test('converting task list to numbered list works', async ({ page, request }) => {
    const { token } = await registerUser(request, 'toolbar-task-to-ol');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

    const wsID = await getWorkspaceId(page);

    // Create a page with a task list
    const createResponse = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: {
        title: 'Task to Numbered Test',
        content: '- [ ] Task one\n- [x] Task two\n- [ ] Task three',
      },
    });
    expect(createResponse.ok()).toBe(true);
    const pageData = await createResponse.json();

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    await page.locator(`[data-testid="sidebar-node-${pageData.id}"]`).click();

    // Wait for WYSIWYG editor to load
    const editor = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
    await expect(editor).toBeVisible({ timeout: 5000 });

    // Click inside the task list and select text
    await editor.locator('li.task-list-item p').first().selectText();

    // Click numbered list button
    const numberedListButton = page.locator('button[title="Numbered List"]');
    await numberedListButton.click();

    // Should be a numbered list now with no task items
    await expect(editor.locator('ol')).toBeVisible({ timeout: 3000 });
    await expect(editor.locator('ol li')).toHaveCount(3);
    await expect(editor.locator('li.task-list-item')).toHaveCount(0);
  });

  test('selection is preserved through all list type transitions', async ({ page, request }) => {
    const { token } = await registerUser(request, 'toolbar-sel-transitions');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

    const wsID = await getWorkspaceId(page);

    // Create a page with 4 lines of plain text
    const createResponse = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: {
        title: 'Selection Transitions Test',
        content: 'Line one\n\nLine two\n\nLine three\n\nLine four',
      },
    });
    expect(createResponse.ok()).toBe(true);
    const pageData = await createResponse.json();

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    await page.locator(`[data-testid="sidebar-node-${pageData.id}"]`).click();

    const editor = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
    await expect(editor).toBeVisible({ timeout: 5000 });

    const bulletButton = page.locator('button[title="Bullet List"]');
    const numberedButton = page.locator('button[title="Numbered List"]');
    const checkboxButton = page.locator('button[title="Task List"]');

    // Verify we have 4 paragraphs
    const paragraphs = editor.locator('p');
    await expect(paragraphs).toHaveCount(4, { timeout: 3000 });

    // Select lines 2 and 3 using keyboard: click in line 2, go to start, select to end of line 3
    await paragraphs.nth(1).click();
    await page.keyboard.press('Home'); // Go to start of line 2
    await page.keyboard.press('Shift+End'); // Select to end of line 2
    await page.keyboard.press('Shift+ArrowDown'); // Extend to start of line 3
    await page.keyboard.press('Shift+End'); // Select to end of line 3

    // Step 1: Click bullet list - should convert lines 2-3 to bullet list
    await bulletButton.click();
    await expect(editor.locator('ul > li')).toHaveCount(2, { timeout: 3000 });
    // Lines 1 and 4 remain as direct paragraph children (not in lists)
    await expect(editor.locator('> p')).toHaveCount(2);

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

    // Re-select the list items (click first list item, shift-click second)
    const listItems = editor.locator('ul > li');
    await listItems.first().click({ position: { x: 0, y: 5 } });
    await listItems.last().click({ position: { x: 100, y: 5 }, modifiers: ['Shift'] });

    // Step 2: Click numbered list - should convert to numbered list
    await numberedButton.click();
    await expect(editor.locator('ol > li')).toHaveCount(2, { timeout: 3000 });
    await expect(editor.locator('ul')).not.toBeVisible();

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
    const orderedItems = editor.locator('ol > li');
    await orderedItems.first().click({ position: { x: 0, y: 5 } });
    await orderedItems.last().click({ position: { x: 100, y: 5 }, modifiers: ['Shift'] });

    // Step 3: Click checkbox - should convert to task list
    await checkboxButton.click();
    await expect(editor.locator('li.task-list-item')).toHaveCount(2, { timeout: 3000 });

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
    const taskItems = editor.locator('li.task-list-item');
    await taskItems.first().click({ position: { x: 0, y: 5 } });
    await taskItems.last().click({ position: { x: 100, y: 5 }, modifiers: ['Shift'] });

    // Step 4: Click checkbox again - should toggle off to paragraphs
    await checkboxButton.click();
    await expect(editor.locator('ul')).not.toBeVisible({ timeout: 3000 });
    await expect(editor.locator('ol')).not.toBeVisible();
    await expect(editor.locator('p')).toHaveCount(4, { timeout: 3000 });

    // Verify selection is preserved: typing should replace the selected text (lines 2-3)
    await page.keyboard.type('Replaced');

    // Should have 3 paragraphs: Line one, Replaced, Line four
    await expect(editor.locator('p')).toHaveCount(3, { timeout: 3000 });

    // Verify markdown - lines 2-3 should be replaced with "Replaced"
    markdownEditor = await switchToMarkdownMode(page);
    markdown = await markdownEditor.inputValue();
    expect(markdown).toBe('Line one\n\nReplaced\n\nLine four');
  });


});

test.describe('Editor Toolbar Button States', () => {
  test('task list only highlights checkbox button, not bullet or numbered list buttons', async ({ page, request }) => {
    const { token } = await registerUser(request, 'toolbar-task-highlight');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

    const wsID = await getWorkspaceId(page);

    // Create a page with a task list
    const createResponse = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: {
        title: 'Task List Highlight Test',
        content: '- [ ] Task one\n- [x] Task two\n- [ ] Task three',
      },
    });
    expect(createResponse.ok()).toBe(true);
    const pageData = await createResponse.json();

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Navigate to the page
    await page.locator(`[data-testid="sidebar-node-${pageData.id}"]`).click();

    // Wait for WYSIWYG editor to load
    const editor = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
    await expect(editor).toBeVisible({ timeout: 5000 });

    // Click inside the first task list item and select text
    await editor.locator('li.task-list-item p').first().selectText();

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
    const createResponse = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: {
        title: 'Bullet List Highlight Test',
        content: '- Item one\n- Item two\n- Item three',
      },
    });
    expect(createResponse.ok()).toBe(true);
    const pageData = await createResponse.json();

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Navigate to the page
    await page.locator(`[data-testid="sidebar-node-${pageData.id}"]`).click();

    // Wait for WYSIWYG editor to load
    const editor = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
    await expect(editor).toBeVisible({ timeout: 5000 });

    // Click inside the first bullet list item and select text
    await editor.locator('ul li p').first().selectText();

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
    const createResponse = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: {
        title: 'Numbered List Highlight Test',
        content: '1. Item one\n2. Item two\n3. Item three',
      },
    });
    expect(createResponse.ok()).toBe(true);
    const pageData = await createResponse.json();

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Navigate to the page
    await page.locator(`[data-testid="sidebar-node-${pageData.id}"]`).click();

    // Wait for WYSIWYG editor to load
    const editor = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
    await expect(editor).toBeVisible({ timeout: 5000 });

    // Click inside the first numbered list item and select text
    await editor.locator('ol li p').first().selectText();

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