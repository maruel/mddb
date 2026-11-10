// Verifies live page updates and draft protection for writes from the same account.

import { test, expect, registerUser, waitForWorkspace, createClient, gotoWithSession } from "./helpers";
import { gate } from "./gomode-fixtures";

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

for (const dirty of [false, true]) {
  test(`first subscription refreshes missed changes and preserves a ${dirty ? "dirty" : "clean"} editor`, async ({
    page,
    request,
  }) => {
    const { token } = await registerUser(request, "initial-stream-gap");
    const client = createClient(request, token);
    const org = await client.createOrganization({ name: "Initial synchronization" });
    const workspace = await client.org(org.id).createWorkspace({ name: "Initial gap" });
    const ws = client.ws(workspace.id);
    const node = await ws.createPage("0", { title: "Before subscription", content: "Initial content" });
    await client.switchWorkspace({ ws_id: workspace.id });
    const stream = gate();
    await page.route(`**/workspaces/${workspace.id}/events**`, async (route) => {
      await stream.promise;
      await route.continue();
    });
    try {
      await gotoWithSession(page, token, `/w/@${workspace.id}/@${node.id}`);
      const title = page.getByRole("textbox", { name: "Title", exact: true });
      await expect(title).toHaveValue("Before subscription");
      await expect(page.getByTestId("connection-status")).toHaveText("Reconnecting");
      if (dirty) {
        await page.clock.install();
        await page.clock.pauseAt(new Date());
        await title.fill("Draft created before subscription");
      }
      await ws.updatePage(node.id, {
        expected_revision: (await ws.getNode(node.id)).revision ?? "",
        title: "Changed before subscription",
        content: "Updated remote content",
      });
      const missed = await ws.createPage("0", { title: "Created before subscription", content: "Missed create" });
      stream.resolve();
      await expect(page.getByTestId("connection-status")).toHaveText("Connected");
      await expect(page.getByTestId(`sidebar-node-${missed.id}`)).toBeVisible();
      await expect(title).toHaveValue(dirty ? "Draft created before subscription" : "Changed before subscription");
      const editor = page.getByTestId("wysiwyg-editor").locator(".ProseMirror");
      await expect(editor).toContainText(dirty ? "Initial content" : "Updated remote content");
      if (dirty) {
        await expect(
          page.getByText("This page changed elsewhere. Refresh to load it and discard your unsaved edits."),
        ).toBeVisible();
      }
    } finally {
      stream.resolve();
      await page.unrouteAll({ behavior: "ignoreErrors" });
    }
  });
}

test("a delayed pre-subscription page response cannot publish after first synchronization", async ({
  page,
  request,
}) => {
  const { token } = await registerUser(request, "delayed-initial-page");
  const client = createClient(request, token);
  const org = await client.createOrganization({ name: "Delayed initial read" });
  const workspace = await client.org(org.id).createWorkspace({ name: "Read ownership" });
  const ws = client.ws(workspace.id);
  const node = await ws.createPage("0", { title: "Obsolete initial page", content: "Obsolete content" });
  await client.switchWorkspace({ ws_id: workspace.id });
  const stream = gate();
  const initialRead = gate();
  const releaseRead = gate();
  let pageReads = 0;
  await page.route(`**/workspaces/${workspace.id}/events**`, async (route) => {
    await stream.promise;
    await route.continue();
  });
  await page.route(`**/nodes/${node.id}`, async (route) => {
    pageReads++;
    if (pageReads !== 1) {
      await route.continue();
      return;
    }
    const response = await route.fetch();
    initialRead.resolve();
    await releaseRead.promise;
    await route.fulfill({ response });
  });
  try {
    await gotoWithSession(page, token, `/w/@${workspace.id}/@${node.id}`);
    await initialRead.promise;
    await ws.updatePage(node.id, {
      expected_revision: (await ws.getNode(node.id)).revision ?? "",
      title: "Fresh synchronized page",
      content: "Fresh synchronized content",
    });
    stream.resolve();
    await expect(page.getByTestId("connection-status")).toHaveText("Connected");
    releaseRead.resolve();
    await expect(page.getByRole("textbox", { name: "Title", exact: true })).toHaveValue("Fresh synchronized page");
    await expect(page.getByTestId("wysiwyg-editor").locator(".ProseMirror")).toContainText(
      "Fresh synchronized content",
    );
    expect(pageReads).toBeGreaterThanOrEqual(2);
  } finally {
    stream.resolve();
    releaseRead.resolve();
    await page.unrouteAll({ behavior: "ignoreErrors" });
  }
});

for (const { returnToSource, openBeforeReturn } of [
  { returnToSource: false, openBeforeReturn: false },
  { returnToSource: true, openBeforeReturn: false },
  { returnToSource: true, openBeforeReturn: true },
]) {
  test(`first subscription synchronizes the ${returnToSource ? "returned source" : "pending destination"}${openBeforeReturn ? " after open" : ""} and edits that document`, async ({
    page,
    request,
  }) => {
    const { token } = await registerUser(request, "first-open-navigation");
    const client = createClient(request, token);
    const org = await client.createOrganization({ name: "Subscription navigation" });
    const workspace = await client.org(org.id).createWorkspace({ name: "Destination ownership" });
    const ws = client.ws(workspace.id);
    const source = await ws.createPage("0", { title: "Navigation source", content: "Source body" });
    const target = await ws.createPage("0", { title: "Navigation target", content: "Target body" });
    await client.switchWorkspace({ ws_id: workspace.id });
    const stream = gate(),
      reading = gate(),
      release = gate();
    let reads = 0;
    await page.route(`**/workspaces/${workspace.id}/events**`, async (route) => {
      await stream.promise;
      await route.continue();
    });
    await page.route(`**/nodes/${target.id}`, async (route) => {
      if (++reads !== 1) {
        await route.continue();
        return;
      }
      const response = await route.fetch();
      reading.resolve();
      await release.promise;
      await route.fulfill({ response });
    });
    try {
      await gotoWithSession(page, token, `/w/@${workspace.id}/@${source.id}`);
      const title = page.getByRole("textbox", { name: "Title", exact: true });
      await expect(title).toHaveValue("Navigation source");
      await page.getByTestId(`sidebar-node-${target.id}`).locator("> div").first().click();
      await reading.promise;
      if (returnToSource) {
        await ws.updatePage(source.id, {
          title: "Fresh returned source",
          content: "Fresh source body",
          expected_revision: (await ws.getNode(source.id)).revision ?? "",
        });
        if (openBeforeReturn) {
          stream.resolve();
          await expect(page.getByTestId("connection-status")).toHaveText("Connected");
        }
        await page.goBack();
        await expect(page).toHaveURL(new RegExp(`@${source.id}`));
      }
      stream.resolve();
      await expect(page.getByTestId("connection-status")).toHaveText("Connected");
      if (returnToSource) {
        await expect(title).toHaveValue("Fresh returned source");
        await expect(title).toBeEnabled();
        release.resolve();
        await title.fill("Edited returned source");
        await expect.poll(async () => (await ws.getNode(source.id)).title).toBe("Edited returned source");
        await expect(title).toHaveValue("Edited returned source");
        expect((await ws.getNode(target.id)).title).toBe("Navigation target");
        return;
      }
      await expect(title).toBeDisabled();
      release.resolve();
      await expect(title).toHaveValue("Navigation target");
      await expect(page).toHaveURL(new RegExp(`@${target.id}`));
      await title.fill("Edited destination");
      await expect.poll(async () => (await ws.getNode(target.id)).title).toBe("Edited destination");
      expect((await ws.getNode(source.id)).title).toBe("Navigation source");
      expect(reads).toBeGreaterThanOrEqual(2);
    } finally {
      stream.resolve();
      release.resolve();
      await page.unrouteAll({ behavior: "ignoreErrors" });
    }
  });
}
