// Verifies browser voice reserves viewport space while document, table and settings bottoms remain reachable.

import type { APIRequestContext, Locator, Page } from "@playwright/test";
import { test, expect, registerUser, createClient, getWorkspaceId, gotoWithSession } from "./helpers";

async function expectReservedViewport(page: Page) {
  const panel = page.getByTestId("voice-overlay");
  await expect(panel).toBeVisible();
  await expect
    .poll(async () => page.locator("main").evaluate((main) => main.getBoundingClientRect().bottom))
    .toBeLessThanOrEqual((await panel.boundingBox())!.y + 1);
  expect(await page.evaluate(() => document.documentElement.scrollHeight <= innerHeight + 1)).toBe(true);
  await panel.getByRole("button", { name: "Voice settings", exact: true }).click({ trial: true });
}

async function scrollToBottom(page: Page, target: Locator, panel: Locator) {
  const area = (await page.locator("main").boundingBox())!;
  await page.mouse.move(area.x + area.width / 2, area.y + area.height / 2);
  await page.mouse.wheel(0, 100_000);
  await expect(target).toBeInViewport();
  await expect
    .poll(async () => (await target.boundingBox())!.y + (await target.boundingBox())!.height)
    .toBeLessThanOrEqual((await panel.boundingBox())!.y + 1);
}

async function exerciseReservedViewport(page: Page, request: APIRequestContext, baseURL: string) {
  // Model an already dismissed installation offer; it has its own foreground journey.
  await page.addInitScript(() => localStorage.setItem("mddb_pwa_dismissed", new Date().toISOString()));
  const errors: string[] = [];
  const consoleErrors: Array<{ url: string; text: string }> = [];
  const httpFailures: Array<{ url: string; status: number }> = [];
  const failedRequests: Array<{ url: string; error: string | null }> = [];
  page.on("pageerror", (error) => errors.push(error.message));
  page.on("console", (message) => {
    if (message.type() === "error") consoleErrors.push({ url: message.location().url, text: message.text() });
  });
  page.on("response", (response) => {
    if (response.status() >= 400) httpFailures.push({ url: response.url(), status: response.status() });
  });
  page.on("requestfailed", (failed) =>
    failedRequests.push({ url: failed.url(), error: failed.failure()?.errorText ?? null }),
  );
  const { token } = await registerUser(request, "voice-layout");
  await page.route("**/.well-known/gomode.json", async (route) => {
    const response = await route.fetch();
    const manifest = await response.json();
    manifest.webShell.voiceGateway = { url: baseURL };
    await route.fulfill({ response, json: manifest });
  });
  await page.route("**/api/voicegateway/v1/voice/choices", (route) =>
    route.fulfill({
      json: { choices: [{ id: "layout", label: "Layout voice", transports: ["webrtc"] }], defaultChoiceID: "layout" },
    }),
  );
  await gotoWithSession(page, token);
  await expect(page.getByTestId("user-menu-button")).toBeVisible();
  const workspaceId = await getWorkspaceId(page);
  const workspace = createClient(request, token).ws(workspaceId);
  const document = await workspace.createPage("0", {
    title: "Voice viewport document",
    content:
      Array.from({ length: 40 }, (_, i) => `Document paragraph ${i + 1}`).join("\n\n") + "\n\nDocument bottom reached",
  });
  const table = await workspace.createTable("0", {
    title: "Voice viewport table",
    properties: [{ name: "Name", type: "text" }],
  });
  for (let i = 0; i < 30; i++) await workspace.createRecord(table.id, { data: { Name: `Record ${i + 1}` } });
  await page.goto(`/w/@${workspaceId}/@${document.id}`);
  const panel = page.getByTestId("voice-overlay");
  await expect(page.getByTestId("wysiwyg-editor")).toBeVisible();
  await expectReservedViewport(page);
  await scrollToBottom(page, page.getByText("Document bottom reached", { exact: true }), panel);
  await page.screenshot({ path: test.info().outputPath("document-bottom.png") });
  await page.goto(`/w/@${workspaceId}/@${table.id}`);
  await expect(page.getByRole("cell", { name: "Record 30", exact: true })).toBeAttached();
  await expectReservedViewport(page);
  await scrollToBottom(page, page.getByRole("cell", { name: "Record 30", exact: true }), panel);
  await page.screenshot({ path: test.info().outputPath("table-bottom.png") });
  await page.getByTestId("user-menu-button").click();
  await page.getByRole("menuitem", { name: "Profile" }).click();
  await expect(page.getByRole("heading", { name: "Settings", exact: true })).toBeVisible();
  await expectReservedViewport(page);
  const signOut = page.getByRole("button", { name: "Sign out session", exact: true }).last();
  await scrollToBottom(page, signOut, panel);
  await signOut.click({ trial: true });
  await page.screenshot({ path: test.info().outputPath("settings-bottom.png") });
  await test.info().attach("browser-diagnostics", {
    contentType: "application/json",
    body: JSON.stringify({ consoleErrors, httpFailures, failedRequests }, null, 2),
  });
  expect(errors).toEqual([]);
  expect(consoleErrors.filter((error) => error.url.startsWith(`${baseURL}/`))).toEqual([]);
  expect(httpFailures.filter((failure) => failure.url.startsWith(`${baseURL}/`))).toEqual([]);
}

for (const viewport of [
  { width: 1280, height: 900 },
  { width: 390, height: 844 },
]) {
  test(`browser voice reserves document, table and settings space at ${viewport.width}px`, async ({
    page,
    request,
    baseURL,
  }) => {
    await page.setViewportSize(viewport);
    await exerciseReservedViewport(page, request, baseURL!);
  });
}

test("browser voice reserves usable route space at native 200% browser zoom", async ({
  browser,
  request,
  baseURL,
}, testInfo) => {
  const { mkdir, writeFile } = await import("node:fs/promises");
  const path = await import("node:path");
  const extension = testInfo.outputPath("zoom-extension");
  await mkdir(extension, { recursive: true });
  await writeFile(
    path.join(extension, "manifest.json"),
    JSON.stringify({
      manifest_version: 3,
      name: "Voice viewport zoom",
      version: "1.0",
      background: { service_worker: "background.js" },
    }),
  );
  await writeFile(path.join(extension, "background.js"), "chrome.runtime.onInstalled.addListener(() => {});");
  const context = await browser.browserType().launchPersistentContext(testInfo.outputPath("zoom-profile"), {
    channel: "chromium",
    headless: true,
    baseURL,
    viewport: { width: 1280, height: 1000 },
    args: [`--disable-extensions-except=${extension}`, `--load-extension=${extension}`],
  });
  try {
    const worker = context.serviceWorkers()[0] ?? (await context.waitForEvent("serviceworker"));
    const page = context.pages()[0];
    if (!page) throw new Error("Missing zoom browser page");
    await page.goto("/privacy");
    await worker.evaluate("chrome.tabs.setZoom(2)");
    await expect.poll(() => page.evaluate(() => innerWidth)).toBe(640);
    await exerciseReservedViewport(page, request, baseURL!);
  } finally {
    await context.close();
  }
});
