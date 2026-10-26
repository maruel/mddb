// Browser coverage for toolbar selection and undo across asynchronous editor metadata updates.

import { test, expect, registerUser, getWorkspaceId, createClient, gotoWithSession } from "./helpers";

test.describe("Floating Toolbar Visibility", () => {
  test("toolbar lifecycle: hidden -> visible on selection -> hidden on blur", async ({ page, request }) => {
    const { token } = await registerUser(request, "float-toolbar-lifecycle");
    await gotoWithSession(page, token);
    await expect(page.locator("aside")).toBeVisible({ timeout: 15000 });

    const wsID = await getWorkspaceId(page);

    // Create a page with simple content
    const client = createClient(request, token);
    const pageData = await client.ws(wsID).createPage("0", {
      title: "Toolbar Visibility Test",
      content: "Hello world",
    });

    await page.reload();
    await expect(page.locator("aside")).toBeVisible({ timeout: 10000 });

    // Navigate to the page
    await page.locator(`[data-testid="sidebar-node-${pageData.id}"]`).click();

    // Wait for WYSIWYG editor to load
    const editor = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
    await expect(editor).toBeVisible({ timeout: 5000 });
    await expect(editor.locator("p").first()).toHaveText("Hello world");

    // 1. Initial State: Toolbar should be HIDDEN
    const toolbar = page.locator('[data-testid="floating-toolbar"]');
    await expect(toolbar).not.toBeVisible();

    // Mode toggle should be VISIBLE always (bottom-right indicator)
    const modeToggle = page.locator('[data-testid="editor-mode-visual"]');
    await expect(modeToggle).toBeVisible();

    // 2. Type text: Toolbar should REMAIN HIDDEN
    await editor.locator("p").first().click();
    await page.keyboard.type(" more text");
    await expect(toolbar).not.toBeVisible();

    // 3. Select text: Toolbar should become VISIBLE
    await editor.locator("p").first().selectText();
    await expect(toolbar).toBeVisible({ timeout: 3000 });

    // 4. Click away (clear selection): Toolbar should become HIDDEN
    await editor.locator("p").first().click();
    await expect(toolbar).not.toBeVisible({ timeout: 3000 });

    // Mode toggle should STILL be visible
    await expect(modeToggle).toBeVisible();
  });

  test.describe("Delayed metadata", () => {
    // Service-worker interception can bypass page.route's response barrier.
    test.use({ serviceWorkers: "block" });

    for (const input of [
      { name: "typed text", text: " more text", newParagraph: false },
      { name: "trailing spaces", text: " more ", newParagraph: false },
      { name: "empty paragraphs", text: " more ", newParagraph: true },
    ]) {
      test(`late link titles preserve ${input.name}, selection, and undo`, async ({ page, request }) => {
        const { token } = await registerUser(request, "toolbar-late-titles");
        const client = createClient(request, token);
        await gotoWithSession(page, token);
        const wsID = await getWorkspaceId(page);
        const target = await client.ws(wsID).createPage("0", { title: "Linked target", content: "Target content" });
        const node = await client.ws(wsID).createPage("0", {
          title: "Delayed link titles",
          content: `Hello world\n\n[Linked target](/w/@${wsID}/@${target.id})`,
        });
        await page.reload();
        await expect(page.locator("aside")).toBeVisible();

        let metadataRequested!: () => void;
        const requested = new Promise<void>((resolve) => {
          metadataRequested = resolve;
        });
        let releaseMetadata!: () => void;
        const released = new Promise<void>((resolve) => {
          releaseMetadata = resolve;
        });
        await page.route("**/nodes/titles?*", async (route) => {
          const ids = new URL(route.request().url()).searchParams.get("ids")?.split(",") ?? [];
          if (!ids.includes(target.id)) {
            await route.continue();
            return;
          }
          const response = await route.fetch();
          metadataRequested();
          await released;
          await route.fulfill({ response });
        });
        try {
          await page.getByTestId(`sidebar-node-${node.id}`).click();
          await requested;
          const editor = page.getByTestId("wysiwyg-editor").locator(".ProseMirror");
          const paragraph = editor.locator("p").first();
          await expect(paragraph).toHaveText("Hello world");
          await expect(editor.locator(".invalid-link")).toHaveCount(1);
          await paragraph.click();
          await page.keyboard.press("End");
          await page.keyboard.type(input.text);
          if (input.newParagraph) {
            await page.keyboard.press("Enter");
            await expect(editor.locator("p")).toHaveCount(3);
            await page.keyboard.press("ArrowUp");
          }
          await page.keyboard.press("Home");
          await page.keyboard.down("Shift");
          await page.keyboard.press("End");
          await page.keyboard.up("Shift");
          const toolbar = page.getByTestId("floating-toolbar");
          await expect(toolbar).toBeVisible();

          releaseMetadata();
          await expect(editor.locator(".invalid-link")).toHaveCount(0);
          expect(await paragraph.textContent()).toBe("Hello world" + input.text);
          if (input.newParagraph) {
            await expect(editor.locator("p")).toHaveCount(3);
            await expect(editor.locator("p").nth(1)).toHaveText("");
          }
          await expect(toolbar).toBeVisible();
          await expect
            .poll(() => page.evaluate(() => window.getSelection()?.toString()))
            .toBe("Hello world" + input.text);
          await page.keyboard.press("ControlOrMeta+z");
          if (input.newParagraph) {
            await expect(editor.locator("p")).toHaveCount(2);
          } else {
            await expect(paragraph).toHaveText("Hello world");
          }
        } finally {
          releaseMetadata();
          await page.unroute("**/nodes/titles?*");
        }
      });
    }
  });

  test("toolbar appears on double-click word selection", async ({ page, request }) => {
    const { token } = await registerUser(request, "float-toolbar-dblclick");
    await gotoWithSession(page, token);
    await expect(page.locator("aside")).toBeVisible({ timeout: 15000 });

    const wsID = await getWorkspaceId(page);

    // Create a page with multiple words
    const client = createClient(request, token);
    const pageData = await client.ws(wsID).createPage("0", {
      title: "Double Click Test",
      content: "Hello world testing",
    });

    await page.reload();
    await expect(page.locator("aside")).toBeVisible({ timeout: 10000 });

    await page.locator(`[data-testid="sidebar-node-${pageData.id}"]`).click();

    const editor = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
    await expect(editor).toBeVisible({ timeout: 5000 });

    // Wait for the actual page content to load (not just editor visibility)
    await expect(editor.locator("p")).toContainText("Hello", { timeout: 5000 });

    const toolbar = page.locator('[data-testid="floating-toolbar"]');
    await expect(toolbar).not.toBeVisible();

    // Double-click on text to select a word
    // Note: We use mouse.dblclick with coordinates near the text start because
    // locator.dblclick() clicks the center of the element which may be empty space
    const paragraph = editor.locator("p").first();
    const box = await paragraph.boundingBox();
    expect(box).toBeTruthy();

    // Click near the start of the paragraph (where the text begins)
    const x = box!.x + 30;
    const y = box!.y + box!.height / 2;
    await page.mouse.dblclick(x, y);

    // Toolbar should appear when text is selected
    await expect(toolbar).toBeVisible({ timeout: 3000 });

    // Verify text is selected
    const selection = await page.evaluate(() => window.getSelection()?.toString());
    expect(selection?.length).toBeGreaterThan(0);
  });

  test("toolbar stays within editor bounds when sidebar is open", async ({ page, request }) => {
    const { token } = await registerUser(request, "float-toolbar-bounds");
    await gotoWithSession(page, token);
    await expect(page.locator("aside")).toBeVisible({ timeout: 15000 });

    const wsID = await getWorkspaceId(page);

    // Create a page with content
    const client = createClient(request, token);
    const pageData = await client.ws(wsID).createPage("0", {
      title: "Toolbar Bounds Test",
      content: "Test content for toolbar positioning",
    });

    await page.reload();
    await expect(page.locator("aside")).toBeVisible({ timeout: 10000 });

    // Navigate to the page
    await page.locator(`[data-testid="sidebar-node-${pageData.id}"]`).click();

    const editor = page.locator('[data-testid="wysiwyg-editor"]');
    const prosemirror = editor.locator(".ProseMirror");
    await expect(prosemirror).toBeVisible({ timeout: 5000 });

    // Wait for content to load
    await expect(prosemirror.locator("p")).toContainText("Test content", { timeout: 5000 });

    // Ensure sidebar is open
    const sidebar = page.locator("aside");
    await expect(sidebar).toBeVisible();

    // Select text to show toolbar
    await prosemirror.locator("p").first().selectText();

    const toolbar = page.locator('[data-testid="floating-toolbar"]');
    await expect(toolbar).toBeVisible({ timeout: 3000 });

    // Wait for toolbar position to stabilize (clamping happens in requestAnimationFrame)
    const editorBox = await editor.boundingBox();
    expect(editorBox).toBeTruthy();

    await expect(async () => {
      const toolbarBox = await toolbar.boundingBox();
      expect(toolbarBox).toBeTruthy();
      // Toolbar should be fully within editor horizontal bounds
      expect(toolbarBox!.x).toBeGreaterThanOrEqual(editorBox!.x);
      expect(toolbarBox!.x + toolbarBox!.width).toBeLessThanOrEqual(editorBox!.x + editorBox!.width);
    }).toPass({ timeout: 3000 });
  });
});
