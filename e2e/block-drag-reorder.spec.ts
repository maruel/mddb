// E2E tests for block drag-and-drop reordering functionality.
// Tests use synthetic drag events via dispatchEvent since Playwright's native drag
// methods (dragTo, mouse.down/move/up) do not reliably trigger the browser's native
// drag events needed for ProseMirror's drag-and-drop system.

import type { Page } from '@playwright/test';
import { test, expect, registerUser, getWorkspaceId, createClient } from './helpers';

// Helper to setup editor with test content
async function setupEditorWithBlocks(page: Page, request: Parameters<typeof registerUser>[0]) {
  const { token } = await registerUser(request, 'block-dnd');
  await page.goto(`/?token=${token}`);
  await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

  const wsId = await getWorkspaceId(page);

  // Create test page with simple blocks for drag testing
  const markdownContent = `First paragraph

Second paragraph

Third paragraph`;

  const client = createClient(request, token);
  const pageResp = await client.ws(wsId).nodes.page.createPage('0', {
    title: 'Drag Test',
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

/**
 * Simulate drag-and-drop of a block using synthetic DOM events.
 * This is necessary because Playwright's native drag methods don't reliably trigger
 * the browser's native drag events for elements with draggable="true".
 */
async function simulateBlockDrag(
  page: Page,
  sourceIndex: number,
  targetIndex: number,
  dropBelow: boolean = true
) {
  return await page.evaluate(
    async ({ sourceIdx, targetIdx, below }: { sourceIdx: number; targetIdx: number; below: boolean }) => {
      const editor = document.querySelector('[data-testid="wysiwyg-editor"] .ProseMirror') as HTMLElement;
      const blocks = editor.querySelectorAll('.block-row');
      const handles = editor.querySelectorAll('[data-testid="row-handle"]') as NodeListOf<HTMLElement>;

      if (blocks.length <= Math.max(sourceIdx, targetIdx) || handles.length <= sourceIdx) {
        return { success: false, error: 'Invalid block indices' };
      }

      // Non-null assertion is safe: indices already validated above
      const sourceHandle = handles[sourceIdx]!;
      const targetBlock = blocks[targetIdx] as HTMLElement;

      // Create DataTransfer
      const dataTransfer = new DataTransfer();
      dataTransfer.effectAllowed = 'move';

      // Get coordinates
      const handleRect = sourceHandle.getBoundingClientRect();
      const targetBlockRect = targetBlock.getBoundingClientRect();

      // 1. Dispatch dragstart on the source handle
      const dragStartEvent = new DragEvent('dragstart', {
        bubbles: true,
        cancelable: true,
        dataTransfer,
        clientX: handleRect.left + handleRect.width / 2,
        clientY: handleRect.top + handleRect.height / 2,
      });
      sourceHandle.dispatchEvent(dragStartEvent);

      await new Promise((r) => setTimeout(r, 50));

      // 2. Dispatch dragover on editor to calculate drop target
      // To drop above a block, position mouse in the upper third of the block
      // To drop below a block, position mouse in the lower third of the block
      const blockHeight = targetBlockRect.height;
      const dropY = below
        ? targetBlockRect.top + blockHeight * 0.75  // Lower part of block -> drop below
        : targetBlockRect.top + blockHeight * 0.25; // Upper part of block -> drop above

      const dragOverEvent = new DragEvent('dragover', {
        bubbles: true,
        cancelable: true,
        clientX: targetBlockRect.left + targetBlockRect.width / 2,
        clientY: dropY,
        dataTransfer,
      });
      editor.dispatchEvent(dragOverEvent);

      await new Promise((r) => setTimeout(r, 50));

      // 3. Dispatch drop
      const dropEvent = new DragEvent('drop', {
        bubbles: true,
        cancelable: true,
        clientX: targetBlockRect.left + targetBlockRect.width / 2,
        clientY: dropY,
        dataTransfer,
      });
      editor.dispatchEvent(dropEvent);

      await new Promise((r) => setTimeout(r, 50));

      // 4. Dispatch dragend
      const dragEndEvent = new DragEvent('dragend', {
        bubbles: true,
        cancelable: true,
        dataTransfer,
      });
      sourceHandle.dispatchEvent(dragEndEvent);

      // Get final order
      const newBlocks = editor.querySelectorAll('.block-row');
      const newTexts = Array.from(newBlocks).map((b) => (b as HTMLElement).innerText.trim());

      return { success: true, newTexts };
    },
    { sourceIdx: sourceIndex, targetIdx: targetIndex, below: dropBelow }
  );
}

test.describe('Block drag and drop reordering', () => {
  test('drag first block to after third block', async ({ page, request }) => {
    const { prosemirror } = await setupEditorWithBlocks(page, request);

    const blocks = prosemirror.locator('.block-row');
    await expect(blocks).toHaveCount(3);

    const initialTexts = await blocks.allInnerTexts();
    expect(initialTexts[0]).toContain('First');
    expect(initialTexts[1]).toContain('Second');
    expect(initialTexts[2]).toContain('Third');

    // Hover to reveal handles (for visual feedback)
    await blocks.nth(0).hover();
    await page.waitForTimeout(100);

    // Drag first block to after third block
    const result = await simulateBlockDrag(page, 0, 2, true);

    expect(result.success).toBe(true);
    expect(result.newTexts).toEqual(['Second paragraph', 'Third paragraph', 'First paragraph']);

    // Verify final state
    const finalTexts = await blocks.allInnerTexts();
    expect(finalTexts[0]).toContain('Second');
    expect(finalTexts[1]).toContain('Third');
    expect(finalTexts[2]).toContain('First');
  });

  test('drag third block to before first block', async ({ page, request }) => {
    const { prosemirror } = await setupEditorWithBlocks(page, request);

    const blocks = prosemirror.locator('.block-row');
    await expect(blocks).toHaveCount(3);

    // Drag third block to before first block
    const result = await simulateBlockDrag(page, 2, 0, false);

    expect(result.success).toBe(true);
    expect(result.newTexts).toEqual(['Third paragraph', 'First paragraph', 'Second paragraph']);

    // Verify final state
    const finalTexts = await blocks.allInnerTexts();
    expect(finalTexts[0]).toContain('Third');
    expect(finalTexts[1]).toContain('First');
    expect(finalTexts[2]).toContain('Second');
  });

  test('drag middle block to end', async ({ page, request }) => {
    const { prosemirror } = await setupEditorWithBlocks(page, request);

    const blocks = prosemirror.locator('.block-row');
    await expect(blocks).toHaveCount(3);

    // Drag second block to after third
    const result = await simulateBlockDrag(page, 1, 2, true);

    expect(result.success).toBe(true);
    expect(result.newTexts).toEqual(['First paragraph', 'Third paragraph', 'Second paragraph']);
  });

  test('drag middle block to beginning', async ({ page, request }) => {
    const { prosemirror } = await setupEditorWithBlocks(page, request);

    const blocks = prosemirror.locator('.block-row');
    await expect(blocks).toHaveCount(3);

    // Drag second block to before first
    const result = await simulateBlockDrag(page, 1, 0, false);

    expect(result.success).toBe(true);
    expect(result.newTexts).toEqual(['Second paragraph', 'First paragraph', 'Third paragraph']);
  });

  test('drop indicator appears during drag', async ({ page, request }) => {
    const { prosemirror } = await setupEditorWithBlocks(page, request);

    // Wait for blocks to be ready
    const blocks = prosemirror.locator('.block-row');
    await expect(blocks).toHaveCount(3);

    // Hover to make handles visible
    await blocks.nth(0).hover();
    const handle = blocks.nth(0).locator('[data-testid="row-handle"]');
    await expect(handle).toBeVisible({ timeout: 3000 });

    // Check for drop indicator during drag operation
    const indicatorVisible = await page.evaluate(async () => {
      const editor = document.querySelector('[data-testid="wysiwyg-editor"] .ProseMirror') as HTMLElement;
      const handles = editor.querySelectorAll('[data-testid="row-handle"]') as NodeListOf<HTMLElement>;
      const blockElements = editor.querySelectorAll('.block-row');

      if (handles.length < 1 || blockElements.length < 3) {
        return { success: false, error: 'Not enough blocks or handles' };
      }

      const sourceHandle = handles[0] as HTMLElement;
      const targetBlock = blockElements[2] as HTMLElement;

      // Create DataTransfer
      const dataTransfer = new DataTransfer();
      dataTransfer.effectAllowed = 'move';

      // Get coordinates
      const handleRect = sourceHandle.getBoundingClientRect();
      const targetBlockRect = targetBlock.getBoundingClientRect();

      // 1. Dispatch dragstart
      const dragStartEvent = new DragEvent('dragstart', {
        bubbles: true,
        cancelable: true,
        dataTransfer,
        clientX: handleRect.left + handleRect.width / 2,
        clientY: handleRect.top + handleRect.height / 2,
      });
      sourceHandle.dispatchEvent(dragStartEvent);

      await new Promise((r) => setTimeout(r, 50));

      // 2. Dispatch dragover to trigger indicator
      const dropY = targetBlockRect.top + targetBlockRect.height * 0.75;
      const dragOverEvent = new DragEvent('dragover', {
        bubbles: true,
        cancelable: true,
        clientX: targetBlockRect.left + targetBlockRect.width / 2,
        clientY: dropY,
        dataTransfer,
      });
      editor.dispatchEvent(dragOverEvent);

      // Wait for RAF to update the indicator
      await new Promise((r) => requestAnimationFrame(() => setTimeout(r, 50)));

      // Check for indicator - it should be visible in the editor container's parent
      // The EditorDropIndicator is a sibling of [data-testid="wysiwyg-editor"], not a child
      const editorElement = document.querySelector('[data-testid="wysiwyg-editor"]');
      const editorContainer = editorElement?.parentElement;

      // Look for the indicator - it has role="presentation" and aria-hidden="true"
      const indicator = editorContainer?.querySelector('[role="presentation"][aria-hidden="true"]');
      const indicatorFound = indicator !== null;

      // Also check that indicator has a reasonable top position
      let indicatorTop = null;
      if (indicator) {
        const style = window.getComputedStyle(indicator);
        indicatorTop = style.top;
      }


      // 3. Cleanup - dispatch dragend on the editor (where ProseMirror listens)
      const dragEndEvent = new DragEvent('dragend', {
        bubbles: true,
        cancelable: true,
        dataTransfer,
      });
      editor.dispatchEvent(dragEndEvent);

      // Wait longer for state to settle
      await new Promise((r) => setTimeout(r, 100));

      // Verify indicator is hidden after drag ends
      const editorElement2 = document.querySelector('[data-testid="wysiwyg-editor"]');
      const editorContainer2 = editorElement2?.parentElement;
      const indicatorAfter = editorContainer2?.querySelector('[role="presentation"][aria-hidden="true"]');
      const indicatorHiddenAfter = indicatorAfter === null;

      return {
        success: true,
        indicatorFound,
        indicatorTop,
        indicatorHiddenAfter,
      };
    });

    expect(indicatorVisible.success).toBe(true);
    expect(indicatorVisible.indicatorFound).toBe(true);
    // Indicator should have a valid top position (number > 0)
    expect(indicatorVisible.indicatorTop).toMatch(/^\d+(\.\d+)?px$/);
    expect(indicatorVisible.indicatorHiddenAfter).toBe(true);
  });
});
