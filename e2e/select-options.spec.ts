// E2E tests for select/multi_select option management via SelectOptionsEditor.

import type { Page, APIRequestContext } from '@playwright/test';
import { test, expect, registerUser, getWorkspaceId, createClient } from './helpers';
import type { Property } from '../sdk/types.gen';

// Helper: create a table with a select column and navigate to it.
async function setupSelectTable(page: Page, request: APIRequestContext, prefix: string) {
  const { token } = await registerUser(request, prefix);
  await page.goto(`/?token=${token}`);
  await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });
  const wsID = await getWorkspaceId(page);
  const client = createClient(request, token);

  const tableData = await client.ws(wsID).nodes.table.createTable('0', {
    title: `${prefix} Table`,
    properties: [
      {
        name: 'Status',
        type: 'select',
        options: [
          { id: 'opt1', name: 'Alpha' },
          { id: 'opt2', name: 'Beta' },
        ],
      },
    ],
  });

  await page.reload();
  await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });
  await page.locator(`[data-testid="sidebar-node-${tableData.id}"]`).click();
  await expect(page.locator('table')).toBeVisible({ timeout: 5000 });

  return { token, wsID, client, tableId: tableData.id };
}

// Helper: open the "Edit options" panel for the Status column.
async function openOptionsEditor(page: Page) {
  await page.locator('th').filter({ hasText: 'Status' }).first().click({ button: 'right' });
  await expect(page.locator('[data-testid="context-menu-edit-options"]')).toBeVisible({ timeout: 3000 });
  await page.locator('[data-testid="context-menu-edit-options"]').click();
  await expect(page.locator('[data-testid="select-options-editor"]')).toBeVisible({ timeout: 3000 });
}

test.describe('Select Options Editor', () => {
  test('opens from column header context menu and closes with Escape', async ({ page, request }) => {
    await setupSelectTable(page, request, 'opts-open');
    await openOptionsEditor(page);

    // Both existing options should be visible
    await expect(page.locator('[data-testid="option-name-opt1"]')).toBeVisible();
    await expect(page.locator('[data-testid="option-name-opt2"]')).toBeVisible();

    await page.keyboard.press('Escape');
    await expect(page.locator('[data-testid="select-options-editor"]')).not.toBeVisible({
      timeout: 3000,
    });
  });

  test('add a new option', async ({ page, request }) => {
    const { client, wsID, tableId } = await setupSelectTable(page, request, 'opts-add');
    await openOptionsEditor(page);

    await page.locator('[data-testid="add-option-btn"]').click();

    // A new empty input appears as the last option-name-* element
    const inputs = page.locator('[data-testid^="option-name-"]');
    const newInput = inputs.last();
    await expect(newInput).toBeVisible({ timeout: 3000 });

    await newInput.fill('Gamma');
    await newInput.blur();

    // Verify via API
    await expect(async () => {
      const schema = await client.ws(wsID).nodes.table.getTable(tableId);
      const statusCol = schema.properties?.find((p: Property) => p.name === 'Status');
      const gamma = statusCol?.options?.find((o) => o.name === 'Gamma');
      expect(gamma).toBeTruthy();
    }).toPass({ timeout: 5000 });
  });

  test('rename an option', async ({ page, request }) => {
    const { client, wsID, tableId } = await setupSelectTable(page, request, 'opts-rename');
    await openOptionsEditor(page);

    const nameInput = page.locator('[data-testid="option-name-opt1"]');
    await nameInput.fill('Renamed');
    await nameInput.blur();

    await expect(async () => {
      const schema = await client.ws(wsID).nodes.table.getTable(tableId);
      const statusCol = schema.properties?.find((p: Property) => p.name === 'Status');
      const opt1 = statusCol?.options?.find((o) => o.id === 'opt1');
      expect(opt1?.name).toBe('Renamed');
    }).toPass({ timeout: 5000 });
  });

  test('recolor an option via the swatch picker', async ({ page, request }) => {
    const { client, wsID, tableId } = await setupSelectTable(page, request, 'opts-recolor');
    await openOptionsEditor(page);

    // Open the color picker for opt1
    await page.locator('[data-testid="option-color-opt1"]').click();
    await expect(page.locator('[data-testid="swatch-picker"]')).toBeVisible({ timeout: 3000 });

    // Pick the red swatch
    await page.locator('[data-testid="swatch-#e03e3e"]').click();

    // Picker should close
    await expect(page.locator('[data-testid="swatch-picker"]')).not.toBeVisible({ timeout: 2000 });

    // Verify via API
    await expect(async () => {
      const schema = await client.ws(wsID).nodes.table.getTable(tableId);
      const statusCol = schema.properties?.find((p: Property) => p.name === 'Status');
      const opt1 = statusCol?.options?.find((o) => o.id === 'opt1');
      expect(opt1?.color).toBe('#e03e3e');
    }).toPass({ timeout: 5000 });
  });

  test('delete an unused option removes it from schema', async ({ page, request }) => {
    const { client, wsID, tableId } = await setupSelectTable(page, request, 'opts-delete');
    await openOptionsEditor(page);

    await page.locator('[data-testid="option-delete-opt2"]').click();

    // opt2 row should be gone from the panel immediately
    await expect(page.locator('[data-testid="option-name-opt2"]')).not.toBeVisible({ timeout: 3000 });

    // Verify via API
    await expect(async () => {
      const schema = await client.ws(wsID).nodes.table.getTable(tableId);
      const statusCol = schema.properties?.find((p: Property) => p.name === 'Status');
      const opt2 = statusCol?.options?.find((o) => o.id === 'opt2');
      expect(opt2).toBeUndefined();
    }).toPass({ timeout: 5000 });
  });

  test('shows usage count badge for options used by records', async ({ page, request }) => {
    const { client, wsID, tableId } = await setupSelectTable(page, request, 'opts-usage');

    // Create a record that uses opt1
    await client.ws(wsID).nodes.table.records.createRecord(tableId, { data: { Status: 'opt1' } });

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });
    await page.locator(`[data-testid="sidebar-node-${tableId}"]`).click();
    await expect(page.locator('table')).toBeVisible({ timeout: 5000 });

    await openOptionsEditor(page);

    // The opt1 row should have a usage badge showing "1"
    const opt1Row = page.locator('[data-testid="option-row-opt1"]');
    await expect(opt1Row).toBeVisible();
    const usageBadge = opt1Row.locator('span[title*="1"]');
    await expect(usageBadge).toBeVisible({ timeout: 3000 });
    await expect(usageBadge).toHaveText('1');
  });

  test('option order preserved across page reload after drag-reorder', async ({ page, request }) => {
    const { client, wsID, tableId } = await setupSelectTable(page, request, 'opts-order');

    // Reorder via API: put opt2 before opt1
    await client.ws(wsID).nodes.table.updateTable(tableId, {
      title: 'opts-order Table',
      properties: [
        {
          name: 'Status',
          type: 'select',
          options: [
            { id: 'opt2', name: 'Beta' },
            { id: 'opt1', name: 'Alpha' },
          ],
        },
      ],
    });

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });
    await page.locator(`[data-testid="sidebar-node-${tableId}"]`).click();
    await expect(page.locator('table')).toBeVisible({ timeout: 5000 });

    await openOptionsEditor(page);

    // opt2 should be listed before opt1
    const allNames = page.locator('[data-testid^="option-name-"]');
    await expect(allNames).toHaveCount(2, { timeout: 3000 });
    await expect(allNames.nth(0)).toHaveValue('Beta');
    await expect(allNames.nth(1)).toHaveValue('Alpha');
  });

  test('drag handle reorders options and persists to API', async ({ page, request }) => {
    const { client, wsID, tableId } = await setupSelectTable(page, request, 'opts-drag');
    await openOptionsEditor(page);

    // Initial order: Alpha (opt1) first, Beta (opt2) second
    const allNames = page.locator('[data-testid^="option-name-"]');
    await expect(allNames).toHaveCount(2, { timeout: 3000 });
    await expect(allNames.nth(0)).toHaveValue('Alpha');
    await expect(allNames.nth(1)).toHaveValue('Beta');

    // Drag opt2 (Beta) row onto opt1 (Alpha) row — Beta inserts before Alpha
    const opt1Row = page.locator('[data-testid="option-row-opt1"]');
    const opt2Row = page.locator('[data-testid="option-row-opt2"]');
    await opt2Row.dragTo(opt1Row);

    // UI should immediately reflect the new order
    await expect(allNames.nth(0)).toHaveValue('Beta', { timeout: 3000 });
    await expect(allNames.nth(1)).toHaveValue('Alpha');

    // API should persist the new order
    await expect(async () => {
      const schema = await client.ws(wsID).nodes.table.getTable(tableId);
      const statusCol = schema.properties?.find((p: Property) => p.name === 'Status');
      expect(statusCol?.options?.[0]?.name).toBe('Beta');
      expect(statusCol?.options?.[1]?.name).toBe('Alpha');
    }).toPass({ timeout: 5000 });
  });
});

test.describe('Select Dropdown Interaction', () => {
  // Helper: setup table with a select column and one record, navigate to it.
  async function setupAndNavigate(page: Page, request: APIRequestContext, prefix: string) {
    const { token } = await registerUser(request, prefix);
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });
    const wsID = await getWorkspaceId(page);
    const client = createClient(request, token);

    const tableData = await client.ws(wsID).nodes.table.createTable('0', {
      title: `${prefix} Table`,
      properties: [
        {
          name: 'Status',
          type: 'select',
          options: [
            { id: 'opt1', name: 'Alpha' },
            { id: 'opt2', name: 'Beta' },
          ],
        },
      ],
    });
    await client.ws(wsID).nodes.table.records.createRecord(tableData.id, { data: { Status: '' } });

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });
    await page.locator(`[data-testid="sidebar-node-${tableData.id}"]`).click();
    await expect(page.locator('table')).toBeVisible({ timeout: 5000 });
    // Wait for a data row to render (add-row has no row-handle)
    await expect(page.locator('[data-testid="row-handle"]').first()).toBeVisible({ timeout: 5000 });

    return { wsID, client, tableId: tableData.id };
  }

  test('search input is focused when single-select dropdown opens', async ({ page, request }) => {
    await setupAndNavigate(page, request, 'drop-focus');

    // Click the Status cell to open the dropdown
    await page.locator('table tbody tr').first().locator('td').last().click();

    const searchInput = page.locator('[data-testid="select-dropdown"] input').first();
    await expect(searchInput).toBeVisible({ timeout: 5000 });
    await expect(searchInput).toBeFocused({ timeout: 5000 });
  });

  test('clicking the search input does not dismiss the dropdown', async ({ page, request }) => {
    await setupAndNavigate(page, request, 'drop-nodismiss');

    // Click the Status cell to open the dropdown
    await page.locator('table tbody tr').first().locator('td').last().click();

    const dropdown = page.locator('[data-testid="select-dropdown"]');
    const searchInput = dropdown.locator('input').first();
    await expect(searchInput).toBeVisible({ timeout: 5000 });

    // Click directly on the search input — dropdown must stay open
    await searchInput.click();
    await expect(dropdown).toBeVisible({ timeout: 3000 });
    await expect(searchInput).toBeVisible();
  });

  test('typing in search input filters options without closing dropdown', async ({
    page,
    request,
  }) => {
    await setupAndNavigate(page, request, 'drop-filter');

    // Click the Status cell to open the dropdown
    await page.locator('table tbody tr').first().locator('td').last().click();

    const dropdown = page.locator('[data-testid="select-dropdown"]');
    const searchInput = dropdown.locator('input').first();
    await expect(searchInput).toBeVisible({ timeout: 5000 });

    // Type a partial name — only Alpha should remain
    await searchInput.fill('Al');
    await expect(dropdown).toBeVisible();
    await expect(dropdown.getByText('Alpha', { exact: true })).toBeVisible();
    await expect(dropdown.getByText('Beta', { exact: true })).not.toBeVisible();

    // Clear and type something else — only Beta should remain
    await searchInput.fill('Be');
    await expect(dropdown.getByText('Beta', { exact: true })).toBeVisible();
    await expect(dropdown.getByText('Alpha', { exact: true })).not.toBeVisible();
  });
});

test.describe('Select Column UX', () => {
  test('select chip renders in table cell (read mode)', async ({ page, request }) => {
    const { token } = await registerUser(request, 'chip-render');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });
    const wsID = await getWorkspaceId(page);
    const client = createClient(request, token);

    const tableData = await client.ws(wsID).nodes.table.createTable('0', {
      title: 'Chip Table',
      properties: [
        {
          name: 'Tag',
          type: 'select',
          options: [{ id: 'todo', name: 'To Do', color: '#e03e3e' }],
        },
      ],
    });
    await client.ws(wsID).nodes.table.records.createRecord(tableData.id, { data: { Tag: 'todo' } });

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });
    await page.locator(`[data-testid="sidebar-node-${tableData.id}"]`).click();
    await expect(page.locator('table')).toBeVisible({ timeout: 5000 });

    // Chip should appear in the table cell
    await expect(page.locator('td').getByText('To Do', { exact: true })).toBeVisible({ timeout: 5000 });
  });

  test('keyboard navigation selects option in single-select dropdown', async ({ page, request }) => {
    const { token } = await registerUser(request, 'kbd-select');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });
    const wsID = await getWorkspaceId(page);
    const client = createClient(request, token);

    const tableData = await client.ws(wsID).nodes.table.createTable('0', {
      title: 'KBD Table',
      properties: [
        {
          name: 'Status',
          type: 'select',
          options: [
            { id: 'opt1', name: 'Alpha' },
            { id: 'opt2', name: 'Beta' },
          ],
        },
      ],
    });
    const rec = await client.ws(wsID).nodes.table.records.createRecord(tableData.id, {
      data: { Status: '' },
    });

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });
    await page.locator(`[data-testid="sidebar-node-${tableData.id}"]`).click();
    await expect(page.locator('table')).toBeVisible({ timeout: 5000 });

    // Click the Status cell to open the dropdown
    const row = page.locator('table tbody tr').first();
    await row.locator('td').last().click();

    // ArrowDown once moves to 'Alpha' (index 0 in filteredOptions; the -- none item is outside keyboard nav)
    const searchInput = page.locator('input[placeholder*="Search"]').last();
    await expect(searchInput).toBeVisible({ timeout: 3000 });
    await searchInput.press('ArrowDown');
    // Press Enter to select it
    await searchInput.press('Enter');

    // Verify via API
    await expect(async () => {
      const data = await client
        .ws(wsID)
        .nodes.table.records.getRecord(tableData.id, rec.id);
      expect(String(data.data['Status'])).toBe('opt1');
    }).toPass({ timeout: 5000 });
  });

  test('multi-select dropdown opens and shows checkmarks when all options selected', async ({
    page,
    request,
  }) => {
    const { token } = await registerUser(request, 'multi-all-sel');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });
    const wsID = await getWorkspaceId(page);
    const client = createClient(request, token);

    const tableData = await client.ws(wsID).nodes.table.createTable('0', {
      title: 'MultiSel Table',
      properties: [
        {
          name: 'Tags',
          type: 'multi_select',
          options: [
            { id: 'a', name: 'Apple' },
            { id: 'b', name: 'Banana' },
          ],
        },
      ],
    });
    // Record with all options already selected
    await client.ws(wsID).nodes.table.records.createRecord(tableData.id, {
      data: { Tags: 'a,b' },
    });

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });
    await page.locator(`[data-testid="sidebar-node-${tableData.id}"]`).click();
    await expect(page.locator('table')).toBeVisible({ timeout: 5000 });

    // Click the Tags cell to open MultiSelectEditor
    const row = page.locator('table tbody tr').first();
    await row.locator('td').last().click();

    // Dropdown should open even though all options are selected
    const searchInput = page.locator('input[placeholder*="Search"]').last();
    await expect(searchInput).toBeVisible({ timeout: 3000 });

    // Both options should be listed — both with checkmarks (✓) since all are selected
    const dropdown = page.locator('[data-testid="select-dropdown"]').last();
    await expect(dropdown.getByText('Apple')).toBeVisible({ timeout: 3000 });
    await expect(dropdown.getByText('Banana')).toBeVisible();
    // The checkmark span should appear for each selected option
    const checkmarks = dropdown.locator('[class*="optionCheckmark"]').filter({ hasText: '✓' });
    await expect(checkmarks).toHaveCount(2);
  });
});

test.describe('Select Filter Option Picker', () => {
  test('filter select column by clicking option chip in filter panel', async ({ page, request }) => {
    const { token } = await registerUser(request, 'filter-picker');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });
    const wsID = await getWorkspaceId(page);
    const client = createClient(request, token);

    const tableData = await client.ws(wsID).nodes.table.createTable('0', {
      title: 'Filter Picker Table',
      properties: [
        {
          name: 'Status',
          type: 'select',
          options: [
            { id: 'todo', name: 'To Do' },
            { id: 'done', name: 'Done' },
          ],
        },
      ],
    });
    await client.ws(wsID).nodes.table.records.createRecord(tableData.id, {
      data: { Status: 'todo' },
    });
    await client.ws(wsID).nodes.table.records.createRecord(tableData.id, {
      data: { Status: 'done' },
    });

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });
    await page.locator(`[data-testid="sidebar-node-${tableData.id}"]`).click();
    await expect(page.locator('table')).toBeVisible({ timeout: 5000 });

    // Both records visible initially
    await expect(page.getByText('To Do', { exact: true })).toBeVisible({ timeout: 5000 });
    await expect(page.getByText('Done', { exact: true })).toBeVisible();

    // Open filter panel via context menu
    await page.locator('th').filter({ hasText: 'Status' }).first().click({ button: 'right' });
    await page.locator('[data-testid="context-menu-filter-by"]').click();
    await expect(page.locator('[data-testid="filter-panel"]')).toBeVisible({ timeout: 3000 });

    // Should show option picker chips (not a text input)
    await expect(page.locator('[data-testid="filter-option-todo"]')).toBeVisible({ timeout: 3000 });
    await expect(page.locator('[data-testid="filter-option-done"]')).toBeVisible();

    // Click "To Do" to select it as filter value
    await page.locator('[data-testid="filter-option-todo"]').click();
    await page.locator('[data-testid="filter-apply"]').click();

    // Only "To Do" record should remain visible
    await expect(async () => {
      await expect(page.getByText('To Do', { exact: true })).toBeVisible();
      await expect(page.getByText('Done', { exact: true })).not.toBeVisible();
    }).toPass({ timeout: 5000 });
  });
});
