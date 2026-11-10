// Verifies live page updates and draft protection for writes from the same account.

import { test, expect, registerUser, waitForWorkspace, createClient, gotoWithSession } from "./helpers";

test("same-account API edits update the open page and preserve unsaved drafts", async ({ page, request }) => {
  const { token } = await registerUser(request, "live-updates");
  const client = createClient(request, token);
  await gotoWithSession(page, token);
  await waitForWorkspace(page);
  await expect(page.getByTestId("connection-status")).toHaveText("Connected");
  const wsID = await waitForWorkspace(page);
  const ws = client.ws(wsID);

  const parent = await ws.createPage("0", { title: "Expanded parent", content: "Parent content" });
  const node = await ws.createPage(parent.id, { title: "Live page", content: "Initial content" });
  await page.reload();
  await expect(page.getByTestId("connection-status")).toHaveText("Connected");
  const sidebarNode = page.getByTestId(`sidebar-node-${node.id}`);
  await expect(sidebarNode).toBeVisible();
  await sidebarNode.click();
  const title = page.locator('input[placeholder*="Title"]');
  const editor = page.getByTestId("wysiwyg-editor").locator(".ProseMirror");
  await expect(title).toHaveValue("Live page");
  await expect(editor).toContainText("Initial content");
  const mountedNode = await sidebarNode.elementHandle();

  const visibleStarted = performance.now();
  await ws.updatePage(node.id, {
    expected_revision: (await ws.getNode(node.id)).revision ?? "",
    title: "Updated elsewhere",
    content: "Live content",
  });
  await expect(title).toHaveValue("Updated elsewhere");
  await expect(editor).toContainText("Live content");
  await expect(sidebarNode).toContainText("Updated elsewhere");
  console.log(JSON.stringify({ workload: "committed-page-to-visible", latencyMs: performance.now() - visibleStarted }));
  expect(await mountedNode!.evaluate((element) => element.isConnected)).toBe(true);

  await ws.updatePageFrontmatter(node.id, { icon: "🚀", cover: "" });
  await expect(sidebarNode).toContainText("🚀");
  await ws.updatePageFrontmatter(node.id, { icon: "", cover: "" });
  await expect(sidebarNode).not.toContainText("🚀");
  expect(await mountedNode!.evaluate((element) => element.isConnected)).toBe(true);

  const observer = await page.context().newPage();
  await observer.goto(page.url());
  await expect(observer.getByTestId("connection-status")).toHaveText("Connected");
  const observerTitle = observer.getByRole("textbox", { name: "Title", exact: true });
  await expect(observerTitle).toHaveValue("Updated elsewhere");

  // Freeze browser timers so the draft cannot autosave before the external edit.
  await page.clock.install();
  await page.clock.pauseAt(new Date());
  await title.fill("Unsaved draft");
  const notice = page.getByText("This page changed elsewhere. Refresh to load it and discard your unsaved edits.");

  // Upload and frontmatter echoes from this tab must not refresh away its draft.
  await page.locator('input[type="file"][accept="image/*"]').setInputFiles({
    name: "cover.svg",
    mimeType: "image/svg+xml",
    buffer: Buffer.from('<svg xmlns="http://www.w3.org/2000/svg" width="1" height="1"/>'),
  });
  await expect(async () => {
    const saved = await ws.getNode(node.id);
    expect(saved.cover).toBe("cover.svg");
  }).toPass();
  await expect(title).toHaveValue("Unsaved draft");
  await expect(notice).not.toBeVisible();

  await ws.updatePage(node.id, {
    expected_revision: (await ws.getNode(node.id)).revision ?? "",
    title: "Concurrent edit",
    content: "Concurrent content",
  });
  await expect(notice).toBeVisible();
  await expect(title).toHaveValue("Unsaved draft");
  await expect(editor).toContainText("Live content");

  await page.getByRole("button", { name: "Refresh", exact: true }).click();
  await expect(title).toHaveValue("Concurrent edit");
  await expect(editor).toContainText("Concurrent content");
  await expect(notice).not.toBeVisible();

  // Edits made after a live refresh starts must survive its delayed response.
  let releaseRefresh!: () => void;
  const refreshGate = new Promise<void>((resolve) => {
    releaseRefresh = resolve;
  });
  let refreshStarted!: () => void;
  const refreshing = new Promise<void>((resolve) => {
    refreshStarted = resolve;
  });
  const nodeURL = `**/nodes/${node.id}`;
  await page.route(nodeURL, async (route) => {
    const response = await route.fetch();
    refreshStarted();
    await refreshGate;
    await route.fulfill({ response });
  });
  await ws.updatePage(node.id, {
    expected_revision: (await ws.getNode(node.id)).revision ?? "",
    title: "Delayed refresh",
    content: "Fetched content",
  });
  await refreshing;
  await title.fill("Typed during refresh");
  releaseRefresh();
  await expect(notice).toBeVisible();
  await expect(title).toHaveValue("Typed during refresh");
  await page.unroute(nodeURL);
  await page.getByRole("button", { name: "Refresh", exact: true }).click();
  await expect(title).toHaveValue("Delayed refresh");
  await expect(editor).toContainText("Fetched content");
  await expect(notice).not.toBeVisible();

  // Force the SSE echo to arrive while the browser still awaits its save response.
  let releaseSave!: () => void;
  const saveGate = new Promise<void>((resolve) => {
    releaseSave = resolve;
  });
  let saveWritten!: () => void;
  const written = new Promise<void>((resolve) => {
    saveWritten = resolve;
  });
  await page.route(`**/nodes/${node.id}/page`, async (route) => {
    const response = await route.fetch();
    saveWritten();
    await saveGate;
    await route.fulfill({ response });
  });
  await title.fill("Saved locally");
  await page.clock.runFor(2100);
  await written;
  await expect(observerTitle).toHaveValue("Saved locally");
  await expect(sidebarNode).toContainText("Saved locally");
  await expect(notice).not.toBeVisible();
  releaseSave();
  await expect(page.getByRole("status").filter({ hasText: /^Saved$/ })).toHaveText("Saved");
  await expect(notice).not.toBeVisible();
  await expect(title).toHaveValue("Saved locally");
  expect(await mountedNode!.evaluate((element) => element.isConnected)).toBe(true);
  await observer.close();
});

test("hybrid page removal refreshes another client's tree without discarding its draft", async ({ page, request }) => {
  const { readFile, writeFile } = await import("node:fs/promises");
  const { token } = await registerUser(request, "hybrid-live-removal");
  const client = createClient(request, token);
  await gotoWithSession(page, token);
  await expect(page.getByTestId("connection-status")).toHaveText("Connected");
  const wsID = await waitForWorkspace(page);
  const ws = client.ws(wsID);
  const node = await ws.createPage("0", { title: "Hybrid", content: "Private page content" });
  const schema = await ws.createTable("0", { title: "Hybrid", properties: [] });
  // A hybrid is a supported on-disk combination. Reuse an API-created schema
  // rather than duplicating its storage format in this fixture.
  await writeFile(
    `data-e2e/${wsID}/${node.id}/metadata.json`,
    await readFile(`data-e2e/${wsID}/${schema.id}/metadata.json`),
  );
  await ws.updatePageFrontmatter(node.id, { icon: "🚀", cover: "old-cover.svg" });
  await page.reload();
  await expect(page.getByTestId("connection-status")).toHaveText("Connected");
  await page.getByTestId(`sidebar-node-${node.id}`).click();
  const observer = await page.context().newPage();
  await observer.goto(page.url());
  await expect(observer.getByTestId("connection-status")).toHaveText("Connected");
  await expect(observer.getByTestId(`sidebar-node-${node.id}`)).toContainText("🚀");
  await observer.clock.install();
  await observer.clock.pauseAt(new Date());
  const title = observer.getByRole("textbox", { name: "Title", exact: true });
  await title.fill("Unsaved hybrid draft");
  await ws.deletePage(node.id);
  expect((await ws.getNode(node.id)).has_page).toBe(false);
  expect((await ws.getNode(node.id)).has_table).toBe(true);
  for (const tab of [page, observer]) {
    await expect(tab.getByTestId(`sidebar-node-${node.id}`)).not.toContainText("🚀");
  }
  await expect(title).toHaveValue("Unsaved hybrid draft");
  await expect(
    observer.getByText("This page changed elsewhere. Refresh to load it and discard your unsaved edits."),
  ).toBeVisible();
  await observer.close();
});

test("network recovery refreshes remote changes and retains an unsaved draft", async ({ page, request }) => {
  const { token } = await registerUser(request, "network-recovery");
  const client = createClient(request, token);
  await gotoWithSession(page, token);
  await expect(page.getByTestId("connection-status")).toHaveText("Connected");
  const ws = client.ws(await waitForWorkspace(page));
  const node = await ws.createPage("0", { title: "Network page", content: "Original" });
  await page.reload();
  await page.getByTestId(`sidebar-node-${node.id}`).click();
  const title = page.locator('input[placeholder*="Title"]');
  await expect(title).toHaveValue("Network page");
  const origin = await page.evaluate(() => performance.timeOrigin);
  await page.context().setOffline(true);
  await expect(page.getByTestId("connection-status")).toHaveText("Reconnecting");
  await ws.updatePage(node.id, {
    expected_revision: (await ws.getNode(node.id)).revision ?? "",
    title: "Changed while offline",
    content: "Fresh remote content",
  });
  await page.context().setOffline(false);
  await expect(page.getByTestId("connection-status")).toHaveText("Connected");
  await expect(title).toHaveValue("Changed while offline");
  await title.fill("Keep this unsaved draft");
  await page.context().setOffline(true);
  await expect(page.getByTestId("connection-status")).toHaveText("Reconnecting");
  await ws.updatePage(node.id, {
    expected_revision: (await ws.getNode(node.id)).revision ?? "",
    title: "Another remote edit",
    content: "Latest remote content",
  });
  await page.context().setOffline(false);
  await expect(page.getByTestId("connection-status")).toHaveText("Connected");
  await expect(title).toHaveValue("Keep this unsaved draft");
  await expect(
    page.getByText("This page changed elsewhere. Refresh to load it and discard your unsaved edits."),
  ).toBeVisible();
  expect(await page.evaluate(() => performance.timeOrigin)).toBe(origin);
});
