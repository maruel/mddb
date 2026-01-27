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

// Extended test with takeScreenshot fixture
export const test = base.extend<{ takeScreenshot: ScreenshotFn }>({
  takeScreenshot: [async ({ page }, use, testInfo) => {
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

      // Add annotation on first screenshot - shows as tag in HTML report
      if (!hasScreenshot) {
        testInfo.annotations.push({ type: 'screenshot', description: 'Has screenshots' });
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

export { expect } from '@playwright/test';
