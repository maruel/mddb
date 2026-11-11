// Exercises selected-node cancellation, preparation recovery, resumable ancestry, moves, and collapsed-tree preservation.

import { test, expect, registerUser, createClient, gotoWithSession, waitForWorkspace } from "./helpers";
import { gate } from "./gomode-fixtures";

test("a deep-linked document survives delayed root preparation failure and retry", async ({ page, request }) => {
  const { token } = await registerUser(request, "selected-preparation-retry");
  const client = createClient(request, token);
  const organization = await client.createOrganization({ name: "Selection recovery" });
  const workspace = await client.org(organization.id).createWorkspace({ name: "Selection recovery" });
  await client.switchWorkspace({ ws_id: workspace.id });
  const ws = client.ws(workspace.id);
  const parent = await ws.createPage("0", { title: "Recovered parent", content: "Parent body" });
  const child = await ws.createPage(parent.id, { title: "Recovered child", content: "Deep-linked body" });
  const requested = gate();
  const release = gate();
  let rootReads = 0;
  let documentReads = 0;
  page.on("request", (outgoing) => {
    if (outgoing.url().endsWith(`/nodes/${child.id}`)) documentReads++;
  });
  await page.route(`**/api/v1/workspaces/${workspace.id}/nodes/0/children`, async (route) => {
    if (++rootReads !== 1) return route.continue();
    requested.resolve();
    await release.promise;
    await route.fulfill({
      status: 404,
      json: { error: { code: "NOT_FOUND", message: "Delayed roots unavailable" } },
    });
  });
  // Keep this preparation failure outside subscription synchronization; the
  // first-open retry contract has its own live-update journeys.
  const stream = gate();
  await page.route(`**/workspaces/${workspace.id}/events**`, async (route) => {
    await stream.promise;
    await route.continue();
  });
  try {
    try {
      await gotoWithSession(page, token, `/w/@${workspace.id}/@${child.id}`);
      await requested.promise;
      await expect(page.getByRole("textbox", { name: "Title", exact: true })).toHaveValue("Recovered child");
      await expect(page.getByTestId("wysiwyg-editor")).toContainText("Deep-linked body");
    } finally {
      release.resolve();
    }
    await expect(page.getByRole("alert")).toContainText("Delayed roots unavailable");
    await expect(page.getByRole("textbox", { name: "Title", exact: true })).toHaveValue("Recovered child");
    await page.getByRole("button", { name: "Try again", exact: true }).click();
    await expect(page.getByRole("alert")).toHaveCount(0);
    await expect(
      page
        .getByRole("navigation", { name: "Breadcrumbs" })
        .getByRole("button", { name: "Recovered parent", exact: true }),
    ).toBeVisible();
    await expect(page.getByTestId(`sidebar-node-${child.id}`)).toBeVisible();
    await expect(page.getByTestId("wysiwyg-editor")).toContainText("Deep-linked body");
    expect(documentReads).toBe(1);
    expect(rootReads).toBe(2);
  } finally {
    release.resolve();
    stream.resolve();
    await page.unrouteAll({ behavior: "ignoreErrors" });
  }
});

test("returning to the selected page aborts a delayed target and permits a later visit", async ({ page, request }) => {
  const { token } = await registerUser(request, "selected-node-retirement");
  await gotoWithSession(page, token);
  const workspaceId = await waitForWorkspace(page);
  const ws = createClient(request, token).ws(workspaceId);
  const source = await ws.createPage("0", { title: "Selected source", content: "Source stays selected" });
  const target = await ws.createPage("0", { title: "Delayed target", content: "Target after deliberate visit" });
  await page.goto(`/w/@${workspaceId}/@${source.id}`);
  await expect(page.getByTestId("connection-status")).toHaveText("Connected");
  await waitForWorkspace(page);
  const title = page.getByRole("textbox", { name: "Title", exact: true });
  await expect(title).toHaveValue("Selected source");

  const requested = gate();
  const release = gate();
  const completed = gate();
  const endpoint = `**/nodes/${target.id}`;
  await page.route(endpoint, async (route) => {
    const response = await route.fetch();
    requested.resolve();
    await release.promise;
    await route.fulfill({ response });
    completed.resolve();
  });
  await page.getByTestId(`sidebar-node-${target.id}`).locator("> div").first().click();
  await requested.promise;
  const aborted = page.waitForEvent("requestfailed", (req) => req.url().endsWith(`/nodes/${target.id}`));
  await page.getByTestId(`sidebar-node-${source.id}`).locator("> div").first().click();
  expect((await aborted).failure()?.errorText).toBe("net::ERR_ABORTED");
  await expect(title).toHaveValue("Selected source");
  await expect(page.getByTestId("wysiwyg-editor")).toContainText("Source stays selected");
  release.resolve();
  await completed.promise;
  await expect(page).toHaveURL(new RegExp(`/@${source.id}(?:\\+|$)`));
  await expect(title).toHaveValue("Selected source");

  await page.unroute(endpoint);
  await page.getByTestId(`sidebar-node-${target.id}`).locator("> div").first().click();
  await expect(title).toHaveValue("Delayed target");
  await expect(page.getByTestId("wysiwyg-editor")).toContainText("Target after deliberate visit");
});

test("same-node refresh preserves manual ancestor collapse through path completion", async ({ page, request }) => {
  const { token } = await registerUser(request, "selected-refresh-collapse");
  await gotoWithSession(page, token);
  const workspaceId = await waitForWorkspace(page);
  const ws = createClient(request, token).ws(workspaceId);
  const parent = await ws.createPage("0", { title: "Collapsed parent", content: "Parent body" });
  const child = await ws.createPage(parent.id, { title: "Refresh child", content: "Original body" });
  const ancestor = await ws.getNode(parent.id);
  await page.goto(`/w/@${workspaceId}/@${child.id}`);
  // Finish initial synchronization before capturing the external refresh.
  await expect(page.getByTestId("connection-status")).toHaveText("Connected");
  await waitForWorkspace(page);
  const title = page.getByRole("textbox", { name: "Title", exact: true });
  const breadcrumbs = page.getByRole("navigation", { name: "Breadcrumbs" });
  await expect(title).toHaveValue("Refresh child");
  await expect(breadcrumbs.getByRole("button", { name: "Collapsed parent", exact: true })).toBeVisible();
  const parentItem = page.getByRole("treeitem", { name: "Collapsed parent", exact: true });
  await expect(parentItem).toHaveAttribute("aria-expanded", "true");
  await page.getByTestId(`expand-icon-${parent.id}`).click();
  await expect(parentItem).toHaveAttribute("aria-expanded", "false");
  const requested = gate();
  const release = gate();
  await page.route(`**/nodes/${parent.id}`, async (route) => {
    requested.resolve();
    await release.promise;
    await route.fulfill({ json: { ...ancestor, title: "Completed ancestor" } });
  });
  await ws.updatePage(child.id, {
    expected_revision: (await ws.getNode(child.id)).revision ?? "",
    title: "Refreshed child",
    content: "Refreshed body",
  });
  await requested.promise;
  await expect(title).toHaveValue("Refreshed child");
  await expect(breadcrumbs.getByRole("button", { name: "Collapsed parent", exact: true })).toBeVisible();
  await expect(parentItem).toHaveAttribute("aria-expanded", "false");
  release.resolve();
  await expect(breadcrumbs.getByRole("button", { name: "Completed ancestor", exact: true })).toBeVisible();
  await expect(parentItem).toHaveAttribute("aria-expanded", "false");
});

test("returning to a selected node resumes cancelled ancestry without resetting the editor", async ({
  page,
  request,
}) => {
  const { token } = await registerUser(request, "selected-ancestry-resume");
  await gotoWithSession(page, token);
  const workspaceId = await waitForWorkspace(page);
  const ws = createClient(request, token).ws(workspaceId);
  const parent = await ws.createPage("0", { title: "Resumed parent", content: "Parent body" });
  const child = await ws.createPage(parent.id, { title: "Resumed child", content: "Original body" });
  const target = await ws.createPage("0", { title: "Held target", content: "Target body" });
  const ancestor = await ws.getNode(parent.id);
  const parentRequested = gate();
  const releaseParent = gate();
  const oldParentCompleted = gate();
  const targetRequested = gate();
  const releaseTarget = gate();
  const oldTargetCompleted = gate();
  let parentReads = 0;
  let childReads = 0;
  page.on("request", (outgoing) => {
    if (outgoing.url().endsWith(`/nodes/${child.id}`)) childReads++;
  });
  await page.route(`**/nodes/${parent.id}`, async (route) => {
    parentReads++;
    if (parentReads === 1) {
      parentRequested.resolve();
      await releaseParent.promise;
      await route.fulfill({ json: ancestor });
      oldParentCompleted.resolve();
    } else await route.fulfill({ json: ancestor });
  });
  await page.route(`**/nodes/${target.id}`, async (route) => {
    const response = await route.fetch();
    targetRequested.resolve();
    await releaseTarget.promise;
    await route.fulfill({ response });
    oldTargetCompleted.resolve();
  });
  // Count only resumable ancestry reads here, independently of the initial
  // subscription's deliberate snapshot retries.
  const stream = gate();
  await page.route(`**/workspaces/${workspaceId}/events**`, async (route) => {
    await stream.promise;
    await route.continue();
  });
  try {
    await page.goto(`/w/@${workspaceId}/@${child.id}`);
    // Ancestry is deliberately pending, so the tree remains busy while the
    // selected editor and route actions are already available.
    const title = page.getByRole("textbox", { name: "Title", exact: true });
    await expect(title).toHaveValue("Resumed child");
    await parentRequested.promise;
    const paragraph = page.getByTestId("wysiwyg-editor").locator(".ProseMirror p").first();
    await paragraph.click();
    await page.keyboard.press("Control+End");
    await page.keyboard.type(" kept");
    await expect(paragraph).toHaveText("Original body kept");
    const parentAborted = page.waitForEvent("requestfailed", (outgoing) =>
      outgoing.url().endsWith(`/nodes/${parent.id}`),
    );
    await page.getByRole("treeitem", { name: "Held target", exact: true }).click();
    await targetRequested.promise;
    expect((await parentAborted).failure()?.errorText).toBe("net::ERR_ABORTED");
    const targetAborted = page.waitForEvent("requestfailed", (outgoing) =>
      outgoing.url().endsWith(`/nodes/${target.id}`),
    );
    await page.getByRole("treeitem", { name: "Resumed child", exact: true }).click();
    expect((await targetAborted).failure()?.errorText).toBe("net::ERR_ABORTED");
    const breadcrumbs = page.getByRole("navigation", { name: "Breadcrumbs" });
    await expect(breadcrumbs.getByRole("button", { name: "Resumed parent", exact: true })).toBeVisible();
    await expect(paragraph).toHaveText("Original body kept");
    expect(childReads).toBe(1);
    expect(parentReads).toBe(2);
    await paragraph.click();
    await page.keyboard.press("Control+z");
    await expect(paragraph).toHaveText("Original body");
    releaseParent.resolve();
    releaseTarget.resolve();
    await Promise.all([oldParentCompleted.promise, oldTargetCompleted.promise]);
    await expect(title).toHaveValue("Resumed child");
    await expect(breadcrumbs.getByRole("button", { name: "Resumed parent", exact: true })).toBeVisible();
  } finally {
    releaseParent.resolve();
    releaseTarget.resolve();
    stream.resolve();
    await page.unrouteAll({ behavior: "ignoreErrors" });
  }
});

for (const move of ["selected", "ancestor", "remote ancestor"] as const) {
  test(`${move} move retires captured ancestry and preserves the private draft`, async ({ page, request }) => {
    const { token } = await registerUser(request, "selected-lineage-move");
    await gotoWithSession(page, token);
    const workspaceId = await waitForWorkspace(page);
    const ws = createClient(request, token).ws(workspaceId);
    const source = await ws.createPage("0", { title: "Original ancestor", content: "Parent body" });
    const destination = await ws.createPage("0", { title: "New ancestor", content: "Destination body" });
    const child = await ws.createPage(source.id, { title: "Moving child", content: "Original body" });
    // Keep the existing subscription while entering the child so its captured
    // ancestry belongs to the move, rather than initial stream synchronization.
    await expect(page.getByTestId("connection-status")).toHaveText("Connected");
    await waitForWorkspace(page);
    const sourceItem = page.getByTestId(`sidebar-node-${source.id}`);
    await expect(sourceItem).toBeVisible();
    await sourceItem.locator("> div").first().click();
    await expect(page.getByRole("textbox", { name: "Title", exact: true })).toHaveValue("Original ancestor");
    const childItem = page.getByTestId(`sidebar-node-${child.id}`);
    await expect(childItem).toBeVisible();
    const requested = gate();
    const release = gate();
    const completed = gate();
    let held = false;
    await page.route(`**/nodes/${source.id}`, async (route) => {
      if (held) {
        await route.continue();
        return;
      }
      held = true;
      const response = await route.fetch();
      requested.resolve();
      await release.promise;
      await route.fulfill({ response });
      completed.resolve();
    });
    await childItem.locator("> div").first().click();
    const title = page.getByRole("textbox", { name: "Title", exact: true });
    await expect(title).toHaveValue("Moving child");
    await requested.promise;
    await page.clock.install();
    await page.clock.pauseAt(new Date());
    await title.fill("Private draft survives move");
    const aborted = page.waitForEvent("requestfailed", (outgoing) => outgoing.url().endsWith(`/nodes/${source.id}`));
    const moving = move === "selected" ? child : source;
    if (move === "remote ancestor") await ws.moveNode(moving.id, { new_parent_id: destination.id });
    else {
      await page.getByTestId(`sidebar-node-${moving.id}`).getByRole("treeitem").first().press("Shift+F10");
      await page.getByTestId("context-menu-move-node").click();
      const target = page.getByRole("treeitem", { name: "New ancestor", exact: true });
      await target.focus();
      const moved = page.waitForResponse(
        (response) => response.url().endsWith(`/nodes/${moving.id}/move`) && response.ok(),
      );
      await page.keyboard.press("Control+v");
      await moved;
    }
    expect((await aborted).failure()?.errorText).toBe("net::ERR_ABORTED");
    const breadcrumbs = page.getByRole("navigation", { name: "Breadcrumbs" });
    await expect(breadcrumbs.getByRole("button", { name: "New ancestor", exact: true })).toBeVisible();
    await expect(title).toHaveValue("Private draft survives move");
    expect((await ws.getNode(moving.id)).parent_id).toBe(destination.id);
    release.resolve();
    await completed.promise;
    await expect(breadcrumbs.getByRole("button", { name: "New ancestor", exact: true })).toBeVisible();
    await expect(title).toHaveValue("Private draft survives move");
    await page.clock.resume();
  });
}
