// Shared e2e helpers for registration, workspace management, screenshots, and host auth bridges.

import {
  test as base,
  expect,
  type BrowserContext,
  type Page,
  type APIRequestContext,
  type APIResponse,
  type TestInfo,
} from "@playwright/test";
import type {
  TestType,
  PlaywrightTestArgs,
  PlaywrightTestOptions,
  PlaywrightWorkerArgs,
  PlaywrightWorkerOptions,
} from "@playwright/test";
import { randomUUID } from "node:crypto";
import type { DeviceLoginCode, DeviceLoginRequest } from "@maruel/gomode/web/DeviceLogin";
import { APIError, createApiClient, type APIClient } from "../sdk/api.gen";

// Install the asynchronous native auth bridge used by host journeys.
export interface AuthBridgeOptions {
  document: string;
  token: string;
  appendRequest?: boolean;
  eventsKey?: string;
  recordFunction?: string;
  voiceConnected?: boolean;
}

export async function installAuthBridge(target: Page | BrowserContext, options: AuthBridgeOptions) {
  await target.addInitScript((config) => {
    const bridge: {
      onmessage?: ((event: { data: string }) => void) | null;
      postMessage(message: string): void;
    } = {
      postMessage(message: string) {
        const { frontendDocument, frontendRequest, ...auth } = JSON.parse(message) as Record<string, unknown>;
        if (frontendDocument !== config.document || typeof frontendRequest !== "string")
          throw new Error("Invalid bridge owner");
        if (config.eventsKey) {
          const events = JSON.parse(sessionStorage.getItem(config.eventsKey) ?? "[]") as unknown[];
          events.push(auth);
          sessionStorage.setItem(config.eventsKey, JSON.stringify(events));
        }
        if (config.recordFunction) {
          const record = (window as unknown as Record<string, unknown>)[config.recordFunction];
          if (typeof record !== "function") throw new Error(`Missing ${config.recordFunction}`);
          void record(message);
        }
        if (auth.authChanged === true && auth.nativeAccess === true) {
          const frontendToken = config.appendRequest === true ? `${config.token}${frontendRequest}` : config.token;
          bridge.onmessage?.({ data: JSON.stringify({ frontendDocument, frontendRequest, frontendToken }) });
        }
      },
    };
    Object.assign(window, {
      goModeHost: config.voiceConnected === true ? { isVoiceConnected: () => true } : {},
      goModeFrontendDocument: Promise.resolve(config.document),
      gomodeAuth: bridge,
    });
  }, options);
}

// Helper to create a typed API client from Playwright's request context
export function createClient(request: APIRequestContext, token?: string): APIClient {
  const fetchFn = async (url: string, init?: RequestInit) => {
    const headers: Record<string, string> = {};
    if (token) {
      headers["Authorization"] = `Bearer ${token}`;
    }
    if (init?.headers) {
      Object.assign(headers, init.headers);
    }

    // Playwright's fetch expects 'data' for the body, not 'body'
    // The SDK serializes the body to a string in init.body
    const response = await request.fetch(url, {
      method: init?.method || "GET",
      data: init?.body,
      headers,
    });

    // Adapt Playwright APIResponse to standard Response-like object expected by SDK
    // SDK uses .ok (property) and .status (property), but Playwright has .ok() and .status() methods
    return {
      ok: response.ok(),
      status: response.status(),
      json: async () => response.json(),
      text: async () => response.text(),
    } as unknown as Response;
  };

  return createApiClient(fetchFn);
}

// Installs a normal API-issued credential as the host cookie for unrelated UI
// fixtures. Browser credential tests exercise the actual login form separately.
export async function gotoWithSession(page: Page, token: string, path?: string) {
  const serviceURL = String(test.info().project.use.baseURL);
  await page.context().addCookies([
    {
      name: "__Host-mddb_session",
      value: token,
      domain: new URL(serviceURL).hostname,
      path: "/",
      httpOnly: true,
      secure: true,
      sameSite: "Lax",
    },
  ]);
  await page.goto(path ?? "/");
}

// Helper to register a user and get token (with retry for rate limiting)
export async function registerUser(request: APIRequestContext, prefix: string) {
  const email = `${prefix}-${randomUUID()}@example.com`;
  const client = createClient(request);
  const maxAttempts = 3;

  for (let attempt = 1; attempt <= maxAttempts; attempt++) {
    try {
      const auth = await client.register({
        email,
        password: "testpassword123",
        name: `${prefix} Test User`,
      });
      if (!auth || typeof auth.token !== "string" || !auth.token.trim()) {
        throw new Error("Registration returned no usable token");
      }
      return { email, token: auth.token };
    } catch (err) {
      if (err instanceof SyntaxError) {
        // eslint-disable-next-line preserve-caught-error -- JSON parser errors can quote private response data.
        throw new Error("Registration returned invalid JSON");
      }
      if (!(err instanceof APIError)) throw err;
      if (err.status !== 429 || attempt === maxAttempts) {
        // eslint-disable-next-line preserve-caught-error -- SDK errors contain response data; report only HTTP status.
        throw new Error(`Registration failed (HTTP ${err.status})`);
      }
      const retryAfter = err.details?.retry_after_seconds ?? 1;
      // Bound fixture waits instead of letting a malformed or unexpectedly long
      // server delay overflow setTimeout or conceal a setup failure.
      if (typeof retryAfter !== "number" || !Number.isFinite(retryAfter) || retryAfter < 0 || retryAfter > 60) {
        // eslint-disable-next-line preserve-caught-error -- SDK error details may contain private response data.
        throw new Error("Registration rate limit returned an invalid retry delay");
      }
      await new Promise((resolve) => setTimeout(resolve, retryAfter * 1000 + 100));
    }
  }

  throw new Error(`Registration failed after ${maxAttempts} attempts`);
}

// Native device-login routes are owned by Go Mode and have no mddb SDK entry.
// Validate only the owner-defined fields consumed by the fixture.
export async function readDeviceLoginCode(
  response: Pick<APIResponse, "ok" | "status" | "json">,
): Promise<Pick<DeviceLoginCode, "user_code" | "device_code">> {
  const body = await readFixtureResponse(response, "Device login start");
  if (
    typeof body !== "object" ||
    body === null ||
    !("user_code" in body) ||
    typeof body.user_code !== "string" ||
    !body.user_code.trim() ||
    !("device_code" in body) ||
    typeof body.device_code !== "string" ||
    !body.device_code.trim()
  ) {
    throw new Error("Device login start returned invalid user_code or device_code");
  }
  return { user_code: body.user_code, device_code: body.device_code };
}

export async function readDeviceLoginProof(
  response: Pick<APIResponse, "ok" | "status" | "json">,
): Promise<Pick<DeviceLoginRequest, "csrf_token">> {
  const body = await readFixtureResponse(response, "Device login lookup");
  if (
    typeof body !== "object" ||
    body === null ||
    !("csrf_token" in body) ||
    typeof body.csrf_token !== "string" ||
    !body.csrf_token.trim()
  ) {
    throw new Error("Device login lookup returned invalid csrf_token");
  }
  return { csrf_token: body.csrf_token };
}

async function readFixtureResponse(
  response: Pick<APIResponse, "ok" | "status" | "json">,
  operation: string,
): Promise<unknown> {
  if (!response.ok()) throw new Error(`${operation} failed (HTTP ${response.status()})`);
  try {
    return await response.json();
  } catch {
    throw new Error(`${operation} returned invalid JSON`);
  }
}

// Helper to get workspace ID from URL
export async function getWorkspaceId(page: Page): Promise<string> {
  await expect(page).toHaveURL(/\/w\/@[^/]+/, { timeout: 5000 });
  const url = page.url();
  const wsMatch = url.match(/\/w\/@([^+/]+)/);
  expect(wsMatch).toBeTruthy();
  const workspaceId = wsMatch?.[1];
  if (!workspaceId) {
    throw new Error("Failed to extract workspace ID from URL");
  }
  return workspaceId;
}

// Convert text to a filesystem-safe slug
function slugify(text: string): string {
  return text
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-|-$/g, "")
    .slice(0, 50);
}

// Screenshot helper type
type ScreenshotFn = (name: string, options?: { fullPage?: boolean }) => Promise<void>;

// Custom fixtures for extended test
type CustomFixtures = { takeScreenshot: ScreenshotFn };

// All fixtures combined
type AllFixtures = PlaywrightTestArgs & PlaywrightTestOptions & CustomFixtures;
type AllWorkerFixtures = PlaywrightWorkerArgs & PlaywrightWorkerOptions;

// Test callback type for screenshot tests
type ScreenshotTestFn = (args: AllFixtures & AllWorkerFixtures, testInfo: TestInfo) => void | Promise<void>;

// Base extended test type
type ExtendedTestType = TestType<AllFixtures, AllWorkerFixtures>;

// Extended test type with screenshot helper method
interface TestWithScreenshot extends ExtendedTestType {
  screenshot: (title: string, fn: ScreenshotTestFn) => void;
}

// Extended test with takeScreenshot fixture
const baseTest = base.extend<CustomFixtures>({
  takeScreenshot: [
    async ({ page }, use, testInfo: TestInfo) => {
      let hasScreenshot = false;
      let screenshotIndex = 0;

      const screenshotFn: ScreenshotFn = async (name, options = {}) => {
        screenshotIndex++;
        const nameSlug = slugify(name);

        // Create meaningful filename: index_screenshot-name.png
        // Playwright stores in test output dir which already has test name in path
        const filename = `${screenshotIndex.toString().padStart(2, "0")}_${nameSlug}.png`;

        // Use testInfo.outputPath for proper test output directory
        const screenshotPath = testInfo.outputPath(filename);

        // Save screenshot with meaningful name
        await page.screenshot({
          path: screenshotPath,
          fullPage: options.fullPage ?? false,
        });

        // Add annotation on first screenshot - visible in test details panel
        if (!hasScreenshot) {
          testInfo.annotations.push({ type: "screenshot", description: "This test captures screenshots" });
          hasScreenshot = true;
        }

        // Attach to test report for inline viewing (uses the file we just saved)
        await testInfo.attach(name, {
          path: screenshotPath,
          contentType: "image/png",
        });
      };
      await use(screenshotFn);
    },
    { scope: "test" },
  ],
});

// Helper to create a test with @screenshot tag visible in test list
// Usage: test.screenshot('test name', async ({ page, takeScreenshot }) => { ... })
(baseTest as TestWithScreenshot).screenshot = (title: string, fn: ScreenshotTestFn) => {
  return baseTest(title, { tag: "@screenshot" }, fn);
};

export const test: TestWithScreenshot = baseTest as TestWithScreenshot;

// Helper to switch editor to markdown mode and get the textarea
export async function switchToMarkdownMode(page: Page) {
  const markdownEditor = page.locator('[data-testid="markdown-editor"]');
  // If already in markdown mode, just return the editor
  if (await markdownEditor.isVisible()) {
    return markdownEditor;
  }
  // Click the markdown mode button (always visible at bottom-right)
  await page.locator('[data-testid="editor-mode-markdown"]').click();
  await expect(markdownEditor).toBeVisible({ timeout: 3000 });
  return markdownEditor;
}

// Helper to get editor content (works with both modes)
export async function getEditorContent(page: Page): Promise<string> {
  // Check if markdown editor is visible
  const markdownEditor = page.locator('[data-testid="markdown-editor"]');
  if (await markdownEditor.isVisible()) {
    return await markdownEditor.inputValue();
  }
  // Otherwise get content from WYSIWYG editor
  const wysiwygEditor = page.locator('[data-testid="wysiwyg-editor"] .ProseMirror');
  return await wysiwygEditor.innerText();
}

// Helper to fill editor content (switches to markdown mode for reliability)
export async function fillEditorContent(page: Page, content: string) {
  const textarea = await switchToMarkdownMode(page);
  await textarea.fill(content);
}

export { expect } from "@playwright/test";
