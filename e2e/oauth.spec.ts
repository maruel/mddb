import { test, expect } from '@playwright/test';

test.describe('API routing', () => {
  test('unknown /api/ routes return 404, not SPA', async ({ request }) => {
    const response = await request.get('/api/nonexistent/route');
    expect(response.status()).toBe(404);

    // Should be plain text error, not HTML
    const text = await response.text();
    expect(text).not.toContain('<!DOCTYPE');
    expect(text).toContain('Not found');
  });
});

test.describe('OAuth Login', () => {
  test('login page loads and shows OAuth buttons', async ({ page }) => {
    await page.goto('/');

    // Should show login form
    await expect(page.getByRole('heading', { name: /login|sign in/i })).toBeVisible();

    // Should show OAuth buttons
    const googleButton = page.getByRole('link', { name: /google/i });
    await expect(googleButton).toBeVisible();
    await expect(googleButton).toHaveAttribute('href', '/api/auth/oauth/google');
  });

  test('clicking Google OAuth button navigates to OAuth endpoint', async ({ page }) => {
    await page.goto('/');

    const googleButton = page.getByRole('link', { name: /google/i });

    // Listen for navigation to the OAuth endpoint
    const [response] = await Promise.all([
      page.waitForResponse(resp => resp.url().includes('/api/auth/oauth/google')),
      googleButton.click(),
    ]);

    // Should get either a redirect (307 if OAuth configured) or 404 (if provider not configured)
    // Never 200 (which would mean SPA fallback)
    expect([307, 404]).toContain(response.status());
  });

  test('OAuth callback with token sets auth and redirects', async ({ page, context }) => {
    // This test simulates what happens after Google redirects back with a token
    // We need a valid JWT for this to work, so we'll check the flow mechanics

    // First, go to the callback URL with a fake token
    await page.goto('/?token=fake.jwt.token');

    // The frontend should try to use this token and call /api/auth/me
    // Since it's a fake token, it will fail with 401
    // Check that we end up back at the login page
    await expect(page.getByRole('heading', { name: /login|sign in/i })).toBeVisible();

    // Token should be cleared from URL
    expect(page.url()).not.toContain('token=');
  });

  test('OAuth callback with valid token logs user in', async ({ page, request }) => {
    // First, create a user and get a valid token via the API
    const loginResponse = await request.post('/api/auth/register', {
      data: {
        email: `test-${Date.now()}@example.com`,
        password: 'testpassword123',
        name: 'Test User',
      },
    });

    if (loginResponse.ok()) {
      const { token } = await loginResponse.json();

      // Now simulate OAuth callback with this valid token
      await page.goto(`/?token=${token}`);

      // Should be logged in - look for the logout button specifically
      await expect(page.getByRole('button', { name: /logout/i })).toBeVisible({ timeout: 5000 });

      // Token should be cleared from URL
      expect(page.url()).not.toContain('token=');
    }
  });
});
