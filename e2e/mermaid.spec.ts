// Browser coverage for lazy Pele diagrams, editable source, safe fallback, and history disclosure.
import { test, expect, registerUser, getWorkspaceId, createClient, gotoWithSession } from "./helpers";

// Isolate application imports from the service worker's background asset precache.
test.use({ serviceWorkers: "block" });

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
    await editableSource.click({ position: { x: 5, y: 5 } });
    await page.keyboard.press("Home");
    await page.keyboard.press("ArrowDown");
    await page.keyboard.press("ArrowDown");
    await page.keyboard.press("ArrowDown");
    await page.keyboard.press("End");
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
