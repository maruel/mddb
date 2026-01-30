# E2E testing guidelines and best practices for Playwright tests

## Running Tests

From the parent directory:

```bash
# Run all e2e tests
make e2e

# Run a specific test file
npx playwright test e2e/workspace-org.spec.ts

# Run a specific test by line number (useful for debugging)
npx playwright test e2e/workspace-org.spec.ts:88

# Run with UI mode for debugging
npx playwright test --ui
```

## Available Helpers

Import from `./helpers`:

```typescript
import { test, expect, registerUser, getWorkspaceId, createClient } from './helpers';
```

- **`registerUser(request, prefix)`** - Creates a new user with unique email, returns `{ email, token }`
- **`getWorkspaceId(page)`** - Extracts workspace ID from current URL
- **`createClient(request, token)`** - Creates a typed SDK client with proper authorization headers
- **`test.screenshot(title, fn)`** - Creates a test tagged with `@screenshot`
- **`takeScreenshot(name)`** - Fixture for capturing screenshots in tests

### Use the SDK Client for API Calls

Always use the typed SDK client (`createClient`) instead of raw HTTP requests. This provides:
- Type safety and IDE autocompletion
- Automatic authorization headers
- Proper error handling
- Validation of request/response structures

```typescript
// GOOD - use the SDK client
const client = createClient(request, token);
const pageData = await client.ws(wsID).nodes.page.createPage('0', {
  title: 'Test Page',
  content: 'Test content',
});

// BAD - raw HTTP requests lack type safety
const response = await request.post(`/api/workspaces/${wsId}/nodes/0/page/create`, {
  headers: { Authorization: `Bearer ${token}` },
  data: { title: 'Test Page', content: 'Test content' },
});
const pageData = await response.json();
```

The SDK client is generated from Go structs (`sdk/api.gen.ts` and `sdk/types.gen.ts`) and ensures all tests use the same API contracts as the backend.

## Best Practices

### Avoid `waitForTimeout`

Never use `page.waitForTimeout()`. It makes tests slow and flaky. Instead:

```typescript
// BAD - slow and flaky
await page.waitForTimeout(2000);
await expect(page.getByText('Saved')).toBeVisible();

// GOOD - use expect.toPass() for polling
await expect(async () => {
  await expect(page.getByText('Saved')).toBeVisible();
}).toPass({ timeout: 5000 });

// GOOD - direct assertion with timeout
await expect(page.getByText('Saved')).toBeVisible({ timeout: 5000 });
```

### Trigger Autosave Explicitly

The app has a 2-second autosave debounce. To trigger saves immediately, blur the element:

```typescript
// Type content
await editor.fill('New content');

// Trigger autosave by blurring
await editor.blur();

// Wait for save to complete
await expect(async () => {
  const response = await request.get(`/api/workspaces/${wsId}/nodes/${nodeId}/page`);
  const data = await response.json();
  expect(data.content).toContain('New content');
}).toPass({ timeout: 5000 });
```

### Standard Timeouts

Use appropriate timeouts for different operations:

```typescript
// Sidebar visibility (initial load) - 15s
await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

// URL changes - 5-10s
await expect(page).toHaveURL(/\/w\/[^/]+/, { timeout: 10000 });

// Element visibility - 3-5s
await expect(page.getByText('Content')).toBeVisible({ timeout: 5000 });

// Polling assertions - 5s
await expect(async () => { ... }).toPass({ timeout: 5000 });
```

### API Response Structure

Check actual API responses - field names may differ from expectations:

```typescript
// Check the actual response structure
const response = await request.get(`/api/workspaces/${wsId}/nodes/${nodeId}/table/records`);
const data = await response.json();
// Use data.records, not data.items
expect(data.records.length).toBe(1);
```

### Test Isolation

Each test should register its own user for isolation. **Always use the `registerUser` helper** - never call the registration API directly. The helper includes retry logic for rate limiting:

```typescript
// GOOD - uses helper with retry logic
test('my test', async ({ page, request }) => {
  const { token } = await registerUser(request, 'my-test');
  await page.goto(`/?token=${token}`);
  // ... test with isolated user
});

// BAD - direct API call without retry logic
const response = await request.post('/api/auth/register', { ... });
```

### Avoid `page.goBack()` in SPAs

Browser history navigation is unreliable in SPAs. Use fresh navigation instead:

```typescript
// BAD - unreliable in SPAs, may fail to restore state
await page.goto('/some-page');
await page.goBack();
await expect(page.locator('aside')).toBeVisible(); // May fail!

// GOOD - navigate fresh
await page.goto('/some-page');
await page.goto(`/?token=${token}`); // Navigate fresh
await expect(page.locator('aside')).toBeVisible();
```

### Wait for Initial Values Before Filling

When editing inputs that load async data, wait for the initial value first:

```typescript
// BAD - may fill before initial value loads, causing race condition
const titleInput = page.locator('input[placeholder*="Title"]');
await titleInput.fill('New Title');

// GOOD - wait for initial value, then fill
const titleInput = page.locator('input[placeholder*="Title"]');
await expect(titleInput).toHaveValue('Original Title', { timeout: 5000 });
await titleInput.fill('New Title');
```

### Use Specific Selectors

Avoid ambiguous selectors that might match multiple elements:

```typescript
// BAD - might match multiple links
const privacyLink = page.getByRole('link', { name: /privacy/i });

// GOOD - specific selector
const privacyLink = page.locator('aside a[href="/privacy"]');

// BAD - fragile positional selector
await node.locator('span').first().click();

// GOOD - use data-testid
await page.locator(`[data-testid="expand-icon-${nodeId}"]`).click();
```

### Click on Visible Portions of Overlays

When clicking overlays/backdrops that sit behind other elements (lower z-index), Playwright clicks the center by default. If another element covers the center, the click will be intercepted:

```typescript
// BAD - clicks center of backdrop, which may be covered by sidebar
const backdrop = page.locator('[class*="backdrop"]');
await backdrop.click();

// GOOD - calculate click position in visible area
const backdrop = page.locator('[class*="backdrop"]');
const sidebarBox = await sidebar.boundingBox();
const backdropBox = await backdrop.boundingBox();
// Click horizontally centered between sidebar's right edge and viewport edge
const clickX = sidebarBox!.width + (backdropBox!.width - sidebarBox!.width) / 2;
await backdrop.click({ position: { x: clickX, y: backdropBox!.height / 2 } });
```

### Verify Interactive Elements Have Adequate Size

Small click targets can cause flaky tests. Verify interactive elements meet minimum size requirements:

```typescript
const expandIcon = page.locator(`[data-testid="expand-icon-${nodeId}"]`);
await expect(expandIcon).toBeVisible();

// Verify minimum clickable size (at least 16x16 pixels)
const box = await expandIcon.boundingBox();
expect(box).toBeTruthy();
expect(box!.width).toBeGreaterThanOrEqual(16);
expect(box!.height).toBeGreaterThanOrEqual(16);

await expandIcon.click();
```

This catches UI regressions where elements become too small to reliably click.

### Hold Shift for Multi-Element Keyboard Selection

When selecting across multiple elements (e.g., paragraphs) using keyboard, use `keyboard.down('Shift')` to hold Shift continuously rather than `keyboard.press('Shift+Key')` for each key. The latter releases and re-presses Shift between keys, causing timing inconsistencies on CI:

```typescript
// BAD - Shift is released and re-pressed for each key, unreliable on CI
await element.click();
await page.keyboard.press('Home');
await page.keyboard.press('Shift+End');
await page.keyboard.press('Shift+ArrowDown');
await page.keyboard.press('Shift+End');

// GOOD - hold Shift continuously for reliable selection
await element.click();
await page.keyboard.press('Home');
await page.keyboard.down('Shift');
await page.keyboard.press('End');
await page.keyboard.press('ArrowDown');
await page.keyboard.press('End');
await page.keyboard.up('Shift');
```

### Use Promise.all for Click + Navigation

When clicking a link that navigates, use `Promise.all` to avoid race conditions:

```typescript
// BAD - navigation might complete before waitForURL is called
await link.click();
await page.waitForURL('/target');

// GOOD - wait and click simultaneously
await Promise.all([
  page.waitForURL('/target', { timeout: 10000 }),
  link.click(),
]);
```

### Handle Multiple Acceptable Outcomes

When a test can have multiple valid end states, use `expect.toPass()`:

```typescript
// Test passes if either error message OR redirect happens
await expect(async () => {
  const hasError = await errorMessage.isVisible();
  const urlChanged = !page.url().includes('invalid-id');
  expect(hasError || urlChanged).toBe(true);
}).toPass({ timeout: 10000 });
```

### Debugging Flaky Tests

Tests have **zero retries** - they must be deterministic. If a test fails:

```bash
# Run specific test in isolation
npx playwright test e2e/file.spec.ts:lineNumber

# Run with trace for debugging
npx playwright test --trace on

# View trace
npx playwright show-trace test-results/.../trace.zip
```

## Parallelism and Resource Contention

Tests run with full parallelism. The `data-e2e/` directory is automatically cleaned before each test run.

**Rate limiting**: With `TEST_FAST_RATE_LIMIT=1` (default in `make e2e`), rate limits are increased 10000x. The `registerUser` helper includes retry logic for 429 responses as a safety net.

Use `make e2e-slow` to test with normal rate limits (runs sequentially with single worker).

## Common Patterns

### Wait for Sidebar Before Interactions

```typescript
await page.goto(`/?token=${token}`);
await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });
// Now safe to interact with sidebar elements
```

### Create Content via API, Verify in UI

```typescript
// Create via API
const response = await request.post(`/api/workspaces/${wsId}/nodes/0/page/create`, {
  headers: { Authorization: `Bearer ${token}` },
  data: { title: 'Test Page', content: 'Test content' },
});
const pageData = await response.json();

// Reload and verify in UI
await page.reload();
await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });
await page.locator(`[data-testid="sidebar-node-${pageData.id}"]`).click();
await expect(page.getByText('Test content')).toBeVisible({ timeout: 5000 });
```

### Hover-Revealed Buttons

Some buttons only appear on hover:

```typescript
const nodeItem = page.locator(`[data-testid="sidebar-node-${nodeId}"]`);
await nodeItem.hover();
const deleteButton = nodeItem.locator('[data-testid="delete-node-button"]');
await expect(deleteButton).toBeVisible({ timeout: 3000 });
await deleteButton.click();
```

### Wait for Animated Elements Before Interacting

Playwright auto-waits for elements to be visible and stable before clicking, so explicit visibility checks are **not needed for static elements**. However, for elements that appear after animations (dropdowns, modals, slide-ins, accordions), add explicit visibility checks:

```typescript
// Static element - Playwright auto-waits, no explicit check needed
await page.locator('[data-testid="sidebar-node-123"]').click();

// Dropdown menu - explicit wait needed
await menuButton.click();
const option = page.locator('button', { hasText: /Settings/ });
await expect(option).toBeVisible({ timeout: 3000 });
await option.click();

// Modal/dialog - explicit wait needed
await openModalButton.click();
const modal = page.locator('[role="dialog"]');
await expect(modal).toBeVisible({ timeout: 3000 });
await modal.locator('button', { hasText: 'Confirm' }).click();

// Slide-in sidebar - explicit wait needed
await hamburgerButton.click();
const sidebar = page.locator('aside');
await expect(sidebar).toHaveClass(/mobileOpen/, { timeout: 3000 });
await sidebar.locator('[data-testid="nav-item"]').click();
```

Why: Animated elements may exist in the DOM before becoming visible/interactive. Playwright's auto-wait might find a stale or animating element. Explicit checks ensure the animation completes.
