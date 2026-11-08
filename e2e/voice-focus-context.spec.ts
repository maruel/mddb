// Verifies cancellable browser focus preparation, live selection, and workspace isolation across real voice runtimes.

import { test, expect, registerUser, createClient, gotoWithSession } from "./helpers";
import { gate, installBrowserTools } from "./gomode-fixtures";
import type { Page, Request } from "@playwright/test";

test.use({ launchOptions: { args: ["--use-fake-ui-for-media-stream", "--use-fake-device-for-media-stream"] } });

const frames = (page: Page) => page.evaluate(() => window.browserToolFixture.sent);

test("canceling a stalled focus publication lets the next connection prepare before its response", async ({
  page,
  context,
  request,
}) => {
  await context.grantPermissions(["microphone"]);
  const { token } = await registerUser(request, "voice-focus-reconnect");
  await installBrowserTools(page);
  await gotoWithSession(page, token);
  const requested = gate();
  const release = gate();
  let held = false;
  let heldRequest: Request | undefined;
  await page.route("**/gomode/focus", async (route) => {
    if (held) {
      await route.continue();
      return;
    }
    held = true;
    heldRequest = route.request();
    const response = await route.fetch();
    requested.resolve();
    await release.promise;
    await route.fulfill({ response }).catch((error: unknown) => {
      expect(route.request().failure(), String(error)).not.toBeNull();
    });
  });
  try {
    await page.getByRole("button", { name: "Connect voice assistant", exact: true }).click();
    await requested.promise;
    await page.getByRole("button", { name: "Cancel connection", exact: true }).click();
    // The held response remains unavailable: promise settlement and reconnect
    // alone cannot prove cancellation reached the browser's HTTP transport.
    await expect.poll(() => heldRequest?.failure()?.errorText).toBe("net::ERR_ABORTED");
    await page.getByRole("button", { name: "Connect voice assistant", exact: true }).click();
    await expect.poll(() => page.evaluate(() => window.browserToolFixture.peerCount)).toBe(1);
    await expect.poll(() => frames(page)).toContainEqual(expect.objectContaining({ kind: "session.setup" }));
  } finally {
    release.resolve();
  }
});

test("retiring a host while initial focus is pending cancels setup and never retries its lease", async ({
  page,
  context,
  request,
}) => {
  await context.grantPermissions(["microphone"]);
  const { token } = await registerUser(request, "voice-focus-retirement");
  await installBrowserTools(page);
  await gotoWithSession(page, token);
  await expect(page.getByTestId("user-menu-button")).toBeVisible();
  const requested = gate();
  const release = gate();
  const completed = gate();
  let publications = 0;
  let setupReads = 0;
  page.on("request", (outgoing) => {
    if (outgoing.url().endsWith("/gomode/focus")) publications++;
    if (outgoing.url().includes("/gomode/mcp") && outgoing.postDataJSON()?.params?.uri === "gomode://items")
      setupReads++;
  });
  await page.route("**/gomode/focus", async (route) => {
    const response = await route.fetch();
    requested.resolve();
    await release.promise;
    await route.fulfill({ response });
    completed.resolve();
  });
  await page.getByRole("button", { name: "Connect voice assistant", exact: true }).click();
  await requested.promise;
  await page.getByTestId("user-menu-button").click();
  await page.getByRole("menuitem", { name: "Logout", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Login to mddb", exact: true })).toBeVisible();
  release.resolve();
  await completed.promise;
  await expect(page.getByTestId("voice-overlay")).toHaveCount(0);
  expect(await page.evaluate(() => window.browserToolFixture.peerCount)).toBe(0);
  expect(setupReads).toBe(0);
  expect(publications).toBe(1);
});

test("two tabs publish their own selected nested node before setup and update or clear live focus", async ({
  page,
  context,
  request,
}) => {
  await context.grantPermissions(["microphone"]);
  const { token } = await registerUser(request, "voice-focus-context");
  const client = createClient(request, token);
  const org = await client.createOrganization({ name: "Voice context" });
  const first = await client.org(org.id).createWorkspace({ name: "First tab" });
  const second = await client.org(org.id).createWorkspace({ name: "Second tab" });
  const root = await client.ws(first.id).createPage("0", { title: "First root", content: "Root content" });
  const nested = await client
    .ws(first.id)
    .createPage(root.id, { title: "First nested selection", content: "First nested content" });
  const other = await client.ws(second.id).createPage("0", { title: "Second selection", content: "Second content" });
  await client.switchWorkspace({ ws_id: first.id });
  await installBrowserTools(page);
  await gotoWithSession(page, token, `/w/@${first.id}/@${nested.id}`);
  await expect(page.getByTestId("wysiwyg-editor")).toContainText("First nested content");

  const requested = gate();
  const release = gate();
  let held = false;
  let itemReads = 0;
  const subscriptions: Array<{ workspace: string | null; session: string }> = [];
  const observeSubscriptions = (outgoing: Request) => {
    if (outgoing.headers()["mcp-method"] === "subscriptions/listen")
      subscriptions.push({
        workspace: new URL(outgoing.url()).searchParams.get("workspaceId"),
        session: outgoing.postDataJSON().params._meta["openai/session"],
      });
  };
  page.on("request", (outgoing) => {
    observeSubscriptions(outgoing);
    if (outgoing.url().includes("/gomode/mcp") && outgoing.postDataJSON()?.params?.uri === "gomode://items")
      itemReads++;
  });
  await page.route("**/gomode/focus", async (route) => {
    if (held) {
      await route.continue();
      return;
    }
    held = true;
    const response = await route.fetch();
    requested.resolve();
    await release.promise;
    await route.fulfill({ response });
  });
  await page.getByRole("button", { name: "Connect voice assistant", exact: true }).click();
  await requested.promise;
  expect(itemReads).toBe(0);
  expect((await frames(page)).filter((frame) => frame.kind === "session.setup")).toHaveLength(0);
  release.resolve();
  await expect(page.getByRole("button", { name: "End voice session", exact: true })).toBeVisible();
  const firstSetup = (await frames(page)).find((frame) => frame.kind === "session.setup");
  expect(firstSetup).toMatchObject({
    context: { text: expect.stringContaining(`Active service item: mddb://workspaces/${first.id}/nodes/${nested.id}`) },
  });

  // Changing the server's shared workspace history must not redirect the first
  // tab's already prepared MCP connection to the second tab's workspace.
  await client.switchWorkspace({ ws_id: second.id });
  const tab = await context.newPage();
  tab.on("request", observeSubscriptions);
  await installBrowserTools(tab);
  await gotoWithSession(tab, token, `/w/@${second.id}/@${other.id}`);
  await expect(tab.getByTestId("wysiwyg-editor")).toContainText("Second content");
  await tab.getByRole("button", { name: "Connect voice assistant", exact: true }).click();
  await expect(tab.getByRole("button", { name: "End voice session", exact: true })).toBeVisible();
  expect((await frames(tab)).find((frame) => frame.kind === "session.setup")).toMatchObject({
    context: { text: expect.stringContaining(`Active service item: mddb://workspaces/${second.id}/nodes/${other.id}`) },
  });
  await page.getByRole("button", { name: "First root", exact: true }).click();
  await expect.poll(() => subscriptions.length).toBe(2);
  expect(subscriptions.map((subscription) => subscription.workspace)).toEqual([first.id, second.id]);
  expect(new Set(subscriptions.map((subscription) => subscription.session)).size).toBe(2);
  await expect
    .poll(async () => JSON.stringify(await frames(page)))
    .toContain(`Active service item: mddb://workspaces/${first.id}/nodes/${root.id}`);
  expect(JSON.stringify(await frames(tab))).not.toContain(nested.id);
  expect(JSON.stringify(await frames(page))).not.toContain(other.id);
  await page.getByTestId("user-menu-button").click();
  await page.getByRole("menuitem", { name: "Profile", exact: true }).click();
  await expect.poll(async () => JSON.stringify(await frames(page))).toContain("No active service item.");
  expect((await frames(page)).filter((frame) => frame.kind === "notification")).toHaveLength(0);
  await tab.close();
});
