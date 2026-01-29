import { test, expect, registerUser, getWorkspaceId, switchToMarkdownMode, fillEditorContent } from './helpers';

test.describe('Page CRUD Operations', () => {
  test('delete a page - page removed from sidebar and content area cleared', async ({ page, request }) => {
    const { token } = await registerUser(request, 'delete-page');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    const wsID = await getWorkspaceId(page);

    // Create a page to delete
    const createResponse = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: {
        title: 'Page To Delete',
        content: 'This page will be deleted',
      },
    });
    expect(createResponse.ok()).toBe(true);
    const pageData = await createResponse.json();
    const pageID = pageData.id;

    // Reload to see the page
    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Navigate to the page
    const pageNode = page.locator(`[data-testid="sidebar-node-${pageID}"]`);
    await expect(pageNode).toBeVisible({ timeout: 5000 });
    await pageNode.click();
    await expect(page.getByText('This page will be deleted')).toBeVisible({ timeout: 5000 });

    // Set up dialog handler BEFORE any action that might trigger it
    // Use 'once' to handle exactly one dialog
    page.once('dialog', async (dialog) => {
      await dialog.accept();
    });

    // Hover over the sidebar node to reveal the delete button (🗑)
    await pageNode.hover();

    // Click the delete button (appears on hover)
    const deleteButton = pageNode.locator('button[class*="hoverDeleteButton"]');
    await expect(deleteButton).toBeVisible({ timeout: 2000 });
    await deleteButton.click({ force: true });

    // Wait for the page to be removed from sidebar
    await expect(pageNode).not.toBeVisible({ timeout: 5000 });

    // Content area should be cleared (no title input visible or shows different content)
    const titleInput = page.locator('input[placeholder*="Title"]');
    // Either the title input doesn't have the deleted page's title, or it's not visible
    await expect(async () => {
      const isVisible = await titleInput.isVisible();
      if (isVisible) {
        const value = await titleInput.inputValue();
        expect(value).not.toBe('Page To Delete');
      }
    }).toPass({ timeout: 5000 });

    // Verify via API that the page no longer exists
    const getResponse = await request.get(`/api/workspaces/${wsID}/nodes/${pageID}/page`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    expect(getResponse.ok()).toBe(false);
    expect(getResponse.status()).toBe(404);
  });

  test('page title updates in sidebar as user types (real-time sync)', async ({ page, request }) => {
    const { token } = await registerUser(request, 'sidebar-sync');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    const wsID = await getWorkspaceId(page);

    // Create a page
    const createResponse = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: {
        title: 'Original Title',
        content: 'Content here',
      },
    });
    expect(createResponse.ok()).toBe(true);
    const pageData = await createResponse.json();
    const pageID = pageData.id;

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Navigate to the page
    const pageNode = page.locator(`[data-testid="sidebar-node-${pageID}"]`);
    await pageNode.click();

    // Wait for title input to be ready with correct value
    const titleInput = page.locator('input[placeholder*="Title"]');
    await expect(titleInput).toHaveValue('Original Title', { timeout: 5000 });

    // Get sidebar text element - title is in span with class pageTitleText
    const sidebarTitle = pageNode.locator('[class*="pageTitleText"]');
    await expect(sidebarTitle).toContainText('Original Title');

    // Type a new title
    await titleInput.fill('Updated Title');

    // Sidebar should update immediately (optimistic update)
    await expect(sidebarTitle).toContainText('Updated Title', { timeout: 5000 });
  });

  test('unsaved indicator appears when editing and disappears after save', async ({ page, request }) => {
    const { token } = await registerUser(request, 'unsaved-ind');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    const wsID = await getWorkspaceId(page);

    // Create a page
    const createResponse = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: {
        title: 'Test Page',
        content: 'Initial content',
      },
    });
    expect(createResponse.ok()).toBe(true);
    const pageData = await createResponse.json();
    const pageID = pageData.id;

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Navigate to the page
    await page.locator(`[data-testid="sidebar-node-${pageID}"]`).click();
    await expect(page.getByText('Initial content')).toBeVisible({ timeout: 5000 });

    // Initially, no unsaved indicator (use class selector)
    const unsavedIndicator = page.locator('[class*="unsavedIndicator"]');
    await expect(unsavedIndicator).not.toBeVisible();

    // Edit the content (switch to markdown mode for reliable interaction)
    await fillEditorContent(page, 'Modified content');

    // Unsaved indicator should appear
    await expect(unsavedIndicator).toBeVisible({ timeout: 2000 });

    // Wait for autosave to complete - the unsaved indicator should disappear
    // (saving indicator may flash too quickly to catch reliably)
    await expect(unsavedIndicator).not.toBeVisible({ timeout: 10000 });

    // Verify content was saved via API
    const getResponse = await request.get(`/api/workspaces/${wsID}/nodes/${pageID}/page`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    const savedData = await getResponse.json();
    expect(savedData.content).toBe('Modified content');
  });
});

test.describe('Page Navigation', () => {
  test.screenshot('browser back button navigates between pages', async ({ page, request, takeScreenshot }) => {
    const { token } = await registerUser(request, 'browser-nav');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    const wsID = await getWorkspaceId(page);

    // Create two pages
    const page1Response = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { title: 'Nav Page 1', content: 'Content of page 1' },
    });
    const page1Data = await page1Response.json();

    const page2Response = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { title: 'Nav Page 2', content: 'Content of page 2' },
    });
    const page2Data = await page2Response.json();

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });
    await takeScreenshot('workspace-with-pages');

    // Navigate to page 1
    await page.locator(`[data-testid="sidebar-node-${page1Data.id}"]`).click();
    await expect(page.getByText('Content of page 1')).toBeVisible({ timeout: 5000 });
    await takeScreenshot('page1-view');

    // Navigate to page 2
    await page.locator(`[data-testid="sidebar-node-${page2Data.id}"]`).click();
    await expect(page.getByText('Content of page 2')).toBeVisible({ timeout: 5000 });
    await takeScreenshot('page2-view');

    // Click browser back button
    await page.goBack();

    // Should show page 1 again
    await expect(page.getByText('Content of page 1')).toBeVisible({ timeout: 5000 });

    // Forward button should return to page 2
    await page.goForward();
    await expect(page.getByText('Content of page 2')).toBeVisible({ timeout: 5000 });
  });

  test('URL updates with page slug when navigating', async ({ page, request }) => {
    const { token } = await registerUser(request, 'url-slug');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    const wsID = await getWorkspaceId(page);

    // Create a page with a specific title
    const createResponse = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { title: 'My Awesome Page', content: 'Content here' },
    });
    const pageData = await createResponse.json();

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Navigate to the page
    await page.locator(`[data-testid="sidebar-node-${pageData.id}"]`).click();
    await expect(page.getByText('Content here')).toBeVisible({ timeout: 5000 });

    // URL should contain workspace ID and page ID with slug
    await expect(page).toHaveURL(new RegExp(`/w/${wsID}[^/]*/${pageData.id}\\+my-awesome-page`));
  });

  test('direct URL navigation loads correct page', async ({ page, request }) => {
    const { token } = await registerUser(request, 'direct-url');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    const wsID = await getWorkspaceId(page);

    // Create a page
    const createResponse = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { title: 'Direct URL Page', content: 'Loaded via direct URL' },
    });
    const pageData = await createResponse.json();

    // Navigate directly to the page URL
    await page.goto(`/w/${wsID}/${pageData.id}?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Page content should be visible
    await expect(page.getByText('Loaded via direct URL')).toBeVisible({ timeout: 5000 });

    // Title input should have the correct value
    const titleInput = page.locator('input[placeholder*="Title"]');
    await expect(titleInput).toHaveValue('Direct URL Page');
  });

  test('breadcrumb navigation works for nested pages', async ({ page, request }) => {
    const { token } = await registerUser(request, 'breadcrumb');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    const wsID = await getWorkspaceId(page);

    // Create parent page
    const parentResponse = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { title: 'Alpha', content: 'Parent content' },
    });
    const parentData = await parentResponse.json();

    // Create child page
    const childResponse = await request.post(`/api/workspaces/${wsID}/nodes/${parentData.id}/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { title: 'Beta', content: 'Child content' },
    });
    const childData = await childResponse.json();

    // Create grandchild page
    const grandchildResponse = await request.post(`/api/workspaces/${wsID}/nodes/${childData.id}/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { title: 'Gamma', content: 'Grandchild content' },
    });
    const grandchildData = await grandchildResponse.json();

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Expand hierarchy and navigate to grandchild
    await page.locator(`[data-testid="sidebar-node-${parentData.id}"]`).click();
    await expect(page.locator(`[data-testid="sidebar-node-${childData.id}"]`)).toBeVisible({ timeout: 5000 });
    // Click the expand icon (verify it's visible and has adequate clickable size)
    const expandIcon = page.locator(`[data-testid="expand-icon-${childData.id}"]`);
    await expect(expandIcon).toBeVisible();
    const box = await expandIcon.boundingBox();
    expect(box).toBeTruthy();
    expect(box!.width).toBeGreaterThanOrEqual(16);
    expect(box!.height).toBeGreaterThanOrEqual(16);
    await expandIcon.click();
    await expect(page.locator(`[data-testid="sidebar-node-${grandchildData.id}"]`)).toBeVisible({ timeout: 5000 });
    await page.locator(`[data-testid="sidebar-node-${grandchildData.id}"]`).click();

    await expect(page.getByText('Grandchild content')).toBeVisible({ timeout: 5000 });

    // Check breadcrumbs are visible (use exact match)
    const breadcrumbs = page.locator('nav[class*="breadcrumbs"]');
    await expect(breadcrumbs.getByText('Alpha', { exact: true })).toBeVisible();
    await expect(breadcrumbs.getByText('Beta', { exact: true })).toBeVisible();
    await expect(breadcrumbs.getByText('Gamma', { exact: true })).toBeVisible();

    // Click on parent breadcrumb
    await breadcrumbs.getByText('Alpha', { exact: true }).click();

    // Should navigate to parent
    await expect(page.getByText('Parent content')).toBeVisible({ timeout: 5000 });
  });
});

test.describe('Editor Features', () => {
  test.screenshot('WYSIWYG editor renders markdown correctly', async ({ page, request, takeScreenshot }) => {
    const { token } = await registerUser(request, 'wysiwyg-editor');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    const wsID = await getWorkspaceId(page);

    // Create a page with markdown content
    const createResponse = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: {
        title: 'Markdown Test',
        content: '# Heading 1\n\n**Bold text**\n\n- List item 1\n- List item 2\n\n`code inline`',
      },
    });
    const pageData = await createResponse.json();

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    await page.locator(`[data-testid="sidebar-node-${pageData.id}"]`).click();

    // Wait for WYSIWYG editor to load and render content
    const editor = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
    await expect(editor).toBeVisible({ timeout: 5000 });

    // Check for rendered markdown elements in WYSIWYG editor
    await expect(editor.locator('h1')).toContainText('Heading 1');
    await expect(editor.locator('strong')).toContainText('Bold text');
    await expect(editor.locator('li').first()).toContainText('List item 1');
    await expect(editor.locator('code')).toContainText('code inline');

    await takeScreenshot('wysiwyg-editor');
  });

  test.screenshot(
    'WYSIWYG to markdown round-trip preserves all formatting',
    async ({ page, request, takeScreenshot }) => {
      const { token } = await registerUser(request, 'round-trip');
      await page.goto(`/?token=${token}`);
      await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

      const wsID = await getWorkspaceId(page);

      // Create an empty page via API
      const createResponse = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
        headers: { Authorization: `Bearer ${token}` },
        data: { title: 'Round Trip Test', content: '' },
      });
      expect(createResponse.ok()).toBe(true);
      const pageData = await createResponse.json();

      // Reload and verify in UI
      await page.reload();
      await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

      await page.locator(`[data-testid="sidebar-node-${pageData.id}"]`).click();

      // Wait for WYSIWYG editor
      const editor = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
      await expect(editor).toBeVisible({ timeout: 5000 });

      // === Switch to Markdown mode and enter content directly ===
      await page.locator('[data-testid="editor-mode-markdown"]').click();
      const markdownEditor = page.locator('[data-testid="markdown-editor"]');
      await expect(markdownEditor).toBeVisible({ timeout: 3000 });

      // Define comprehensive markdown content with all formatting styles
      const originalMarkdown = `# Heading One

## Heading Two

### Heading Three

This is **bold text** in a paragraph.

This is *italic text* in a paragraph.

Inline \`code\` here.

- First bullet
- Second bullet
- Third bullet

1. First item
2. Second item
3. Third item

- [ ] Unchecked task
- [x] Checked task
- [ ] Another unchecked task

> This is a blockquote
> with multiple lines

\`\`\`
const x = 42;
function hello() {
  return "world";
}
\`\`\`

---

[Link text](https://example.com)`;

      // Fill the markdown editor
      await markdownEditor.fill(originalMarkdown);

      await takeScreenshot('markdown-original');

      // === Switch to Visual mode ===
      await page.locator('[data-testid="editor-mode-visual"]').click();
      await expect(editor).toBeVisible({ timeout: 3000 });

      await takeScreenshot('wysiwyg-rendered');

      // Verify all elements render correctly in WYSIWYG
      await expect(editor.locator('h1')).toContainText('Heading One', { timeout: 5000 });
      await expect(editor.locator('h2')).toContainText('Heading Two', { timeout: 3000 });
      await expect(editor.locator('h3')).toContainText('Heading Three', { timeout: 3000 });
      await expect(editor.locator('strong')).toContainText('bold text', { timeout: 3000 });
      await expect(editor.locator('em')).toContainText('italic text', { timeout: 3000 });
      await expect(editor.locator('p code')).toContainText('code', { timeout: 3000 });
      await expect(editor.locator('ul li').first()).toContainText('First bullet', { timeout: 3000 });
      await expect(editor.locator('ul li').nth(1)).toContainText('Second bullet', { timeout: 3000 });
      await expect(editor.locator('ol li').first()).toContainText('First item', { timeout: 3000 });
      await expect(editor.locator('ol li').nth(1)).toContainText('Second item', { timeout: 3000 });

      // Verify task list items (checkboxes)
      const taskItems = editor.locator('li.task-list-item');
      await expect(taskItems).toHaveCount(3, { timeout: 3000 });
      await expect(taskItems.first()).toContainText('Unchecked task', { timeout: 3000 });
      await expect(taskItems.nth(1)).toContainText('Checked task', { timeout: 3000 });
      // Verify checkbox states via data-checked attribute
      await expect(taskItems.first()).toHaveAttribute('data-checked', 'false');
      await expect(taskItems.nth(1)).toHaveAttribute('data-checked', 'true');
      await expect(taskItems.nth(2)).toHaveAttribute('data-checked', 'false');

      await expect(editor.locator('blockquote')).toContainText('This is a blockquote', { timeout: 3000 });
      await expect(editor.locator('pre code')).toContainText('const x = 42;', { timeout: 3000 });
      await expect(editor.locator('hr')).toBeVisible({ timeout: 3000 });
      await expect(editor.locator('a[href="https://example.com"]')).toContainText('Link text', { timeout: 3000 });

      // === Switch back to Markdown mode ===
      await page.locator('[data-testid="editor-mode-markdown"]').click();
      await expect(markdownEditor).toBeVisible({ timeout: 3000 });

      const markdownAfterRoundTrip = await markdownEditor.inputValue();

      await takeScreenshot('markdown-after-round-trip');

      // Verify markdown still contains all expected elements after round-trip
      expect(markdownAfterRoundTrip).toContain('# Heading One');
      expect(markdownAfterRoundTrip).toContain('## Heading Two');
      expect(markdownAfterRoundTrip).toContain('### Heading Three');
      expect(markdownAfterRoundTrip).toContain('**bold text**');
      expect(markdownAfterRoundTrip).toContain('*italic text*');
      expect(markdownAfterRoundTrip).toContain('`code`');
      expect(markdownAfterRoundTrip).toContain('- First bullet');
      expect(markdownAfterRoundTrip).toContain('- Second bullet');
      expect(markdownAfterRoundTrip).toContain('1. First item');
      expect(markdownAfterRoundTrip).toContain('2. Second item');
      // Verify task list syntax is preserved
      expect(markdownAfterRoundTrip).toContain('[ ] Unchecked task');
      expect(markdownAfterRoundTrip).toContain('[x] Checked task');
      expect(markdownAfterRoundTrip).toContain('[ ] Another unchecked task');
      expect(markdownAfterRoundTrip).toContain('> This is a blockquote');
      expect(markdownAfterRoundTrip).toContain('```');
      expect(markdownAfterRoundTrip).toContain('const x = 42;');
      expect(markdownAfterRoundTrip).toContain('---');
      expect(markdownAfterRoundTrip).toContain('[Link text](https://example.com)');

      // === Switch to Visual one more time to confirm stability ===
      await page.locator('[data-testid="editor-mode-visual"]').click();
      await expect(editor).toBeVisible({ timeout: 3000 });

      // All elements should still be present
      await expect(editor.locator('h1')).toContainText('Heading One', { timeout: 5000 });
      await expect(editor.locator('strong')).toContainText('bold text', { timeout: 3000 });
      await expect(editor.locator('pre code')).toContainText('const x = 42;', { timeout: 3000 });
      // Task list items should still be present with correct states
      const finalTaskItems = editor.locator('li.task-list-item');
      await expect(finalTaskItems).toHaveCount(3, { timeout: 3000 });
      await expect(finalTaskItems.nth(1)).toHaveAttribute('data-checked', 'true');

      await takeScreenshot('wysiwyg-final');
    }
  );

  test('markdown editor fills available vertical space', async ({ page, request }) => {
    const { token } = await registerUser(request, 'editor-height');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    const wsID = await getWorkspaceId(page);

    // Create a page with 10 lines of content
    const multiLineContent = Array.from({ length: 10 }, (_, i) => `Line ${i + 1} of content`).join('\n');
    const createResponse = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { title: 'Height Test', content: multiLineContent },
    });
    expect(createResponse.ok()).toBe(true);
    const pageData = await createResponse.json();

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Navigate to the page
    await page.locator(`[data-testid="sidebar-node-${pageData.id}"]`).click();

    // Switch to markdown mode
    await page.locator('[data-testid="editor-mode-markdown"]').click();
    const markdownEditor = page.locator('[data-testid="markdown-editor"]');
    await expect(markdownEditor).toBeVisible({ timeout: 3000 });

    // Get bounding boxes
    const editorBox = await markdownEditor.boundingBox();
    expect(editorBox).toBeTruthy();

    // Get the editor container (parent of toolbar and editor)
    const editorContainer = page.locator('[data-testid="markdown-editor"]').locator('..');
    const containerBox = await editorContainer.boundingBox();
    expect(containerBox).toBeTruthy();

    // The markdown editor should take up significant vertical space
    // It should be at least 200px tall (reasonable minimum for an editor)
    expect(editorBox!.height).toBeGreaterThan(200);

    // The editor should extend close to the bottom of its container
    // Allow some margin for toolbar (roughly 50px) and padding
    const editorBottom = editorBox!.y + editorBox!.height;
    const containerBottom = containerBox!.y + containerBox!.height;
    const bottomGap = containerBottom - editorBottom;

    // The gap between editor bottom and container bottom should be small (< 20px for padding)
    expect(bottomGap).toBeLessThan(20);
  });

  test.screenshot('slash command menu appears and applies block types', async ({ page, request, takeScreenshot }) => {
    const { token } = await registerUser(request, 'slash-cmd');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    const wsID = await getWorkspaceId(page);

    // Create an empty page
    const createResponse = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { title: 'Slash Command Test', content: '' },
    });
    if (!createResponse.ok()) {
      const body = await createResponse.text();
      throw new Error(`Failed to create page: ${createResponse.status()} - ${body}`);
    }
    const pageData = await createResponse.json();

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Navigate to the page
    await page.locator(`[data-testid="sidebar-node-${pageData.id}"]`).click();

    // Wait for WYSIWYG editor to load
    const editor = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
    await expect(editor).toBeVisible({ timeout: 5000 });

    // Click in the editor to focus it
    await editor.click();

    // Type "/" to trigger slash menu
    await page.keyboard.type('/');

    // Slash menu should appear
    const slashMenu = page.locator('[data-testid="slash-command-menu"]');
    await expect(slashMenu).toBeVisible({ timeout: 3000 });
    await takeScreenshot('slash-menu-visible');

    // Menu should show command options
    const menuItems = slashMenu.locator('[class*="slashMenuItem"]');
    await expect(menuItems).toHaveCount(10, { timeout: 3000 });

    // First item should be selected by default
    await expect(menuItems.first()).toHaveClass(/selected/);

    // Arrow down should move selection to second item
    await page.keyboard.press('ArrowDown');
    await expect(menuItems.nth(0)).not.toHaveClass(/selected/);
    await expect(menuItems.nth(1)).toHaveClass(/selected/);

    // Arrow down again to third item
    await page.keyboard.press('ArrowDown');
    await expect(menuItems.nth(2)).toHaveClass(/selected/);

    // Arrow up should go back to second item
    await page.keyboard.press('ArrowUp');
    await expect(menuItems.nth(1)).toHaveClass(/selected/);

    // Type to filter commands (this will reset selection)
    await page.keyboard.type('head');
    await takeScreenshot('slash-menu-filtered');

    // Should show only heading commands (heading1, heading2, heading3)
    await expect(slashMenu.locator('[class*="slashMenuItem"]')).toHaveCount(3, { timeout: 3000 });

    // Press Enter to select first option (Heading 1)
    await page.keyboard.press('Enter');

    // Menu should close
    await expect(slashMenu).not.toBeVisible({ timeout: 3000 });

    // Editor should now have an h1 element
    await expect(editor.locator('h1')).toBeVisible({ timeout: 3000 });
    await takeScreenshot('heading1-applied');

    // Type some content in the heading
    await page.keyboard.type('My Heading');
    await expect(editor.locator('h1')).toContainText('My Heading');

    // Press Enter to create new paragraph, then type "/" again
    await page.keyboard.press('Enter');
    await page.keyboard.type('/');
    await expect(slashMenu).toBeVisible({ timeout: 3000 });

    // Use arrow keys to navigate and select "Bullet List"
    await page.keyboard.type('bullet');
    await expect(slashMenu.locator('[class*="slashMenuItem"]')).toHaveCount(1, { timeout: 3000 });
    await page.keyboard.press('Enter');

    // Menu should close
    await expect(slashMenu).not.toBeVisible({ timeout: 3000 });

    // Editor should have a bullet list
    await expect(editor.locator('ul')).toBeVisible({ timeout: 3000 });
    await takeScreenshot('bullet-list-applied');

    // Test Escape to close menu without selecting
    await page.keyboard.press('Enter');
    await page.keyboard.type('/');
    await expect(slashMenu).toBeVisible({ timeout: 3000 });
    await page.keyboard.press('Escape');
    await expect(slashMenu).not.toBeVisible({ timeout: 3000 });
    await takeScreenshot('slash-menu-escaped');

    // Test clicking outside to close menu
    await page.keyboard.type('/');
    await expect(slashMenu).toBeVisible({ timeout: 3000 });
    // Click on the title input to close menu
    await page.locator('input[placeholder*="Title"]').click();
    await expect(slashMenu).not.toBeVisible({ timeout: 3000 });

    // Verify slash command does NOT trigger mid-word
    await editor.click();
    await page.keyboard.press('End'); // Go to end of current line
    await page.keyboard.press('Enter'); // Start a fresh line
    await page.keyboard.type('test/noslash');
    await expect(slashMenu).not.toBeVisible({ timeout: 1000 });
  });

  test.screenshot('slash menu stays visible when cursor is near bottom of viewport', async ({
    page,
    request,
    takeScreenshot,
  }) => {
    const { token } = await registerUser(request, 'slash-bottom');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    const wsID = await getWorkspaceId(page);

    // Create a page with lots of content to push cursor near bottom
    const manyLines = Array(30).fill('This is a line of text to fill the page.').join('\n\n');
    const createResponse = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { title: 'Bottom Slash Test', content: manyLines },
    });
    if (!createResponse.ok()) {
      const body = await createResponse.text();
      throw new Error(`Failed to create page: ${createResponse.status()} - ${body}`);
    }
    const pageData = await createResponse.json();

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Navigate to the page
    await page.locator(`[data-testid="sidebar-node-${pageData.id}"]`).click();

    // Wait for WYSIWYG editor to load
    const editor = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
    await expect(editor).toBeVisible({ timeout: 5000 });

    // Click at the end of the very last paragraph using evaluate
    // This ensures we get the actual DOM element at click time
    await editor.evaluate((el) => {
      const paragraphs = el.querySelectorAll('p');
      if (paragraphs.length > 0) {
        const lastP = paragraphs[paragraphs.length - 1];
        // Scroll the last paragraph into view
        lastP.scrollIntoView({ block: 'center' });
        // Create a range at the end of the last paragraph
        const range = document.createRange();
        range.selectNodeContents(lastP);
        range.collapse(false); // Collapse to end
        // Set the selection
        const sel = window.getSelection();
        sel?.removeAllRanges();
        sel?.addRange(range);
        // Focus the editor
        (el as HTMLElement).focus();
      }
    });

    // Press Enter to create a new line at the bottom (so "/" is at line start)
    await page.keyboard.press('Enter');

    await takeScreenshot('cursor-at-bottom');

    // Type "/" to trigger slash menu
    await page.keyboard.type('/');

    // Slash menu should appear
    const slashMenu = page.locator('[data-testid="slash-command-menu"]');
    await expect(slashMenu).toBeVisible({ timeout: 3000 });
    await takeScreenshot('slash-menu-at-bottom');

    // Get viewport height
    const viewportSize = page.viewportSize();
    const viewportHeight = viewportSize?.height ?? 720;

    // Check that the menu is fully visible within the viewport
    const menuBox = await slashMenu.boundingBox();
    expect(menuBox).toBeTruthy();

    // The menu should not extend beyond the viewport bottom
    const menuBottom = menuBox!.y + menuBox!.height;
    expect(menuBottom).toBeLessThanOrEqual(viewportHeight);

    // Navigate down through all items and verify each selected item is visible
    const menuItems = slashMenu.locator('[class*="slashMenuItem"]');
    const itemCount = await menuItems.count();

    for (let i = 1; i < itemCount; i++) {
      await page.keyboard.press('ArrowDown');

      // The selected item should be visible within the menu's scroll area
      const selectedItem = menuItems.nth(i);
      await expect(selectedItem).toHaveClass(/selected/);

      // Verify the selected item is within the viewport
      const itemBox = await selectedItem.boundingBox();
      expect(itemBox).toBeTruthy();
      expect(itemBox!.y).toBeGreaterThanOrEqual(0);
      expect(itemBox!.y + itemBox!.height).toBeLessThanOrEqual(viewportHeight);
    }

    await takeScreenshot('slash-menu-scrolled-to-last');
  });

  test.screenshot('version history loads and displays commits', async ({ page, request, takeScreenshot }) => {
    const { token } = await registerUser(request, 'version-history');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    const wsID = await getWorkspaceId(page);

    // Create a page
    const createResponse = await request.post(`/api/workspaces/${wsID}/nodes/0/page/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { title: 'History Test', content: 'Initial content' },
    });
    expect(createResponse.ok()).toBe(true);
    const pageData = await createResponse.json();
    expect(pageData.id).toBeTruthy();

    // Update the page a few times to create history
    for (let i = 1; i <= 3; i++) {
      await request.post(`/api/workspaces/${wsID}/nodes/${pageData.id}/page`, {
        headers: { Authorization: `Bearer ${token}` },
        data: { title: 'History Test', content: `Content version ${i}` },
      });
    }

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Right-click on the sidebar node to open context menu
    const sidebarNode = page.locator(`[data-testid="sidebar-node-${pageData.id}"]`);
    await sidebarNode.click({ button: 'right' });

    // Click History option in context menu (has clock emoji prefix)
    const historyButton = page.locator('button', { hasText: /🕐.*History/ });
    await expect(historyButton).toBeVisible({ timeout: 3000 });
    await historyButton.click();

    // History panel should appear
    const historyPanel = page.locator('[class*="historyPanel"]');
    await expect(historyPanel).toBeVisible({ timeout: 5000 });

    // Should show commits (initial create + 3 updates = 4 commits)
    const historyItems = historyPanel.locator('li[class*="historyItem"]');
    await expect(historyItems).toHaveCount(4, { timeout: 5000 });

    await takeScreenshot('version-history-panel');
  });
});
