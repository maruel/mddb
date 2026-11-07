// Exercises serialized editor acknowledgements, stale-write rejection, preserved drafts, and explicit conflict recovery.

import { test, expect, registerUser, createClient, getWorkspaceId, gotoWithSession } from "./helpers";
import { gate, installNativeTools, invokeNativeFocus, nativeResult, waitNativeReady } from "./gomode-fixtures";
import type { UpdatePageRequest } from "../sdk/types.gen";

test("navigation waits for both serialized saves and uses the acknowledged revision", async ({ page, request }) => {
  const { token } = await registerUser(request, "conditional-queue");
  await gotoWithSession(page, token);
  const wsID = await getWorkspaceId(page);
  const ws = createClient(request, token).ws(wsID);
  const source = await ws.createPage("0", { title: "Queue source", content: "body" });
  const target = await ws.createPage("0", { title: "Queue target", content: "target" });
  await page.goto(`/w/@${wsID}/@${source.id}`);
  const title = page.getByRole("textbox", { name: "Title", exact: true });
  await expect(title).toHaveValue("Queue source");
  const firstWritten = gate(),
    firstReply = gate(),
    secondWritten = gate(),
    secondReply = gate();
  const requests: UpdatePageRequest[] = [];
  let firstRevision = "";
  await page.route(`**/nodes/${source.id}/page`, async (route) => {
    requests.push(route.request().postDataJSON() as UpdatePageRequest);
    const response = await route.fetch();
    if (requests.length === 1) {
      firstRevision = ((await response.json()) as { node: { revision: string } }).node.revision;
      firstWritten.resolve();
      await firstReply.promise;
    } else {
      secondWritten.resolve();
      await secondReply.promise;
    }
    await route.fulfill({ response });
  });
  try {
    await title.fill("First draft");
    await firstWritten.promise;
    await title.fill("Second draft");
    await page.getByTestId(`sidebar-node-${target.id}`).locator("> div").first().click();
    expect(requests).toHaveLength(1);
    await expect(page).toHaveURL(new RegExp(`/@${source.id}(?:\\+|$)`));
    firstReply.resolve();
    await secondWritten.promise;
    expect(requests[1]?.expected_revision).toBe(firstRevision);
    expect(requests[1]?.title).toBe("Second draft");
    await expect(page).toHaveURL(new RegExp(`/@${source.id}(?:\\+|$)`));
    secondReply.resolve();
    await expect(page).toHaveURL(new RegExp(`/@${target.id}(?:\\+|$)`));
    expect((await ws.getNode(source.id)).title).toBe("Second draft");
  } finally {
    firstReply.resolve();
    secondReply.resolve();
  }
});

test("a conflict preserves draft and focus until explicit copy and discard recovery", async ({
  page,
  context,
  request,
}) => {
  const { token } = await registerUser(request, "conditional-conflict");
  await installNativeTools(page, request);
  await gotoWithSession(page, token);
  await waitNativeReady(page);
  const wsID = await getWorkspaceId(page);
  const ws = createClient(request, token).ws(wsID);
  const source = await ws.createPage("0", { title: "Conflict source", content: "Original body" });
  const target = await ws.createPage("0", { title: "Conflict target", content: "Other body" });
  await page.goto(`/w/@${wsID}/@${source.id}`);
  await waitNativeReady(page);
  const title = page.getByRole("textbox", { name: "Title", exact: true });
  await expect(title).toHaveValue("Conflict source");
  await page.clock.install();
  await page.clock.pauseAt(new Date());
  await title.fill("Private draft");
  const latest = await ws.updatePage(source.id, {
    title: "External title",
    content: "External body",
    expected_revision: (await ws.getNode(source.id)).revision ?? "",
  });
  const conflicted = page.waitForResponse(
    (response) => response.url().endsWith(`/nodes/${source.id}/page`) && response.status() === 409,
  );
  await page.getByTestId(`sidebar-node-${target.id}`).locator("> div").first().click();
  await conflicted;
  await expect(page.getByRole("button", { name: "Copy draft", exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Dismiss", exact: true })).not.toBeVisible();
  await page.getByTestId(`sidebar-node-${target.id}`).locator("> div").first().click();
  await expect(page.getByRole("button", { name: "Discard edits and reload", exact: true })).toBeVisible();
  await expect(title).toHaveValue("Private draft");
  await expect(page).toHaveURL(new RegExp(`/@${source.id}(?:\\+|$)`));
  await invokeNativeFocus(page, target.id, "conflicted-focus", "conditional-conversation");
  expect(await nativeResult(page, "conflicted-focus")).toHaveProperty("error");
  await expect(page).toHaveURL(new RegExp(`/@${source.id}(?:\\+|$)`));
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await page.getByRole("button", { name: "Copy draft", exact: true }).click();
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe("# Private draft\n\nOriginal body");
  expect((await ws.getNode(source.id)).revision).toBe(latest.node.revision);
  await page.getByRole("button", { name: "Discard edits and reload", exact: true }).click();
  await expect(title).toHaveValue("External title");
  await expect(page.getByRole("button", { name: "Copy draft", exact: true })).not.toBeVisible();
  const saving = page.waitForResponse(
    (response) => response.url().endsWith(`/nodes/${source.id}/page`) && response.ok(),
  );
  await title.fill("Recovered edit");
  await page.clock.runFor(2100);
  await saving;
  expect((await ws.getNode(source.id)).title).toBe("Recovered edit");
});
