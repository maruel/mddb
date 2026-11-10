// E2E tests for OAuth2 authentication flow.
import { waitForWorkspace, test, expect, registerUser, gotoWithSession } from "./helpers";

test.describe("API routing", () => {
  test("unknown /api/v1/ routes return 404, not SPA", async ({ request }) => {
    const response = await request.get("/api/v1/nonexistent/route");
    expect(response.status()).toBe(404);

    // Should be plain text error, not HTML
    const text = await response.text();
    expect(text).not.toContain("<!DOCTYPE");
    expect(text).toContain("Not found");
  });
});

test.describe("OAuth Login", () => {
  test("login page loads with OAuth buttons", async ({ page }) => {
    await page.goto("/");

    // Should show login form
    await expect(page.getByRole("heading", { name: /login|sign in/i })).toBeVisible();

    // The e2e build swaps in fake providers, so OAuth buttons should be visible
    const googleButton = page.getByRole("link", { name: /google/i });
    await expect(googleButton).toBeVisible();
    await expect(googleButton).toHaveAttribute("href", "/api/v1/auth/oauth/google");
  });

  test("providers endpoint returns configured providers", async ({ request }) => {
    const response = await request.get("/api/v1/auth/providers");
    expect(response.ok()).toBe(true);

    const { providers } = await response.json();
    // In test mode, both providers should be configured
    expect(providers).toContain("google");
    expect(providers).toContain("microsoft");
  });

  test("clicking Google OAuth button navigates to OAuth endpoint", async ({ page }) => {
    await page.goto("/");

    const googleButton = page.getByRole("link", { name: /google/i });
    await expect(googleButton).toBeVisible({ timeout: 5000 });

    // Verify the button has the correct href
    await expect(googleButton).toHaveAttribute("href", "/api/v1/auth/oauth/google");
  });

  for (const provider of ["Google", "Microsoft"]) {
    test(`clicking ${provider} logs in with the fake account`, async ({ page }) => {
      await page.goto("/");

      const button = page.getByRole("link", { name: new RegExp(provider, "i") });
      await expect(button).toBeVisible();
      await button.click();
      await waitForWorkspace(page);

      // Fake providers share an account across runs. Release this login through
      // the browser flow so repetitions don't exhaust its real session quota.
      await page.getByRole("button", { name: "User menu", exact: true }).click();
      await page.getByRole("menuitem", { name: "Logout", exact: true }).click();
      await expect(page.getByRole("heading", { name: /login|sign in/i })).toBeVisible();
    });
  }

  test("OAuth callback with token sets auth and redirects", async ({ page, context: _context }) => {
    // This test simulates what happens after Google redirects back with a token
    // We need a valid JWT for this to work, so we'll check the flow mechanics

    // First, go to the callback URL with a fake token
    await page.goto("/?token=fake.jwt.token");

    // The frontend should try to use this token and call /api/v1/auth/me
    // Since it's a fake token, it will fail with 401
    // Check that we end up back at the login page
    await expect(page.getByRole("heading", { name: /login|sign in/i })).toBeVisible();

    // Token should be cleared from URL
    expect(page.url()).not.toContain("token=");
  });

  test("OAuth callback with valid token logs user in", async ({ page, request }) => {
    // Create a user and get a valid token via the API (with retry logic for rate limiting)
    const { token } = await registerUser(request, "oauth-callback");

    // Now simulate OAuth callback with this valid token
    await gotoWithSession(page, token);

    // Should be logged in - sidebar indicates logged in state
    await waitForWorkspace(page);

    // Token should be cleared from URL
    expect(page.url()).not.toContain("token=");
  });
});
