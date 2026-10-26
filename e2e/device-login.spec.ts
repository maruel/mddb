// Pairs isolated display and approver browsers through normal mddb sessions and revocation.

import { test, expect, registerUser, createClient, gotoWithSession } from "./helpers";
import type { UserResponse } from "../sdk/types.gen";

test("another-device login preserves user permissions and supports individual revocation", async ({
  browser,
  request,
}) => {
  const { token, email } = await registerUser(request, "device-pair");
  const approverContext = await browser.newContext();
  const displayContext = await browser.newContext({ viewport: { width: 480, height: 800 } });
  try {
    const approver = await approverContext.newPage();
    const display = await displayContext.newPage();
    const consoleErrors: string[] = [];
    for (const page of [approver, display]) page.on("pageerror", (err) => consoleErrors.push(err.message));
    await gotoWithSession(approver, token);
    await expect(approver).toHaveURL(/\/w\/@/, { timeout: 15000 });
    await approver.goto("/device");
    await display.goto("/login");
    await display.getByRole("link", { name: "Sign in with another device" }).click();
    await display.getByRole("button", { name: "Sign in with another device" }).click();
    const code = display.locator("section p").filter({ hasText: /^[A-Z2-9]{4}-[A-Z2-9]{4}-[A-Z2-9]{4}$/ });
    await expect(code).toBeVisible();
    await expect(display.getByText(/Expires in/)).toBeVisible();
    await display.setViewportSize({ width: 320, height: 800 });
    expect(await display.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
    expect(await code.evaluate((el) => parseFloat(getComputedStyle(el).fontSize))).toBeGreaterThanOrEqual(32);
    await display.screenshot({ path: test.info().outputPath("device-code-320.png"), fullPage: true });
    await expect(display.getByText("http://localhost:8080/device", { exact: true })).toBeVisible();
    await approver.getByRole("textbox", { name: "Device code" }).fill(await code.innerText());
    await approver.getByRole("button", { name: "Check code" }).click();
    await expect(approver.getByText("Network", { exact: true })).toBeVisible();
    await expect(approver.getByText(/Never approve a code sent by someone else/)).toBeVisible();
    await approver.setViewportSize({ width: 320, height: 800 });
    expect(await approver.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
    await approver.getByRole("button", { name: "Approve sign-in", exact: true }).focus();
    await approver.screenshot({ path: test.info().outputPath("device-approve-320.png"), fullPage: true });
    await expect(display.getByText("Waiting for approval…")).toBeVisible();
    const pollResponse = display.waitForResponse((r) => r.url().endsWith("/auth/browser/device/poll") && r.ok());
    await approver.getByRole("button", { name: "Approve sign-in", exact: true }).click();
    await expect(approver.getByText("Sign-in approved. You can close this page.")).toBeVisible();
    const response = await pollResponse;
    const user: UserResponse = await response.json();
    expect("token" in user).toBe(false);
    expect(user.email).toBe(email);
    await expect(display).toHaveURL(/\/w\/@/);
    expect(await display.evaluate(() => localStorage.getItem("mddb_token"))).toBeNull();
    expect(await display.evaluate(() => document.cookie.includes("__Host-mddb_session="))).toBe(false);
    const displayApi = createClient(displayContext.request);
    expect((await displayApi.getMe()).email).toBe(email);
    const me = await displayApi.getMe();
    const draft = await displayApi
      .ws(me.workspace_id!)
      .createPage("0", { title: "Paired device draft", content: "Device content" });
    await display.goto(`/w/@${me.workspace_id}/@${draft.id}`);
    await display.getByPlaceholder("Title", { exact: true }).fill("Edited from paired device");
    await expect
      .poll(async () => (await displayApi.ws(me.workspace_id!).getNode(draft.id)).title)
      .toBe("Edited from paired device");
    await display.goto("/settings/user");
    await expect(display.getByRole("button", { name: "Enable browser notifications", exact: true })).toBeVisible();
    const approverApi = createClient(request, token);
    const sessions = await approverApi.listSessions();
    const created = sessions.sessions.find((s) => !s.is_current);
    expect(created).toBeDefined();
    if (!created) throw new Error("Missing display session");
    expect(created.expires_at - created.created).toBeGreaterThan(31 * 24 * 3600 - 1);
    expect(created.expires_at - created.created).toBeLessThanOrEqual(31 * 24 * 3600);
    await approverApi.revokeSession({ session_id: created.id });
    await expect(displayApi.getMe()).rejects.toThrow();
    expect((await approverApi.getMe()).email).toBe(email);
    await display.reload();
    await expect(display).toHaveURL(/\/login/);
    await expect.poll(() => display.evaluate(() => localStorage.getItem("mddb_token"))).toBeNull();
    expect(consoleErrors).toEqual([]);
  } finally {
    await approverContext.close();
    await displayContext.close();
  }
});

test("signed-out provider approval returns to the verification page and identifies the account", async ({
  browser,
}) => {
  const displayContext = await browser.newContext();
  const approverContext = await browser.newContext();
  try {
    const display = await displayContext.newPage();
    const approver = await approverContext.newPage();
    await display.goto("/device-login");
    await display.getByRole("button", { name: "Sign in with another device" }).click();
    const code = await display.getByLabel("Device code", { exact: true }).innerText();
    await approver.goto("/device");
    await approver.getByRole("link", { name: /Google/ }).click();
    await expect(approver).toHaveURL("http://localhost:8080/device");
    await expect(approver.getByText("Signed in as Google Test User (test-google@example.com).")).toBeVisible();
    await approver.getByRole("textbox", { name: "Device code" }).fill(code);
    await approver.getByRole("button", { name: "Check code" }).click();
    await approver.getByRole("button", { name: "Deny sign-in", exact: true }).click();
    await expect(display.getByText("Sign-in denied.", { exact: true })).toBeVisible({ timeout: 10000 });
    await approver.getByRole("button", { name: "Use another account" }).click();
    await expect(approver.getByRole("textbox", { name: "Email" })).toBeVisible();
    await expect(approver.getByRole("link", { name: /Google/ })).toHaveAttribute(
      "href",
      "/api/v1/auth/oauth/google?next=%2Fdevice",
    );
  } finally {
    await displayContext.close();
    await approverContext.close();
  }
});

test("changed approving account clears the old decision and keeps the replacement session", async ({
  browser,
  request,
}) => {
  const first = await registerUser(request, "device-first");
  const second = await registerUser(request, "device-second");
  const context = await browser.newContext();
  const displayContext = await browser.newContext();
  try {
    const approver = await context.newPage();
    const display = await displayContext.newPage();
    await gotoWithSession(approver, first.token, "/device");
    await display.goto("/device-login");
    await display.getByRole("button", { name: "Sign in with another device" }).click();
    const code = await display.getByLabel("Device code", { exact: true }).innerText();
    await approver.getByRole("textbox", { name: "Device code" }).fill(code);
    await approver.getByRole("button", { name: "Check code" }).click();
    await expect(approver.getByRole("button", { name: "Approve sign-in", exact: true })).toBeVisible();
    const cookie = (await context.cookies()).find((entry) => entry.name === "__Host-mddb_session");
    if (!cookie) throw new Error("Missing approver cookie");
    await context.addCookies([{ ...cookie, value: second.token }]);
    await approver.getByRole("button", { name: "Approve sign-in", exact: true }).click();
    await expect(approver.getByText(second.email)).toBeVisible();
    await expect(approver.getByRole("textbox", { name: "Device code" })).toHaveValue("");
    await expect(approver.getByRole("button", { name: "Approve sign-in", exact: true })).toHaveCount(0);
    expect((await createClient(context.request).getMe()).email).toBe(second.email);
    await approver.getByRole("textbox", { name: "Device code" }).fill(code);
    await approver.getByRole("button", { name: "Check code" }).click();
    await approver.getByRole("button", { name: "Approve sign-in", exact: true }).click();
    await expect(display).toHaveURL(/\/w\/@/, { timeout: 15000 });
    expect((await createClient(displayContext.request).getMe()).email).toBe(second.email);
  } finally {
    await context.close();
    await displayContext.close();
  }
});

test("another-device denial leaves the display signed out and approval requires sign in", async ({
  browser,
  request,
}) => {
  const { token } = await registerUser(request, "device-deny");
  const approverContext = await browser.newContext();
  const displayContext = await browser.newContext();
  try {
    const display = await displayContext.newPage();
    const approver = await approverContext.newPage();
    await approver.goto("/device");
    await expect(approver.getByRole("textbox", { name: "Email" })).toBeVisible();
    await expect(approver.getByRole("button", { name: "Approve sign-in" })).toHaveCount(0);
    await expect(approver.getByRole("link", { name: "Sign in with another device" })).toHaveCount(0);
    // Use the ordinary token handoff to authenticate, then return to manual approval.
    await gotoWithSession(approver, token);
    await expect(approver).toHaveURL(/\/w\/@/, { timeout: 15000 });
    await approver.goto("/device");
    await display.goto("/device-login");
    await display.getByRole("button", { name: "Sign in with another device" }).click();
    const code = display.locator("section p").filter({ hasText: /^[A-Z2-9]{4}-[A-Z2-9]{4}-[A-Z2-9]{4}$/ });
    await expect(code).toBeVisible();
    await approver.getByRole("textbox", { name: "Device code" }).fill(await code.innerText());
    await approver.getByRole("button", { name: "Check code" }).click();
    await approver.getByRole("button", { name: "Deny sign-in", exact: true }).click();
    await expect(approver.getByText("Sign-in denied. You can close this page.")).toBeVisible();
    await expect(display.getByText("Sign-in denied.", { exact: true })).toBeVisible({ timeout: 10000 });
    expect(await display.evaluate(() => localStorage.getItem("mddb_token"))).toBeNull();
  } finally {
    await approverContext.close();
    await displayContext.close();
  }
});

test("approval cannot use a replacement session for the same account", async ({ browser, request }) => {
  const { token, email } = await registerUser(request, "device-session-pin");
  const context = await browser.newContext();
  try {
    const approver = await context.newPage();
    await gotoWithSession(approver, token, "/device");
    const started = (await (await request.post("/api/v1/auth/device/start", { data: {} })).json()) as {
      user_code: string;
      device_code: string;
    };
    await approver.getByRole("textbox", { name: "Device code" }).fill(started.user_code);
    const checked = approver.waitForResponse((response) => response.url().endsWith("/auth/device/lookup"));
    await approver.getByRole("button", { name: "Check code" }).click();
    const proof = (await (await checked).json()) as { csrf_token: string };
    await expect(approver.getByRole("button", { name: "Approve sign-in", exact: true })).toBeVisible();
    const original = await createClient(request, token).getMe();
    const replacement = await createClient(request).login({ email, password: "testpassword123" });
    const cookie = (await context.cookies()).find((entry) => entry.name === "__Host-mddb_session");
    if (!cookie) throw new Error("Missing approver cookie");
    await context.addCookies([{ ...cookie, value: replacement.token }]);
    await approver.getByRole("button", { name: "Approve sign-in", exact: true }).click();
    await expect(approver.getByRole("textbox", { name: "Device code" })).toHaveValue("");
    await expect(approver.getByRole("button", { name: "Approve sign-in", exact: true })).toHaveCount(0);
    expect((await createClient(context.request).getMe()).session_id).toBe(replacement.user?.session_id);
    // Independently exercise the server pin boundary: bypassing the browser's
    // fresh identity check must still reject a stale session.
    const stale = await context.request.post("/api/v1/auth/device/approve", {
      headers: { "X-MDDB-Session": original.session_id ?? "", Origin: "http://localhost:8080" },
      data: { user_code: started.user_code, approve: true },
    });
    expect(stale.status()).toBe(401);
    for (const pin of ["", replacement.user?.session_id ?? ""]) {
      const replay = await context.request.post("/api/v1/auth/device/approve", {
        headers: { "X-MDDB-Session": pin, "X-Device-Login-CSRF": proof.csrf_token, Origin: "http://localhost:8080" },
        data: { user_code: started.user_code, approve: true },
      });
      expect(replay.status()).toBe(403);
    }
    await approver.getByRole("textbox", { name: "Device code" }).fill(started.user_code);
    await approver.getByRole("button", { name: "Check code" }).click();
    await approver.getByRole("button", { name: "Approve sign-in", exact: true }).click();
    await expect(approver.getByRole("status")).toHaveText("Sign-in approved. You can close this page.");
  } finally {
    await context.close();
  }
});

test("pairing failures offer explicit localized recovery without replaying decisions", async ({ browser, request }) => {
  const { token } = await registerUser(request, "device-errors");
  const displayContext = await browser.newContext({ viewport: { width: 320, height: 960 } });
  const approverContext = await browser.newContext({ viewport: { width: 320, height: 960 } });
  try {
    const display = await displayContext.newPage();
    const approver = await approverContext.newPage();
    await display.route("**/auth/device/start", (route) =>
      route.fulfill({
        json: {
          device_code: "private-test-secret",
          user_code: "BCDF-GHJK-MNPQ",
          verification_uri: "http://localhost:8080/device",
          expires_in: 600,
          interval: 1,
        },
      }),
    );
    for (const [error, message] of [
      ["expired_token", "This code expired. Create a new code."],
      ["access_denied", "Sign-in denied."],
      ["rate_limited", "Too many attempts. Wait 600 seconds before creating a new code."],
      ["audit_failed", "Sign-in could not be recorded. Create a new code."],
      ["session_failed", "A session could not be completed. Create a new code."],
      ["network", "Sign-in could not be confirmed. Check whether this browser is signed in, or create a new code."],
    ]) {
      await display.route("**/auth/browser/device/poll", (route) =>
        error === "network"
          ? route.abort()
          : route.fulfill({ status: error === "rate_limited" ? 429 : 400, json: { error } }),
      );
      await display.goto("/device-login");
      await display.getByRole("button", { name: "Sign in with another device", exact: true }).click();
      await expect(display.getByRole("status")).toHaveText(message ?? "");
      await expect(display.getByRole("button", { name: "Create a new code", exact: true })).toBeVisible();
      if (error === "network")
        await expect(display.getByRole("button", { name: "Check sign-in", exact: true })).toBeVisible();
      expect(await display.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
      await display.unroute("**/auth/browser/device/poll");
    }
    await display.getByRole("button", { name: "Check sign-in", exact: true }).click();
    await expect(display.getByRole("status")).toHaveText(
      "Sign-in could not be confirmed. Check whether this browser is signed in, or create a new code.",
    );
    await expect(display.getByRole("button", { name: "Create a new code", exact: true })).toBeVisible();
    await displayContext.addCookies([
      {
        name: "__Host-mddb_session",
        value: token,
        domain: "localhost",
        path: "/",
        secure: true,
        httpOnly: true,
        sameSite: "Lax",
      },
    ]);
    await display.getByRole("button", { name: "Check sign-in", exact: true }).click();
    await expect(display).toHaveURL(/\/w\/@/, { timeout: 15000 });
    await gotoWithSession(approver, token, "/device");
    const details = {
      user_code: "BCDF-GHJK-MNPQ",
      ip: "192.0.2.0/24",
      user_agent: "Touchscreen car browser",
      created_at: new Date().toISOString(),
      expires_at: new Date(Date.now() + 600000).toISOString(),
      csrf_token: "test-csrf",
    };
    for (const [error, message] of [
      ["expired_token", "This code expired. Create a new code on the device you want to sign in."],
      ["rate_limited", "Too many attempts. Wait 60 seconds before checking the code again."],
      ["audit_failed", "Your decision could not be recorded. Check the code again before approving or denying."],
      ["csrf_failed", "Your account or request changed. Check the code again."],
      ["already_decided", "This code is being processed or already used. Check the other device."],
      ["network", "Your decision could not be confirmed. Check the code again, and check the other device."],
    ]) {
      const lookupFailure = error === "expired_token" || error === "rate_limited";
      await approver.route("**/auth/device/lookup", (route) =>
        route.fulfill(
          lookupFailure
            ? { status: error === "rate_limited" ? 429 : 400, headers: { "Retry-After": "60" }, json: { error } }
            : { json: details },
        ),
      );
      await approver.route("**/auth/device/approve", (route) =>
        error === "network" ? route.abort() : route.fulfill({ status: 500, json: { error } }),
      );
      await approver.goto("/device");
      await approver.getByRole("textbox", { name: "Device code" }).fill(details.user_code);
      await approver.getByRole("button", { name: "Check code", exact: true }).click();
      if (!lookupFailure) await approver.getByRole("button", { name: "Approve sign-in", exact: true }).click();
      await expect(approver.getByRole("status")).toHaveText(message ?? "");
      await expect(approver.getByRole("button", { name: "Approve sign-in", exact: true })).toHaveCount(0);
      expect(await approver.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
      await approver.unroute("**/auth/device/lookup");
      await approver.unroute("**/auth/device/approve");
    }
  } finally {
    await displayContext.close();
    await approverContext.close();
  }
});

test("pairing code, countdown, account and decisions reflow at native 200% and 320px", async ({
  browser,
  request,
}, testInfo) => {
  const { mkdir, writeFile } = await import("node:fs/promises");
  const path = await import("node:path");
  const extension = testInfo.outputPath("zoom-extension");
  await mkdir(extension, { recursive: true });
  await writeFile(
    path.join(extension, "manifest.json"),
    JSON.stringify({
      manifest_version: 3,
      name: "Device pairing zoom",
      version: "1.0",
      background: { service_worker: "background.js" },
    }),
  );
  await writeFile(path.join(extension, "background.js"), "chrome.runtime.onInstalled.addListener(() => {});");
  const context = await browser.browserType().launchPersistentContext(testInfo.outputPath("zoom-profile"), {
    channel: "chromium",
    headless: true,
    baseURL: "http://localhost:8080",
    viewport: { width: 1280, height: 960 },
    args: [`--disable-extensions-except=${extension}`, `--load-extension=${extension}`],
  });
  try {
    const worker = context.serviceWorkers()[0] ?? (await context.waitForEvent("serviceworker"));
    const page = context.pages()[0];
    if (!page) throw new Error("Missing browser page");
    await page.goto("/device-login");
    await page.evaluate(() =>
      window.dispatchEvent(
        Object.assign(new Event("beforeinstallprompt"), {
          prompt: async () => undefined,
          userChoice: Promise.resolve({ outcome: "dismissed" }),
        }),
      ),
    );
    await expect(page.getByText("Install mddb", { exact: true })).toHaveCount(0);
    await page.getByRole("button", { name: "Sign in with another device" }).click();
    const code = await page.getByLabel("Device code", { exact: true }).innerText();
    await worker.evaluate("chrome.tabs.setZoom(2)");
    const cdp = await context.newCDPSession(page);
    await expect.poll(() => page.evaluate(() => innerWidth)).toBe(640);
    await expect(page.getByText(/Expires in/)).toBeVisible();
    const displayCapture = await cdp.send("Page.captureScreenshot", { format: "png", fromSurface: false });
    await writeFile(testInfo.outputPath("display-200-percent.png"), Buffer.from(displayCapture.data, "base64"));
    const { token } = await registerUser(request, "device-zoom");
    await gotoWithSession(page, token, "/device");
    await page.evaluate(() =>
      window.dispatchEvent(
        Object.assign(new Event("beforeinstallprompt"), {
          prompt: async () => undefined,
          userChoice: Promise.resolve({ outcome: "dismissed" }),
        }),
      ),
    );
    await expect(page.getByText("Install mddb", { exact: true })).toHaveCount(0);
    await page.getByRole("textbox", { name: "Device code" }).fill(code);
    await page.getByRole("button", { name: "Check code" }).click();
    for (const width of [1280, 640]) {
      await page.setViewportSize({ width, height: 960 });
      await expect.poll(() => page.evaluate(() => innerWidth)).toBe(width / 2);
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
      for (const name of ["Approve sign-in", "Deny sign-in", "Use another account"]) {
        const button = page.getByRole("button", { name, exact: true });
        await button.scrollIntoViewIfNeeded();
        const box = await button.boundingBox();
        if (!box) throw new Error("Missing decision control");
        expect(box.x).toBeGreaterThanOrEqual(0);
        expect(box.x + box.width).toBeLessThanOrEqual(width / 2);
        expect(box.height).toBeGreaterThanOrEqual(44);
      }
      await page.getByRole("button", { name: "Deny sign-in", exact: true }).scrollIntoViewIfNeeded();
      const capture = await cdp.send("Page.captureScreenshot", { format: "png", fromSurface: false });
      await writeFile(
        testInfo.outputPath(`approval-${width / 2}-200-percent.png`),
        Buffer.from(capture.data, "base64"),
      );
    }
    await worker.evaluate("chrome.tabs.setZoom(1)");
    await page.setViewportSize({ width: 320, height: 960 });
    await page.screenshot({ path: testInfo.outputPath("approval-320-full.png"), fullPage: true });
    await page.setViewportSize({ width: 640, height: 960 });
    await worker.evaluate("chrome.tabs.setZoom(2)");
    await expect.poll(() => page.evaluate(() => innerWidth)).toBe(320);
    await page.getByRole("button", { name: "Deny sign-in", exact: true }).click();
    await expect(page.getByRole("status")).toHaveText("Sign-in denied. You can close this page.");
    await cdp.detach();
  } finally {
    await context.close();
  }
});

test("fresh approval probe losing its session returns to sign-in without a decision", async ({ browser, request }) => {
  const { token } = await registerUser(request, "device-signedout");
  const context = await browser.newContext();
  try {
    const page = await context.newPage();
    await gotoWithSession(page, token, "/device");
    const started = (await (await request.post("/api/v1/auth/device/start", { data: {} })).json()) as {
      user_code: string;
    };
    await page.getByRole("textbox", { name: "Device code" }).fill(started.user_code);
    await page.getByRole("button", { name: "Check code" }).click();
    await expect(page.getByRole("button", { name: "Approve sign-in", exact: true })).toBeVisible();
    await context.clearCookies();
    await page.getByRole("button", { name: "Approve sign-in", exact: true }).click();
    await expect(page.getByRole("textbox", { name: "Email", exact: true })).toBeVisible();
    await expect(page.getByRole("button", { name: "Approve sign-in", exact: true })).toHaveCount(0);
    const pending = await request.post("/api/v1/auth/device/lookup", {
      headers: { Authorization: `Bearer ${token}` },
      data: { user_code: started.user_code },
    });
    expect(pending.status()).toBe(200);
  } finally {
    await context.close();
  }
});

test("a delayed signed-out probe rediscovers a replacement cookie before settling native identity", async ({
  browser,
  request,
}) => {
  const { token, email } = await registerUser(request, "device-late401");
  const context = await browser.newContext();
  await context.addInitScript(() => {
    Object.defineProperty(window, "goModeHost", { value: {} });
    Object.defineProperty(window, "gomodeAuth", {
      value: {
        postMessage: (message: string) => {
          const events: unknown[] = JSON.parse(sessionStorage.getItem("test-auth-events") ?? "[]");
          events.push(JSON.parse(message));
          sessionStorage.setItem("test-auth-events", JSON.stringify(events));
        },
      },
    });
  });
  try {
    const page = await context.newPage();
    await gotoWithSession(page, token, "/device");
    const started = (await (await request.post("/api/v1/auth/device/start", { data: {} })).json()) as {
      user_code: string;
    };
    await page.getByRole("textbox", { name: "Device code" }).fill(started.user_code);
    await page.getByRole("button", { name: "Check code" }).click();
    await expect(page.getByRole("button", { name: "Approve sign-in", exact: true })).toBeVisible();
    await context.clearCookies();
    await page.evaluate(() => sessionStorage.removeItem("test-auth-events"));
    await page.route(
      "**/api/v1/auth/me",
      async (route) => {
        const stale = await route.fetch();
        expect(stale.status()).toBe(401);
        const loggedIn = await context.request.post("/api/v1/auth/browser/login", {
          data: { email, password: "testpassword123" },
          headers: { Origin: "http://localhost:8080" },
        });
        expect(loggedIn.ok()).toBe(true);
        await route.fulfill({ response: stale });
      },
      { times: 1 },
    );
    await page.getByRole("button", { name: "Approve sign-in", exact: true }).click();
    await expect(page.getByRole("textbox", { name: "Device code" })).toHaveValue("");
    await expect(page.getByText(email, { exact: false })).toBeVisible();
    const events = (await page.evaluate(() =>
      JSON.parse(sessionStorage.getItem("test-auth-events") ?? "[]"),
    )) as unknown[];
    expect(events.length).toBeGreaterThanOrEqual(2);
    for (const event of events.slice(0, -1)) expect(event).toEqual({ authChanging: true });
    expect(events[events.length - 1]).toEqual({ authChanged: true, nativeAccess: true });
    expect((await createClient(context.request).getMe()).email).toBe(email);
  } finally {
    await context.close();
  }
});
