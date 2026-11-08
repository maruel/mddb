// Exercises manual native selections, awaited initial publication, and conversation retirement without tool calls.

import type { UpdateVoiceFocusRequest } from "../sdk/mddb/ts/v1/types.gen";
import type { APIRequestContext } from "@playwright/test";
import { test, expect, registerUser, createClient, gotoWithSession } from "./helpers";
import {
  changeNativeVoiceSession,
  gate,
  installNativeTools,
  invokeNativeFocus,
  nativeResult,
  waitNativeReady,
} from "./gomode-fixtures";

async function activeNodes(request: APIRequestContext, token: string, workspace: string, conversation: string) {
  const response = await request.post(`/api/v1/gomode/mcp?workspaceId=${workspace}`, {
    headers: {
      Authorization: `Bearer ${token}`,
      "Mcp-Method": "resources/read",
      "Mcp-Name": "gomode://items",
      "Mcp-Protocol-Version": "2026-07-28",
    },
    data: {
      jsonrpc: "2.0",
      id: "native-focus",
      method: "resources/read",
      params: {
        uri: "gomode://items",
        _meta: {
          "openai/session": conversation,
          "io.modelcontextprotocol/protocolVersion": "2026-07-28",
          "io.modelcontextprotocol/clientInfo": { name: "native-focus-test", version: "1" },
          "io.modelcontextprotocol/clientCapabilities": {},
        },
      },
    },
  });
  expect(response.status()).toBe(200);
  const body = await response.json();
  expect(body.error).toBeUndefined();
  expect(body.result.contents).toHaveLength(1);
  const items = JSON.parse(body.result.contents[0].text).items as Array<{ id: string; active?: boolean }>;
  return items.filter((item) => item.active).map((item) => item.id);
}

test("manual native navigation tracks and clears the current conversation before any tool invocation", async ({
  page,
  request,
}) => {
  const { token } = await registerUser(request, "manual-native-focus");
  const client = createClient(request, token);
  const org = await client.createOrganization({ name: "Native manual focus" });
  const workspace = await client.org(org.id).createWorkspace({ name: "Manual selection" });
  const first = await client.ws(workspace.id).createPage("0", { title: "Manual first", content: "First native page" });
  const second = await client
    .ws(workspace.id)
    .createPage("0", { title: "Manual second", content: "Second native page" });
  await client.switchWorkspace({ ws_id: workspace.id });
  let focusWrites = 0;
  page.on("request", (outgoing) => {
    if (outgoing.url().endsWith("/gomode/focus")) focusWrites++;
  });
  await installNativeTools(page, request, "native-first");
  await gotoWithSession(page, token, `/w/@${workspace.id}/@${first.id}`);
  await waitNativeReady(page);
  await expect.poll(() => activeNodes(request, token, workspace.id, "native-first")).toEqual([first.id]);
  await expect
    .poll(() => page.evaluate(() => window.nativeToolFixture.readiness.at(-1)?.voiceSessionPrepared))
    .toBe("native-first");
  expect(await page.evaluate(() => window.nativeToolFixture.results)).toEqual([]);
  await expect(page.getByTestId("connection-status")).toHaveText("Connected");
  const initialFocusWrites = focusWrites;
  await client.ws(workspace.id).updatePage(first.id, {
    expected_revision: (await client.ws(workspace.id).getNode(first.id)).revision ?? "",
    title: "Refreshed first node",
    content: "Refreshed native content",
  });
  await expect(page.getByTestId("wysiwyg-editor")).toContainText("Refreshed native content");
  expect(focusWrites).toBe(initialFocusWrites);

  await page.getByTestId(`sidebar-node-${second.id}`).click();
  await expect(page.getByTestId("wysiwyg-editor")).toContainText("Second native page");
  await expect.poll(() => activeNodes(request, token, workspace.id, "native-first")).toEqual([second.id]);
  await page.getByTestId("user-menu-button").click();
  await page.getByRole("menuitem", { name: "Profile", exact: true }).click();
  await expect.poll(() => activeNodes(request, token, workspace.id, "native-first")).toEqual([]);
  await page.goBack();
  await expect(page.getByTestId("wysiwyg-editor")).toContainText("Second native page");
  await expect.poll(() => activeNodes(request, token, workspace.id, "native-first")).toEqual([second.id]);
  await changeNativeVoiceSession(page, null);
  await expect.poll(() => activeNodes(request, token, workspace.id, "native-first")).toEqual([]);
  await page.getByTestId(`sidebar-node-${first.id}`).click();
  await expect(page.getByTestId("wysiwyg-editor")).toContainText("Refreshed native content");
  expect(await activeNodes(request, token, workspace.id, "native-first")).toEqual([]);
  await changeNativeVoiceSession(page, "native-second");
  await expect.poll(() => activeNodes(request, token, workspace.id, "native-second")).toEqual([first.id]);
  expect(await activeNodes(request, token, workspace.id, "native-first")).toEqual([]);
  expect(await page.evaluate(() => window.nativeToolFixture.results)).toEqual([]);
});

test("explicit native focus retries the same selected node after failed publication", async ({ page, request }) => {
  const { token } = await registerUser(request, "native-focus-retry");
  const client = createClient(request, token);
  const org = await client.createOrganization({ name: "Retry native focus" });
  const workspace = await client.org(org.id).createWorkspace({ name: "Retry publication" });
  const node = await client.ws(workspace.id).createPage("0", { title: "Retry node", content: "Retry native context" });
  await client.switchWorkspace({ ws_id: workspace.id });
  await installNativeTools(page, request, "before-retry");
  await gotoWithSession(page, token, `/w/@${workspace.id}/@${node.id}`);
  await waitNativeReady(page);
  await expect(page.getByTestId("wysiwyg-editor")).toContainText("Retry native context");
  await changeNativeVoiceSession(page, null);
  await expect.poll(() => activeNodes(request, token, workspace.id, "before-retry")).toEqual([]);
  let fail = true;
  let attempts = 0;
  await page.route("**/gomode/focus", async (route) => {
    const payload = route.request().postDataJSON() as UpdateVoiceFocusRequest;
    if (!payload.release) attempts++;
    if (fail && !payload.release) await route.fulfill({ json: { applied: false, version: 0 } });
    else await route.continue();
  });
  await changeNativeVoiceSession(page, "retry-native");
  await expect
    .poll(() => page.evaluate(() => window.nativeToolFixture.readiness.at(-1)?.voiceSessionPrepared))
    .toBe("retry-native");
  expect(attempts).toBeGreaterThan(0);
  expect(await activeNodes(request, token, workspace.id, "retry-native")).toEqual([]);
  const failedAttempts = attempts;
  fail = false;
  await invokeNativeFocus(page, node.id, "retry-selected", "retry-native");
  expect(await nativeResult(page, "retry-selected")).toEqual({ nodeId: node.id, status: "focused" });
  await expect.poll(() => attempts).toBeGreaterThan(failedAttempts);
  await expect.poll(() => activeNodes(request, token, workspace.id, "retry-native")).toEqual([node.id]);
});

for (const boundary of ["retirement", "logout"] as const) {
  test(`native ${boundary} aborts initial publication without acknowledging a late completion`, async ({
    page,
    request,
  }) => {
    const { token } = await registerUser(request, "native-publisher-retirement");
    const client = createClient(request, token);
    const org = await client.createOrganization({ name: "Native retirement" });
    const workspace = await client.org(org.id).createWorkspace({ name: "Pending publication" });
    const node = await client
      .ws(workspace.id)
      .createPage("0", { title: "Pending node", content: "Pending native focus" });
    await client.switchWorkspace({ ws_id: workspace.id });
    const requested = gate(),
      release = gate();
    let held = 0;
    let fulfilled = 0;
    await page.route("**/gomode/focus", async (route) => {
      // Route loading can supersede the initial empty selection. Keep every
      // preparation publication pending, while allowing retired-lease cleanup.
      if ((route.request().postDataJSON() as UpdateVoiceFocusRequest).release) {
        await route.continue();
        return;
      }
      held++;
      const response = await route.fetch();
      requested.resolve();
      await release.promise;
      await route.fulfill({ response });
      fulfilled++;
    });
    await installNativeTools(page, request, "pending-native");
    await gotoWithSession(page, token, `/w/@${workspace.id}/@${node.id}`);
    await requested.promise;
    expect(
      await page.evaluate(() =>
        window.nativeToolFixture.readiness.some((item) => item.voiceSessionPrepared === "pending-native"),
      ),
    ).toBe(false);
    if (boundary === "retirement") await changeNativeVoiceSession(page, null);
    else {
      await page.getByTestId("user-menu-button").click();
      await page.getByRole("menuitem", { name: "Logout", exact: true }).click();
      await expect(page.getByRole("heading", { name: "Login to mddb", exact: true })).toBeVisible();
    }
    release.resolve();
    await expect.poll(() => held - fulfilled).toBe(0);
    if (boundary === "retirement")
      await expect.poll(() => activeNodes(request, token, workspace.id, "pending-native")).toEqual([]);
    expect(
      await page.evaluate(() =>
        window.nativeToolFixture.readiness.some((item) => item.voiceSessionPrepared === "pending-native"),
      ),
    ).toBe(false);
  });
}
