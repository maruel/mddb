// E2E test for workspace switch 404 loop bug.
// When switching workspaces while viewing a node, the frontend should not
// repeatedly request the old node from the new workspace.

import { test, expect, registerUser, getWorkspaceId, createClient } from './helpers';

test('switching workspace while viewing a node does not cause 404 loop', async ({ page, request }) => {
  const { token } = await registerUser(request, 'ws-loop');
  await page.goto(`/?token=${token}`);
  await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

  const wsID1 = await getWorkspaceId(page);

  // Create a page in workspace 1 and navigate to it
  const client = createClient(request, token);
  const pageData = await client.ws(wsID1).nodes.page.createPage('0', {
    title: 'WS1 Only Page',
    content: 'This page only exists in workspace 1',
  });

  await page.reload();
  await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });
  await page.locator(`[data-testid="sidebar-node-${pageData.id}"]`).click();
  await expect(page.getByText('This page only exists in workspace 1')).toBeVisible({ timeout: 5000 });

  // Create a second workspace
  const meResponse = await request.get('/api/v1/auth/me', {
    headers: { Authorization: `Bearer ${token}` },
  });
  const userData = await meResponse.json();
  const orgId = userData.organization_id;

  const ws2Response = await request.post(`/api/v1/organizations/${orgId}/workspaces`, {
    headers: { Authorization: `Bearer ${token}` },
    data: { name: 'Loop Test WS2' },
  });
  expect(ws2Response.ok()).toBe(true);
  const ws2Data = await ws2Response.json();

  // Reload to pick up the new workspace in the workspace list
  await page.reload();
  await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

  // Navigate back to the WS1 page so URL contains the old node ID
  await page.locator(`[data-testid="sidebar-node-${pageData.id}"]`).click();
  await expect(page.getByText('This page only exists in workspace 1')).toBeVisible({ timeout: 5000 });

  // Collect 404 responses after the switch
  const notFoundRequests: string[] = [];
  page.on('response', (response) => {
    if (response.status() === 404 && response.url().includes('/nodes/')) {
      notFoundRequests.push(response.url());
    }
  });

  // Expand "Other workspaces" section in the sidebar and switch
  const otherWsToggle = page.locator('aside button', { hasText: /Other workspaces/i });
  await expect(otherWsToggle).toBeVisible({ timeout: 3000 });
  await otherWsToggle.click();

  const ws2Button = page.locator('aside button', { hasText: 'Loop Test WS2' });
  await expect(ws2Button).toBeVisible({ timeout: 3000 });
  await ws2Button.click();

  // Should navigate to workspace 2 without the old node ID in the URL
  await expect(page).toHaveURL(new RegExp(`/w/@${ws2Data.id}`), { timeout: 10000 });

  // The URL should NOT contain the old node ID
  await expect(async () => {
    expect(page.url()).not.toContain(pageData.id);
  }).toPass({ timeout: 5000 });

  // Wait for the sidebar to stabilize in the new workspace
  await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

  // The old node's content must not be visible
  await expect(page.getByText('This page only exists in workspace 1')).not.toBeVisible({ timeout: 3000 });

  // Allow a brief settling period, then check that we didn't get a flood of 404s.
  // A single transient 404 is tolerable; a loop produces many.
  await page.waitForTimeout(2000);
  expect(notFoundRequests.length).toBeLessThanOrEqual(1);
});
