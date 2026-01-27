import { test, expect, registerUser, getWorkspaceId } from './helpers';

test.describe('Table Creation and Basic Operations', () => {
  test('create a table with properties and view it', async ({ page, request, takeScreenshot }) => {
    const { token } = await registerUser(request, 'table-create');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    const wsID = await getWorkspaceId(page);

    // Create a table with properties via API
    const createResponse = await request.post(`/api/workspaces/${wsID}/nodes/0/table/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: {
        title: 'Test Table',
        properties: [
          { name: 'Name', type: 'text', required: true },
          { name: 'Status', type: 'select', options: [{ id: 'todo', name: 'To Do' }, { id: 'done', name: 'Done' }] },
          { name: 'Priority', type: 'number' },
          { name: 'Due Date', type: 'date' },
        ],
      },
    });
    expect(createResponse.ok()).toBe(true);
    const tableData = await createResponse.json();

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

    // Create a table
    const createResponse = await request.post(`/api/workspaces/${wsID}/nodes/0/table/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: {
        title: 'Records Test Table',
        properties: [
          { name: 'Name', type: 'text', required: true },
          { name: 'Value', type: 'number' },
        ],
      },
    });
    const tableData = await createResponse.json();

    // Add some records via API
    await request.post(`/api/workspaces/${wsID}/nodes/${tableData.id}/table/records/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { data: { Name: 'Item 1', Value: 100 } },
    });
    await request.post(`/api/workspaces/${wsID}/nodes/${tableData.id}/table/records/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { data: { Name: 'Item 2', Value: 200 } },
    });

    // Reload to see records (BUG: records created via API don't auto-refresh in UI)
    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Navigate to table
    await page.locator(`[data-testid="sidebar-node-${tableData.id}"]`).click();

    // Wait for table to load
    await expect(page.locator('table')).toBeVisible({ timeout: 5000 });

    // Records should be visible
    await expect(page.getByText('Item 1')).toBeVisible({ timeout: 5000 });
    await expect(page.getByText('Item 2')).toBeVisible({ timeout: 5000 });
    await expect(page.getByText('100')).toBeVisible();
    await expect(page.getByText('200')).toBeVisible();

    // Click on a table cell to edit it - find the cell that contains exactly "Item 1"
    // Using getByText with exact match for the cell content
    const item1Text = page.locator('td').getByText('Item 1', { exact: true });
    await item1Text.click();

    // Wait for edit mode - the row with Item 1 should now have an input
    // Use getByRole to find the row containing the edit UI (shows ✕ Item 1 ✓ ✕)
    const editRow = page.getByRole('row', { name: /Item 1/ });
    const editInput = editRow.getByRole('textbox');
    await expect(editInput).toBeVisible({ timeout: 5000 });

    // Change the value
    await editInput.fill('Edited Item 1');

    // Save the edit using the save button (checkmark) within the same row
    const saveButton = editRow.locator('button').filter({ hasText: '✓' });
    await saveButton.click();

    // Wait for save to process
    await page.waitForTimeout(1000);

    // Verify via API that the record was updated
    const recordsResponse = await request.get(`/api/workspaces/${wsID}/nodes/${tableData.id}/table/records`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    const recordsData = await recordsResponse.json();
    const editedRecord = recordsData.items?.find((r: { data: { Name: string } }) => r.data.Name === 'Edited Item 1');

    // If API shows the edit was saved, verify UI. If not, this is a backend bug.
    if (editedRecord) {
      // Reload to get fresh UI state
      await page.reload();
      await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });
      await page.locator(`[data-testid="sidebar-node-${tableData.id}"]`).click();
      await expect(page.locator('table')).toBeVisible({ timeout: 5000 });
      await expect(page.locator('td').getByText('Edited Item 1')).toBeVisible({ timeout: 5000 });
    } else {
      // Check if original value still exists - this would be a bug
      const originalRecord = recordsData.items?.find((r: { data: { Name: string } }) => r.data.Name === 'Item 1');
      // If the record still has original name, the save failed
      expect(originalRecord).toBeFalsy();
    }
  });

  // Testing record deletion
  test('delete a record from table', async ({ page, request }) => {
    const { token } = await registerUser(request, 'table-delete');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    const wsID = await getWorkspaceId(page);

    // Create a table
    const createResponse = await request.post(`/api/workspaces/${wsID}/nodes/0/table/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: {
        title: 'Delete Record Table',
        properties: [{ name: 'Name', type: 'text' }],
      },
    });
    const tableData = await createResponse.json();

    // Add a record
    await request.post(`/api/workspaces/${wsID}/nodes/${tableData.id}/table/records/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { data: { Name: 'Record To Delete' } },
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

    // Wait a moment for the delete to process
    await page.waitForTimeout(1000);

    // Record should disappear from the table
    await expect(page.locator('td').filter({ hasText: 'Record To Delete' })).not.toBeVisible({ timeout: 5000 });
  });
});

test.describe('Table View Modes', () => {
  test('switch between table, grid, gallery, and board views', async ({ page, request, takeScreenshot }) => {
    const { token } = await registerUser(request, 'view-modes');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    const wsID = await getWorkspaceId(page);

    // Create a table with records
    const createResponse = await request.post(`/api/workspaces/${wsID}/nodes/0/table/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: {
        title: 'View Modes Table',
        properties: [
          { name: 'Name', type: 'text' },
          { name: 'Status', type: 'select', options: [
            { id: 'todo', name: 'To Do' },
            { id: 'progress', name: 'In Progress' },
            { id: 'done', name: 'Done' }
          ]},
        ],
      },
    });
    const tableData = await createResponse.json();

    // Add records
    await request.post(`/api/workspaces/${wsID}/nodes/${tableData.id}/table/records/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { data: { Name: 'Task 1', Status: 'todo' } },
    });
    await request.post(`/api/workspaces/${wsID}/nodes/${tableData.id}/table/records/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { data: { Name: 'Task 2', Status: 'progress' } },
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

    // Check that view toggle buttons exist
    const tableButton = page.locator('button', { hasText: 'Table' });
    const gridButton = page.locator('button', { hasText: 'Grid' });
    const galleryButton = page.locator('button', { hasText: 'Gallery' });
    const boardButton = page.locator('button', { hasText: 'Board' });

    await expect(tableButton).toBeVisible();
    await expect(gridButton).toBeVisible();
    await expect(galleryButton).toBeVisible();
    await expect(boardButton).toBeVisible();

    // Table view should be active by default
    await expect(tableButton).toHaveClass(/active/i);
    await takeScreenshot('view-table');

    // Switch to Grid view
    await gridButton.click();
    await expect(gridButton).toHaveClass(/active/i);
    // Grid view renders cards
    await expect(page.locator('[class*="grid"], [class*="Grid"]')).toBeVisible({ timeout: 3000 });
    await takeScreenshot('view-grid');

    // Switch to Gallery view
    await galleryButton.click();
    await expect(galleryButton).toHaveClass(/active/i);
    await expect(page.locator('[class*="gallery"], [class*="Gallery"]')).toBeVisible({ timeout: 3000 });
    await takeScreenshot('view-gallery');

    // Switch to Board view
    await boardButton.click();
    await expect(boardButton).toHaveClass(/active/i);
    await expect(page.locator('[class*="board"], [class*="Board"]')).toBeVisible({ timeout: 3000 });
    await takeScreenshot('view-board');

    // Switch back to Table view
    await tableButton.click();
    await expect(tableButton).toHaveClass(/active/i);
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

    // Create a table
    const createResponse = await request.post(`/api/workspaces/${wsID}/nodes/0/table/create`, {
      headers: { Authorization: `Bearer ${token}` },
      data: {
        title: 'Table Only Node',
        properties: [{ name: 'Item', type: 'text' }],
      },
    });
    const tableData = await createResponse.json();

    await page.reload();
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    await page.locator(`[data-testid="sidebar-node-${tableData.id}"]`).click();

    // Wait for title to confirm we're on the right node
    const titleInput = page.locator('input[placeholder*="Title"]');
    await expect(titleInput).toHaveValue('Table Only Node', { timeout: 5000 });

    // Table view should be visible (not markdown textarea for table-only nodes)
    const tableElement = page.locator('table');
    await expect(tableElement).toBeVisible({ timeout: 5000 });

    // Should have view mode toggle buttons
    await expect(page.locator('button', { hasText: 'Table' })).toBeVisible();
    await expect(page.locator('button', { hasText: 'Grid' })).toBeVisible();

    // Markdown textarea should NOT be visible (table-only node)
    const contentArea = page.locator('textarea[placeholder*="markdown"]');
    await expect(contentArea).not.toBeVisible();
  });
});
