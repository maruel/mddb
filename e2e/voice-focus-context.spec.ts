// Verifies cancellable browser focus preparation, live selection, and workspace isolation across real voice runtimes.

import { test, expect, registerUser, createClient, gotoWithSession } from "./helpers";
import { gate, installBrowserTools, browserResult } from "./gomode-fixtures";
import type { Page, Request } from "@playwright/test";

test.use({ launchOptions: { args: ["--use-fake-ui-for-media-stream", "--use-fake-device-for-media-stream"] } });

const frames = (page: Page) => page.evaluate(() => window.browserToolFixture.sent);

// gomode sends each resource text verbatim, separated by blank lines; the context resource carries "focus".
// Changes queued during a turn accumulate oldest first, so the newest block wins.
function publicContext(
  frame: Record<string, unknown> | undefined,
): { focus: { id: string; uri: string } | null } | null {
  const text = (frame?.context as { text?: string } | undefined)?.text;
  if (!text) return null;
  for (const block of text.split("\n\n").reverse()) {
    try {
      const value = JSON.parse(block) as Record<string, unknown>;
      if ("focus" in value) return value as { focus: { id: string; uri: string } | null };
    } catch {
      // Not JSON: another resource's text.
    }
  }
  return null;
}

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
    if (outgoing.url().includes("/gomode/mcp") && outgoing.postDataJSON()?.params?.uri === "mddb://catalog")
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
  await page.addInitScript(() => {
    const probe = { active: 0, maximum: 0, closed: 0 };
    Object.assign(window, { subscriptionProbe: probe });
    const nativeFetch = window.fetch.bind(window);
    window.fetch = (input, init) => {
      if (new Headers(init?.headers).get("Mcp-Method") !== "subscriptions/listen") return nativeFetch(input, init);
      probe.active++;
      probe.maximum = Math.max(probe.maximum, probe.active);
      let closed = false;
      const close = () => {
        if (!closed) {
          closed = true;
          probe.active--;
          probe.closed++;
        }
      };
      init?.signal?.addEventListener("abort", close, { once: true });
      if (init?.signal?.aborted) close();
      return nativeFetch(input, init).catch((error) => {
        close();
        throw error;
      });
    };
  });
  const { token } = await registerUser(request, "voice-focus-context");
  const client = createClient(request, token);
  const org = await client.createOrganization({ name: "Voice context" });
  const first = await client.org(org.id).createWorkspace({ name: "First tab" });
  const second = await client.org(org.id).createWorkspace({ name: "Second tab" });
  const root = await client.ws(first.id).createPage("0", { title: "First root", content: "Root content" });
  const nested = await client
    .ws(first.id)
    .createPage(root.id, { title: "First nested selection", content: "First nested content" });
  const offscreen = await client
    .ws(first.id)
    .createPage(root.id, { title: "Offscreen dependency", content: "Watched outside selection" });
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
    if (outgoing.url().includes("/gomode/mcp") && outgoing.postDataJSON()?.params?.uri === "mddb://catalog")
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
  expect(publicContext(firstSetup)?.focus).toEqual({
    id: nested.id,
    uri: `mddb://workspaces/${first.id}/nodes/${nested.id}`,
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
  expect(publicContext((await frames(tab)).find((frame) => frame.kind === "session.setup"))?.focus).toEqual({
    id: other.id,
    uri: `mddb://workspaces/${second.id}/nodes/${other.id}`,
  });

  await page.getByRole("button", { name: "First root", exact: true }).click();
  expect(subscriptions).toEqual([]); // Both conversations reuse their existing screen feed.
  await page.evaluate(
    (nodeId) =>
      window.browserToolFixture.receive({
        kind: "tool.call",
        id: "offscreen-read",
        name: "node_read",
        args: { nodeId },
      }),
    offscreen.id,
  );
  const readResult = await browserResult(page, "offscreen-read");
  expect(readResult).toHaveProperty("node.id", offscreen.id);
  expect(readResult).not.toHaveProperty("_meta");
  // Updates carry changed resources only, and queued changes accumulate oldest
  // first: read the newest context block of the newest update that has one.
  const watchedState = (watchedURI: string) =>
    page.evaluate((uri) => {
      const updates = window.browserToolFixture.sent.filter((frame) => frame.kind === "context.update");
      for (const frame of updates.reverse()) {
        const text = (frame.context as { text?: string } | undefined)?.text ?? "";
        for (const block of text.split("\n\n").reverse()) {
          let model: { focus?: unknown; resources?: Array<{ uri: string; state: string }> };
          try {
            model = JSON.parse(block) as typeof model;
          } catch {
            continue; // Not JSON: another resource's text.
          }
          if (!("focus" in model)) continue;
          return model.resources?.find((resource) => resource.uri === uri)?.state ?? null;
        }
      }
      return null;
    }, watchedURI);
  await expect.poll(() => watchedState(`mddb://workspaces/${first.id}/nodes/${offscreen.id}`)).toBe("read");
  const overviewReads = itemReads;
  await client.ws(first.id).updatePage(offscreen.id, {
    expected_revision: (await client.ws(first.id).getNode(offscreen.id)).revision ?? "",
    title: "Offscreen dependency",
    content: "Offscreen changed independently",
  });
  await expect.poll(() => watchedState(`mddb://workspaces/${first.id}/nodes/${offscreen.id}`)).toBe("changed");
  expect(itemReads).toBe(overviewReads);
  // Listing a nested offscreen window watches its result, including returned
  // HasChildren summaries, rather than every listed child's page body.
  const readListing = async (callID: string) => {
    await page.evaluate(
      ({ id, parentId }) =>
        window.browserToolFixture.receive({ kind: "tool.call", id, name: "nodes_list", args: { parentId } }),
      { id: callID, parentId: offscreen.id },
    );
    const result = await browserResult(page, callID);
    expect(result).not.toHaveProperty("_meta");
    return result as { uri: string; nodes: unknown[] };
  };
  const emptyListing = await readListing("offscreen-list-empty");
  expect(emptyListing.nodes).toHaveLength(0);
  await expect.poll(() => watchedState(emptyListing.uri)).toBe("read");
  const listedChild = await client
    .ws(first.id)
    .createPage(offscreen.id, { title: "Listed child", content: "Unwatched body" });
  await expect.poll(() => watchedState(emptyListing.uri)).toBe("changed");
  const populatedListing = await readListing("offscreen-list-child");
  expect(populatedListing.nodes).toHaveLength(1);
  await expect.poll(() => watchedState(populatedListing.uri)).toBe("read");
  await client
    .ws(first.id)
    .createPage(listedChild.id, { title: "Changes represented HasChildren", content: "Nested body" });
  await expect.poll(() => watchedState(populatedListing.uri)).toBe("changed");

  await expect
    .poll(async () => (await frames(page)).some((frame) => publicContext(frame)?.focus?.id === root.id))
    .toBe(true);
  expect(JSON.stringify(await frames(tab))).not.toContain(nested.id);
  expect(JSON.stringify(await frames(page))).not.toContain(other.id);
  await page.getByTestId("user-menu-button").click();
  await page.getByRole("menuitem", { name: "Profile", exact: true }).click();
  await expect
    .poll(async () => {
      // Updates carry changed resources only; the newest one with context decides.
      const updates = (await frames(page)).filter((frame) => frame.kind === "context.update");
      return updates
        .reverse()
        .map(publicContext)
        .find((value) => value !== null)?.focus;
    })
    .toBeNull();
  expect((await frames(page)).filter((frame) => frame.kind === "context.update" && frame.generate)).toHaveLength(0);
  await expect
    .poll(() =>
      page.evaluate(() => (window as unknown as { subscriptionProbe: { active: number } }).subscriptionProbe.active),
    )
    .toBe(1);
  const observations = await page.evaluate(
    () =>
      (window as unknown as { subscriptionProbe: { active: number; maximum: number; closed: number } })
        .subscriptionProbe,
  );
  expect(observations.maximum).toBe(1);
  expect(observations.closed).toBe(subscriptions.length - 1);
  expect(subscriptions.length).toBeGreaterThan(0);
  expect(subscriptions.every((subscription) => subscription.workspace === first.id)).toBe(true);
  expect(new Set(subscriptions.map((subscription) => subscription.session)).size).toBe(1);
  await page.getByRole("button", { name: "End voice session", exact: true }).click();
  await expect
    .poll(() =>
      page.evaluate(() => (window as unknown as { subscriptionProbe: { active: number } }).subscriptionProbe.active),
    )
    .toBe(0);
  await tab.close();
});
