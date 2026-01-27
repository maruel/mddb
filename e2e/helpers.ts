// Shared e2e test helpers for registration, workspace management, and screenshots.

import { test as base, expect, Page, APIRequestContext } from '@playwright/test';

// Helper to register a user and get token
export async function registerUser(request: APIRequestContext, prefix: string) {
  const email = `${prefix}-${Date.now()}@example.com`;
  const registerResponse = await request.post('/api/auth/register', {
    data: {
      email,
      password: 'testpassword123',
      name: `${prefix} Test User`,
    },
  });
  expect(registerResponse.ok()).toBe(true);
  const { token } = await registerResponse.json();
  return { email, token };
}

// Helper to get workspace ID from URL
export async function getWorkspaceId(page: Page): Promise<string> {
  await expect(page).toHaveURL(/\/w\/[^/]+/, { timeout: 5000 });
  const url = page.url();
  const wsMatch = url.match(/\/w\/([^+/]+)/);
  expect(wsMatch).toBeTruthy();
  return wsMatch![1];
}

// Screenshot helper type
type ScreenshotFn = (name: string, options?: { fullPage?: boolean }) => Promise<void>;

// Extended test with takeScreenshot fixture
export const test = base.extend<{ takeScreenshot: ScreenshotFn }>({
  takeScreenshot: [async ({ page }, use, testInfo) => {
    const screenshotFn: ScreenshotFn = async (name, options = {}) => {
      const screenshot = await page.screenshot({
        fullPage: options.fullPage ?? false,
      });

      // Attach to test report - will be visible in HTML report
      await testInfo.attach(name, {
        body: screenshot,
        contentType: 'image/png',
      });
    };
    await use(screenshotFn);
  }, { scope: 'test' }],
});

export { expect } from '@playwright/test';
