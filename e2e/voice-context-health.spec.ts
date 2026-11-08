// Verifies advisory voice context recovery in browser/native hosts, mobile German, real zoom, and default-font preferences.

import type { APIRequestContext, Page, TestInfo } from "@playwright/test";
import { test, expect, registerUser, createClient, gotoWithSession } from "./helpers";
import { gate, installBrowserTools, installNativeTools, waitNativeReady } from "./gomode-fixtures";

test.use({ launchOptions: { args: ["--use-fake-ui-for-media-stream", "--use-fake-device-for-media-stream"] } });

async function exerciseRecovery(
  page: Page,
  request: APIRequestContext,
  info: TestInfo,
  native: boolean,
  german: boolean,
) {
  const { token } = await registerUser(request, "context-health");
  const client = createClient(request, token);
  const org = await client.createOrganization({ name: "Context recovery" });
  const workspace = await client.org(org.id).createWorkspace({ name: "Voice documents" });
  const node = await client
    .ws(workspace.id)
    .createPage("0", { title: "Current document", content: "Voice context recovery" });
  await client.switchWorkspace({ ws_id: workspace.id });
  if (german) await client.updateUserSettings({ settings: { language: "de", theme: "light" } });
  const runtimeRequests: string[] = [];
  const discoveryRequests: string[] = [];
  page.on("request", (outgoing) => {
    if (outgoing.url().includes("BrowserVoiceRuntime")) runtimeRequests.push(outgoing.url());
    if (outgoing.url().endsWith("/.well-known/gomode.json")) discoveryRequests.push(outgoing.url());
  });
  if (native) await installNativeTools(page, request, "context-health-native");
  else {
    await page.context().grantPermissions(["microphone"]);
    await installBrowserTools(page);
  }
  let failing = true;
  const retryRequested = gate(),
    retryRelease = gate();
  await page.route("**/gomode/focus", async (route) => {
    if (route.request().postDataJSON().release) return route.continue();
    if (failing) return route.fulfill({ json: { version: 0, applied: false } });
    retryRequested.resolve();
    await retryRelease.promise;
    await route.continue();
  });
  await gotoWithSession(page, token, `/w/@${workspace.id}/@${node.id}`);
  if (native) await waitNativeReady(page);
  else await page.getByRole("button", { name: "Connect voice assistant", exact: true }).click();
  const notice = page
    .getByRole("status")
    .filter({ hasText: german ? "Der Sprachkontext" : "Voice context could not update" });
  await expect(notice).toHaveCount(1);
  await expect(notice).toBeVisible();
  if (native) {
    await expect
      .poll(() => page.evaluate(() => window.nativeToolFixture.readiness.at(-1)?.voiceSessionPrepared))
      .toBe("context-health-native");
    expect(runtimeRequests).toEqual([]);
    expect(discoveryRequests).toEqual([]);
  } else {
    await expect(page.getByRole("button", { name: "End voice session", exact: true })).toBeVisible();
    expect(runtimeRequests.some((url) => url.endsWith(".js"))).toBe(true);
    const panel = await page.getByTestId("voice-overlay").boundingBox();
    const panelNotice = await notice.boundingBox();
    expect(panelNotice!.y + panelNotice!.height).toBeLessThanOrEqual(panel!.y + 1);
  }
  const box = (await notice.boundingBox())!;
  const viewport = await page.evaluate(() => ({ width: innerWidth, height: innerHeight }));
  expect(box.x).toBeGreaterThanOrEqual(0);
  expect(box.x + box.width).toBeLessThanOrEqual(viewport.width + 1);
  expect(box.y + box.height).toBeLessThanOrEqual(viewport.height + 1);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
  await page.screenshot({ path: info.outputPath("context-advisory.png") });
  const retry = notice.getByRole("button", { name: german ? "Erneut versuchen" : "Retry", exact: true });
  failing = false;
  await retry.focus();
  await page.keyboard.press("Enter");
  try {
    await retryRequested.promise;
    await expect(retry).toBeDisabled();
    await expect(notice).toHaveCount(1);
  } finally {
    retryRelease.resolve();
  }
  await expect(notice).toHaveCount(0);
  if (native) expect(runtimeRequests).toEqual([]);
  else await expect(page.getByRole("button", { name: "End voice session", exact: true })).toBeVisible();
}

test("browser context failure stays advisory and recovers through one keyboard Retry", async ({
  page,
  request,
}, info) => {
  await exerciseRecovery(page, request, info, false, false);
});

test("authenticated native recovery fits German mobile without loading browser voice", async ({
  page,
  request,
}, info) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await exerciseRecovery(page, request, info, true, true);
});

for (const preference of ["native zoom", "default font"] as const) {
  test(`native context recovery respects enlarged ${preference}`, async ({ browser, request, baseURL }, info) => {
    const { mkdir, writeFile } = await import("node:fs/promises");
    const path = await import("node:path");
    const extension = info.outputPath("preferences-extension");
    await mkdir(extension, { recursive: true });
    await writeFile(
      path.join(extension, "manifest.json"),
      JSON.stringify({
        manifest_version: 3,
        name: "Voice context preferences",
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
      viewport: { width: 1280, height: 1000 },
      args: [`--disable-extensions-except=${extension}`, `--load-extension=${extension}`],
    });
    try {
      const worker = context.serviceWorkers()[0] ?? (await context.waitForEvent("serviceworker"));
      const page = context.pages()[0]!;
      await page.goto("/privacy");
      if (preference === "native zoom") {
        const initialDPR = await page.evaluate(() => devicePixelRatio);
        await worker.evaluate("chrome.tabs.setZoom(2)");
        await expect.poll(() => page.evaluate(() => innerWidth)).toBe(640);
        await expect.poll(() => page.evaluate(() => devicePixelRatio)).toBeCloseTo(initialDPR * 2);
      } else {
        await worker.evaluate("chrome.fontSettings.setDefaultFontSize({ pixelSize: 24 })");
        await page.reload();
        await expect.poll(() => page.evaluate(() => getComputedStyle(document.documentElement).fontSize)).toBe("24px");
      }
      await exerciseRecovery(page, request, info, true, false);
    } finally {
      await context.close();
    }
  });
}
