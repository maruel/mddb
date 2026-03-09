// E2E screenshot test: table with every column type shown in all four view modes.

import { test, expect, registerUser, getWorkspaceId, createClient } from './helpers';

const ALL_PROPERTIES = [
  { name: 'Name', type: 'text' as const },
  { name: 'Score', type: 'number' as const },
  { name: 'Done', type: 'checkbox' as const },
  { name: 'Due', type: 'date' as const },
  {
    name: 'Status',
    type: 'select' as const,
    options: [
      { id: 'todo', name: 'To Do' },
      { id: 'in_progress', name: 'In Progress' },
      { id: 'done', name: 'Done' },
    ],
  },
  {
    name: 'Tags',
    type: 'multi_select' as const,
    options: [
      { id: 'frontend', name: 'Frontend' },
      { id: 'backend', name: 'Backend' },
      { id: 'design', name: 'Design' },
    ],
  },
  { name: 'Website', type: 'url' as const },
  { name: 'Email', type: 'email' as const },
  { name: 'Phone', type: 'phone' as const },
];

const SAMPLE_RECORDS = [
  {
    Name: 'Alice Johnson',
    Score: 95,
    Done: 'true',
    Due: '2026-04-01',
    Status: 'done',
    Tags: 'frontend,design',
    Website: 'https://alice.example.com',
    Email: 'alice@example.com',
    Phone: '+1-555-0101',
  },
  {
    Name: 'Bob Smith',
    Score: 72,
    Done: 'false',
    Due: '2026-05-15',
    Status: 'in_progress',
    Tags: 'backend',
    Website: 'https://bob.example.com',
    Email: 'bob@example.com',
    Phone: '+1-555-0202',
  },
  {
    Name: 'Carol White',
    Score: 88,
    Done: 'false',
    Due: '2026-06-30',
    Status: 'todo',
    Tags: 'frontend,backend',
    Website: 'https://carol.example.com',
    Email: 'carol@example.com',
    Phone: '+1-555-0303',
  },
];

test.screenshot('all column types in all view modes', async ({ page, request, takeScreenshot }) => {
  const { token } = await registerUser(request, 'all-col-views');
  await page.goto(`/?token=${token}`);
  await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

  const wsID = await getWorkspaceId(page);
  const client = createClient(request, token);

  // Create table with every column type
  const tableData = await client.ws(wsID).nodes.table.createTable('0', {
    title: 'All Column Types',
    properties: ALL_PROPERTIES,
  });

  // Populate with sample records
  for (const data of SAMPLE_RECORDS) {
    await client.ws(wsID).nodes.table.records.createRecord(tableData.id, { data });
  }

  // Navigate to the table
  await page.reload();
  await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });
  await page.locator(`[data-testid="sidebar-node-${tableData.id}"]`).click();
  await expect(page.locator('table')).toBeVisible({ timeout: 5000 });

  // Verify all column headers are visible in default table view
  const headers = page.locator('th');
  for (const prop of ALL_PROPERTIES) {
    await expect(headers.getByText(prop.name, { exact: true })).toBeVisible({ timeout: 3000 });
  }

  // Screenshot 1: table view (default)
  await takeScreenshot('table-view');

  // Helper: create a new view via the add-view dropdown and wait for it to activate.
  const addView = async (type: 'table' | 'list' | 'gallery' | 'board') => {
    await page.locator('[data-testid="add-view-button"]').click();
    await expect(page.locator('[data-testid="view-type-menu"]')).toBeVisible({ timeout: 3000 });
    await page.locator(`[data-testid="view-type-${type}"]`).click();
    await expect(page.locator('[data-testid="view-type-menu"]')).not.toBeVisible({ timeout: 3000 });
  };

  // Screenshot 2: list (grid cards) view
  await addView('list');
  await expect(page.locator('[data-testid="list-view"]')).toBeVisible({ timeout: 5000 });
  await takeScreenshot('list-view');

  // Screenshot 3: gallery view
  await addView('gallery');
  await expect(page.locator('[data-testid="gallery-view"]')).toBeVisible({ timeout: 5000 });
  // Wait for records to load into gallery cards
  await expect(page.locator('[data-testid="gallery"]')).toBeVisible({ timeout: 5000 });
  await takeScreenshot('gallery-view');

  // Screenshot 4: board view (groups by Status — the first select column)
  await addView('board');
  await expect(page.locator('[data-testid="board"]')).toBeVisible({ timeout: 5000 });
  // Wait for at least one column to render
  await expect(page.locator('[data-testid="board-column"]').first()).toBeVisible({ timeout: 5000 });
  await takeScreenshot('board-view');
});
