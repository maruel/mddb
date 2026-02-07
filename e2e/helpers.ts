// Shared e2e test helpers for registration, workspace management, and screenshots.

import { test as base, expect, type Page, type APIRequestContext, type TestInfo } from '@playwright/test';
import type { TestType, PlaywrightTestArgs, PlaywrightTestOptions, PlaywrightWorkerArgs, PlaywrightWorkerOptions } from '@playwright/test';
import { createAPIClient, type APIClient } from '../sdk/api.gen';

// Helper to create a typed API client from Playwright's request context
export function createClient(request: APIRequestContext, token?: string): APIClient {
  const fetchFn = async (url: string, init?: RequestInit) => {
    const headers: Record<string, string> = {};
    if (token) {
      headers['Authorization'] = `Bearer ${token}`;
    }
    if (init?.headers) {
      Object.assign(headers, init.headers);
    }

    // Playwright's fetch expects 'data' for the body, not 'body'
    // The SDK serializes the body to a string in init.body
    const response = await request.fetch(url, {
      method: init?.method || 'GET',
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

  return createAPIClient(fetchFn);
}

// Helper to register a user and get token (with retry for rate limiting)
export async function registerUser(request: APIRequestContext, prefix: string) {
  const email = `${prefix}-${Date.now()}@example.com`;
  const maxRetries = 3;

  for (let attempt = 1; attempt <= maxRetries; attempt++) {
    const registerResponse = await request.post('/api/v1/auth/register', {
      data: {
        email,
        password: 'testpassword123',
        name: `${prefix} Test User`,
      },
    });

    if (registerResponse.ok()) {
      const { token } = await registerResponse.json();
      return { email, token };
    }

    // Retry on rate limiting (429)
    if (registerResponse.status() === 429 && attempt < maxRetries) {
      const retryBody = await registerResponse.json().catch(() => ({}));
      const retryAfter = retryBody?.details?.retry_after_seconds || 1;
      await new Promise((resolve) => setTimeout(resolve, retryAfter * 1000 + 100));
      continue;
    }

    const errorBody = await registerResponse.text();
    throw new Error(`Registration failed for ${email}: ${registerResponse.status()} - ${errorBody}`);
  }

  throw new Error(`Registration failed for ${email} after ${maxRetries} retries`);
}

// Helper to get workspace ID from URL
export async function getWorkspaceId(page: Page): Promise<string> {
  await expect(page).toHaveURL(/\/w\/@[^/]+/, { timeout: 5000 });
  const url = page.url();
  const wsMatch = url.match(/\/w\/@([^+/]+)/);
  expect(wsMatch).toBeTruthy();
  const workspaceId = wsMatch?.[1];
  if (!workspaceId) {
    throw new Error('Failed to extract workspace ID from URL');
  }
  return workspaceId;
}

// Convert text to a filesystem-safe slug
function slugify(text: string): string {
  return text
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-|-$/g, '')
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
type ScreenshotTestFn = (
  args: AllFixtures & AllWorkerFixtures,
  testInfo: TestInfo
) => void | Promise<void>;

// Base extended test type
type ExtendedTestType = TestType<AllFixtures, AllWorkerFixtures>;

// Extended test type with screenshot helper method
interface TestWithScreenshot extends ExtendedTestType {
  screenshot: (title: string, fn: ScreenshotTestFn) => void;
}

// Extended test with takeScreenshot fixture
const baseTest = base.extend<CustomFixtures>({
  takeScreenshot: [async ({ page }, use, testInfo: TestInfo) => {
    let hasScreenshot = false;
    let screenshotIndex = 0;

    const screenshotFn: ScreenshotFn = async (name, options = {}) => {
      screenshotIndex++;
      const nameSlug = slugify(name);

      // Create meaningful filename: index_screenshot-name.png
      // Playwright stores in test output dir which already has test name in path
      const filename = `${screenshotIndex.toString().padStart(2, '0')}_${nameSlug}.png`;

      // Use testInfo.outputPath for proper test output directory
      const screenshotPath = testInfo.outputPath(filename);

      // Save screenshot with meaningful name
      await page.screenshot({
        path: screenshotPath,
        fullPage: options.fullPage ?? false,
      });

      // Add annotation on first screenshot - visible in test details panel
      if (!hasScreenshot) {
        testInfo.annotations.push({ type: 'screenshot', description: 'This test captures screenshots' });
        hasScreenshot = true;
      }

      // Attach to test report for inline viewing (uses the file we just saved)
      await testInfo.attach(name, {
        path: screenshotPath,
        contentType: 'image/png',
      });
    };
    await use(screenshotFn);
  }, { scope: 'test' }],
});

// Helper to create a test with @screenshot tag visible in test list
// Usage: test.screenshot('test name', async ({ page, takeScreenshot }) => { ... })
(baseTest as TestWithScreenshot).screenshot = (title: string, fn: ScreenshotTestFn) => {
  return baseTest(title, { tag: '@screenshot' }, fn);
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

export { expect } from '@playwright/test';
