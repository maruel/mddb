// E2E tests for table creation, record CRUD, view modes, and sort UI.

import type { Page, APIRequestContext } from '@playwright/test';
import { test, expect, registerUser, getWorkspaceId, createClient } from './helpers';
import type { DataRecordResponse, Property } from '../sdk/types.gen';

// Helper: create a table with records and navigate to it.
async function setupTable(
  page: Page,
  request: APIRequestContext,
  prefix: string,
  properties: Property[],
  records: Record<string, unknown>[]
) {
  const { token } = await registerUser(request, prefix);
  await page.goto(`/?token=${token}`);
  await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });
  const wsID = await getWorkspaceId(page);
  const client = createClient(request, token);

  const tableData = await client.ws(wsID).nodes.table.createTable('0', {
    title: `${prefix} Table`,
    properties,
  });

  for (const data of records) {
    await client.ws(wsID).nodes.table.records.createRecord(tableData.id, { data });
  }

  await page.reload();
  await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });
  await page.locator(`[data-testid="sidebar-node-${tableData.id}"]`).click();
  await expect(page.locator('table')).toBeVisible({ timeout: 5000 });

  return { token, wsID, client, tableId: tableData.id };
}

// Helper: extract the order of known names from table rows.
async function getRowOrder(page: Page, names: string[]): Promise<string[]> {
  const rows = page.locator('table tbody tr');
  const texts = await rows.allTextContents();
  return texts
    .map((t) => names.find((n) => t.includes(n)) ?? '')
    .filter(Boolean);
}

test.describe('Table Creation and Basic Operations', () => {
  test.screenshot('create a table with properties and view it', async ({ page, request, takeScreenshot }) => {
    const { token } = await registerUser(request, 'table-create');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    const wsID = await getWorkspaceId(page);
    const client = createClient(request, token);
    const tableData = await client.ws(wsID).nodes.table.createTable('0', {
      title: 'Test Table',
      properties: [
        { name: 'Name', type: 'text', required: true },
        { name: 'Status', type: 'select', options: [{ id: 'todo', name: 'To Do' }, { id: 'done', name: 'Done' }] },
        { name: 'Priority', type: 'number' },
        { name: 'Due Date', type: 'date' },
      ],
    });

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });
    await page.locator(`[data-testid="sidebar-node-${tableData.id}"]`).click();

    const tableElement = page.locator('table');
    await expect(tableElement).toBeVisible({ timeout: 5000 });

    const tableHeaders = page.locator('th');
    await expect(tableHeaders.getByText('Name')).toBeVisible();
    await expect(tableHeaders.getByText('Status')).toBeVisible();
    await expect(tableHeaders.getByText('Priority')).toBeVisible();
    await expect(tableHeaders.getByText('Due Date')).toBeVisible();

    await takeScreenshot('table-view');
  });

  test('add and edit records in table', async ({ page, request }) => {
    const { client, wsID, tableId } = await setupTable(page, request, 'table-records', [
      { name: 'Name', type: 'text' },
      { name: 'Value', type: 'number' },
    ], [
      { Name: 'Item 1', Value: 100 },
      { Name: 'Item 2', Value: 200 },
    ]);

    await expect(page.getByText('Item 1')).toBeVisible({ timeout: 5000 });
    await expect(page.getByText('200')).toBeVisible();

    // Click to edit
    await page.locator('td').getByText('Item 1', { exact: true }).click();
    const editInput = page.locator('table td input[type="text"]').first();
    await expect(editInput).toBeVisible({ timeout: 5000 });
    await editInput.fill('Edited Item 1');
    await editInput.press('Enter');

    // Verify via API
    const listParams = { ViewID: '', Filters: '', Sorts: '', Offset: 0, Limit: 100 };
    await expect(async () => {
      const data = await client.ws(wsID).nodes.table.records.listRecords(tableId, listParams);
      const edited = data.records.find((r: DataRecordResponse) => (r.data.Name as string) === 'Edited Item 1');
      expect(edited).toBeTruthy();
    }).toPass({ timeout: 5000 });
  });

  test('delete a record from table', async ({ page, request }) => {
    await setupTable(page, request, 'table-delete', [
      { name: 'Name', type: 'text' },
    ], [
      { Name: 'Record To Delete' },
    ]);

    await expect(page.getByText('Record To Delete')).toBeVisible({ timeout: 5000 });

    page.once('dialog', async (dialog) => await dialog.accept());
    const recordRow = page.locator('tr').filter({ hasText: 'Record To Delete' });
    await recordRow.locator('button', { hasText: '✕' }).click();

    await expect(page.locator('td').filter({ hasText: 'Record To Delete' })).not.toBeVisible({ timeout: 5000 });
  });

  test('add record via UI button', async ({ page, request }) => {
    const { client, wsID, tableId } = await setupTable(page, request, 'table-add-ui', [
      { name: 'Name', type: 'text' },
    ], []);

    // Empty table should show "+ Add Record"
    await expect(page.getByText(/add record/i)).toBeVisible({ timeout: 5000 });

    // Click to add
    await page.getByText(/add record/i).click();

    // Verify record was created via API
    const listParams = { ViewID: '', Filters: '', Sorts: '', Offset: 0, Limit: 100 };
    await expect(async () => {
      const data = await client.ws(wsID).nodes.table.records.listRecords(tableId, listParams);
      expect(data.records.length).toBe(1);
    }).toPass({ timeout: 5000 });
  });
});

test.describe('Table View Modes', () => {
  test.screenshot('table has view tabs and add view dropdown', async ({ page, request, takeScreenshot }) => {
    await setupTable(page, request, 'view-modes', [
      { name: 'Name', type: 'text' },
      { name: 'Status', type: 'select', options: [
        { id: 'todo', name: 'To Do' },
        { id: 'progress', name: 'In Progress' },
        { id: 'done', name: 'Done' },
      ]},
    ], [
      { Name: 'Task 1', Status: 'todo' },
      { Name: 'Task 2', Status: 'progress' },
    ]);

    await expect(page.locator('td').getByText('Task 1')).toBeVisible({ timeout: 5000 });

    // Default view tab active
    const defaultViewTab = page.locator('button').filter({ hasText: 'All' });
    await expect(defaultViewTab).toBeVisible();
    await expect(defaultViewTab).toHaveClass(/active/i);
    await takeScreenshot('view-table');

    // Add view dropdown
    const addViewButton = page.locator('[data-testid="add-view-button"]');
    await addViewButton.click();
    const viewMenu = page.locator('[data-testid="view-type-menu"]');
    await expect(viewMenu).toBeVisible({ timeout: 3000 });
    await expect(page.locator('[data-testid="view-type-table"]')).toBeVisible();
    await expect(page.locator('[data-testid="view-type-gallery"]')).toBeVisible();
    await expect(page.locator('[data-testid="view-type-board"]')).toBeVisible();
    await takeScreenshot('view-dropdown');

    // Close by clicking outside
    await page.locator('body').click({ position: { x: 400, y: 400 } });
    await expect(viewMenu).not.toBeVisible({ timeout: 3000 });
  });
});

test.describe('Table Sort UI', () => {
  test('column header sort menu appears and closes', async ({ page, request }) => {
    await setupTable(page, request, 'sort-ui', [
      { name: 'Name', type: 'text' },
    ], [
      { Name: 'Alice' },
    ]);

    await expect(page.getByText('Alice')).toBeVisible({ timeout: 5000 });

    // Click column header to open context menu
    await page.locator('th').filter({ hasText: 'Name' }).first().click();
    await expect(page.locator('[data-testid="context-menu-sort-asc"]')).toBeVisible({ timeout: 3000 });
    await expect(page.locator('[data-testid="context-menu-sort-desc"]')).toBeVisible();

    // Close with Escape
    await page.keyboard.press('Escape');
    await expect(page.locator('[data-testid="context-menu-sort-asc"]')).not.toBeVisible({ timeout: 3000 });
  });

  test('sort ascending via column header', async ({ page, request }) => {
    await setupTable(page, request, 'sort-asc', [
      { name: 'Name', type: 'text' },
    ], [
      { Name: 'Zebra' },
      { Name: 'Apple' },
      { Name: 'Mango' },
    ]);

    await expect(page.getByText('Zebra')).toBeVisible({ timeout: 5000 });

    // Sort ascending via column header menu
    await page.locator('th').filter({ hasText: 'Name' }).first().click();
    await page.locator('[data-testid="context-menu-sort-asc"]').click();

    // Records should be: Apple, Mango, Zebra
    await expect(async () => {
      expect(await getRowOrder(page, ['Apple', 'Mango', 'Zebra'])).toEqual(['Apple', 'Mango', 'Zebra']);
    }).toPass({ timeout: 5000 });

    // Sort indicator should appear on header
    await expect(page.locator('[data-testid="sort-indicator"]')).toBeVisible();
  });

  test('sort descending via column header', async ({ page, request }) => {
    await setupTable(page, request, 'sort-desc', [
      { name: 'Name', type: 'text' },
    ], [
      { Name: 'Zebra' },
      { Name: 'Apple' },
      { Name: 'Mango' },
    ]);

    await expect(page.getByText('Zebra')).toBeVisible({ timeout: 5000 });

    // Sort descending via column header menu
    await page.locator('th').filter({ hasText: 'Name' }).first().click();
    await page.locator('[data-testid="context-menu-sort-desc"]').click();

    await expect(async () => {
      expect(await getRowOrder(page, ['Apple', 'Mango', 'Zebra'])).toEqual(['Zebra', 'Mango', 'Apple']);
    }).toPass({ timeout: 5000 });
  });

  test('remove sort clears ordering', async ({ page, request }) => {
    await setupTable(page, request, 'sort-remove', [
      { name: 'Name', type: 'text' },
    ], [
      { Name: 'Zebra' },
      { Name: 'Apple' },
    ]);

    await expect(page.getByText('Zebra')).toBeVisible({ timeout: 5000 });

    // Add ascending sort
    await page.locator('th').filter({ hasText: 'Name' }).first().click();
    await page.locator('[data-testid="context-menu-sort-asc"]').click();
    await expect(page.locator('[data-testid="sort-indicator"]')).toBeVisible({ timeout: 3000 });

    // Remove sort via column header menu
    await page.locator('th').filter({ hasText: 'Name' }).first().click();
    await expect(page.locator('[data-testid="context-menu-remove-sort"]')).toBeVisible({ timeout: 3000 });
    await page.locator('[data-testid="context-menu-remove-sort"]').click();

    // Sort indicator should be gone
    await expect(page.locator('[data-testid="sort-indicator"]')).not.toBeVisible({ timeout: 3000 });
  });

  test('multiple sorts apply compound ordering', async ({ page, request }) => {
    await setupTable(page, request, 'sort-multi', [
      { name: 'Color', type: 'text' },
      { name: 'Size', type: 'number' },
    ], [
      { Color: 'Red', Size: 3 },
      { Color: 'Blue', Size: 1 },
      { Color: 'Red', Size: 1 },
      { Color: 'Blue', Size: 2 },
    ]);

    await expect(page.getByText('Red').first()).toBeVisible({ timeout: 5000 });

    // Sort by Color ascending
    await page.locator('th').filter({ hasText: 'Color' }).first().click();
    await page.locator('[data-testid="context-menu-sort-asc"]').click();

    // Sort by Size ascending (additive)
    await page.locator('th').filter({ hasText: 'Size' }).first().click();
    await page.locator('[data-testid="context-menu-sort-asc"]').click();

    // Both columns should show sort indicators
    await expect(page.locator('[data-testid="sort-indicator"]')).toHaveCount(2, { timeout: 3000 });

    // Order: Blue/1, Blue/2, Red/1, Red/3
    await expect(async () => {
      const rows = page.locator('table tbody tr').filter({ has: page.locator('td') });
      const count = await rows.count();
      const pairs: string[] = [];
      for (let i = 0; i < count; i++) {
        const cells = rows.nth(i).locator('td');
        const cellTexts = await cells.allTextContents();
        const color = cellTexts.find((c) => c === 'Blue' || c === 'Red');
        const size = cellTexts.find((c) => /^[123]$/.test(c.trim()));
        if (color && size) pairs.push(`${color}/${size.trim()}`);
      }
      expect(pairs).toEqual(['Blue/1', 'Blue/2', 'Red/1', 'Red/3']);
    }).toPass({ timeout: 5000 });
  });

  test('sort persists on saved view', async ({ page, request }) => {
    const { client, wsID, tableId } = await setupTable(page, request, 'sort-persist', [
      { name: 'Name', type: 'text' },
    ], [
      { Name: 'Zebra' },
      { Name: 'Apple' },
      { Name: 'Mango' },
    ]);

    await expect(page.getByText('Zebra')).toBeVisible({ timeout: 5000 });

    // Create a saved view via API
    const viewData = await client.ws(wsID).nodes.views.createView(tableId, {
      name: 'Sorted',
      type: 'table',
    });

    // Reload to pick up the new view
    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });
    await page.locator(`[data-testid="sidebar-node-${tableId}"]`).click();
    await expect(page.locator('table')).toBeVisible({ timeout: 5000 });

    // Switch to the Sorted view tab
    await page.locator('button').filter({ hasText: 'Sorted' }).click();

    // Sort ascending via column header menu
    await page.locator('th').filter({ hasText: 'Name' }).first().click();
    await page.locator('[data-testid="context-menu-sort-asc"]').click();

    // Wait for records to sort
    await expect(async () => {
      expect(await getRowOrder(page, ['Apple', 'Mango', 'Zebra'])).toEqual(['Apple', 'Mango', 'Zebra']);
    }).toPass({ timeout: 5000 });

    // Verify sort was persisted via API
    await expect(async () => {
      const listParams = {
        ViewID: viewData.id,
        Filters: '',
        Sorts: '',
        Offset: 0,
        Limit: 100,
      };
      const data = await client.ws(wsID).nodes.table.records.listRecords(tableId, listParams);
      const names = data.records.map((r: DataRecordResponse) => r.data.Name);
      expect(names).toEqual(['Apple', 'Mango', 'Zebra']);
    }).toPass({ timeout: 5000 });
  });
});

test.describe('Table Filter UI', () => {
  test('filter panel opens from column header menu', async ({ page, request }) => {
    await setupTable(
      page,
      request,
      'filter-open',
      [{ name: 'Name', type: 'text' }],
      [{ Name: 'Alice' }, { Name: 'Bob' }]
    );

    await page.locator('th').filter({ hasText: 'Name' }).first().click();
    await expect(page.locator('[data-testid="context-menu-filter-by"]')).toBeVisible({ timeout: 3000 });
    await page.locator('[data-testid="context-menu-filter-by"]').click();
    await expect(page.locator('[data-testid="filter-panel"]')).toBeVisible({ timeout: 3000 });
    await expect(page.locator('[data-testid="filter-operator"]')).toBeVisible();
    await expect(page.locator('[data-testid="filter-apply"]')).toBeVisible();
  });

  test('filter by contains hides non-matching records', async ({ page, request }) => {
    await setupTable(
      page,
      request,
      'filter-contains',
      [{ name: 'Name', type: 'text' }],
      [{ Name: 'Alice' }, { Name: 'Bob' }, { Name: 'Charlie' }]
    );

    await expect(page.getByText('Alice')).toBeVisible({ timeout: 5000 });
    await expect(page.getByText('Bob')).toBeVisible();
    await expect(page.getByText('Charlie')).toBeVisible();

    // Open filter panel for Name column
    await page.locator('th').filter({ hasText: 'Name' }).first().click();
    await page.locator('[data-testid="context-menu-filter-by"]').click();
    await expect(page.locator('[data-testid="filter-panel"]')).toBeVisible({ timeout: 3000 });

    // Set operator to "contains" and value to "li"
    await page.locator('[data-testid="filter-operator"]').selectOption('contains');
    await page.locator('[data-testid="filter-value"]').fill('li');
    await page.locator('[data-testid="filter-apply"]').click();

    // Only Alice and Charlie contain "li"
    await expect(async () => {
      await expect(page.getByText('Alice')).toBeVisible();
      await expect(page.getByText('Charlie')).toBeVisible();
      await expect(page.getByText('Bob')).not.toBeVisible();
    }).toPass({ timeout: 5000 });
  });

  test('filter by equals shows only exact match', async ({ page, request }) => {
    await setupTable(
      page,
      request,
      'filter-equals',
      [{ name: 'Name', type: 'text' }],
      [{ Name: 'Alice' }, { Name: 'Bob' }, { Name: 'Alice B' }]
    );

    await expect(page.getByText('Alice')).toBeVisible({ timeout: 5000 });

    await page.locator('th').filter({ hasText: 'Name' }).first().click();
    await page.locator('[data-testid="context-menu-filter-by"]').click();
    await expect(page.locator('[data-testid="filter-panel"]')).toBeVisible({ timeout: 3000 });

    await page.locator('[data-testid="filter-operator"]').selectOption('equals');
    await page.locator('[data-testid="filter-value"]').fill('Alice');
    await page.locator('[data-testid="filter-apply"]').click();

    await expect(async () => {
      const rows = page.locator('table tbody tr:not(.newRow)');
      const visible = await rows.filter({ hasText: 'Alice' }).count();
      expect(visible).toBe(1);
      await expect(page.getByText('Bob')).not.toBeVisible();
    }).toPass({ timeout: 5000 });
  });

  test('remove filter restores all records', async ({ page, request }) => {
    await setupTable(
      page,
      request,
      'filter-remove',
      [{ name: 'Name', type: 'text' }],
      [{ Name: 'Alice' }, { Name: 'Bob' }]
    );

    await expect(page.getByText('Alice')).toBeVisible({ timeout: 5000 });

    // Apply a filter
    await page.locator('th').filter({ hasText: 'Name' }).first().click();
    await page.locator('[data-testid="context-menu-filter-by"]').click();
    await expect(page.locator('[data-testid="filter-panel"]')).toBeVisible({ timeout: 3000 });
    await page.locator('[data-testid="filter-operator"]').selectOption('equals');
    await page.locator('[data-testid="filter-value"]').fill('Alice');
    await page.locator('[data-testid="filter-apply"]').click();

    await expect(async () => {
      await expect(page.getByText('Bob')).not.toBeVisible();
    }).toPass({ timeout: 5000 });

    // Remove the filter
    await page.locator('th').filter({ hasText: 'Name' }).first().click();
    await page.locator('[data-testid="context-menu-filter-by"]').click();
    await expect(page.locator('[data-testid="filter-panel"]')).toBeVisible({ timeout: 3000 });
    await expect(page.locator('[data-testid="filter-remove"]')).toBeVisible();
    await page.locator('[data-testid="filter-remove"]').click();

    await expect(async () => {
      await expect(page.getByText('Alice')).toBeVisible();
      await expect(page.getByText('Bob')).toBeVisible();
    }).toPass({ timeout: 5000 });
  });

  test('filter indicator appears in column header when filter is active', async ({ page, request }) => {
    await setupTable(
      page,
      request,
      'filter-indicator',
      [{ name: 'Name', type: 'text' }],
      [{ Name: 'Alice' }, { Name: 'Bob' }]
    );

    // No indicator initially
    await expect(page.locator('[data-testid="filter-indicator"]')).not.toBeVisible();

    // Apply a filter
    await page.locator('th').filter({ hasText: 'Name' }).first().click();
    await page.locator('[data-testid="context-menu-filter-by"]').click();
    await expect(page.locator('[data-testid="filter-panel"]')).toBeVisible({ timeout: 3000 });
    await page.locator('[data-testid="filter-value"]').fill('Alice');
    await page.locator('[data-testid="filter-apply"]').click();

    // Indicator appears
    await expect(page.locator('[data-testid="filter-indicator"]')).toBeVisible({ timeout: 3000 });
  });

  test('two-column filters compound (AND)', async ({ page, request }) => {
    await setupTable(
      page,
      request,
      'filter-compound',
      [
        { name: 'Name', type: 'text' },
        { name: 'Age', type: 'number' },
      ],
      [
        { Name: 'Alice', Age: 30 },
        { Name: 'Bob', Age: 25 },
        { Name: 'Charlie', Age: 30 },
      ]
    );

    await expect(page.getByText('Alice')).toBeVisible({ timeout: 5000 });

    // Filter Name contains 'li'
    await page.locator('th').filter({ hasText: 'Name' }).first().click();
    await page.locator('[data-testid="context-menu-filter-by"]').click();
    await expect(page.locator('[data-testid="filter-panel"]')).toBeVisible({ timeout: 3000 });
    await page.locator('[data-testid="filter-operator"]').selectOption('contains');
    await page.locator('[data-testid="filter-value"]').fill('li');
    await page.locator('[data-testid="filter-apply"]').click();

    // Alice and Charlie pass; Bob is hidden
    await expect(async () => {
      await expect(page.getByText('Bob')).not.toBeVisible();
    }).toPass({ timeout: 5000 });

    // Filter Age equals 30
    await page.locator('th').filter({ hasText: 'Age' }).first().click();
    await page.locator('[data-testid="context-menu-filter-by"]').click();
    await expect(page.locator('[data-testid="filter-panel"]')).toBeVisible({ timeout: 3000 });
    await page.locator('[data-testid="filter-operator"]').selectOption('equals');
    await page.locator('[data-testid="filter-value"]').fill('30');
    await page.locator('[data-testid="filter-apply"]').click();

    // Only Alice and Charlie remain (both have li AND age=30)
    await expect(async () => {
      await expect(page.getByText('Alice')).toBeVisible();
      await expect(page.getByText('Charlie')).toBeVisible();
      await expect(page.getByText('Bob')).not.toBeVisible();
    }).toPass({ timeout: 5000 });
  });
});

test.describe('Table and Page Hybrid', () => {
  test('table node shows table view with records section', async ({ page, request }) => {
    const { token } = await registerUser(request, 'hybrid-node');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    const wsID = await getWorkspaceId(page);
    const client = createClient(request, token);
    const tableData = await client.ws(wsID).nodes.table.createTable('0', {
      title: 'Table Only Node',
      properties: [{ name: 'Item', type: 'text' }],
    });

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });
    await page.locator(`[data-testid="sidebar-node-${tableData.id}"]`).click();

    await expect(page.locator('input[placeholder*="Title"]')).toHaveValue('Table Only Node', { timeout: 5000 });
    await expect(page.locator('table')).toBeVisible({ timeout: 5000 });
    await expect(page.locator('button').filter({ hasText: 'All' })).toBeVisible();
    await expect(page.getByTitle('New View')).toBeVisible();
    await expect(page.locator('textarea[placeholder*="markdown"]')).not.toBeVisible();
  });
});
