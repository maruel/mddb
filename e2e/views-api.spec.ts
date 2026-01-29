import { test, expect, registerUser, getWorkspaceId } from './helpers';

test.describe('Table Views API', () => {
  test('create view and list records with filter', async ({ page, request }) => {
    const { token } = await registerUser(request, 'views-api');
    const headers = { Authorization: `Bearer ${token}` };
    
    // We need to visit the page to get the workspace ID, or we can just fetch it via API if we knew how to get user info.
    // The helper `getWorkspaceId` relies on page navigation.
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });
    const wsID = await getWorkspaceId(page);

    // 1. Create Table
    const createTableRes = await request.post(`/api/workspaces/${wsID}/nodes/0/table/create`, {
      headers,
      data: {
        title: 'Views Test Table',
        properties: [
          { name: 'Name', type: 'text' },
          { name: 'Age', type: 'number' },
        ],
      },
    });
    expect(createTableRes.ok()).toBe(true);
    const tableData = await createTableRes.json();
    const tableID = tableData.id;

    // 2. Add Records
    // Alice: 25
    await request.post(`/api/workspaces/${wsID}/nodes/${tableID}/table/records/create`, {
      headers,
      data: { data: { Name: 'Alice', Age: 25 } },
    });
    // Bob: 10
    await request.post(`/api/workspaces/${wsID}/nodes/${tableID}/table/records/create`, {
      headers,
      data: { data: { Name: 'Bob', Age: 10 } },
    });

    // 3. Verify all records returned by default
    const listRes = await request.get(`/api/workspaces/${wsID}/nodes/${tableID}/table/records?limit=100`, { headers });
    expect(listRes.ok()).toBe(true);
    const listData = await listRes.json();
    expect(listData.records.length).toBe(2);

    // 4. Create a View (Filter Age > 18)
    const createViewRes = await request.post(`/api/workspaces/${wsID}/nodes/${tableID}/views/create`, {
      headers,
      data: {
        name: 'Adults',
        type: 'table',
      },
    });
    expect(createViewRes.ok()).toBe(true);
    const viewData = await createViewRes.json();
    const viewID = viewData.id;

    // Update the view with filters (could be done in create if API supported it, but our API splits it? 
    // Wait, CreateViewRequest only has Name and Type. We must update to add filters.
    const updateViewRes = await request.post(`/api/workspaces/${wsID}/nodes/${tableID}/views/${viewID}`, {
      headers,
      data: {
        filters: [
          { property: 'Age', operator: 'gt', value: 18 }
        ]
      },
    });
    expect(updateViewRes.ok()).toBe(true);

    // 5. List Records with ViewID
    const listViewRes = await request.get(`/api/workspaces/${wsID}/nodes/${tableID}/table/records?view_id=${viewID}&limit=100`, { headers });
    expect(listViewRes.ok()).toBe(true);
    const listViewData = await listViewRes.json();
    expect(listViewData.records.length).toBe(1);
    expect(listViewData.records[0].data.Name).toBe('Alice');

    // 6. List Records with Ad-hoc Filter (Age < 15)
    // We need to URL encode the JSON string
    const filters = JSON.stringify([{ property: 'Age', operator: 'lt', value: 15 }]);
    const listAdHocRes = await request.get(`/api/workspaces/${wsID}/nodes/${tableID}/table/records?filters=${encodeURIComponent(filters)}&limit=100`, { headers });
    expect(listAdHocRes.ok()).toBe(true);
    const listAdHocData = await listAdHocRes.json();
    expect(listAdHocData.records.length).toBe(1);
    expect(listAdHocData.records[0].data.Name).toBe('Bob');
  });
});
