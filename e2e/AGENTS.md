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
import { test, expect, registerUser, getWorkspaceId } from './helpers';
```

- **`registerUser(request, prefix)`** - Creates a new user with unique email, returns `{ email, token }`
- **`getWorkspaceId(page)`** - Extracts workspace ID from current URL
- **`test.screenshot(title, fn)`** - Creates a test tagged with `@screenshot`
- **`takeScreenshot(name)`** - Fixture for capturing screenshots in tests

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

Each test should register its own user for isolation:

```typescript
test('my test', async ({ page, request }) => {
  const { token } = await registerUser(request, 'my-test');
  await page.goto(`/?token=${token}`);
  // ... test with isolated user
});
```

### Debugging Flaky Tests

Tests may fail in parallel but pass individually due to resource contention:

```bash
# Run specific test in isolation
npx playwright test e2e/file.spec.ts:lineNumber

# Run with trace for debugging
npx playwright test --trace on

# View trace
npx playwright show-trace test-results/.../trace.zip
```

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
