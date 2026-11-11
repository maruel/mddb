// Exercises production voice setup recovery, resource ownership, translated text, and native browser preferences.

import type { APIRequestContext, Page, TestInfo } from "@playwright/test";
import { gate } from "./gomode-fixtures";
import type { Settings } from "@maruel/gomode/sdk/gomode/v1/types.gen.ts";
import { test, expect, registerUser, gotoWithSession, waitForWorkspace, createClient } from "./helpers";

// A service worker cache would mask the network failures this journey owns.
test.use({ serviceWorkers: "block" });

async function advertiseVoice(page: Page, baseURL: string) {
  await page.route("**/.well-known/gomode.json", async (route) => {
    const response = await route.fetch();
    expect(response.ok()).toBe(true);
    const manifest: Settings = await response.json();
    expect(manifest.service).toBe("mddb");
    expect(manifest.apiVersion).toBe(1);
    manifest.webShell.voiceGateway = { ...manifest.webShell.voiceGateway, url: baseURL };
    await route.fulfill({ response, json: manifest });
  });
  await page.route("**/api/voicegateway/v2/voice/choices", (route) =>
    route.fulfill({
      json: {
        choices: [{ id: "test", label: "Test voice", transports: ["webrtc"] }],
        defaults: [{ transport: "webrtc", id: "test" }],
      },
    }),
  );
}

for (const extension of ["js", "css"]) {
  test(`voice setup retries a failed production ${extension} asset in the same document`, async ({
    page,
    request,
    baseURL,
  }) => {
    const { token } = await registerUser(request, `voice-setup-${extension}`);
    await advertiseVoice(page, baseURL!);
    let requests = 0;
    let failing = true;
    let failedURL = "";
    await page.route(new RegExp(`/assets/BrowserVoiceRuntime-[^/]+\\.${extension}$`), async (route) => {
      requests++;
      if (failing) {
        failedURL = route.request().url();
        await route.abort("failed");
      } else await route.continue();
    });
    await gotoWithSession(page, token);
    await waitForWorkspace(page);
    const retry = page.getByRole("button", { name: "Retry", exact: true });
    await expect(retry).toBeVisible();
    {
      const beforeRetry = requests;
      await retry.focus();
      await page.keyboard.press("Enter");
      await expect.poll(() => requests).toBe(beforeRetry + 1);
      await expect(retry).toBeEnabled();
      await expect(page.getByTestId("voice-overlay")).toHaveCount(0);
    }
    const failures = requests;
    expect(failures).toBeGreaterThan(0);
    const document = await page.evaluate(() => performance.timeOrigin);
    await retry.focus();
    failing = false;
    await page.keyboard.press("Enter");
    await expect(page.getByTestId("voice-overlay")).toBeVisible();
    expect(requests).toBe(failures + 1);
    expect(await page.evaluate(() => performance.timeOrigin)).toBe(document);
    await expect(page.getByRole("alert").filter({ hasText: "Voice assistant unavailable" })).toHaveCount(0);
    if (extension === "css")
      expect(
        await page
          .locator('link[rel="stylesheet"]')
          .evaluateAll(
            (links, url) =>
              links.some((link) => (link as HTMLLinkElement).href === url && (link as HTMLLinkElement).sheet),
            failedURL,
          ),
      ).toBe(true);
  });
}

async function exerciseGermanRecovery(
  page: Page,
  request: APIRequestContext,
  baseURL: string,
  info: TestInfo,
  capture: string,
  fontSize: string,
) {
  const { token } = await registerUser(request, "voice-setup-discovery");
  await createClient(request, token).updateUserSettings({ settings: { language: "de", theme: "light" } });
  await advertiseVoice(page, baseURL!);
  let requests = 0;
  const release = gate();
  await page.route("**/.well-known/gomode.json", async (route) => {
    requests++;
    if (requests === 2) await release.promise;
    if (requests <= 2) await route.fulfill({ status: 503, json: { error: "Voice unavailable" } });
    else await route.fallback();
  });
  try {
    await gotoWithSession(page, token);
    await waitForWorkspace(page);
    expect(await page.evaluate(() => getComputedStyle(globalThis.document.documentElement).fontSize)).toBe(fontSize);
    const retry = page.getByRole("button", { name: "Erneut versuchen", exact: true });
    await expect(retry).toBeVisible();
    const document = await page.evaluate(() => performance.timeOrigin);
    const notice = page.getByRole("alert").filter({ hasText: "Sprachassistent nicht verfügbar" });
    const noticeBox = (await notice.boundingBox())!;
    expect(noticeBox.x).toBeGreaterThanOrEqual(0);
    expect(noticeBox.x + noticeBox.width).toBeLessThanOrEqual((await page.evaluate(() => innerWidth)) + 1);
    const retryBox = (await retry.boundingBox())!;
    expect(retryBox.x).toBeGreaterThanOrEqual(0);
    expect(retryBox.x + retryBox.width).toBeLessThanOrEqual((await page.evaluate(() => innerWidth)) + 1);
    expect(await page.evaluate(() => globalThis.document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
    await page.screenshot({ path: info.outputPath(`${capture}-setup-failed.png`) });
    await retry.focus();
    await page.keyboard.press("Enter");
    await expect.poll(() => requests).toBe(2);
    await expect(retry).toBeDisabled();
    await expect(retry).toBeFocused();
    await page.keyboard.press("Enter");
    await page.keyboard.press("Space");
    await expect
      .poll(() =>
        retry.evaluate((button) => {
          const box = button.getBoundingClientRect();
          return button.contains(globalThis.document.elementFromPoint(box.x + box.width / 2, box.y + box.height / 2));
        }),
      )
      .toBe(true);
    // Bypass only aria-disabled actionability; still dispatch a real mouse click.
    await retry.click({ force: true });
    expect(requests).toBe(2);
    await expect(retry).toBeFocused();
    await page.screenshot({ path: info.outputPath(`${capture}-setup-disabled.png`) });
    release.resolve();
    await expect(retry).toBeEnabled();
    await retry.focus();
    await page.keyboard.press("Enter");
    await expect.poll(() => requests).toBe(3);
    await expect(page.getByTestId("voice-overlay")).toBeVisible();
    expect(await page.evaluate(() => performance.timeOrigin)).toBe(document);
    await expect(notice).toHaveCount(0);
    await page.screenshot({ path: info.outputPath(`${capture}-setup-success.png`) });
  } finally {
    release.resolve();
  }
}

test("German voice discovery recovery fits narrow width and keeps keyboard focus", async ({
  page,
  request,
  baseURL,
}, info) => {
  await page.setViewportSize({ width: 320, height: 640 });
  await exerciseGermanRecovery(page, request, baseURL!, info, "narrow", "16px");
});

for (const preference of ["native zoom", "default font"] as const) {
  test(`German setup recovery respects enlarged ${preference}`, async ({ browser, request, baseURL }, info) => {
    const { mkdir, writeFile } = await import("node:fs/promises");
    const path = await import("node:path");
    const extension = info.outputPath("preferences-extension");
    await mkdir(extension, { recursive: true });
    await writeFile(
      path.join(extension, "manifest.json"),
      JSON.stringify({
        manifest_version: 3,
        name: "Voice setup preferences",
        version: "1.0",
        permissions: ["fontSettings"],
        background: { service_worker: "background.js" },
      }),
    );
    await writeFile(path.join(extension, "background.js"), "chrome.runtime.onInstalled.addListener(() => {});");
    const context = await browser.browserType().launchPersistentContext(info.outputPath("preferences-profile"), {
      channel: "chromium",
      headless: true,
      baseURL,
      serviceWorkers: "allow",
      viewport: preference === "native zoom" ? { width: 640, height: 1280 } : { width: 320, height: 640 },
      args: [`--disable-extensions-except=${extension}`, `--load-extension=${extension}`],
    });
    try {
      // Installation offers have their own foreground journey.
      await context.addInitScript(() => localStorage.setItem("mddb_pwa_dismissed", new Date().toISOString()));
      const worker = context.serviceWorkers()[0] ?? (await context.waitForEvent("serviceworker"));
      const page = context.pages()[0]!;
      await page.goto("/privacy");
      if (preference === "native zoom") {
        const initialDPR = await page.evaluate(() => devicePixelRatio);
        for (const factor of [1, 1.25, 1.5, 2]) {
          await worker.evaluate(`chrome.tabs.setZoom(${factor})`);
          await expect.poll(() => page.evaluate(() => innerWidth)).toBe(Math.round(640 / factor));
          await expect.poll(() => page.evaluate(() => devicePixelRatio)).toBeCloseTo(initialDPR * factor);
          await exerciseGermanRecovery(page, request, baseURL!, info, `zoom-${factor}`, "16px");
          await page.unrouteAll({ behavior: "wait" });
        }
      } else {
        await worker.evaluate("chrome.fontSettings.setDefaultFontSize({ pixelSize: 24 })");
        await page.reload();
        await expect.poll(() => page.evaluate(() => getComputedStyle(document.documentElement).fontSize)).toBe("24px");
        await exerciseGermanRecovery(page, request, baseURL!, info, "font-24", "24px");
      }
    } finally {
      await context.close();
    }
  });
}

for (const boundary of ["pending", "failed", "pending-success"]) {
  test(`voice stylesheet recovery survives shell replacement after ${boundary} import`, async ({
    page,
    request,
    baseURL,
  }) => {
    const account = await registerUser(request, `voice-style-${boundary}`);
    await advertiseVoice(page, baseURL!);
    let release!: () => void;
    const held = new Promise<void>((resolve) => {
      release = resolve;
    });
    let requests = 0;
    await page.route(/\/assets\/BrowserVoiceRuntime-[^/]+\.css$/, async (route) => {
      requests++;
      if (requests === 1) {
        if (boundary !== "failed") await held;
        if (boundary === "pending-success") await route.continue();
        else await route.abort("failed");
      } else await route.continue();
    });
    await gotoWithSession(page, account.token);
    await waitForWorkspace(page);
    await expect.poll(() => requests).toBe(1);
    if (boundary === "failed") await expect(page.getByRole("button", { name: "Retry", exact: true })).toBeVisible();
    const document = await page.evaluate(() => performance.timeOrigin);
    await page.getByTestId("user-menu-button").click();
    await page.getByRole("menuitem", { name: "Logout" }).click();
    await expect(page.getByRole("heading", { name: "Login to mddb" })).toBeVisible();
    if (boundary === "pending-success") {
      const loaded = page.waitForResponse((response) =>
        /\/assets\/BrowserVoiceRuntime-[^/]+\.css$/.test(response.url()),
      );
      release();
      await loaded;
      await expect(page.getByTestId("voice-overlay")).toHaveCount(0);
      await expect(page.getByRole("button", { name: "Retry", exact: true })).toHaveCount(0);
    }
    await page.getByLabel("Email", { exact: true }).fill(account.email);
    await page.getByLabel("Password", { exact: true }).fill("testpassword123");
    await page.getByRole("button", { name: "Login", exact: true }).click();
    await waitForWorkspace(page);
    if (boundary === "pending") {
      expect(requests).toBe(1);
      await expect(page.getByTestId("voice-overlay")).toHaveCount(0);
      release();
      const retry = page.getByRole("button", { name: "Retry", exact: true });
      await expect(retry).toBeVisible();
      await retry.focus();
      await page.keyboard.press("Enter");
    }
    await expect(page.getByTestId("voice-overlay")).toBeVisible();
    expect(requests).toBe(boundary === "pending-success" ? 1 : 2);
    expect(await page.evaluate(() => performance.timeOrigin)).toBe(document);
    expect(
      await page
        .locator('link[rel="stylesheet"]')
        .evaluateAll((links) =>
          links.some(
            (link) =>
              (link as HTMLLinkElement).href.includes("BrowserVoiceRuntime-") && (link as HTMLLinkElement).sheet,
          ),
        ),
    ).toBe(true);
    // A later account lifetime reuses successfully loaded shared styles.
    await page.getByTestId("user-menu-button").click();
    await page.getByRole("menuitem", { name: "Logout" }).click();
    await page.getByLabel("Email", { exact: true }).fill(account.email);
    await page.getByLabel("Password", { exact: true }).fill("testpassword123");
    await page.getByRole("button", { name: "Login", exact: true }).click();
    await expect(page.getByTestId("voice-overlay")).toBeVisible();
    expect(requests).toBe(boundary === "pending-success" ? 1 : 2);
  });
}
