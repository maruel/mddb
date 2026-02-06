// E2E tests for block context menu interactions.
//
// Note: Undo/redo tests are not included because keyboard shortcuts (Ctrl+Z/Ctrl+Y)
// are not currently bound in the editor. The prosemirror-history plugin tracks history
// but keybindings need to be added separately via keymap() with undo/redo commands.

import type { Page } from '@playwright/test';
import { test, expect, registerUser, getWorkspaceId, createClient } from './helpers';

// Helper to setup editor with test content
async function setupEditorWithBlocks(page: Page, request: Parameters<typeof registerUser>[0]) {
  const { token } = await registerUser(request, 'block-ctx');
  await page.goto(`/?token=${token}`);
  await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

  const wsId = await getWorkspaceId(page);

  // Create test page with simple blocks for context menu testing
  const markdownContent = `First paragraph

Second paragraph

Third paragraph`;

  const client = createClient(request, token);
  const pageResp = await client.ws(wsId).nodes.page.createPage('0', {
    title: 'Context Menu Test',
    content: markdownContent,
  });

  await page.reload();
  await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });
  await page.locator(`[data-testid="sidebar-node-${pageResp.id}"]`).click();
  await expect(page.locator('[data-testid="wysiwyg-editor"]')).toBeVisible({ timeout: 5000 });

  const prosemirror = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
  await expect(prosemirror).toBeVisible({ timeout: 5000 });

  return { prosemirror, wsId, nodeId: pageResp.id, token };
}

// Helper to open context menu on a block via right-click on its handle
async function openContextMenu(page: Page, blockIndex: number) {
  const prosemirror = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
  const blocks = prosemirror.locator('.block-row');
  const block = blocks.nth(blockIndex);
  await expect(block).toBeVisible({ timeout: 3000 });

  // Hover over the block to reveal the handle
  await block.hover();

  // Get the handle within this block and right-click on it
  const handle = block.locator('[data-testid="row-handle"]');
  await expect(handle).toBeVisible({ timeout: 3000 });
  await handle.click({ button: 'right' });

  // Wait for context menu to appear
  const contextMenu = page.locator('[role="menu"][aria-label="Context menu"]');
  await expect(contextMenu).toBeVisible({ timeout: 3000 });

  return contextMenu;
}

test.describe('Block context menu interactions', () => {
  test('right-click on a block opens context menu', async ({ page, request }) => {
    await setupEditorWithBlocks(page, request);

    const prosemirror = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
    const blocks = prosemirror.locator('.block-row');
    await expect(blocks).toHaveCount(3);

    // Right-click on the first block
    const contextMenu = await openContextMenu(page, 0);

    // Verify context menu is visible and has expected items
    await expect(contextMenu).toBeVisible();
    const menuItemCount = await contextMenu.locator('[role="menuitem"]').count();
    expect(menuItemCount).toBeGreaterThan(0);

    // Verify basic menu items exist
    await expect(contextMenu.locator('text=Delete')).toBeVisible();
    await expect(contextMenu.locator('text=Duplicate')).toBeVisible();
  });

  test('hover each menu item highlights it', async ({ page, request }) => {
    await setupEditorWithBlocks(page, request);

    const contextMenu = await openContextMenu(page, 0);

    // Get all menu items
    const menuItems = contextMenu.locator('[role="menuitem"]');
    const itemCount = await menuItems.count();
    expect(itemCount).toBeGreaterThan(0);

    // Hover over each item and verify it gets the focused class
    for (let i = 0; i < Math.min(5, itemCount); i++) {
      const item = menuItems.nth(i);
      await item.hover();

      // The focused item should have the focused class (via classList in SolidJS)
      // Check that the item has the background color change via computed style
      const hasFocusedStyle = await item.evaluate((el) => {
        const style = window.getComputedStyle(el);
        const classes = el.className;
        // The item should have the 'focused' class when hovered
        return classes.includes('focused') || style.backgroundColor !== 'rgba(0, 0, 0, 0)';
      });
      expect(hasFocusedStyle).toBe(true);
    }
  });

  test('arrow down key navigates to next item', async ({ page, request }) => {
    await setupEditorWithBlocks(page, request);

    const contextMenu = await openContextMenu(page, 0);
    const menuItems = contextMenu.locator('[role="menuitem"]');

    // Initially, first item should be focused (index 0)
    const firstItem = menuItems.nth(0);
    await expect(firstItem).toHaveClass(/focused/);

    // Press arrow down to move to next item
    await page.keyboard.press('ArrowDown');

    // Second item should now be focused
    const secondItem = menuItems.nth(1);
    await expect(secondItem).toHaveClass(/focused/);

    // Press arrow down again
    await page.keyboard.press('ArrowDown');

    // Third item should now be focused
    const thirdItem = menuItems.nth(2);
    await expect(thirdItem).toHaveClass(/focused/);
  });

  test('arrow up key navigates to previous item', async ({ page, request }) => {
    await setupEditorWithBlocks(page, request);

    const contextMenu = await openContextMenu(page, 0);
    const menuItems = contextMenu.locator('[role="menuitem"]');
    const itemCount = await menuItems.count();

    // Press arrow down twice to move to third item
    await page.keyboard.press('ArrowDown');
    await page.keyboard.press('ArrowDown');

    const thirdItem = menuItems.nth(2);
    await expect(thirdItem).toHaveClass(/focused/);

    // Press arrow up to go back to second item
    await page.keyboard.press('ArrowUp');

    const secondItem = menuItems.nth(1);
    await expect(secondItem).toHaveClass(/focused/);

    // Arrow up wraps around - go back to first
    await page.keyboard.press('ArrowUp');

    const firstItem = menuItems.nth(0);
    await expect(firstItem).toHaveClass(/focused/);

    // Arrow up from first item should wrap to last
    await page.keyboard.press('ArrowUp');

    const lastItem = menuItems.nth(itemCount - 1);
    await expect(lastItem).toHaveClass(/focused/);
  });

  test('enter key executes the focused action', async ({ page, request }) => {
    await setupEditorWithBlocks(page, request);

    const prosemirror = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
    const blocks = prosemirror.locator('.block-row');
    await expect(blocks).toHaveCount(3);

    // Open context menu on first block
    const contextMenu = await openContextMenu(page, 0);

    // "Duplicate" is the first item, already focused
    // Press enter to execute duplicate action
    await page.keyboard.press('Enter');

    // Context menu should close
    await expect(contextMenu).not.toBeVisible({ timeout: 3000 });

    // Should now have 4 blocks (original 3 + duplicate)
    await expect(blocks).toHaveCount(4);

    // First two blocks should have same content
    const blockTexts = await blocks.allInnerTexts();
    expect(blockTexts[0]).toBe(blockTexts[1]);
  });

  test('escape key closes the context menu', async ({ page, request }) => {
    await setupEditorWithBlocks(page, request);

    const contextMenu = await openContextMenu(page, 0);
    await expect(contextMenu).toBeVisible();

    // Press Escape to close
    await page.keyboard.press('Escape');

    // Context menu should be closed
    await expect(contextMenu).not.toBeVisible({ timeout: 3000 });

    // Editor should still be visible and functional
    const prosemirror = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
    await expect(prosemirror).toBeVisible();
  });

  test('delete block action works', async ({ page, request }) => {
    await setupEditorWithBlocks(page, request);

    const prosemirror = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
    const blocks = prosemirror.locator('.block-row');
    await expect(blocks).toHaveCount(3);

    // Get the text of the first block before deletion
    const firstBlockText = await blocks.nth(0).innerText();

    // Open context menu on first block
    const contextMenu = await openContextMenu(page, 0);

    // Click "Delete" (first menu item)
    await contextMenu.locator('text=Delete').first().click();

    // Context menu should close
    await expect(contextMenu).not.toBeVisible({ timeout: 3000 });

    // Should now have 2 blocks
    await expect(blocks).toHaveCount(2);

    // First block should now be what was the second block
    const newFirstBlockText = await blocks.nth(0).innerText();
    expect(newFirstBlockText).not.toBe(firstBlockText);
  });

  test('duplicate block action works', async ({ page, request }) => {
    await setupEditorWithBlocks(page, request);

    const prosemirror = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
    const blocks = prosemirror.locator('.block-row');
    await expect(blocks).toHaveCount(3);

    // Open context menu on first block
    const contextMenu = await openContextMenu(page, 0);

    // Click "Duplicate"
    await contextMenu.locator('text=Duplicate').first().click();

    // Context menu should close
    await expect(contextMenu).not.toBeVisible({ timeout: 3000 });

    // Should now have 4 blocks
    await expect(blocks).toHaveCount(4);

    // The first and second blocks should have the same content
    const blockTexts = await blocks.allInnerTexts();
    expect(blockTexts[0]).toBe(blockTexts[1]);
  });

  test('clicking outside closes the context menu', async ({ page, request }) => {
    await setupEditorWithBlocks(page, request);

    const contextMenu = await openContextMenu(page, 0);
    await expect(contextMenu).toBeVisible();

    // Click outside the menu (on the editor area but not on a block)
    const editor = page.locator('[data-testid="wysiwyg-editor"]');
    const editorBox = await editor.boundingBox();
    if (editorBox) {
      // Click near the bottom of the editor area
      await page.mouse.click(editorBox.x + 50, editorBox.y + editorBox.height - 20);
    }

    // Context menu should be closed
    await expect(contextMenu).not.toBeVisible({ timeout: 3000 });
  });

  test('context menu shows correct options for single block', async ({ page, request }) => {
    await setupEditorWithBlocks(page, request);

    const contextMenu = await openContextMenu(page, 0);

    // Single block menu should have standard options
    await expect(contextMenu.locator('text=Delete')).toBeVisible();
    await expect(contextMenu.locator('text=Duplicate')).toBeVisible();
    await expect(contextMenu.locator('text=Indent')).toBeVisible();
    await expect(contextMenu.locator('text=Outdent')).toBeVisible();

    // Should have block type conversion options (with icons)
    await expect(contextMenu.locator('text=Paragraph')).toBeVisible();
    await expect(contextMenu.locator('text=Bullet list')).toBeVisible();
    await expect(contextMenu.locator('text=Numbered list')).toBeVisible();
  });

  test('space key also executes the focused action', async ({ page, request }) => {
    await setupEditorWithBlocks(page, request);

    const prosemirror = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
    const blocks = prosemirror.locator('.block-row');
    await expect(blocks).toHaveCount(3);

    // Open context menu on first block
    const contextMenu = await openContextMenu(page, 0);

    // "Duplicate" is the first item, already focused
    // Press space to execute duplicate action
    await page.keyboard.press(' ');

    // Context menu should close
    await expect(contextMenu).not.toBeVisible({ timeout: 3000 });

    // Should now have 4 blocks
    await expect(blocks).toHaveCount(4);
  });

  test('arrow down wraps around to first item', async ({ page, request }) => {
    await setupEditorWithBlocks(page, request);

    const contextMenu = await openContextMenu(page, 0);
    const menuItems = contextMenu.locator('[role="menuitem"]');
    const itemCount = await menuItems.count();

    // Navigate to the last item by pressing ArrowUp from start (wraps to end)
    await page.keyboard.press('ArrowUp');
    const lastItem = menuItems.nth(itemCount - 1);
    await expect(lastItem).toHaveClass(/focused/);

    // Press arrow down to wrap to first item
    await page.keyboard.press('ArrowDown');
    const firstItem = menuItems.nth(0);
    await expect(firstItem).toHaveClass(/focused/);
  });

  test('convert paragraph to Heading 1 changes block type', async ({ page, request }) => {
    await setupEditorWithBlocks(page, request);

    const prosemirror = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
    const blocks = prosemirror.locator('.block-row');
    const firstBlock = blocks.nth(0);

    // Verify it starts as paragraph
    await expect(firstBlock).toHaveAttribute('data-type', 'paragraph');

    const contextMenu = await openContextMenu(page, 0);
    await contextMenu.locator('text=Heading 1').click();
    await expect(contextMenu).not.toBeVisible({ timeout: 3000 });

    // Should now be heading with <h1> content element
    const updatedBlock = prosemirror.locator('.block-row').nth(0);
    await expect(updatedBlock).toHaveAttribute('data-type', 'heading');
    const tag = await updatedBlock.locator('.block-content').evaluate((el) => el.tagName.toLowerCase());
    expect(tag).toBe('h1');
  });

  test('convert Heading 1 to Heading 2 changes content element tag', async ({ page, request }) => {
    await setupEditorWithBlocks(page, request);

    const prosemirror = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');

    // First convert paragraph to Heading 1
    let contextMenu = await openContextMenu(page, 0);
    await contextMenu.locator('text=Heading 1').click();
    await expect(contextMenu).not.toBeVisible({ timeout: 3000 });

    // Verify it's h1
    const block = prosemirror.locator('.block-row').nth(0);
    const tag1 = await block.locator('.block-content').evaluate((el) => el.tagName.toLowerCase());
    expect(tag1).toBe('h1');

    // Now convert to Heading 2
    contextMenu = await openContextMenu(page, 0);
    await contextMenu.locator('text=Heading 2').click();
    await expect(contextMenu).not.toBeVisible({ timeout: 3000 });

    // Should now be <h2>
    const updatedBlock = prosemirror.locator('.block-row').nth(0);
    await expect(updatedBlock).toHaveAttribute('data-type', 'heading');
    const tag2 = await updatedBlock.locator('.block-content').evaluate((el) => el.tagName.toLowerCase());
    expect(tag2).toBe('h2');
  });
});
