import { test, expect, registerUser, getWorkspaceId, createClient } from './helpers';
import type { DataRecordResponse, ListRecordsResponse } from '../sdk/types.gen';

test.describe('Table Creation and Basic Operations', () => {
  test.screenshot('create a table with properties and view it', async ({ page, request, takeScreenshot }) => {
    const { token } = await registerUser(request, 'table-create');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    const wsID = await getWorkspaceId(page);

    // Create a table with properties via API
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

    // Navigate to the table
    const tableNode = page.locator(`[data-testid="sidebar-node-${tableData.id}"]`);
    await expect(tableNode).toBeVisible({ timeout: 5000 });
    await tableNode.click();

    // Table view should be visible (look for the table element)
    const tableElement = page.locator('table');
    await expect(tableElement).toBeVisible({ timeout: 5000 });

    // Column headers should be visible in the table header
    const tableHeaders = page.locator('th');
    await expect(tableHeaders.getByText('Name')).toBeVisible();
    await expect(tableHeaders.getByText('Status')).toBeVisible();
    await expect(tableHeaders.getByText('Priority')).toBeVisible();
    await expect(tableHeaders.getByText('Due Date')).toBeVisible();

    await takeScreenshot('table-view');
  });

  // Testing cell inline editing
  test('add and edit records in table', async ({ page, request }) => {
    const { token } = await registerUser(request, 'table-records');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    const wsID = await getWorkspaceId(page);
    const client = createClient(request, token);

    // Create a table
    const tableData = await client.ws(wsID).nodes.table.createTable('0', {
      title: 'Records Test Table',
      properties: [
        { name: 'Name', type: 'text', required: true },
        { name: 'Value', type: 'number' },
      ],
    });

    // Add some records via API
    await client.ws(wsID).nodes.table.records.createRecord(tableData.id, {
      data: { Name: 'Item 1', Value: 100 },
    });
    await client.ws(wsID).nodes.table.records.createRecord(tableData.id, {
      data: { Name: 'Item 2', Value: 200 },
    });

    // Reload to see records (BUG: records created via API don't auto-refresh in UI)
    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Navigate to table
    const tableNodeButton = page.locator(`[data-testid="sidebar-node-${tableData.id}"]`);
    await tableNodeButton.click();

    // Wait for table to load - if this times out, the node isn't loading as a table
    await expect(page.locator('table')).toBeVisible({ timeout: 5000 });

    // Records should be visible
    await expect(page.getByText('Item 1')).toBeVisible({ timeout: 5000 });
    await expect(page.getByText('Item 2')).toBeVisible({ timeout: 5000 });
    await expect(page.getByText('100')).toBeVisible();
    await expect(page.getByText('200')).toBeVisible();

    // Click on a table cell to edit it - find the cell that contains exactly "Item 1"
    const item1Text = page.locator('td').getByText('Item 1', { exact: true });
    await item1Text.click();

    // Wait for edit mode - input should appear with focus inside the table cell
    const editInput = page.locator('table td input[type="text"]').first();
    await expect(editInput).toBeVisible({ timeout: 5000 });

    // Change the value
    await editInput.fill('Edited Item 1');

    // Save the edit by pressing Enter
    await editInput.press('Enter');

    // Wait for the API call to complete
    await page.waitForTimeout(1000);

    // Verify the edit was saved via API
    const listParams = {
      ViewID: '',
      Filters: '',
      Sorts: '',
      Offset: 0,
      Limit: 100,
    };

    let recordsData: ListRecordsResponse;
    await expect(async () => {
      recordsData = await client.ws(wsID).nodes.table.records.listRecords(tableData.id, listParams);
      const editedRecord = recordsData.records.find((r: DataRecordResponse) => (r.data.Name as string) === 'Edited Item 1');
      expect(editedRecord).toBeTruthy();
    }).toPass({ timeout: 5000 });
  });

  // Testing record deletion
  test('delete a record from table', async ({ page, request }) => {
    const { token } = await registerUser(request, 'table-delete');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    const wsID = await getWorkspaceId(page);
    const client = createClient(request, token);

    // Create a table
    const tableData = await client.ws(wsID).nodes.table.createTable('0', {
      title: 'Delete Record Table',
      properties: [{ name: 'Name', type: 'text' }],
    });

    // Add a record
    await client.ws(wsID).nodes.table.records.createRecord(tableData.id, {
      data: { Name: 'Record To Delete' },
    });

    // Reload to see records (BUG: records created via API don't auto-refresh in UI)
    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Navigate to table
    await page.locator(`[data-testid="sidebar-node-${tableData.id}"]`).click();

    // Wait for table to load with records
    await expect(page.locator('table')).toBeVisible({ timeout: 5000 });
    await expect(page.getByText('Record To Delete')).toBeVisible({ timeout: 5000 });

    // Set up dialog handler BEFORE any action that might trigger it
    // Use 'once' to handle exactly one dialog
    page.once('dialog', async (dialog) => {
      await dialog.accept();
    });

    // Find the row with our record and click its delete button
    const recordRow = page.locator('tr').filter({ hasText: 'Record To Delete' });
    const deleteButton = recordRow.locator('button', { hasText: '✕' });
    await deleteButton.click();

    // Record should disappear from the table
    await expect(page.locator('td').filter({ hasText: 'Record To Delete' })).not.toBeVisible({ timeout: 5000 });
  });
});

test.describe('Table View Modes', () => {
  test.screenshot('table has view tabs and add view dropdown', async ({ page, request, takeScreenshot }) => {
    const { token } = await registerUser(request, 'view-modes');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    const wsID = await getWorkspaceId(page);
    const client = createClient(request, token);

    // Create a table with records
    const tableData = await client.ws(wsID).nodes.table.createTable('0', {
      title: 'View Modes Table',
      properties: [
        { name: 'Name', type: 'text' },
        { name: 'Status', type: 'select', options: [
          { id: 'todo', name: 'To Do' },
          { id: 'progress', name: 'In Progress' },
          { id: 'done', name: 'Done' }
        ]},
      ],
    });

    // Add records
    await client.ws(wsID).nodes.table.records.createRecord(tableData.id, {
      data: { Name: 'Task 1', Status: 'todo' },
    });
    await client.ws(wsID).nodes.table.records.createRecord(tableData.id, {
      data: { Name: 'Task 2', Status: 'progress' },
    });

    // Reload to see records
    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Navigate to table
    await page.locator(`[data-testid="sidebar-node-${tableData.id}"]`).click();

    // Wait for table to load
    await expect(page.locator('table')).toBeVisible({ timeout: 5000 });
    // Wait for records to load - they should appear in the table
    await expect(page.locator('td').getByText('Task 1')).toBeVisible({ timeout: 5000 });

    // Check that default view tab exists (All view)
    const defaultViewTab = page.locator('button').filter({ hasText: 'All' });
    await expect(defaultViewTab).toBeVisible();
    await expect(defaultViewTab).toHaveClass(/active/i);
    await takeScreenshot('view-table');

    // Check that add view button exists
    const addViewButton = page.locator('[data-testid="add-view-button"]');
    await expect(addViewButton).toBeVisible();

    // Open the add view dropdown
    await addViewButton.click();
    const viewMenu = page.locator('[data-testid="view-type-menu"]');
    await expect(viewMenu).toBeVisible({ timeout: 3000 });

    // Verify dropdown options exist
    await expect(page.locator('[data-testid="view-type-table"]')).toBeVisible();
    await expect(page.locator('[data-testid="view-type-gallery"]')).toBeVisible();
    await expect(page.locator('[data-testid="view-type-board"]')).toBeVisible();
    await takeScreenshot('view-dropdown');

    // Close dropdown by clicking outside
    await page.locator('body').click({ position: { x: 10, y: 10 } });
    await expect(viewMenu).not.toBeVisible({ timeout: 3000 });
  });
});

test.describe('Table and Page Hybrid', () => {
  test('table node shows table view with records section', async ({ page, request }) => {
    // NOTE: Tables by default only show the table view, not markdown content
    // This test verifies the table-specific UI elements
    const { token } = await registerUser(request, 'hybrid-node');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    const wsID = await getWorkspaceId(page);
    const client = createClient(request, token);

    // Create a table
    const tableData = await client.ws(wsID).nodes.table.createTable('0', {
      title: 'Table Only Node',
      properties: [{ name: 'Item', type: 'text' }],
    });

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    await page.locator(`[data-testid="sidebar-node-${tableData.id}"]`).click();

    // Wait for title to confirm we're on the right node
    const titleInput = page.locator('input[placeholder*="Title"]');
    await expect(titleInput).toHaveValue('Table Only Node', { timeout: 5000 });

    // Table view should be visible (not markdown textarea for table-only nodes)
    const tableElement = page.locator('table');
    await expect(tableElement).toBeVisible({ timeout: 5000 });

    // Should have view tabs with default view and add button
    await expect(page.locator('button').filter({ hasText: 'All' })).toBeVisible();
    await expect(page.getByTitle('New View')).toBeVisible();

    // Markdown textarea should NOT be visible (table-only node)
    const contentArea = page.locator('textarea[placeholder*="markdown"]');
    await expect(contentArea).not.toBeVisible();
  });
});
