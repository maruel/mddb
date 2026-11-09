// Browser coverage for Pele diagrams, native caret reconciliation, editable source, and history disclosure.
import { test, expect, registerUser, getWorkspaceId, createClient, gotoWithSession } from "./helpers";
import type { Locator } from "@playwright/test";
import type { EditorView } from "prosemirror-view";

// Native navigation changes DOM selection before ProseMirror receives the
// browser's selectionchange notification. Require both owners to agree before
// typing, without changing either selection from the test.
async function sourceCaret(source: Locator, offset: number) {
  return source.evaluate((code, expectedOffset) => {
    const selection = document.getSelection();
    const editor = code.closest(".ProseMirror") as (HTMLElement & { pmView?: EditorView }) | null;
    const view = editor?.pmView;
    let native = false;
    if (selection?.isCollapsed && selection.focusNode && code.contains(selection.focusNode)) {
      const prefix = document.createRange();
      prefix.selectNodeContents(code);
      prefix.setEnd(selection.focusNode, selection.focusOffset);
      native = prefix.toString().length === expectedOffset;
    }
    return {
      native,
      model: Boolean(
        view?.hasFocus() &&
        view.state.selection.empty &&
        view.state.selection.head === view.posAtDOM(code, 0) + expectedOffset,
      ),
    };
  }, offset);
}

// Isolate application imports from the service worker's background asset precache.
test.use({ serviceWorkers: "block" });

for (const width of [390, 1280]) {
  for (const kind of ["prose", "code", "mermaid"]) {
    test(`Native caret survives focus reconciliation in ${kind} at ${width}px`, async ({ page, request }) => {
      await page.setViewportSize({ width, height: 900 });
      const { token } = await registerUser(request, "caret");
      const client = createClient(request, token);
      await gotoWithSession(page, token);
      await expect(page.locator("header")).toBeVisible({ timeout: 15000 });
      const wsID = await getWorkspaceId(page);
      const source = kind === "prose" ? "First paragraph" : "flowchart LR\naccTitle: Pipeline\nBuild --> Test";
      const content = kind === "prose" ? source : `\`\`\`${kind === "mermaid" ? "mermaid" : "text"}\n${source}\n\`\`\``;
      const node = await client.ws(wsID).createPage("0", { title: "Caret", content });
      await page.goto(`/w/@${wsID}/@${node.id}`);
      const editor = page.getByTestId("wysiwyg-editor");
      const editable = editor.locator(kind === "prose" ? "p" : "pre code").first();
      await expect(editable).toHaveText(source);
      if (kind === "mermaid") await expect(editor.locator(".mermaid-diagram > svg")).toHaveCount(1);

      // Keep the initial focus timer pending until native navigation has moved
      // the caret, then let it run before the selectionchange notification.
      await page.clock.install();
      await page.clock.pauseAt(new Date());
      await editable.click();
      await page.keyboard.press("Control+Home");
      await expect.poll(() => sourceCaret(editable, 0)).toEqual({ native: true, model: true });
      const release = await page.evaluateHandle(() => {
        const hold = (event: Event) => event.stopImmediatePropagation();
        document.addEventListener("selectionchange", hold, true);
        return () => {
          document.removeEventListener("selectionchange", hold, true);
          document.dispatchEvent(new Event("selectionchange"));
        };
      });
      const offset = kind === "prose" ? 1 : 13;
      try {
        await page.keyboard.press(kind === "prose" ? "ArrowRight" : "ArrowDown");
        await expect.poll(() => sourceCaret(editable, offset)).toEqual({ native: true, model: false });
        await page.clock.runFor(50);
        await expect.poll(() => sourceCaret(editable, offset)).toMatchObject({ native: true });
      } finally {
        await release.evaluate((fn) => fn());
        await release.dispose();
        await page.clock.resume();
      }
      await expect.poll(() => sourceCaret(editable, offset)).toEqual({ native: true, model: true });
      await page.keyboard.type("X");
      await expect(editable).toHaveText(source.slice(0, offset) + "X" + source.slice(offset));
    });
  }
}

for (const width of [390, 1280]) {
  test(`Mermaid diagrams in the editor and history at ${width}px`, async ({ page, request }, testInfo) => {
    await page.setViewportSize({ width, height: 900 });
    const errors: string[] = [];
    page.on("pageerror", (error) => errors.push(error.message));
    const rendererRequests: string[] = [];
    page.on("request", (req) => {
      if (/\/assets\/lazy-/.test(req.url())) {
        rendererRequests.push(req.url());
      }
    });
    const { token } = await registerUser(request, "pele");
    const client = createClient(request, token);
    await gotoWithSession(page, token);
    await expect(page.locator("header")).toBeVisible({ timeout: 15000 });
    expect(rendererRequests).toEqual([]);
    const ws = client.ws(await getWorkspaceId(page));
    const flowchart = "flowchart LR\naccTitle: Pipeline\naccDescr: Build and test\nBuild --> Test";
    const sources = [
      flowchart,
      flowchart,
      "sequenceDiagram\nAlice->>Bob: Hello",
      "classDiagram\nAnimal <|-- Duck",
      "stateDiagram-v2\n[*] --> Ready\nReady --> [*]",
      "architecture-beta\nservice api(server)[API]\nservice db(database)[Database]\napi:R -- L:db",
      'flowchart LR\nA["<img src=x onerror=alert(1)>"] --> B\nclick A "https://example.com/"',
      "invalid diagram syntax %%%",
    ];
    const content = sources.map((source) => `\`\`\`mermaid\n${source}\n\`\`\``).join("\n\n");
    const wsID = await getWorkspaceId(page);
    const plain = await ws.createPage("0", { title: "Source", content: content.replace(/```mermaid/g, "```text") });
    const baselineStart = Date.now();
    await page.goto(`/w/@${wsID}/@${plain.id}`);
    await expect(page.getByTestId("wysiwyg-editor").locator("pre code")).toHaveCount(8);
    const sourceLoadMs = Date.now() - baselineStart;
    expect(rendererRequests).toEqual([]);
    const node = await ws.createPage("0", { title: "Diagrams", content });
    const start = Date.now();
    // Navigate directly so this also exercises cold renderer loading.
    await page.goto(`/w/@${wsID}/@${node.id}`);
    const editor = page.getByTestId("wysiwyg-editor");
    await expect(editor.locator(".mermaid-diagram > svg")).toHaveCount(7);
    await testInfo.attach("diagram-load", {
      body: JSON.stringify({ width, sourceLoadMs, diagramLoadMs: Date.now() - start }),
      contentType: "application/json",
    });
    await expect(editor.locator("pre code")).toHaveCount(8);
    expect((await editor.locator(".mermaid-diagram > svg").first().boundingBox())!.width).toBeGreaterThan(150);
    expect(rendererRequests.length).toBeGreaterThan(0);
    expect(
      await editor.locator(".mermaid-diagram a, .mermaid-diagram image, .mermaid-diagram foreignObject").count(),
    ).toBe(0);
    const ids = await editor.locator("svg title").evaluateAll((titles) => titles.map((title) => title.id));
    expect(new Set(ids).size).toBe(ids.length);

    const editableSource = editor.locator("pre code").first();
    await editableSource.click();
    await page.keyboard.press("Control+Home");
    await expect.poll(() => sourceCaret(editableSource, 0)).toEqual({ native: true, model: true });
    let lineStart = 0;
    for (const line of flowchart.split("\n").slice(0, -1)) {
      await page.keyboard.press("ArrowDown");
      lineStart += line.length + 1;
      await expect.poll(() => sourceCaret(editableSource, lineStart)).toEqual({ native: true, model: true });
    }

    // Hold selectionchange so End moves the native caret before the editor
    // receives its notification, then replay it. Neither selection is set here.
    const releaseSelection = await page.evaluateHandle(() => {
      let held: Event | null = null;
      const hold = (event: Event) => {
        held = event;
        event.stopImmediatePropagation();
      };
      document.addEventListener("selectionchange", hold, true);
      return () => {
        document.removeEventListener("selectionchange", hold, true);
        if (held) document.dispatchEvent(held);
      };
    });
    try {
      await page.keyboard.press("End");
      await expect.poll(() => sourceCaret(editableSource, flowchart.length)).toEqual({ native: true, model: false });
    } finally {
      await releaseSelection.evaluate((release) => release());
      await releaseSelection.dispose();
    }
    await expect.poll(() => sourceCaret(editableSource, flowchart.length)).toEqual({ native: true, model: true });
    await page.keyboard.type("ing");
    await expect(editableSource).toContainText("Build --> Testing");
    await expect(editor.locator(".mermaid-diagram > svg").first()).toContainText("Testing");

    await page.screenshot({ path: testInfo.outputPath("editor.png"), fullPage: true });

    // History reads the saved source into the sanitized Markdown preview.
    if (width < 768) await page.getByRole("button", { name: "Toggle menu", exact: true }).click();
    await page.getByTestId(`sidebar-node-${node.id}`).click({ button: "right" });
    await page.getByTestId("context-menu-history").click();
    const preview = page.getByRole("region", { name: "Markdown preview" });
    await expect(preview.locator("[data-mermaid-diagram] > svg")).toHaveCount(7);
    expect((await preview.locator("[data-mermaid-diagram] > svg").first().boundingBox())!.width).toBeGreaterThan(150);
    const disclosure = preview.locator("details").first();
    await expect(disclosure).not.toHaveAttribute("open");
    await disclosure.locator("summary").focus();
    await page.keyboard.press("Enter");
    await expect(disclosure).toHaveAttribute("open");
    await expect(disclosure.locator("code")).toContainText("Build --> Test");
    await expect(preview.locator("details").last()).toHaveAttribute("open");
    await expect(preview.locator("details").last()).toContainText("invalid diagram syntax");
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    await page.screenshot({ path: testInfo.outputPath("history.png"), fullPage: true });
    expect(errors).toEqual([]);
  });
}
