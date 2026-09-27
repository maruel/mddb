// Browser journeys for toolbar selection, block conversion, and markdown mode.

import { test, expect, registerUser, getWorkspaceId, switchToMarkdownMode, createClient } from "./helpers";

test("selecting multiple lines and clicking Checkbox converts all lines to task list", async ({ page, request }) => {
  const { token } = await registerUser(request, "toolbar-checkbox-multi");
  await page.goto(`/?token=${token}`);
  await expect(page.locator("aside")).toBeVisible({ timeout: 15000 });

  const wsID = await getWorkspaceId(page);

  // Create a page with multiple lines
  const client = createClient(request, token);
  const pageData = await client.ws(wsID).createPage("0", {
    title: "Multi-line Checkbox Test",
    content: "Line one\n\nLine two\n\nLine three",
  });

  await page.reload();
  await expect(page.locator("aside")).toBeVisible({ timeout: 10000 });

  // Navigate to the page
  await page.locator(`[data-testid="sidebar-node-${pageData.id}"]`).click();

  // Wait for WYSIWYG editor to load
  const editor = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
  await expect(editor).toBeVisible({ timeout: 5000 });

  // Select all text in the editor using keyboard
  await editor.click();
  await page.keyboard.press("Control+a");

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

test("underline formatting is preserved when switching to markdown mode", async ({ page, request }) => {
  const { token } = await registerUser(request, "toolbar-underline");
  await page.goto(`/?token=${token}`);
  await expect(page.locator("aside")).toBeVisible({ timeout: 15000 });

  const wsID = await getWorkspaceId(page);

  // Create a page with plain text
  const client = createClient(request, token);
  const pageData = await client.ws(wsID).createPage("0", {
    title: "Underline Test",
    content: "Some text to underline",
  });

  await page.reload();
  await expect(page.locator("aside")).toBeVisible({ timeout: 10000 });

  // Navigate to the page
  await page.locator(`[data-testid="sidebar-node-${pageData.id}"]`).click();

  // Wait for WYSIWYG editor to load
  const editor = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
  await expect(editor).toBeVisible({ timeout: 5000 });
  await expect(editor.locator("p")).toContainText("Some text", { timeout: 5000 });

  // Select text using mouse drag
  const paragraph = editor.locator('.block-row[data-type="paragraph"] .block-content').first();
  await paragraph.selectText();

  // Wait for the floating toolbar to appear
  const underlineButton = page.locator('button[title="Underline (Ctrl+U)"]');
  await expect(underlineButton).toBeVisible({ timeout: 3000 });

  // Click the underline button
  await underlineButton.click();

  // Verify the text is underlined in the editor
  await expect(editor.locator("u")).toBeVisible({ timeout: 3000 });

  // Switch to markdown mode and verify underline syntax
  const markdownEditor = await switchToMarkdownMode(page);
  const markdown = await markdownEditor.inputValue();

  // Should have <u> tags for underline
  expect(markdown).toContain("<u>");
  expect(markdown).toContain("</u>");
  expect(markdown).toMatch(/<u>Some text to underline<\/u>/);
});
