// E2E test for registration redirect: after registering, user should land on onboarding, not stay on login.

import { test, expect } from './helpers';

test('registration redirects to onboarding', async ({ page }) => {
  const email = `reg-redirect-${Date.now()}@example.com`;

  // Go to login page
  await page.goto('/login');
  await expect(page.locator('form')).toBeVisible({ timeout: 5000 });

  // Switch to register mode
  const registerToggle = page.locator('button[type="button"]', { hasText: /register/i });
  await registerToggle.click();

  // Fill registration form
  await page.locator('#name').fill('Test User');
  await page.locator('#email').fill(email);
  await page.locator('#password').fill('testpassword123');

  // Submit
  await page.locator('button[type="submit"]').click();

  // Should redirect away from /login — through /onboarding into a workspace
  await expect(page).not.toHaveURL(/\/login/, { timeout: 10000 });
  await expect(page).toHaveURL(/\/w\//, { timeout: 10000 });
});
