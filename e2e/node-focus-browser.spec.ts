// Exercises prepared node focus through the production browser voice session and its WebRTC channel.

import { test, expect, registerUser, createClient, getWorkspaceId, gotoWithSession } from "./helpers";
import { browserFocus, browserResult, gate, installBrowserTools } from "./gomode-fixtures";

test.use({ launchOptions: { args: ["--use-fake-ui-for-media-stream", "--use-fake-device-for-media-stream"] } });

test("browser transport waits for initial data, then focuses an unloaded nested node from settings exactly once", async ({
  page,
  request,
  context,
}) => {
  await context.grantPermissions(["microphone"]);
  await installBrowserTools(page);
  const { token } = await registerUser(request, "browser-focus");
  const client = createClient(request, token);
  const org = await client.createOrganization({ name: "Browser focus" });
  const workspace = await client.org(org.id).createWorkspace({ name: "Browser focus" });
  await client.switchWorkspace({ ws_id: workspace.id });
  const ws = client.ws(workspace.id);
  const parent = await ws.createPage("0", { title: "Unloaded parent", content: "Parent" });
  const target = await ws.createPage(parent.id, {
    title: "Browser nested target",
    content: "Browser-selected nested content",
  });
  const requested = gate();
  const release = gate();
  await page.route("**/nodes/0/children", async (route) => {
    const response = await route.fetch();
    requested.resolve();
    await release.promise;
    await route.fulfill({ response });
  });
  await gotoWithSession(page, token, "/settings/user");
  await requested.promise;
  await expect(page.getByTestId("voice-overlay")).toHaveCount(0);
  expect(await page.evaluate(() => window.browserToolFixture.peerCount)).toBe(0);
  release.resolve();
  await page.getByRole("button", { name: "Connect voice assistant", exact: true }).click();
  await expect(page.getByRole("button", { name: "End voice session", exact: true })).toBeVisible();
  expect(
    await page.evaluate(() => window.browserToolFixture.sent.find((frame) => frame.kind === "session.setup")?.tools),
  ).toEqual(expect.arrayContaining([expect.objectContaining({ name: "focus_node" })]));
  await expect(page.getByTestId(`sidebar-node-${target.id}`)).toHaveCount(0);
  await browserFocus(page, target.id, "browser-nested");
  expect(await browserResult(page, "browser-nested")).toEqual({ nodeId: target.id, status: "focused" });
  await expect(page).toHaveURL(new RegExp(`/@${target.id}(?:\\+|$)`));
  await expect(page.getByTestId("wysiwyg-editor")).toContainText("Browser-selected nested content");
  expect(
    await page.evaluate(
      () =>
        window.browserToolFixture.sent.filter((frame) => frame.kind === "tool.result" && frame.id === "browser-nested")
          .length,
    ),
  ).toBe(1);
});

test("browser hang-up cancels a pending lookup and releases the transport and microphone", async ({
  page,
  request,
  context,
}) => {
  await context.grantPermissions(["microphone"]);
  await installBrowserTools(page);
  const { token } = await registerUser(request, "browser-focus-cancel");
  await gotoWithSession(page, token);
  await expect(page.getByTestId("user-menu-button")).toBeVisible();
  const workspaceId = await getWorkspaceId(page);
  const target = await createClient(request, token)
    .ws(workspaceId)
    .createPage("0", { title: "Late browser target", content: "Late content" });
  await page.getByRole("button", { name: "Connect voice assistant", exact: true }).click();
  await expect(page.getByRole("button", { name: "End voice session", exact: true })).toBeVisible();
  const requested = gate();
  const release = gate();
  const completed = gate();
  await page.route(`**/nodes/${target.id}`, async (route) => {
    const response = await route.fetch();
    requested.resolve();
    await release.promise;
    await route.fulfill({ response });
    completed.resolve();
  });
  const original = page.url();
  await browserFocus(page, target.id, "hang-up");
  await requested.promise;
  await page.getByRole("button", { name: "End voice session", exact: true }).click();
  await expect.poll(() => page.evaluate(() => window.browserToolFixture.closed)).toBe(true);
  await expect
    .poll(() =>
      page.evaluate(
        () =>
          window.browserToolFixture.tracks.length > 0 &&
          window.browserToolFixture.tracks.every((track) => track.readyState === "ended"),
      ),
    )
    .toBe(true);
  release.resolve();
  await completed.promise;
  await expect(page).toHaveURL(original);
  expect(
    await page.evaluate(() =>
      window.browserToolFixture.sent.some((frame) => frame.kind === "tool.result" && frame.id === "hang-up"),
    ),
  ).toBe(false);
});
