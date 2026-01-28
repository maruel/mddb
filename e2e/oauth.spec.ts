import { test, expect, registerUser } from './helpers';

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
  test('login page loads with OAuth buttons', async ({ page }) => {
    await page.goto('/');

    // Should show login form
    await expect(page.getByRole('heading', { name: /login|sign in/i })).toBeVisible();

    // In test mode (TEST_OAUTH=1), OAuth buttons should be visible
    const googleButton = page.getByRole('link', { name: /google/i });
    await expect(googleButton).toBeVisible();
    await expect(googleButton).toHaveAttribute('href', '/api/auth/oauth/google');
  });

  test('providers endpoint returns configured providers', async ({ request }) => {
    const response = await request.get('/api/auth/providers');
    expect(response.ok()).toBe(true);

    const { providers } = await response.json();
    // In test mode, both providers should be configured
    expect(providers).toContain('google');
    expect(providers).toContain('microsoft');
  });

  test('clicking Google OAuth button redirects to Google', async ({ page }) => {
    await page.goto('/');

    const googleButton = page.getByRole('link', { name: /google/i });

    // Click and wait for navigation to Google OAuth
    const [response] = await Promise.all([
      page.waitForResponse(resp => resp.url().includes('/api/auth/oauth/google')),
      googleButton.click(),
    ]);

    // Should get 307 redirect (to Google) since test credentials are configured
    expect(response.status()).toBe(307);
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
    // Create a user and get a valid token via the API (with retry logic for rate limiting)
    const { token } = await registerUser(request, 'oauth-callback');

    // Now simulate OAuth callback with this valid token
    await page.goto(`/?token=${token}`);

    // Should be logged in - sidebar indicates logged in state
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Token should be cleared from URL
    expect(page.url()).not.toContain('token=');
  });
});
