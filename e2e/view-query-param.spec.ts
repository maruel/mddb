// E2E tests for navigating to a table with a specific ?view= query parameter.

import { test, expect, registerUser, getWorkspaceId, createClient } from './helpers';
import { nodeUrl } from '../frontend/src/utils/urls';

test('navigating with ?view= param activates the correct non-default view', async ({ page, request }) => {
  const { token } = await registerUser(request, 'view-param');
  const client = createClient(request, token);

  await page.goto(`/?token=${token}`);
  await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });
  const wsID = await getWorkspaceId(page);

  // Create a table then two extra views so the default (first) view is NOT the requested one.
  const tableData = await client.ws(wsID).nodes.table.createTable('0', {
    title: 'View Param Table',
    properties: [{ name: 'Name', type: 'text' }],
  });
  const tableID = tableData.id;

  await client.ws(wsID).nodes.views.createView(tableID, { name: 'View B', type: 'table' });
  const viewC = await client.ws(wsID).nodes.views.createView(tableID, { name: 'View C', type: 'table' });
  const viewCId = viewC.id;

  // Navigate directly to the table requesting the LAST view (not the default first view).
  const url = nodeUrl(wsID, undefined, tableID) + `?view=${viewCId}`;
  await page.goto(url);
  await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

  // Wait for view tabs to render.
  await expect(page.locator('[data-testid="add-view-button"]')).toBeVisible({ timeout: 10000 });

  // URL must retain the requested view ID.
  await expect(page).toHaveURL(new RegExp(`view=${viewCId}`), { timeout: 5000 });

  // View C must be the active tab.
  const viewCTab = page.locator('button[title="View C"]');
  await expect(viewCTab).toBeVisible({ timeout: 5000 });
  await expect(viewCTab).toHaveClass(/active/);

  // No other tab must be active.
  await expect(page.locator('button[class*="active"]:not([title="View C"])')).toHaveCount(0);
});
