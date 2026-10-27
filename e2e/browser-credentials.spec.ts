// Exercises cookie authentication, cross-site protection, and delayed session/resource ownership in real browsers.

import { test, expect } from "./helpers";
import { createServer } from "node:http";
import type { AddressInfo } from "node:net";

test("browser credentials stay HttpOnly through registration, reload and logout", async ({ page, context }) => {
  const messages: unknown[] = [];
  await page.exposeFunction("recordNativeAuth", (value: string) => {
    const { frontendDocument, frontendRequest, ...auth } = JSON.parse(value);
    expect(frontendDocument).toBe("device-document");
    expect(typeof frontendRequest).toBe("string");
    messages.push(auth);
  });
  await page.addInitScript(() => {
    Object.defineProperty(window, "goModeHost", { value: {} });
    Object.defineProperty(window, "goModeFrontendDocument", { value: Promise.resolve("device-document") });
    Object.defineProperty(window, "gomodeAuth", {
      value: {
        postMessage: (value: string) => {
          const { frontendDocument, frontendRequest, ...auth } = JSON.parse(value);
          if (auth.authChanged === true && auth.nativeAccess === true) {
            const bridge = window as unknown as { gomodeAuth: { onmessage?: (event: { data: string }) => void } };
            bridge.gomodeAuth.onmessage?.({
              data: JSON.stringify({ frontendDocument, frontendRequest, frontendToken: "device-token" }),
            });
          }
          void (window as unknown as { recordNativeAuth: (value: string) => Promise<void> }).recordNativeAuth(value);
        },
      },
    });
  });
  await page.goto("/login");
  await expect(page.getByRole("heading", { name: "Login to mddb" })).toBeVisible();
  await page.getByRole("button", { name: "Register", exact: true }).click();
  await page.getByLabel("Email", { exact: true }).fill(`cookie-browser-${Date.now()}@example.com`);
  await page.getByLabel("Name", { exact: true }).fill("Cookie Browser");
  await page.getByLabel("Password", { exact: true }).fill("testpassword123");
  const registered = page.waitForResponse((r) => r.url().endsWith("/auth/browser/register") && r.ok());
  await page.getByRole("button", { name: "Register", exact: true }).first().click();
  const response = await registered;
  const body: unknown = await response.json();
  expect(typeof body).toBe("object");
  expect(body).not.toHaveProperty("token");
  await expect(page.getByTestId("user-menu-button")).toBeVisible();
  expect(page.url()).not.toContain("token=");
  expect(await page.evaluate(() => localStorage.getItem("mddb_token"))).toBeNull();
  expect(await page.evaluate(() => document.cookie.includes("__Host-mddb_session="))).toBe(false);
  const cookie = (await context.cookies()).find((c) => c.name === "__Host-mddb_session");
  expect(cookie?.secure).toBe(true);
  expect(cookie?.httpOnly).toBe(true);
  expect(cookie?.path).toBe("/");
  await expect.poll(() => messages).toContainEqual({ authChanged: true, nativeAccess: true });
  expect(messages.some((v) => typeof v === "object" && v !== null && "bearerToken" in v)).toBe(false);
  await page.reload();
  await expect(page.getByTestId("user-menu-button")).toBeVisible();
  const rejected = await context.request.post("/api/v1/auth/browser/logout", {
    headers: { Origin: "https://attacker.example", "Sec-Fetch-Site": "cross-site", Authorization: "Bearer forged" },
  });
  expect(rejected.status()).toBe(403);
  expect((await context.request.get("/api/v1/auth/me")).ok()).toBe(true);
  await page.getByTestId("user-menu-button").click();
  await page.getByRole("menuitem", { name: "Logout", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Login to mddb" })).toBeVisible();
  await expect.poll(() => messages).toContainEqual({ authChanged: true, nativeAccess: false });
  expect((await context.cookies()).some((c) => c.name === "__Host-mddb_session")).toBe(false);
  await page.reload();
  await expect(page.getByRole("heading", { name: "Login to mddb" })).toBeVisible();
});

test("same-site cross-origin forms and fetches cannot mutate a cookie session", async ({
  page,
  context,
  request,
  baseURL,
}) => {
  const { registerUser, gotoWithSession } = await import("./helpers");
  const { token } = await registerUser(request, "cookie-csrf");
  await gotoWithSession(page, token);
  await expect(page.getByTestId("user-menu-button")).toBeVisible();
  const attacker = await context.newPage();
  const server = createServer((_req, res) => {
    res.setHeader("Content-Type", "text/html");
    res.end(
      `<iframe name="result"></iframe><form method="POST" action="${baseURL}/api/v1/auth/browser/logout" target="result"><button>Submit</button></form>`,
    );
  });
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  try {
    await attacker.goto(`http://localhost:${(server.address() as AddressInfo).port}/`);
    const submitted = attacker.waitForResponse((r) => r.url().endsWith("/auth/browser/logout"));
    await attacker.getByRole("button", { name: "Submit" }).click();
    const form = await submitted;
    expect(form.status()).toBe(403);
    expect(Boolean((await form.request().allHeaders()).cookie)).toBe(true);
    const fetched = attacker.waitForResponse((r) => r.url().endsWith("/auth/browser/logout"));
    await attacker.evaluate(async (url) => {
      await fetch(url + "/api/v1/auth/browser/logout", {
        method: "POST",
        mode: "no-cors",
        credentials: "include",
        body: "",
      });
    }, baseURL!);
    expect((await fetched).status()).toBe(403);
    await page.reload();
    await expect(page.getByTestId("user-menu-button")).toBeVisible();
  } finally {
    await attacker.close();
    server.closeAllConnections();
    await new Promise<void>((resolve) => server.close(() => resolve()));
  }
});

test("a delayed import cannot switch or poll using a replacement session", async ({ page, context, request }) => {
  const { registerUser, gotoWithSession, createClient } = await import("./helpers");
  const { token, email } = await registerUser(request, "cookie-import-owner");
  await gotoWithSession(page, token);
  await expect(page.locator("aside")).toBeVisible();
  const client = createClient(request, token);
  const original = await client.getMe();
  const imported = await client
    .org(original.organization_id!)
    .createWorkspace({ name: "Imported by original session" });
  const replacement = await createClient(request).login({ email, password: "testpassword123" });
  let release!: () => void;
  const delayed = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route("**/notion/import", async (route) => {
    await delayed;
    await route.fulfill({ json: { workspace_id: imported.id } });
  });
  await page.getByTestId("import-notion-button").click();
  const dialog = page.getByRole("dialog", { name: "Import from Notion" });
  await dialog.locator('input[type="password"]').fill("notion-fixture-token");
  const started = page.waitForRequest((req) => req.url().endsWith("/notion/import"));
  await dialog.getByRole("button", { name: "Start Import" }).click();
  await started;
  const cookie = (await context.cookies()).find((entry) => entry.name === "__Host-mddb_session");
  if (!cookie) throw new Error("Missing browser cookie");
  await context.addCookies([{ ...cookie, value: replacement.token }]);
  const bootstrapped = page.waitForResponse((res) => res.url().endsWith("/auth/me") && res.ok());
  await page.evaluate(() =>
    window.dispatchEvent(
      new StorageEvent("storage", {
        key: "mddb_auth_changed",
        newValue: "nonsecret-change",
        storageArea: localStorage,
      }),
    ),
  );
  expect((await (await bootstrapped).json()).session_id).not.toBe(original.session_id);
  await expect(dialog).not.toBeVisible();
  const laterRequests: string[] = [];
  page.on("request", (req) => laterRequests.push(req.url()));
  const delivered = page.waitForResponse((res) => res.url().endsWith("/notion/import"));
  release();
  await delivered;
  await page.evaluate(
    () => new Promise<void>((resolve) => requestAnimationFrame(() => requestAnimationFrame(() => resolve()))),
  );
  expect(
    laterRequests.some(
      (url) => url.endsWith("/auth/switch-workspace") || url.includes(`/notion/import/${imported.id}/status`),
    ),
  ).toBe(false);
  expect((await createClient(request, replacement.token).getMe()).workspace_id).toBe(original.workspace_id);
});

test("a late autosave response cannot restore the old session's navigation", async ({ page, context, request }) => {
  const { registerUser, gotoWithSession, createClient } = await import("./helpers");
  const { token, email } = await registerUser(request, "cookie-autosave-owner");
  await gotoWithSession(page, token);
  await expect(page.locator("aside")).toBeVisible();
  const original = await createClient(request, token).getMe();
  const replacement = await createClient(request).login({ email, password: "testpassword123" });
  let release!: () => void;
  const delayed = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route("**/nodes/*/page", async (route) => {
    await delayed;
    await route.fulfill({ json: { modified: Math.floor(Date.now() / 1000) } });
  });
  const saving = page.waitForRequest((req) => req.method() === "POST" && /\/nodes\/[^/]+\/page$/.test(req.url()));
  await page.getByPlaceholder("Title", { exact: true }).fill("Old session draft title");
  await saving;
  const cookie = (await context.cookies()).find((entry) => entry.name === "__Host-mddb_session");
  if (!cookie) throw new Error("Missing browser cookie");
  await context.addCookies([{ ...cookie, value: replacement.token }]);
  const bootstrapped = page.waitForResponse((res) => res.url().endsWith("/auth/me") && res.ok());
  await page.evaluate(() =>
    window.dispatchEvent(
      new StorageEvent("storage", {
        key: "mddb_auth_changed",
        newValue: "nonsecret-change",
        storageArea: localStorage,
      }),
    ),
  );
  expect((await (await bootstrapped).json()).session_id).not.toBe(original.session_id);
  await expect(page.getByPlaceholder("Title", { exact: true })).toHaveValue("Welcome");
  const replacementURL = page.url();
  const delivered = page.waitForResponse((res) => /\/nodes\/[^/]+\/page$/.test(res.url()));
  release();
  await delivered;
  await page.evaluate(
    () => new Promise<void>((resolve) => requestAnimationFrame(() => requestAnimationFrame(() => resolve()))),
  );
  expect(page.url()).toBe(replacementURL);
  await expect(page.getByPlaceholder("Title", { exact: true })).toHaveValue("Welcome");
});

test("a delayed table update cannot replace another page's editor or subsequent save", async ({ page, request }) => {
  const { registerUser, gotoWithSession, createClient } = await import("./helpers");
  const { token } = await registerUser(request, "cookie-table-owner");
  const client = createClient(request, token);
  await gotoWithSession(page, token);
  await expect(page.locator("aside")).toBeVisible();
  const user = await client.getMe();
  const ws = client.ws(user.workspace_id!);
  const table = await ws.createTable("0", { title: "Origin table A", properties: [] });
  const target = await ws.createPage("0", { title: "Target page B", content: "Content belonging to B" });
  await page.reload();
  await page.getByTestId(`sidebar-node-${table.id}`).click();
  await expect(page.locator("table")).toBeVisible();
  let release!: () => void;
  const delayed = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route(`**/nodes/${table.id}/table`, async (route) => {
    const response = await route.fetch();
    await delayed;
    await route.fulfill({ response });
  });
  await page.getByTitle("Add Column", { exact: true }).click();
  await page.getByPlaceholder("Column Name", { exact: true }).fill("Delayed column");
  const updating = page.waitForRequest(
    (req) => req.method() === "POST" && req.url().endsWith(`/nodes/${table.id}/table`),
  );
  await page.getByRole("button", { name: "Confirm", exact: true }).click();
  await updating;
  await page.getByTestId(`sidebar-node-${target.id}`).click();
  await expect(page.getByPlaceholder("Title", { exact: true })).toHaveValue("Target page B");
  await expect(page.locator(".ProseMirror")).toContainText("Content belonging to B");
  const delivered = page.waitForResponse((res) => res.url().endsWith(`/nodes/${table.id}/table`));
  release();
  await delivered;
  await page.evaluate(
    () => new Promise<void>((resolve) => requestAnimationFrame(() => requestAnimationFrame(() => resolve()))),
  );
  await expect(page.getByPlaceholder("Title", { exact: true })).toHaveValue("Target page B");
  await expect(page.locator(".ProseMirror")).toContainText("Content belonging to B");
  const saving = page.waitForRequest(
    (req) => req.method() === "POST" && req.url().endsWith(`/nodes/${target.id}/page`),
  );
  await page.getByPlaceholder("Title", { exact: true }).fill("Edited target B");
  const saved = await saving;
  expect(saved.postDataJSON()).toMatchObject({ title: "Edited target B", content: "Content belonging to B" });
  await expect.poll(async () => (await ws.getNode(target.id)).title).toBe("Edited target B");
  expect((await ws.getNode(target.id)).content).toBe("Content belonging to B");
});

test.describe("History preview ownership", () => {
  test.use({ serviceWorkers: "block" });
  test("history previews retain their resource, request order and dismissal", async ({ page, request }) => {
    const { registerUser, gotoWithSession, createClient, getWorkspaceId } = await import("./helpers");
    const { token } = await registerUser(request, "cookie-history-owner");
    await gotoWithSession(page, token);
    await expect(page.locator("aside")).toBeVisible();
    const ws = createClient(request, token).ws(await getWorkspaceId(page));
    const origin = await ws.createPage("0", { title: "History A", content: "Historical origin A" });
    await ws.updatePage(origin.id, { title: "History A", content: "Current origin A" });
    const target = await ws.createPage("0", { title: "Destination B", content: "Current destination B" });
    await page.reload();
    await page.getByTestId(`sidebar-node-${origin.id}`).click();
    const pending: { release: () => void; url: string; fail: boolean }[] = [];
    await page.route(`**/nodes/${origin.id}/history/*`, async (route) => {
      const response = await route.fetch();
      let release!: () => void;
      const delayed = new Promise<void>((resolve) => {
        release = resolve;
      });
      const entry = { release, url: route.request().url(), fail: false };
      pending.push(entry);
      await delayed;
      if (entry.fail) {
        await route.fulfill({
          status: 500,
          json: { error: { code: "internal_error", message: "Obsolete history failure" } },
        });
      } else {
        await route.fulfill({ response });
      }
    });
    const deliver = async (index: number) => {
      const entry = pending[index]!;
      const delivered = page.waitForResponse((res) => res.url() === entry.url);
      entry.release();
      await delivered;
      await page.evaluate(
        () => new Promise<void>((resolve) => requestAnimationFrame(() => requestAnimationFrame(() => resolve()))),
      );
    };
    const openHistory = async () => {
      await page.getByTestId(`sidebar-node-${origin.id}`).click({ button: "right" });
      await page.getByTestId("context-menu-history").click();
    };
    await openHistory();
    await expect.poll(() => pending.length).toBe(1);
    await page.getByTestId(`sidebar-node-${target.id}`).click();
    await expect(page.getByPlaceholder("Title", { exact: true })).toHaveValue("Destination B");
    await deliver(0);
    await expect(page.getByPlaceholder("Title", { exact: true })).toHaveValue("Destination B");
    await expect(page.locator(".ProseMirror")).toContainText("Current destination B");
    const preview = page.getByRole("region", { name: "Markdown preview" });
    await expect(preview).toBeHidden();

    await page.getByTestId(`sidebar-node-${origin.id}`).click();
    await openHistory();
    await expect.poll(() => pending.length).toBe(2);
    const versions = page.locator('button[class*="historyButton"]');
    await versions.nth(1).click();
    await expect.poll(() => pending.length).toBe(3);
    await deliver(2);
    await expect(preview).toContainText("Historical origin A");
    await deliver(1);
    await expect(preview).toContainText("Historical origin A");

    await versions.first().click();
    await expect.poll(() => pending.length).toBe(4);
    await page.getByRole("button", { name: "Close Preview", exact: true }).click();
    await deliver(3);
    await expect(preview).toBeHidden();
    await expect(page.locator(".ProseMirror")).toContainText("Current origin A");
    expect(pending).toHaveLength(4);
    await versions.first().click();
    await expect.poll(() => pending.length).toBe(5);
    await openHistory(); // Hide history while the explicit request is pending.
    pending[4]!.fail = true;
    await deliver(4);
    await expect(preview).toBeHidden();
    await expect(page.getByText(/Obsolete history failure/)).toBeHidden();
    await expect(page.locator(".ProseMirror")).toContainText("Current origin A");
  });
});
