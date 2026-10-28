// Exercises prepared native node focus, cancellation ownership, and draft-preserving navigation.

import {
  test,
  expect,
  registerUser,
  createClient,
  getWorkspaceId,
  gotoWithSession,
  switchToMarkdownMode,
} from "./helpers";
import {
  cancelNativeFocus,
  gate,
  installNativeTools,
  invokeNativeFocus,
  nativeResult,
  waitNativeReady,
} from "./gomode-fixtures";

for (const fromSettings of [false, true]) {
  test(`native focus opens an unloaded nested node from ${fromSettings ? "settings" : "workspace"}`, async ({
    page,
    request,
  }) => {
    const { token } = await registerUser(request, "node-focus");
    await installNativeTools(page, request);
    await gotoWithSession(page, token);
    await waitNativeReady(page);
    const workspaceId = await getWorkspaceId(page);
    const workspace = createClient(request, token).ws(workspaceId);
    const parent = await workspace.createPage("0", { title: "Unexpanded parent", content: "Parent" });
    const branch = await workspace.createPage(parent.id, { title: "Collapsed branch", content: "Branch" });
    const nested = await workspace.createPage(branch.id, {
      title: "Nested voice target",
      content: "Nested focus content",
    });
    if (fromSettings) {
      await page.getByTestId("user-menu-button").click();
      await page.getByRole("menuitem", { name: "Profile" }).click();
      await expect(page).toHaveURL(/\/settings\/user$/);
    }
    await expect(page.getByTestId(`sidebar-node-${nested.id}`)).toHaveCount(0);
    await invokeNativeFocus(page, nested.id, "nested");
    expect(await nativeResult(page, "nested")).toEqual({ nodeId: nested.id, status: "focused" });
    await expect(page).toHaveURL(new RegExp(`/@${nested.id}(?:\\+|$)`));
    await expect(page.getByTestId("wysiwyg-editor")).toContainText("Nested focus content");
  });
}

test("invalid and forbidden native targets leave the draft and route intact", async ({ page, request }) => {
  const { token } = await registerUser(request, "focus-invalid");
  await installNativeTools(page, request);
  await gotoWithSession(page, token);
  await waitNativeReady(page);
  const workspaceId = await getWorkspaceId(page);
  const workspace = createClient(request, token).ws(workspaceId);
  const source = await workspace.createPage("0", { title: "Draft source", content: "Saved source" });
  await page.goto(`/w/@${workspaceId}/@${source.id}`);
  await waitNativeReady(page);
  await switchToMarkdownMode(page);
  const editor = page.getByTestId("markdown-editor");
  await editor.fill("Unsaved draft remains selected");
  const original = page.url();
  for (const [id, status] of [
    ["missing", 404],
    ["forbidden", 403],
  ] as const) {
    await page.route(`**/nodes/${id}`, (route) =>
      route.fulfill({
        status,
        json: { error: { code: status === 403 ? "FORBIDDEN" : "NOT_FOUND", message: "Private target details" } },
      }),
    );
    await invokeNativeFocus(page, id, id);
    expect(await nativeResult(page, id)).toEqual({ error: "Node unavailable." });
    await expect(page).toHaveURL(original);
    await expect(editor).toHaveValue("Unsaved draft remains selected");
  }
  await invokeNativeFocus(page, "", "invalid");
  expect(await nativeResult(page, "invalid")).toEqual({ error: "Invalid node ID." });
  await expect(page).toHaveURL(original);
});

test("canceling or choosing another route during validation prevents a late focus", async ({ page, request }) => {
  const { token } = await registerUser(request, "focus-cancel");
  await installNativeTools(page, request);
  await gotoWithSession(page, token);
  await waitNativeReady(page);
  const workspaceId = await getWorkspaceId(page);
  const workspace = createClient(request, token).ws(workspaceId);
  const target = await workspace.createPage("0", { title: "Delayed target", content: "Late target" });
  for (const cancellation of ["native", "navigation"] as const) {
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
    const envelope = await invokeNativeFocus(page, target.id, cancellation);
    await requested.promise;
    if (cancellation === "native") await cancelNativeFocus(page, envelope);
    else {
      await page.getByTestId("user-menu-button").click();
      await page.getByRole("menuitem", { name: "Profile" }).click();
      await expect(page).toHaveURL(/\/settings\/user$/);
    }
    const chosen = page.url();
    release.resolve();
    await completed.promise;
    await expect(page).toHaveURL(chosen);
    if (cancellation === "native") {
      expect(chosen).toBe(original);
      expect(
        await page.evaluate(() => window.nativeToolFixture.results.some((entry) => entry.invocation === "native")),
      ).toBe(false);
    } else expect(await nativeResult(page, cancellation)).toEqual({ error: "Focus canceled." });
    await page.unroute(`**/nodes/${target.id}`);
  }
});

test("late source autosave cannot rewrite a committed focus URL before target loading completes", async ({
  page,
  request,
}) => {
  const { token } = await registerUser(request, "focus-save");
  await installNativeTools(page, request);
  await gotoWithSession(page, token);
  await waitNativeReady(page);
  const workspaceId = await getWorkspaceId(page);
  const workspace = createClient(request, token).ws(workspaceId);
  const source = await workspace.createPage("0", { title: "Save source", content: "Saved source" });
  const target = await workspace.createPage("0", { title: "Save target", content: "Focused target" });
  await page.goto(`/w/@${workspaceId}/@${source.id}`);
  await waitNativeReady(page);
  await switchToMarkdownMode(page);
  await page.getByTestId("markdown-editor").fill("Draft saved before focus");
  const saved = gate();
  const saveReply = gate();
  const saveFinished = gate();
  const targetRequested = gate();
  const targetReply = gate();
  await page.route(`**/nodes/${source.id}/page`, async (route) => {
    const response = await route.fetch();
    saved.resolve();
    await saveReply.promise;
    await route.fulfill({ response });
    saveFinished.resolve();
  });
  let reads = 0;
  await page.route(`**/nodes/${target.id}`, async (route) => {
    const response = await route.fetch();
    if (++reads === 2) {
      targetRequested.resolve();
      await targetReply.promise;
    }
    await route.fulfill({ response });
  });
  try {
    await invokeNativeFocus(page, target.id, "save-race");
    await saved.promise;
    await targetRequested.promise;
    await expect(page).toHaveURL(new RegExp(`/@${target.id}(?:\\+|$)`));
    saveReply.resolve();
    await saveFinished.promise;
    await expect(page.getByRole("status").filter({ hasText: /^Saved$/ })).toBeVisible();
    await expect(page).toHaveURL(new RegExp(`/@${target.id}(?:\\+|$)`));
    targetReply.resolve();
    expect(await nativeResult(page, "save-race")).toEqual({ nodeId: target.id, status: "focused" });
    await expect(page.getByTestId("wysiwyg-editor")).toContainText("Focused target");
    expect((await workspace.getNode(source.id)).content).toBe("Draft saved before focus");
  } finally {
    saveReply.resolve();
    targetReply.resolve();
  }
});

test("a replacement route owns the view while a committed focus target is still loading", async ({ page, request }) => {
  const { token } = await registerUser(request, "focus-owner");
  await installNativeTools(page, request);
  await gotoWithSession(page, token);
  await waitNativeReady(page);
  const workspaceId = await getWorkspaceId(page);
  const target = await createClient(request, token)
    .ws(workspaceId)
    .createPage("0", { title: "Loading focus target", content: "Retired target content" });
  const requested = gate();
  const release = gate();
  const completed = gate();
  let reads = 0;
  await page.route(`**/nodes/${target.id}`, async (route) => {
    const response = await route.fetch();
    if (++reads === 2) {
      requested.resolve();
      await release.promise;
    }
    await route.fulfill({ response });
    if (reads === 2) completed.resolve();
  });
  await invokeNativeFocus(page, target.id, "route-replaced");
  await requested.promise;
  await page.getByTestId("user-menu-button").click();
  await page.getByRole("menuitem", { name: "Profile" }).click();
  await expect(page).toHaveURL(/\/settings\/user$/);
  expect(await nativeResult(page, "route-replaced")).toEqual({ error: "Focus canceled." });
  release.resolve();
  await completed.promise;
  await expect(page).toHaveURL(/\/settings\/user$/);
  await expect(page.getByText("Retired target content", { exact: true })).toHaveCount(0);
});

for (const replacement of ["session", "workspace"] as const) {
  test(`${replacement} replacement revokes a pending native focus before its lookup returns`, async ({
    page,
    request,
  }) => {
    const { token, email } = await registerUser(request, `focus-${replacement}`);
    await installNativeTools(page, request);
    await gotoWithSession(page, token);
    await waitNativeReady(page);
    const client = createClient(request, token);
    const before = await client.getMe();
    const workspaceId = await getWorkspaceId(page);
    const target = await client
      .ws(workspaceId)
      .createPage("0", { title: "Retired lookup target", content: "Old lookup" });
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
    const envelope = await invokeNativeFocus(page, target.id, "replaced");
    await requested.promise;
    let expectedWorkspace = workspaceId;
    if (replacement === "session") {
      const next = await createClient(request).login({ email, password: "testpassword123" });
      const cookie = (await page.context().cookies()).find((entry) => entry.name === "__Host-mddb_session");
      if (!cookie) throw new Error("Missing browser session cookie");
      await page.context().addCookies([{ ...cookie, value: next.token }]);
    } else {
      if (!before.organization_id) throw new Error("Missing workspace organization");
      const next = await client.org(before.organization_id).createWorkspace({ name: "Replacement voice workspace" });
      expectedWorkspace = next.id;
      await client.switchWorkspace({ ws_id: next.id });
    }
    await page.evaluate(() => window.dispatchEvent(new Event("focus")));
    await expect
      .poll(() => page.evaluate(() => window.nativeToolFixture.readiness.slice(-1)[0]?.scope))
      .not.toBe(envelope.scope);
    await waitNativeReady(page);
    if (replacement === "workspace") await expect(page).toHaveURL(new RegExp(`/w/@${expectedWorkspace}(?:\\+|/)`));
    const chosen = page.url();
    release.resolve();
    await completed.promise;
    await expect(page).toHaveURL(chosen);
    expect(
      await page.evaluate(() => window.nativeToolFixture.results.some((entry) => entry.invocation === "replaced")),
    ).toBe(false);
  });
}

test("a target disappearing after validation reports failure without repeatedly loading the failed route", async ({
  page,
  request,
}) => {
  const { token } = await registerUser(request, "focus-disappeared");
  await installNativeTools(page, request);
  await gotoWithSession(page, token);
  await waitNativeReady(page);
  const workspaceId = await getWorkspaceId(page);
  const target = await createClient(request, token)
    .ws(workspaceId)
    .createPage("0", { title: "Disappearing target", content: "Target" });
  let reads = 0;
  await page.route(`**/nodes/${target.id}`, async (route) => {
    if (++reads === 1) await route.continue();
    else await route.fulfill({ status: 404, json: { error: { code: "NOT_FOUND", message: "Removed" } } });
  });
  await invokeNativeFocus(page, target.id, "removed");
  expect(await nativeResult(page, "removed")).toEqual({ error: "Node unavailable." });
  await expect(page).toHaveURL(new RegExp(`/@${target.id}(?:\\+|$)`));
  expect(reads).toBe(2);
});
