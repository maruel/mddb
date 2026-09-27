// Browser journeys for block handle menus and editor mutations.
//
// Note: Undo/redo tests are not included because keyboard shortcuts (Ctrl+Z/Ctrl+Y)
// are not currently bound in the editor. The prosemirror-history plugin tracks history
// but keybindings need to be added separately via keymap() with undo/redo commands.

import type { Page } from "@playwright/test";
import { test, expect, registerUser, getWorkspaceId, createClient } from "./helpers";

// Helper to setup editor with test content
async function setupEditorWithBlocks(page: Page, request: Parameters<typeof registerUser>[0]) {
  const { token } = await registerUser(request, "block-ctx");
  await page.goto(`/?token=${token}`);
  await expect(page.locator("aside")).toBeVisible({ timeout: 15000 });

  const wsId = await getWorkspaceId(page);

  // Create test page with simple blocks for context menu testing
  const markdownContent = `First paragraph

Second paragraph

Third paragraph`;

  const client = createClient(request, token);
  const pageResp = await client.ws(wsId).createPage("0", {
    title: "Context Menu Test",
    content: markdownContent,
  });

  await page.reload();
  await expect(page.locator("aside")).toBeVisible({ timeout: 10000 });
  await page.locator(`[data-testid="sidebar-node-${pageResp.id}"]`).click();
  await expect(page.locator('[data-testid="wysiwyg-editor"]')).toBeVisible({ timeout: 5000 });

  const prosemirror = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
  await expect(prosemirror).toBeVisible({ timeout: 5000 });

  return { prosemirror, wsId, nodeId: pageResp.id, token };
}

// Helper to open context menu on a block via right-click on its handle
async function openContextMenu(page: Page, blockIndex: number) {
  const prosemirror = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
  const blocks = prosemirror.locator(".block-row");
  const block = blocks.nth(blockIndex);
  await expect(block).toBeVisible({ timeout: 3000 });

  // Hover over the block to reveal the handle
  await block.hover();

  // Get the handle within this block and right-click on it
  const handle = block.locator('[data-testid="row-handle"]');
  await expect(handle).toBeVisible({ timeout: 3000 });
  await handle.click({ button: "right" });

  // Wait for context menu to appear
  const contextMenu = page.locator('[role="menu"][aria-label="Context menu"]');
  await expect(contextMenu).toBeVisible({ timeout: 3000 });

  return contextMenu;
}

test.describe("Block context menu interactions", () => {
  test("right-click on a block opens context menu", async ({ page, request }) => {
    await setupEditorWithBlocks(page, request);

    const prosemirror = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
    const blocks = prosemirror.locator(".block-row");
    await expect(blocks).toHaveCount(3);

    // Right-click on the first block
    const contextMenu = await openContextMenu(page, 0);

    // Verify context menu is visible and has expected items
    await expect(contextMenu).toBeVisible();
    const menuItemCount = await contextMenu.locator('[role="menuitem"]').count();
    expect(menuItemCount).toBeGreaterThan(0);

    // Verify basic menu items exist
    await expect(contextMenu.locator("text=Delete")).toBeVisible();
    await expect(contextMenu.locator("text=Duplicate")).toBeVisible();
  });

  test("delete block action works", async ({ page, request }) => {
    await setupEditorWithBlocks(page, request);

    const prosemirror = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
    const blocks = prosemirror.locator(".block-row");
    await expect(blocks).toHaveCount(3);

    // Get the text of the first block before deletion
    const firstBlockText = await blocks.nth(0).innerText();

    // Open context menu on first block
    const contextMenu = await openContextMenu(page, 0);

    // Click "Delete" (first menu item)
    await contextMenu.locator("text=Delete").first().click();

    // Context menu should close
    await expect(contextMenu).not.toBeVisible({ timeout: 3000 });

    // Should now have 2 blocks
    await expect(blocks).toHaveCount(2);

    // First block should now be what was the second block
    const newFirstBlockText = await blocks.nth(0).innerText();
    expect(newFirstBlockText).not.toBe(firstBlockText);
  });

  test("duplicate block action works", async ({ page, request }) => {
    await setupEditorWithBlocks(page, request);

    const prosemirror = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
    const blocks = prosemirror.locator(".block-row");
    await expect(blocks).toHaveCount(3);

    // Open context menu on first block
    const contextMenu = await openContextMenu(page, 0);

    // Click "Duplicate"
    await contextMenu.locator("text=Duplicate").first().click();

    // Context menu should close
    await expect(contextMenu).not.toBeVisible({ timeout: 3000 });

    // Should now have 4 blocks
    await expect(blocks).toHaveCount(4);

    // The first and second blocks should have the same content
    const blockTexts = await blocks.allInnerTexts();
    expect(blockTexts[0]).toBe(blockTexts[1]);
  });

  test("context menu shows correct options for single block", async ({ page, request }) => {
    await setupEditorWithBlocks(page, request);

    const contextMenu = await openContextMenu(page, 0);

    // Single block menu should have standard options
    await expect(contextMenu.locator("text=Delete")).toBeVisible();
    await expect(contextMenu.locator("text=Duplicate")).toBeVisible();
    await expect(contextMenu.locator("text=Indent")).toBeVisible();
    await expect(contextMenu.locator("text=Outdent")).toBeVisible();

    // Should have block type conversion options (with icons)
    await expect(contextMenu.locator("text=Paragraph")).toBeVisible();
    await expect(contextMenu.locator("text=Bullet list")).toBeVisible();
    await expect(contextMenu.locator("text=Numbered list")).toBeVisible();
  });

  test("convert Heading 1 to Heading 2 changes content element tag", async ({ page, request }) => {
    await setupEditorWithBlocks(page, request);

    const prosemirror = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');

    // First convert paragraph to Heading 1
    let contextMenu = await openContextMenu(page, 0);
    await contextMenu.locator("text=Heading 1").click();
    await expect(contextMenu).not.toBeVisible({ timeout: 3000 });

    // Verify it's h1
    const block = prosemirror.locator(".block-row").nth(0);
    const tag1 = await block.locator(".block-content").evaluate((el) => el.tagName.toLowerCase());
    expect(tag1).toBe("h1");

    // Now convert to Heading 2
    contextMenu = await openContextMenu(page, 0);
    await contextMenu.locator("text=Heading 2").click();
    await expect(contextMenu).not.toBeVisible({ timeout: 3000 });

    // Should now be <h2>
    const updatedBlock = prosemirror.locator(".block-row").nth(0);
    await expect(updatedBlock).toHaveAttribute("data-type", "heading");
    const tag2 = await updatedBlock.locator(".block-content").evaluate((el) => el.tagName.toLowerCase());
    expect(tag2).toBe("h2");
  });
});
