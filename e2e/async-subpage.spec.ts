// Exercises asynchronous subpage insertion, departure ownership, and failed History intents.

import { test, expect, registerUser, createClient, getWorkspaceId, gotoWithSession } from "./helpers";
import { gate } from "./gomode-fixtures";

for (const destination of ["node", "workspace", "deletion", "History"] as const) {
  test(`a subpage creation preserves its parent link before ${destination}`, async ({ page, request }) => {
    const { token } = await registerUser(request, `subpage-owner-${destination.toLowerCase()}`);
    await gotoWithSession(page, token);
    const wsID = await getWorkspaceId(page);
    const client = createClient(request, token);
    const ws = client.ws(wsID);
    const other =
      destination === "workspace"
        ? await client
            .org((await client.getMe()).organization_id!)
            .createWorkspace({ name: "Subpage destination workspace" })
        : null;
    const source = await ws.createPage("0", { title: "Creating parent", content: "" });
    const target = await ws.createPage("0", { title: "Chosen destination", content: "Other body" });
    await page.goto(`/w/@${wsID}/@${source.id}`);
    const title = page.getByRole("textbox", { name: "Title", exact: true });
    await expect(title).toHaveValue("Creating parent");
    const created = gate(),
      createReply = gate(),
      destinationStarted = gate(),
      destinationReply = gate();
    let childID = "";
    await page.route(`**/nodes/${source.id}/page/create`, async (route) => {
      const response = await route.fetch();
      childID = (await response.json()).id;
      created.resolve();
      await createReply.promise;
      await route.fulfill({ response });
    });
    await page.route(
      destination === "deletion" ? `**/nodes/${source.id}/delete` : `**/nodes/${target.id}`,
      async (route) => {
        destinationStarted.resolve();
        await destinationReply.promise;
        if (destination === "deletion")
          await route.fulfill({
            status: 400,
            json: { error: { code: "VALIDATION_FAILED", message: "Deletion rejected" } },
          });
        else await route.continue();
      },
    );
    try {
      await page.locator(".ProseMirror").click();
      await page.keyboard.type("/page");
      await expect(page.getByTestId("slash-command-menu")).toBeVisible();
      await page.keyboard.press("Enter");
      await created.promise;
      if (destination === "node") {
        await page.locator(".ProseMirror").fill("Typed while creating ");
        await page.getByTestId(`sidebar-node-${target.id}`).locator("> div").first().click();
      } else if (other) {
        await page.getByRole("button", { name: "Other workspaces", exact: true }).click();
        await page.getByRole("button", { name: "Subpage destination workspace", exact: true }).click();
      } else {
        const row = page.getByTestId(`sidebar-node-${source.id}`);
        if (destination === "deletion") {
          page.once("dialog", (dialog) => dialog.accept());
          await row.hover();
          await row.getByTestId("delete-node-button").click();
        } else {
          await row.click({ button: "right" });
          await page.getByTestId("context-menu-history").click();
          await expect(page.getByRole("heading", { name: "Version History", exact: true })).toBeVisible();
        }
      }
      const delivered = page.waitForResponse((response) =>
        new URL(response.url()).pathname.endsWith(`/nodes/${source.id}/page/create`),
      );
      createReply.resolve();
      await delivered;
      if (destination === "node" || destination === "deletion") await destinationStarted.promise;
      if (other) await expect(page).toHaveURL(new RegExp(`/w/@${other.id}`));
      if (destination === "History") await expect(page.getByRole("region", { name: "Markdown preview" })).toBeVisible();
      expect((await ws.getNode(source.id)).content).toContain(childID);
      expect((await ws.getNode(childID)).parent_id).toBe(source.id);
      if (destination === "node") expect((await ws.getNode(source.id)).content).toContain("Typed while creating");
      destinationReply.resolve();
      if (destination === "node") await expect(title).toHaveValue("Chosen destination");
      if (destination === "deletion") {
        await expect(title).toBeEnabled();
        await expect(title).toHaveValue("Creating parent");
      }
      if (destination === "History") {
        await page.getByRole("button", { name: "Close Preview", exact: true }).click();
        await expect(page.locator(`.ProseMirror a[href*="${childID}"]`)).toBeVisible();
      }
    } finally {
      createReply.resolve();
      destinationReply.resolve();
    }
  });
}

test("a failed History departure cannot reopen History after later editor-link navigation", async ({
  page,
  request,
}) => {
  const { token } = await registerUser(request, "failed-history-intent");
  await gotoWithSession(page, token);
  const wsID = await getWorkspaceId(page);
  const ws = createClient(request, token).ws(wsID);
  const target = await ws.createPage("0", { title: "History intent target", content: "Target body" });
  const source = await ws.createPage("0", {
    title: "History intent source",
    content: `[Open target](/w/@${wsID}/@${target.id})`,
  });
  await page.goto(`/w/@${wsID}/@${source.id}`);
  const title = page.getByRole("textbox", { name: "Title", exact: true });
  await expect(title).toHaveValue("History intent source");
  await page.route(`**/nodes/${source.id}/page`, (route) =>
    route.fulfill({ status: 409, json: { error: { code: "CONFLICT", message: "Page changed" } } }),
  );
  let targetHistoryReads = 0;
  page.on("request", (call) => {
    if (new URL(call.url()).pathname.endsWith(`/nodes/${target.id}/history`)) targetHistoryReads++;
  });
  await title.fill("Blocked draft");
  await page.getByTestId(`sidebar-node-${target.id}`).click({ button: "right" });
  await page.getByTestId("context-menu-history").click();
  await expect(page.getByRole("button", { name: "Discard edits and reload", exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Discard edits and reload", exact: true }).click();
  await expect(title).toHaveValue("History intent source");
  await page.unroute(`**/nodes/${source.id}/page`);
  await page.locator(`.ProseMirror a[href*="${target.id}"]`).click();
  await expect(title).toHaveValue("History intent target");
  await page.evaluate(
    () => new Promise<void>((resolve) => requestAnimationFrame(() => requestAnimationFrame(() => resolve()))),
  );
  expect(targetHistoryReads).toBe(0);
  await expect(page.getByRole("heading", { name: "Version History", exact: true })).toBeHidden();
  await page.locator(".ProseMirror").fill("Still editable");
  await page.keyboard.press("Control+z");
  await expect(page.locator(".ProseMirror")).toHaveText("Target body");
});

test("a subpage admitted during History fetch is linked before preview replaces the editor", async ({
  page,
  request,
}) => {
  const { token } = await registerUser(request, "subpage-during-history");
  await gotoWithSession(page, token);
  const wsID = await getWorkspaceId(page);
  const ws = createClient(request, token).ws(wsID);
  const source = await ws.createPage("0", { title: "Preview parent", content: "" });
  await page.goto(`/w/@${wsID}/@${source.id}`);
  await expect(page.getByRole("textbox", { name: "Title", exact: true })).toHaveValue("Preview parent");
  const fetching = gate(),
    previewReply = gate(),
    created = gate(),
    createReply = gate();
  let childID = "";
  await page.route(`**/nodes/${source.id}/history/*`, async (route) => {
    const response = await route.fetch();
    fetching.resolve();
    await previewReply.promise;
    await route.fulfill({ response });
  });
  await page.route(`**/nodes/${source.id}/page/create`, async (route) => {
    const response = await route.fetch();
    childID = (await response.json()).id;
    created.resolve();
    await createReply.promise;
    await route.fulfill({ response });
  });
  try {
    await page.getByTestId(`sidebar-node-${source.id}`).click({ button: "right" });
    await page.getByTestId("context-menu-history").click();
    await fetching.promise;
    await page.locator(".ProseMirror").click();
    await page.keyboard.type("/page");
    await expect(page.getByTestId("slash-command-menu")).toBeVisible();
    await page.keyboard.press("Enter");
    await created.promise;
    const previewDelivered = page.waitForResponse((response) =>
      new URL(response.url()).pathname.includes(`/nodes/${source.id}/history/`),
    );
    previewReply.resolve();
    await previewDelivered;
    await page.evaluate(
      () => new Promise<void>((resolve) => requestAnimationFrame(() => requestAnimationFrame(() => resolve()))),
    );
    createReply.resolve();
    await expect(page.getByRole("region", { name: "Markdown preview" })).toBeVisible();
    expect((await ws.getNode(source.id)).content).toContain(childID);
    expect((await ws.getNode(childID)).parent_id).toBe(source.id);
    await page.getByRole("button", { name: "Close Preview", exact: true }).click();
    await expect(page.getByRole("textbox", { name: "Title", exact: true })).toHaveValue("Preview parent");
    await expect(page.locator(`.ProseMirror a[href*="${childID}"]`)).toBeVisible();
  } finally {
    previewReply.resolve();
    createReply.resolve();
  }
});

test("failed subpage creation reports failure and a retry retains the draft and one parent link", async ({
  page,
  request,
}) => {
  const { token } = await registerUser(request, "subpage-creation-retry");
  await gotoWithSession(page, token);
  const wsID = await getWorkspaceId(page);
  const ws = createClient(request, token).ws(wsID);
  const source = await ws.createPage("0", { title: "Retry parent", content: "" });
  await page.goto(`/w/@${wsID}/@${source.id}`);
  const title = page.getByRole("textbox", { name: "Title", exact: true });
  await expect(title).toHaveValue("Retry parent");
  let creates = 0;
  await page.route(`**/nodes/${source.id}/page/create`, async (route) => {
    creates++;
    if (creates === 1)
      await route.fulfill({
        status: 400,
        json: { error: { code: "VALIDATION_FAILED", message: "Subpage creation rejected" } },
      });
    else await route.continue();
  });
  await title.fill("Private parent draft");
  for (const attempt of ["failure", "retry"] as const) {
    await page.locator(".ProseMirror").click();
    await page.keyboard.type("/page");
    await expect(page.getByTestId("slash-command-menu")).toBeVisible();
    await page.keyboard.press("Enter");
    if (attempt === "failure") {
      await expect(page.getByText("Subpage creation rejected", { exact: false })).toBeVisible();
      await expect(title).toHaveValue("Private parent draft");
      expect((await ws.listNodeChildren(source.id)).nodes).toHaveLength(0);
    }
  }
  await expect(title).toHaveValue("Untitled");
  const children = (await ws.listNodeChildren(source.id)).nodes;
  expect(children).toHaveLength(1);
  const parent = await ws.getNode(source.id);
  expect(parent.title).toBe("Private parent draft");
  expect(parent.content).toContain(children[0]!.id);
  expect(creates).toBe(2);
});
